package test

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-apiserver/pkg/repository"
	"github.com/metal-stack/metal-apiserver/pkg/test/scenarios"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type (
	Datacenter struct {
		Tenants    []string
		Projects   map[string][]string
		Partitions map[string]*apiv2.Partition
		Sizes      map[string]*apiv2.Size
		Networks   map[string]*apiv2.Network
		IPs        map[string]*apiv2.IP
		Images     map[string]*apiv2.Image
		Switches   map[string]*apiv2.Switch
		Machines   map[string]*apiv2.Machine

		TestStore *testStore
		t         testing.TB
		closers   []func()
	}
)

func NewDatacenter(t testing.TB, log *slog.Logger, testOpts ...testOpt) *Datacenter {
	testStore, closer := StartRepositoryWithCleanup(t, log, testOpts...)

	dc := &Datacenter{
		t:          t,
		TestStore:  testStore,
		Projects:   make(map[string][]string),
		Partitions: make(map[string]*apiv2.Partition),
		Sizes:      make(map[string]*apiv2.Size),
		Networks:   make(map[string]*apiv2.Network),
		IPs:        make(map[string]*apiv2.IP),
		Images:     make(map[string]*apiv2.Image),
		Switches:   make(map[string]*apiv2.Switch),
		Machines:   make(map[string]*apiv2.Machine),
	}

	dc.closers = append(dc.closers, closer)
	return dc
}

func (dc *Datacenter) Create(spec *scenarios.DatacenterSpec) {
	dc.createPartitions(spec)
	dc.createTenantsAndMembers(spec)
	dc.createImages(spec)
	dc.createSizes(spec)
	CreateSizeReservations(dc.t, dc.TestStore, spec.SizeReservations)
	CreateSizeImageConstraints(dc.t, dc.TestStore, spec.SizeImageConstraints)
	CreateNetworks(dc.t, dc.TestStore, spec.Networks)
	CreateIPs(dc.t, dc.TestStore, spec.IPs)
	dc.createMachines(spec)
	dc.createSwitchesAndStatuses(spec)

	// this is done after creating all entities because some entities affect other entities upon creation and we want to start of with a consistent state between database and datacenter
	entities, err := getCurrentEntities(dc.t.Context(), dc.TestStore)
	require.NoError(dc.t, err)

	dc.Tenants = entities.Tenants
	dc.Projects = entities.Projects
	dc.Partitions = entities.Partitions
	dc.Sizes = entities.Sizes
	dc.Networks = entities.Networks
	dc.IPs = entities.IPs
	dc.Images = entities.Images
	dc.Switches = entities.Switches
	dc.Machines = entities.Machines
}

func (dc *Datacenter) Dump() {
	y, err := yaml.Marshal(dc)
	require.NoError(dc.t, err)

	fmt.Println(string(y))
}

func (dc *Datacenter) Close() {
	for _, close := range dc.closers {
		close()
	}
}

func (dc *Datacenter) CleanUp() {
	dc.TestStore.CleanUp(dc.t)
}

func (dc *Datacenter) createPartitions(spec *scenarios.DatacenterSpec) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintln(w, "a image")
	}))
	dc.closers = append(dc.closers, ts.Close)

	validURL := ts.URL

	var req []*adminv2.PartitionServiceCreateRequest
	for _, id := range spec.Partitions {
		p := &apiv2.Partition{
			Id:          id,
			Description: id,
			BootConfiguration: &apiv2.PartitionBootConfiguration{
				ImageUrl:  validURL,
				KernelUrl: validURL,
			},
		}
		req = append(req, &adminv2.PartitionServiceCreateRequest{Partition: p})
	}
	CreatePartitions(dc.t, dc.TestStore, req)
}

func (dc *Datacenter) createTenantsAndMembers(spec *scenarios.DatacenterSpec) {
	var (
		tenantCreateReq       []*apiv2.TenantServiceCreateRequest
		tenantMemberCreateReq []*repository.TenantMemberCreateRequest
		projectCreateReq      []*apiv2.ProjectServiceCreateRequest
	)

	assert.LessOrEqual(dc.t, len(tenantCreateReq), 9)
	assert.LessOrEqual(dc.t, len(projectCreateReq), 9)

	// TODO only works for 9 tenants with 9 projects
	const uuidtmpl = "%d0000000-0000-0000-0000-00000000000%d"
	for ti, tenant := range spec.Tenants {
		tenantCreateReq = append(tenantCreateReq, &apiv2.TenantServiceCreateRequest{
			Name: tenant,
		})
		for pi := range spec.ProjectsPerTenant {
			projectCreateReq = append(projectCreateReq, &apiv2.ProjectServiceCreateRequest{
				Name:  fmt.Sprintf(uuidtmpl, ti+1, pi+1),
				Login: tenant,
			})
		}
	}

	CreateTenants(dc.t, dc.TestStore, tenantCreateReq)
	CreateProjects(dc.t, dc.TestStore, projectCreateReq)

	for _, tenant := range spec.Tenants {
		tenantMemberCreateReq = append(tenantMemberCreateReq, &repository.TenantMemberCreateRequest{
			MemberID: tenant, Role: apiv2.TenantRole_TENANT_ROLE_OWNER,
		})
		CreateTenantMemberships(dc.t, dc.TestStore, tenant, tenantMemberCreateReq)
	}
}

func (dc *Datacenter) createImages(spec *scenarios.DatacenterSpec) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintln(w, "a image")
	}))
	dc.closers = append(dc.closers, ts.Close)

	var req []*adminv2.ImageServiceCreateRequest
	for name, feature := range spec.Images {
		req = append(req, &adminv2.ImageServiceCreateRequest{
			Image: &apiv2.Image{
				Id:       name,
				Url:      ts.URL,
				Features: []apiv2.ImageFeature{feature},
			},
		})
	}
	CreateImages(dc.t, dc.TestStore, req)
}

func (dc *Datacenter) createSizes(spec *scenarios.DatacenterSpec) {
	var req []*adminv2.SizeServiceCreateRequest
	for _, size := range spec.Sizes {
		req = append(req, &adminv2.SizeServiceCreateRequest{
			Size: size,
		})
	}
	CreateSizes(dc.t, dc.TestStore, req)
}

func (dc *Datacenter) createMachines(spec *scenarios.DatacenterSpec) {
	for _, pair := range spec.Machines {
		m, err := dc.TestStore.ds.Machine().Create(dc.t.Context(), pair.Machine)
		require.NoError(dc.t, err)

		var events []*metal.ProvisioningEventContainer
		ec := &metal.ProvisioningEventContainer{Base: metal.Base{ID: m.ID}, Liveliness: pair.Liveliness}
		if m.Waiting {
			ec.Events = append(ec.Events, metal.ProvisioningEvent{
				Event: metal.ProvisioningEventWaiting,
			})
		}
		if m.Allocation != nil {
			ec.Events = append(ec.Events, metal.ProvisioningEvent{
				Event: metal.ProvisioningEventPhonedHome,
			})
		}
		events = append(events, ec)
		for _, e := range events {
			_, err := dc.TestStore.ds.Event().Create(dc.t.Context(), e)
			require.NoError(dc.t, err)
		}
	}
}

func (dc *Datacenter) createSwitchesAndStatuses(spec *scenarios.DatacenterSpec) {
	reqs := lo.Map(spec.Switches, func(sw *apiv2.Switch, _ int) *repository.SwitchServiceCreateRequest {
		return &repository.SwitchServiceCreateRequest{Switch: sw}
	})
	CreateSwitches(dc.t, dc.TestStore, reqs)

	statuses := lo.Map(spec.Switches, func(sw *apiv2.Switch, _ int) *repository.SwitchStatus {
		return &repository.SwitchStatus{
			ID: sw.Id,
		}
	})
	CreateSwitchStatuses(dc.t, dc.TestStore, statuses)
}

func getCurrentEntities(ctx context.Context, store *testStore) (*Datacenter, error) {
	current := &Datacenter{}

	tenants, err := store.Tenant().List(ctx, &apiv2.TenantServiceListRequest{})
	if err != nil {
		return nil, err
	}
	current.Tenants = lo.Map(tenants, func(t *apiv2.Tenant, _ int) string {
		return t.Login
	})
	projects, err := store.UnscopedProject().List(ctx, &apiv2.ProjectServiceListRequest{})
	if err != nil {
		return nil, err
	}
	current.Projects = map[string][]string{}
	for _, p := range projects {
		current.Projects[p.Tenant] = append(current.Projects[p.Tenant], p.Uuid)
		slices.Sort(current.Projects[p.Tenant])
	}
	partitions, err := store.Partition().List(ctx, &apiv2.PartitionQuery{})
	if err != nil {
		return nil, err
	}
	current.Partitions = map[string]*apiv2.Partition{}
	for _, p := range partitions {
		current.Partitions[p.Id] = p
	}
	sizes, err := store.Size().List(ctx, &apiv2.SizeQuery{})
	if err != nil {
		return nil, err
	}
	current.Sizes = map[string]*apiv2.Size{}
	for _, s := range sizes {
		current.Sizes[s.Id] = s
	}
	networks, err := store.UnscopedNetwork().List(ctx, &apiv2.NetworkQuery{})
	if err != nil {
		return nil, err
	}
	current.Networks = map[string]*apiv2.Network{}
	for _, n := range networks {
		current.Networks[n.Id] = n
	}
	ips, err := store.UnscopedIP().List(ctx, &apiv2.IPQuery{})
	if err != nil {
		return nil, err
	}
	current.IPs = map[string]*apiv2.IP{}
	for _, ip := range ips {
		current.IPs[ip.Ip] = ip
	}
	images, err := store.Image().List(ctx, &apiv2.ImageQuery{})
	if err != nil {
		return nil, err
	}
	current.Images = map[string]*apiv2.Image{}
	for _, i := range images {
		current.Images[i.Id] = i
	}
	switches, err := store.Switch().List(ctx, &apiv2.SwitchQuery{})
	if err != nil {
		return nil, err
	}
	current.Switches = map[string]*apiv2.Switch{}
	for _, sw := range switches {
		current.Switches[sw.Id] = sw
	}
	machines, err := store.UnscopedMachine().List(ctx, &apiv2.MachineQuery{})
	if err != nil {
		return nil, err
	}
	current.Machines = map[string]*apiv2.Machine{}
	for _, m := range machines {
		current.Machines[m.Uuid] = m
	}
	return current, nil
}

// Assert tests whether all changes applied to a Datacenter were intended.
//
// Usage:
//
// After calling Create on a Datacenter create a copy of it by calling Copy.
// Run the functions you are testing.
// Call Assert and pass the copy of the Datacenter, the original (possibly modified) Datacenter, and a modify function.
// The modify function contains all changes that you expect to have been applied by the functions you are testing.
// Assert will apply the modify function and fail if the Datacenters differ after that.
func (dc *Datacenter) Assert(modify func(*Datacenter), opts ...cmp.Option) error {
	current, err := getCurrentEntities(dc.t.Context(), dc.TestStore)
	if err != nil {
		return err
	}

	if modify != nil {
		modify(dc)
	}

	options := slices.Concat(opts, []cmp.Option{
		protocmp.Transform(),
		cmpopts.IgnoreFields(
			Datacenter{}, "TestStore", "t", "closers",
		),
		protocmp.IgnoreFields(
			&apiv2.Partition{}, "meta",
		),
		protocmp.IgnoreFields(
			&apiv2.Size{}, "meta",
		),
		protocmp.IgnoreFields(
			&apiv2.Network{}, "meta",
		),
		protocmp.IgnoreFields(
			&apiv2.IP{}, "meta",
		),
		protocmp.IgnoreFields(
			&apiv2.Image{}, "meta",
		),
		protocmp.IgnoreFields(
			&timestamppb.Timestamp{}, "nanos",
		),
		protocmp.IgnoreFields(
			&apiv2.Switch{}, "meta",
		),
		protocmp.IgnoreFields(
			&apiv2.Machine{}, "meta",
		),
	})

	diff := cmp.Diff(dc, current, options...)
	if diff != "" {
		return fmt.Errorf("datacenters differ: %s", diff)
	}
	return nil
}
