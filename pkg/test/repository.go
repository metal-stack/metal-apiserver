package test

import (
	"log/slog"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/alicebob/miniredis/v2"
	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	ipamv1 "github.com/metal-stack/go-ipam/api/v1"
	"github.com/metal-stack/go-ipam/api/v1/apiv1connect"
	mdcv1 "github.com/metal-stack/masterdata-api/api/v1"
	mdc "github.com/metal-stack/masterdata-api/pkg/client"
	"github.com/metal-stack/metal-apiserver/pkg/certs"
	"github.com/metal-stack/metal-apiserver/pkg/db/generic"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-apiserver/pkg/invite"
	"github.com/metal-stack/metal-apiserver/pkg/repository"
	"github.com/metal-stack/metal-apiserver/pkg/service/token"
	tokencommon "github.com/metal-stack/metal-apiserver/pkg/token"
	"github.com/metal-stack/metal-lib/pkg/pointer"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
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

	projectInviteStore invite.ProjectInviteStore
	tenantInviteStore  invite.TenantInviteStore
	tokenStore         tokencommon.TokenStore

	// only use this when you are very certain about it!!
	tokenService token.TokenService
	mdc          mdc.Client
	rc           *redis.Client
}

func (s *testStore) CleanNetworkTable(t testing.TB) {
	_, err := r.DB(s.dbName).Table("network").Delete().RunWrite(s.queryExecutor)
	require.NoError(t, err)
}

type testOpt any

type testOptCoackroach struct {
	with bool
}

type testOptValkey struct {
	with bool
}

func WithCockroach(with bool) *testOptCoackroach {
	return &testOptCoackroach{
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
		withCockroach = false
		withValkey    = false
	)

	for _, opt := range testOpts {
		switch o := opt.(type) {
		case *testOptCoackroach:
			withCockroach = o.with
		case *testOptValkey:
			withValkey = o.with
		default:
			t.Errorf("unsupported test option: %T", o)
		}
	}

	ds, opts, rethinkCloser := StartRethink(t, log)

	var (
		rc           *redis.Client
		valkeyCloser func()
	)
	if withValkey {
		rc, valkeyCloser = StartValkey(t)
	} else {
		mr := miniredis.RunT(t)
		rc = redis.NewClient(&redis.Options{Addr: mr.Addr()})
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
	)
	if withCockroach {
		mdc, connection, masterdataCloser = StartMasterdataWithCockroach(t, log)
	} else {
		mdc, connection, masterdataCloser = StartMasterdataInMemory(t, log)
	}

	repo, err := repository.New(log, mdc, ds, ipam, rc)
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
		projectInviteStore: projectInviteStore,
		tenantInviteStore:  tenantInviteStore,
		tokenStore:         tokenStore,
		tokenService:       tokenService,
		mdc:                mdc,
		rc:                 rc,
	}, closer
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
	resp, err := t.tokenService.CreateApiTokenWithoutPermissionCheck(t.t.Context(), subject, connect.NewRequest(cr))
	require.NoError(t.t, err)
	return resp.Msg.GetToken()
}

func CreateImages(t testing.TB, repo *repository.Store, images []*adminv2.ImageServiceCreateRequest) map[string]*metal.Image {
	imageMap := map[string]*metal.Image{}
	for _, img := range images {
		i, err := repo.Image().Create(t.Context(), img)
		require.NoError(t, err)
		imageMap[i.ID] = i
	}
	return imageMap
}

func CreateFilesystemLayouts(t testing.TB, repo *repository.Store, fsls []*adminv2.FilesystemServiceCreateRequest) map[string]*metal.FilesystemLayout {
	fslMap := map[string]*metal.FilesystemLayout{}
	for _, fsl := range fsls {
		fsl, err := repo.FilesystemLayout().Create(t.Context(), fsl)
		require.NoError(t, err)
		fslMap[fsl.ID] = fsl
	}
	return fslMap
}

func CreateIPs(t testing.TB, repo *repository.Store, ips []*apiv2.IPServiceCreateRequest) map[string]*metal.IP {
	ipMap := map[string]*metal.IP{}
	for _, ip := range ips {
		i, err := repo.UnscopedIP().Create(t.Context(), ip)
		require.NoError(t, err)
		ipMap[i.IPAddress] = i
	}
	return ipMap
}

func CreateMachinesWithAllocation(t testing.TB, repo *repository.Store, machines []*apiv2.MachineServiceCreateRequest) map[string]*metal.Machine {
	machineMap := map[string]*metal.Machine{}
	for _, machine := range machines {
		m, err := repo.UnscopedMachine().Create(t.Context(), machine)
		require.NoError(t, err)
		machineMap[m.ID] = m
	}
	return machineMap
}

func CreateMachines(t testing.TB, testStore *testStore, machines []*metal.Machine) map[string]*metal.Machine {
	machineMap := map[string]*metal.Machine{}
	for _, machine := range machines {
		m, err := testStore.ds.Machine().Create(t.Context(), machine)
		require.NoError(t, err)
		event := &metal.ProvisioningEventContainer{
			Base: metal.Base{ID: machine.ID},
			Events: metal.ProvisioningEvents{
				{
					Time:    time.Now(),
					Event:   metal.ProvisioningEventAlive,
					Message: "machine created for test",
				},
			},
			Liveliness: metal.MachineLivelinessAlive,
		}
		_, err = testStore.ds.Event().Create(t.Context(), event)
		require.NoError(t, err)
		machineMap[m.ID] = m
	}
	return machineMap
}

func CreateNetworks(t testing.TB, repo *repository.Store, nws []*adminv2.NetworkServiceCreateRequest) map[string]*metal.Network {
	networkMap := map[string]*metal.Network{}

	for _, nw := range nws {
		resp, err := repo.UnscopedNetwork().Create(t.Context(), nw)
		require.NoError(t, err)
		networkMap[resp.ID] = resp
	}
	return networkMap
}

func DeleteNetworks(t testing.TB, testStore *testStore) {
	nsResp, err := testStore.ipam.ListNamespaces(t.Context(), connect.NewRequest(&ipamv1.ListNamespacesRequest{}))
	require.NoError(t, err)

	for _, ns := range nsResp.Msg.Namespace {
		resp, err := testStore.ipam.ListPrefixes(t.Context(), connect.NewRequest(&ipamv1.ListPrefixesRequest{
			Namespace: pointer.Pointer(ns),
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

func AllocateNetworks(t testing.TB, repo *repository.Store, nws []*apiv2.NetworkServiceCreateRequest) map[string]*metal.Network {
	networkMap := map[string]*metal.Network{}
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

		resp, err := repo.UnscopedNetwork().Create(t.Context(), req)
		require.NoError(t, err)
		networkMap[resp.Name] = resp
	}
	return networkMap
}

func CreatePartitions(t testing.TB, repo *repository.Store, partitions []*adminv2.PartitionServiceCreateRequest) map[string]*metal.Partition {
	partitionMap := map[string]*metal.Partition{}
	for _, partition := range partitions {
		p, err := repo.Partition().Create(t.Context(), partition)
		require.NoError(t, err)
		partitionMap[p.ID] = p
	}
	return partitionMap
}

func CreateProjects(t testing.TB, repo *repository.Store, projects []*apiv2.ProjectServiceCreateRequest) {
	for _, p := range projects {
		_, err := repo.UnscopedProject().AdditionalMethods().CreateWithID(t.Context(), p, p.GetName())
		require.NoError(t, err)
	}
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

func CreateTenants(t testing.TB, testStore *testStore, tenants []*apiv2.TenantServiceCreateRequest) {
	for _, tenant := range tenants {
		tok, err := testStore.tokenService.CreateApiTokenWithoutPermissionCheck(t.Context(), tenant.GetName(), connect.NewRequest(&apiv2.TokenServiceCreateRequest{
			Expires:   durationpb.New(time.Minute),
			AdminRole: apiv2.AdminRole_ADMIN_ROLE_EDITOR.Enum(),
		}))
		require.NoError(t, err)

		reqCtx := tokencommon.ContextWithToken(t.Context(), tok.Msg.Token)

		_, err = testStore.Tenant().AdditionalMethods().CreateWithID(reqCtx, tenant, tenant.Name)
		require.NoError(t, err)
	}
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

func CreateSizes(t testing.TB, repo *repository.Store, sizes []*adminv2.SizeServiceCreateRequest) map[string]*metal.Size {
	sizeMap := map[string]*metal.Size{}
	for _, size := range sizes {
		s, err := repo.Size().Create(t.Context(), size)
		require.NoError(t, err)
		sizeMap[s.ID] = s
	}
	return sizeMap
}

func CreateSwitches(t testing.TB, store *repository.Store, switches []*repository.SwitchServiceCreateRequest) map[string]*metal.Switch {
	switchMap := map[string]*metal.Switch{}
	for _, sw := range switches {
		s, err := store.Switch().Create(t.Context(), sw)
		require.NoError(t, err)
		switchMap[s.ID] = s
	}
	return switchMap
}
