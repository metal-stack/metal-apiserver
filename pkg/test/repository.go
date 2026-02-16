package test

import (
	"log/slog"
	"testing"
	"time"

	"connectrpc.com/connect"
	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	infrav2 "github.com/metal-stack/api/go/metalstack/infra/v2"
	ipamv1 "github.com/metal-stack/go-ipam/api/v1"
	"github.com/metal-stack/go-ipam/api/v1/apiv1connect"
	mdcv1 "github.com/metal-stack/masterdata-api/api/v1"
	mdc "github.com/metal-stack/masterdata-api/pkg/client"
	"github.com/metal-stack/metal-apiserver/pkg/async/queue"
	"github.com/metal-stack/metal-apiserver/pkg/async/task"
	"github.com/metal-stack/metal-apiserver/pkg/certs"
	"github.com/metal-stack/metal-apiserver/pkg/db/generic"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-apiserver/pkg/db/queries"
	"github.com/metal-stack/metal-apiserver/pkg/invite"
	"github.com/metal-stack/metal-apiserver/pkg/repository"
	"github.com/metal-stack/metal-apiserver/pkg/service/token"
	tokencommon "github.com/metal-stack/metal-apiserver/pkg/token"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"github.com/valkey-io/valkey-go"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/durationpb"
	r "gopkg.in/rethinkdb/rethinkdb-go.v6"
)

const (
	tokenIssuer = "https://test.io"
)

// TODO should we make all methods return/consume the teststore ?
type testStore struct {
	t testing.TB
	*repository.Store
	ds            generic.Datastore
	dbName        string
	queryExecutor *r.Session
	ipam          apiv1connect.IpamServiceClient
	ipamcloser    func()

	projectInviteStore invite.ProjectInviteStore
	tenantInviteStore  invite.TenantInviteStore
	tokenStore         tokencommon.TokenStore

	// only use this when you are very certain about it!!
	tokenService token.TokenService
	mdc          mdc.Client
	rc           *redis.Client
}

type testOpt any

type testOptPostgres struct {
	with bool
}

type testOptValkey struct {
	with bool
}

func WithPostgres(with bool) *testOptPostgres {
	return &testOptPostgres{
		with: with,
	}
}

func WithValkey(with bool) *testOptValkey {
	return &testOptValkey{
		with: with,
	}
}

func StartRepositoryWithCleanup(t testing.TB, log *slog.Logger, testOpts ...testOpt) (*testStore, func()) {
	var (
		withPostgres = false
		withValkey   = false
	)

	for _, opt := range testOpts {
		switch o := opt.(type) {
		case *testOptPostgres:
			withPostgres = o.with
		case *testOptValkey:
			withValkey = o.with
		default:
			t.Errorf("unsupported test option: %T", o)
		}
	}

	ds, opts, rethinkCloser := StartRethink(t, log)

	var (
		rc           *redis.Client
		vc           valkey.Client
		valkeyCloser func()
	)
	if withValkey {
		rc, vc, valkeyCloser = StartValkey(t)
	} else {
		rc, vc, valkeyCloser = StartValkey(t, WithMiniRedis(true))
	}

	projectInviteStore := invite.NewProjectRedisStore(rc)
	tenantInviteStore := invite.NewTenantRedisStore(rc)
	tokenStore := tokencommon.NewRedisStore(rc)
	certStore := certs.NewRedisStore(&certs.Config{RedisClient: rc})

	tokenService := token.New(token.Config{
		Log:        log,
		TokenStore: tokenStore,
		CertStore:  certStore,
		Issuer:     tokenIssuer,
	})

	ipam, ipamCloser := StartIpam(t)

	var (
		mdc              mdc.Client
		connection       *grpc.ClientConn
		masterdataCloser func()

		task  = task.NewClient(log, rc)
		queue = queue.New(log, vc)
	)
	if withPostgres {
		mdc, connection, masterdataCloser = StartMasterdataWithPostgres(t, log)
	} else {
		mdc, connection, masterdataCloser = StartMasterdataInMemory(t, log)
	}

	config := repository.Config{
		Log:              log,
		MasterdataClient: mdc,
		Datastore:        ds,
		Ipam:             ipam,
		Task:             task,
		Queue:            queue,
	}

	repo, err := repository.New(config)
	require.NoError(t, err)

	asyncCloser := StartAsynqServer(t, log.WithGroup("asynq"), repo, rc)

	closer := func() {
		_ = connection.Close()
		rethinkCloser()
		ipamCloser()
		masterdataCloser()
		asyncCloser()
		_ = rc.Close()
		if valkeyCloser != nil {
			valkeyCloser()
		}
	}

	session, err := r.Connect(opts)
	require.NoError(t, err)

	return &testStore{
		t:                  t,
		Store:              repo,
		ds:                 ds,
		dbName:             opts.Database,
		queryExecutor:      session,
		ipam:               ipam,
		ipamcloser:         ipamCloser,
		projectInviteStore: projectInviteStore,
		tenantInviteStore:  tenantInviteStore,
		tokenStore:         tokenStore,
		tokenService:       tokenService,
		mdc:                mdc,
		rc:                 rc,
	}, closer
}

func (s *testStore) CleanUp(t testing.TB) {

	s.DeleteProjects()
	s.DeleteTenants()
	DeleteIPs(t, s)
	DeleteNetworks(t, s)

	// TODO valkey

	tables := s.ds.GetTableNames()

	for _, tableName := range tables {
		_, err := r.DB(databaseNameFromT(t)).Table(tableName).Delete().RunWrite(s.queryExecutor, r.RunOpts{Context: t.Context()})
		require.NoError(t, err)
	}

	for i := range 99 {
		err := s.ds.AsnPool().ReleaseUniqueInteger(t.Context(), uint(i+1))
		require.NoError(t, err)
		err = s.ds.VrfPool().ReleaseUniqueInteger(t.Context(), uint(i+1))
		require.NoError(t, err)
	}
}

func (t *testStore) GetProjectInviteStore() invite.ProjectInviteStore {
	return t.projectInviteStore
}

func (t *testStore) GetTenantInviteStore() invite.TenantInviteStore {
	return t.tenantInviteStore
}

func (t *testStore) GetTokenStore() tokencommon.TokenStore {
	return t.tokenStore
}

func (t *testStore) GetMasterdataClient() mdc.Client {
	return t.mdc
}

func (t *testStore) GetRedisClient() *redis.Client {
	return t.rc
}

func (t *testStore) GetTokenService() token.TokenService {
	return t.tokenService
}

func (t *testStore) GetToken(subject string, cr *apiv2.TokenServiceCreateRequest) *apiv2.Token {
	resp, err := t.tokenService.CreateApiTokenWithoutPermissionCheck(t.t.Context(), subject, cr)
	require.NoError(t.t, err)
	return resp.GetToken()
}

func (t *testStore) GetEventContainer(machineID string) *metal.ProvisioningEventContainer {
	resp, err := t.ds.Event().Find(t.t.Context(), queries.EventFilter(machineID))
	require.NoError(t.t, err)
	return resp
}

func CreateImages(t testing.TB, testStore *testStore, images []*adminv2.ImageServiceCreateRequest) map[string]*apiv2.Image {
	imageMap := map[string]*apiv2.Image{}
	for _, img := range images {
		i, err := testStore.Image().Create(t.Context(), img)
		require.NoError(t, err)
		imageMap[i.Id] = i
	}
	return imageMap
}

func CreateFilesystemLayouts(t testing.TB, testStore *testStore, fsls []*adminv2.FilesystemServiceCreateRequest) map[string]*apiv2.FilesystemLayout {
	fslMap := map[string]*apiv2.FilesystemLayout{}
	for _, fsl := range fsls {
		fsl, err := testStore.FilesystemLayout().Create(t.Context(), fsl)
		require.NoError(t, err)
		fslMap[fsl.Id] = fsl
	}
	return fslMap
}

func CreateIPs(t testing.TB, testStore *testStore, ips []*apiv2.IPServiceCreateRequest) map[string]*apiv2.IP {
	ipMap := map[string]*apiv2.IP{}
	for _, ip := range ips {
		i, err := testStore.UnscopedIP().Create(t.Context(), ip)
		require.NoError(t, err)
		ipMap[i.Ip] = i
	}
	return ipMap
}

func CreateMachinesWithAllocation(t testing.TB, testStore *testStore, machines []*apiv2.MachineServiceCreateRequest) map[string]*apiv2.Machine {
	machineMap := map[string]*apiv2.Machine{}
	for _, machine := range machines {
		m, err := testStore.UnscopedMachine().Create(t.Context(), machine)
		require.NoError(t, err)
		machineMap[m.Uuid] = m
	}
	return machineMap
}

func CreateMachines(t testing.TB, testStore *testStore, machines []*metal.Machine) map[string]*metal.Machine {
	machineMap := map[string]*metal.Machine{}
	for _, machine := range machines {
		m, err := testStore.ds.Machine().Create(t.Context(), machine)
		require.NoError(t, err)
		event := &metal.ProvisioningEventContainer{
			Base:       metal.Base{ID: machine.ID},
			Events:     metal.ProvisioningEvents{},
			Liveliness: metal.MachineLivelinessAlive,
		}
		_, err = testStore.ds.Event().Create(t.Context(), event)
		require.NoError(t, err)
		machineMap[m.ID] = m
	}
	return machineMap
}

func DhcpMachines(t testing.TB, testStore *testStore, bootRequests []*infrav2.BootServiceDhcpRequest) {
	for _, req := range bootRequests {
		_, err := testStore.UnscopedMachine().AdditionalMethods().Dhcp(t.Context(), req)
		require.NoError(t, err)
	}
}

func RegisterMachines(t testing.TB, testStore *testStore, registerRequests []*infrav2.BootServiceRegisterRequest) {
	for _, req := range registerRequests {
		_, err := testStore.UnscopedMachine().AdditionalMethods().Register(t.Context(), req)
		require.NoError(t, err)
	}
}

func CreateNetworks(t testing.TB, testStore *testStore, nws []*adminv2.NetworkServiceCreateRequest) map[string]*apiv2.Network {
	networkMap := map[string]*apiv2.Network{}

	for _, nw := range nws {
		resp, err := testStore.UnscopedNetwork().Create(t.Context(), nw)
		require.NoError(t, err)
		networkMap[resp.Id] = resp
	}

	return networkMap
}

func DeleteNetworks(t testing.TB, testStore *testStore) {
	nsResp, err := testStore.ipam.ListNamespaces(t.Context(), connect.NewRequest(&ipamv1.ListNamespacesRequest{}))
	require.NoError(t, err)

	for _, ns := range nsResp.Msg.Namespace {
		resp, err := testStore.ipam.ListPrefixes(t.Context(), connect.NewRequest(&ipamv1.ListPrefixesRequest{
			Namespace: new(ns),
		}))
		require.NoError(t, err)

		for _, prefix := range resp.Msg.Prefixes {
			_, err := testStore.ipam.DeletePrefix(t.Context(), connect.NewRequest(&ipamv1.DeletePrefixRequest{Cidr: prefix.Cidr}))
			require.NoError(t, err)
		}
	}

	_, err = r.DB(testStore.dbName).Table("network").Delete().RunWrite(testStore.queryExecutor)
	require.NoError(t, err)

}

func DeleteIPs(t testing.TB, testStore *testStore) {
	ips, err := testStore.ds.IP().List(t.Context())
	require.NoError(t, err)

	for _, ip := range ips {
		_, err = testStore.ipam.ReleaseIP(t.Context(), connect.NewRequest(&ipamv1.ReleaseIPRequest{
			PrefixCidr: ip.ParentPrefixCidr,
			Ip:         ip.IPAddress,
			Namespace:  ip.Namespace,
		}))
		require.NoError(t, err)
	}

	_, err = r.DB(testStore.dbName).Table("ip").Delete().RunWrite(testStore.queryExecutor)
	require.NoError(t, err)
}

func DeleteMachines(t testing.TB, testStore *testStore) {
	_, err := r.DB(testStore.dbName).Table("machine").Delete().RunWrite(testStore.queryExecutor)
	require.NoError(t, err)
}

func (t *testStore) DeleteTenants() {
	ts, err := t.mdc.Tenant().Find(t.t.Context(), &mdcv1.TenantFindRequest{})
	require.NoError(t.t, err)

	for _, tenant := range ts.Tenants {
		_, err = t.mdc.Tenant().Delete(t.t.Context(), &mdcv1.TenantDeleteRequest{Id: tenant.Meta.Id})
		require.NoError(t.t, err)
	}
}

func (t *testStore) DeleteTenantInvites() {
	ts, err := t.mdc.Tenant().Find(t.t.Context(), &mdcv1.TenantFindRequest{})
	require.NoError(t.t, err)

	for _, tenant := range ts.Tenants {
		invites, err := t.tenantInviteStore.ListInvites(t.t.Context(), tenant.Meta.Id)
		require.NoError(t.t, err)

		for _, invite := range invites {
			err = t.tenantInviteStore.DeleteInvite(t.t.Context(), invite)
			require.NoError(t.t, err)
		}
	}
}

func (t *testStore) DeleteProjects() {
	ps, err := t.mdc.Project().Find(t.t.Context(), &mdcv1.ProjectFindRequest{})
	require.NoError(t.t, err)

	for _, p := range ps.Projects {
		_, err = t.mdc.Project().Delete(t.t.Context(), &mdcv1.ProjectDeleteRequest{Id: p.Meta.Id})
		require.NoError(t.t, err)
	}
}

func (t *testStore) DeleteProjectInvites() {
	ts, err := t.mdc.Project().Find(t.t.Context(), &mdcv1.ProjectFindRequest{})
	require.NoError(t.t, err)

	for _, project := range ts.Projects {
		invites, err := t.projectInviteStore.ListInvites(t.t.Context(), project.Meta.Id)
		require.NoError(t.t, err)

		for _, invite := range invites {
			err = t.projectInviteStore.DeleteInvite(t.t.Context(), invite)
			require.NoError(t.t, err)
		}
	}
}

func AllocateNetworks(t testing.TB, testStore *testStore, nws []*apiv2.NetworkServiceCreateRequest) map[string]*apiv2.Network {
	networkMap := map[string]*apiv2.Network{}

	for _, nw := range nws {
		req := &adminv2.NetworkServiceCreateRequest{
			Project:       &nw.Project,
			Name:          nw.Name,
			Description:   nw.Description,
			Partition:     nw.Partition,
			ParentNetwork: nw.ParentNetwork,
			Labels:        nw.Labels,
			Length:        nw.Length,
			AddressFamily: nw.AddressFamily,
			Type:          apiv2.NetworkType_NETWORK_TYPE_CHILD, // Non Admins can only create Child Networks
		}

		resp, err := testStore.UnscopedNetwork().Create(t.Context(), req)
		require.NoError(t, err)

		networkMap[*resp.Name] = resp
	}

	return networkMap
}

func CreatePartitions(t testing.TB, testStore *testStore, partitions []*adminv2.PartitionServiceCreateRequest) map[string]*apiv2.Partition {
	partitionMap := map[string]*apiv2.Partition{}
	for _, partition := range partitions {
		p, err := testStore.Partition().Create(t.Context(), partition)
		require.NoError(t, err)
		partitionMap[p.Id] = p
	}
	return partitionMap
}

func CreateProjects(t testing.TB, testStore *testStore, projects []*apiv2.ProjectServiceCreateRequest) map[string]string {
	projectMap := map[string]string{}
	for _, p := range projects {
		resp, err := testStore.UnscopedProject().AdditionalMethods().CreateWithID(t.Context(), p, p.GetName())
		require.NoError(t, err)
		projectMap[p.Login] = resp.Meta.Id
	}
	return projectMap
}

func CreateProjectMemberships(t testing.TB, testStore *testStore, project string, memberships []*repository.ProjectMemberCreateRequest) {
	for _, membership := range memberships {
		_, err := testStore.Project(project).AdditionalMethods().Member().Create(t.Context(), membership)
		require.NoError(t, err)
	}
}

func CreateProjectInvites(t testing.TB, testStore *testStore, invites []*apiv2.ProjectInvite) {
	for _, invite := range invites {
		err := testStore.projectInviteStore.SetInvite(t.Context(), invite)
		require.NoError(t, err)
	}
}

func CreateTenants(t testing.TB, testStore *testStore, tenants []*apiv2.TenantServiceCreateRequest) []string {
	var tenantList []string
	for _, tenant := range tenants {
		tok, err := testStore.tokenService.CreateApiTokenWithoutPermissionCheck(t.Context(), tenant.GetName(), &apiv2.TokenServiceCreateRequest{
			Expires:   durationpb.New(time.Minute),
			AdminRole: apiv2.AdminRole_ADMIN_ROLE_EDITOR.Enum(),
		})
		require.NoError(t, err)

		reqCtx := tokencommon.ContextWithToken(t.Context(), tok.Token)

		_, err = testStore.Tenant().AdditionalMethods().CreateWithID(reqCtx, tenant, tenant.Name)
		require.NoError(t, err)
		tenantList = append(tenantList, tenant.Name)
	}
	return tenantList
}

func CreateTenantMemberships(t testing.TB, testStore *testStore, tenant string, memberships []*repository.TenantMemberCreateRequest) {
	for _, membership := range memberships {
		_, err := testStore.Tenant().AdditionalMethods().Member(tenant).Create(t.Context(), membership)
		require.NoError(t, err)
	}
}

func CreateTenantInvites(t testing.TB, testStore *testStore, invites []*apiv2.TenantInvite) {
	for _, invite := range invites {
		err := testStore.tenantInviteStore.SetInvite(t.Context(), invite)
		require.NoError(t, err)
	}
}

func CreateSizes(t testing.TB, testStore *testStore, sizes []*adminv2.SizeServiceCreateRequest) map[string]*apiv2.Size {
	sizeMap := map[string]*apiv2.Size{}
	for _, size := range sizes {
		s, err := testStore.Size().Create(t.Context(), size)
		require.NoError(t, err)
		sizeMap[s.Id] = s
	}
	return sizeMap
}

func CreateSizeReservations(t testing.TB, testStore *testStore, sizeReservations []*adminv2.SizeReservationServiceCreateRequest) map[string]*apiv2.SizeReservation {
	sizeReservationMap := map[string]*apiv2.SizeReservation{}
	for _, sr := range sizeReservations {
		s, err := testStore.UnscopedSizeReservation().Create(t.Context(), sr)
		require.NoError(t, err)
		sizeReservationMap[s.Name] = s
	}
	return sizeReservationMap
}

func CreateSwitches(t testing.TB, testStore *testStore, switches []*repository.SwitchServiceCreateRequest) map[string]*apiv2.Switch {
	switchMap := map[string]*apiv2.Switch{}
	for _, sw := range switches {
		s, err := testStore.Switch().Create(t.Context(), sw)
		require.NoError(t, err)
		switchMap[s.Id] = s
	}
	return switchMap
}

func CreateSwitchStatuses(t testing.TB, testStore *testStore, statuses []*repository.SwitchStatus) map[string]*metal.SwitchStatus {
	statusMap := map[string]*metal.SwitchStatus{}
	for _, status := range statuses {
		metalStatus := &metal.SwitchStatus{
			Base: metal.Base{
				ID: status.ID,
			},
		}

		var (
			timestamp time.Time
			duration  time.Duration
		)

		if sync := status.LastSync; sync != nil {
			if sync.Time != nil {
				timestamp = sync.Time.AsTime()
			}
			if sync.Duration != nil {
				duration = sync.Duration.AsDuration()
			}
			metalStatus.LastSync = &metal.SwitchSync{
				Time:     timestamp,
				Duration: duration,
				Error:    sync.Error,
			}
		}

		if sync := status.LastSyncError; sync != nil {
			if sync.Time != nil {
				timestamp = sync.Time.AsTime()
			}
			if sync.Duration != nil {
				duration = sync.Duration.AsDuration()
			}
			metalStatus.LastSyncError = &metal.SwitchSync{
				Time:     timestamp,
				Duration: duration,
				Error:    sync.Error,
			}
		}

		s, err := testStore.ds.SwitchStatus().Create(t.Context(), metalStatus)
		require.NoError(t, err)
		statusMap[s.ID] = s
	}
	return statusMap
}

func (t *testStore) GetSwitchStatus(id string) (*metal.SwitchStatus, error) {
	return t.ds.SwitchStatus().Get(t.t.Context(), id)
}
