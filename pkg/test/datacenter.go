package test

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-apiserver/pkg/repository/api"
	"github.com/metal-stack/metal-apiserver/pkg/test/scenarios"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/testing/protocmp"
)

type (
	Datacenter struct {
		entities  *entities
		testStore *testStore
		t         testing.TB
		closers   []func()
	}

	entities struct {
		tenants        map[string]*apiv2.Tenant
		projects       map[string][]*apiv2.Project
		partitions     map[string]*apiv2.Partition
		sizes          map[string]*apiv2.Size
		networks       map[string]*apiv2.Network
		ips            map[string]*apiv2.IP
		images         map[string]*apiv2.Image
		switches       map[string]*apiv2.Switch
		switchStatuses map[string]*metal.SwitchStatus
		machines       map[string]*apiv2.Machine
	}

	Asserters struct {
		Tenants        func(tenants map[string]*apiv2.Tenant)
		Projects       func(projects map[string][]*apiv2.Project)
		Partitions     func(partitions map[string]*apiv2.Partition)
		Sizes          func(sizes map[string]*apiv2.Size)
		Networks       func(networks map[string]*apiv2.Network)
		IPs            func(ips map[string]*apiv2.IP)
		Images         func(images map[string]*apiv2.Image)
		Switches       func(switches map[string]*apiv2.Switch)
		SwitchStatuses func(switchStatuses map[string]*metal.SwitchStatus)
		Machines       func(machines map[string]*apiv2.Machine)
	}
)

func NewDatacenter(t testing.TB, log *slog.Logger, testOpts ...testOpt) *Datacenter {
	testStore, closer := StartRepositoryWithCleanup(t, log, testOpts...)

	dc := &Datacenter{
		t:         t,
		testStore: testStore,
		entities: &entities{
			tenants:        make(map[string]*apiv2.Tenant),
			projects:       make(map[string][]*apiv2.Project),
			partitions:     make(map[string]*apiv2.Partition),
			sizes:          make(map[string]*apiv2.Size),
			networks:       make(map[string]*apiv2.Network),
			ips:            make(map[string]*apiv2.IP),
			images:         make(map[string]*apiv2.Image),
			switches:       make(map[string]*apiv2.Switch),
			switchStatuses: make(map[string]*metal.SwitchStatus),
			machines:       make(map[string]*apiv2.Machine),
		},
	}

	dc.closers = append(dc.closers, closer)
	return dc
}

func (dc *Datacenter) GetTestStore() *testStore {
	return dc.testStore
}

func (dc *Datacenter) Create(spec *scenarios.DatacenterSpec) {
	dc.createPartitions(spec)
	dc.createTenantsAndMembers(spec)
	dc.createImages(spec)
	dc.createSizes(spec)
	dc.createSizeReservations(spec)
	dc.createSizeImageConstraints(spec)
	dc.createNetworks(spec)
	dc.createIPs(spec)
	dc.createMachines(spec)
	dc.createSwitches(spec)

	// this is done after creating all currentEntities because some currentEntities affect other currentEntities upon creation and we want to start of with a consistent state between database and datacenter
	currentEntities, err := getCurrentEntities(dc.t.Context(), dc.testStore)
	require.NoError(dc.t, err)
	e := &entities{}

	e.tenants = currentEntities.tenants
	e.projects = currentEntities.projects
	e.partitions = currentEntities.partitions
	e.sizes = currentEntities.sizes
	e.networks = currentEntities.networks
	e.ips = currentEntities.ips
	e.images = currentEntities.images
	e.switches = currentEntities.switches
	e.switchStatuses = currentEntities.switchStatuses
	e.machines = currentEntities.machines
	dc.entities = e
}

func (dc *Datacenter) Snapshot() *entities {
	copied, err := dc.entities.deepCopy()
	require.NoError(dc.t, err)
	return copied
}

func (dc *Datacenter) GetTenants() map[string]*apiv2.Tenant {
	return dc.entities.tenants
}

func (dc *Datacenter) GetProjects() map[string][]*apiv2.Project {
	return dc.entities.projects
}

func (dc *Datacenter) GetPartitions() map[string]*apiv2.Partition {
	return dc.entities.partitions
}

func (dc *Datacenter) GetSizes() map[string]*apiv2.Size {
	return dc.entities.sizes
}

func (dc *Datacenter) GetNetworks() map[string]*apiv2.Network {
	return dc.entities.networks
}

func (dc *Datacenter) GetIPs() map[string]*apiv2.IP {
	return dc.entities.ips
}

func (dc *Datacenter) GetImages() map[string]*apiv2.Image {
	return dc.entities.images
}

func (dc *Datacenter) GetSwitches() map[string]*apiv2.Switch {
	return dc.entities.switches
}

func (dc *Datacenter) GetSwitchStatuses() map[string]*metal.SwitchStatus {
	return dc.entities.switchStatuses
}

func (dc *Datacenter) GetMachines() map[string]*apiv2.Machine {
	return dc.entities.machines
}

func (dc *Datacenter) Close() {
	for _, close := range dc.closers {
		close()
	}
}

func (dc *Datacenter) Cleanup() {
	dc.testStore.Cleanup(dc.t)
}

// Assert tests whether all of the intended changes (and no others) were applied to the database.
//
// Usage:
//
// Define modifier functions that express what changes you expect the functions you are testing to apply to the database.
// Run the functions you are testing.
// Call dc.Assert(mods) with the modifiers you defined.
// Assert will fetch all current entities from the database and apply the modifications to the current datacenter.
// If the results differ Assert will return an error containing the diff.
// A `-` in the diff indicates a field that was expected but is not present in the database.
// A `+` in the diff indicates a field that was unexpectedly present in the database.
func (dc *Datacenter) Assert(snapshot *entities, mods *Asserters, opts ...cmp.Option) error {
	copied, err := snapshot.deepCopy()
	require.NoError(dc.t, err)

	if mods != nil {
		if mods.Tenants != nil {
			mods.Tenants(copied.tenants)
		}
		if mods.Projects != nil {
			mods.Projects(copied.projects)
		}
		if mods.Partitions != nil {
			mods.Partitions(copied.partitions)
		}
		if mods.Sizes != nil {
			mods.Sizes(copied.sizes)
		}
		if mods.Networks != nil {
			mods.Networks(copied.networks)
		}
		if mods.IPs != nil {
			mods.IPs(copied.ips)
		}
		if mods.Images != nil {
			mods.Images(copied.images)
		}
		if mods.Switches != nil {
			mods.Switches(copied.switches)
		}
		if mods.SwitchStatuses != nil {
			mods.SwitchStatuses(copied.switchStatuses)
		}
		if mods.Machines != nil {
			mods.Machines(copied.machines)
		}
	}

	current, err := getCurrentEntities(dc.t.Context(), dc.testStore)
	if err != nil {
		return err
	}

	options := slices.Concat(opts, []cmp.Option{
		protocmp.Transform(),
		cmp.AllowUnexported(entities{}),
		cmpopts.IgnoreFields(
			metal.Base{}, "Created", "Changed", "Generation",
		),
		protocmp.IgnoreFields(
			&apiv2.Meta{}, "generation", "updated_at", "created_at",
		),
	})

	diff := cmp.Diff(copied, current, options...)
	if diff != "" {
		return fmt.Errorf("datacenters differ: %s", diff)
	}
	return nil
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
	CreatePartitions(dc.t, dc.testStore, req)
}

func (dc *Datacenter) createTenantsAndMembers(spec *scenarios.DatacenterSpec) {
	var (
		tenantCreateReq       []*apiv2.TenantServiceCreateRequest
		tenantMemberCreateReq []*api.TenantMemberCreateRequest
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

	CreateTenants(dc.t, dc.testStore, tenantCreateReq)
	CreateProjects(dc.t, dc.testStore, projectCreateReq)

	for _, tenant := range spec.Tenants {
		tenantMemberCreateReq = append(tenantMemberCreateReq, &api.TenantMemberCreateRequest{
			MemberID: tenant, Role: apiv2.TenantRole_TENANT_ROLE_OWNER,
		})
		CreateTenantMemberships(dc.t, dc.testStore, tenant, tenantMemberCreateReq)
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
	CreateImages(dc.t, dc.testStore, req)
}

func (dc *Datacenter) createSizes(spec *scenarios.DatacenterSpec) {
	var req []*adminv2.SizeServiceCreateRequest
	for _, size := range spec.Sizes {
		req = append(req, &adminv2.SizeServiceCreateRequest{
			Size: size,
		})
	}
	CreateSizes(dc.t, dc.testStore, req)
}

func (dc *Datacenter) createSizeReservations(spec *scenarios.DatacenterSpec) {
	CreateSizeReservations(dc.t, dc.testStore, spec.SizeReservations)
}

func (dc *Datacenter) createSizeImageConstraints(spec *scenarios.DatacenterSpec) {
	CreateSizeImageConstraints(dc.t, dc.testStore, spec.SizeImageConstraints)
}

func (dc *Datacenter) createNetworks(spec *scenarios.DatacenterSpec) {
	CreateNetworks(dc.t, dc.testStore, spec.Networks)
}

func (dc *Datacenter) createIPs(spec *scenarios.DatacenterSpec) {
	CreateIPs(dc.t, dc.testStore, spec.IPs)
}

func (dc *Datacenter) createMachines(spec *scenarios.DatacenterSpec) {
	for _, pair := range spec.Machines {
		m, err := dc.testStore.ds.Machine().Create(dc.t.Context(), pair.Machine)
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
			_, err := dc.testStore.ds.Event().Create(dc.t.Context(), e)
			require.NoError(dc.t, err)
		}
	}
}

func (dc *Datacenter) createSwitches(spec *scenarios.DatacenterSpec) {
	reqs := lo.Map(spec.Switches, func(sw *apiv2.Switch, _ int) *api.SwitchServiceCreateRequest {
		return &api.SwitchServiceCreateRequest{Switch: sw}
	})
	CreateSwitches(dc.t, dc.testStore, reqs)
}

func (e *entities) deepCopy() (*entities, error) {
	var (
		copied = &entities{}
		err    error
	)

	if copied.tenants, err = deepCopy(e.tenants); err != nil {
		return nil, err
	}
	if copied.projects, err = deepCopy(e.projects); err != nil {
		return nil, err
	}
	if copied.partitions, err = deepCopy(e.partitions); err != nil {
		return nil, err
	}
	if copied.sizes, err = deepCopy(e.sizes); err != nil {
		return nil, err
	}
	if copied.networks, err = deepCopy(e.networks); err != nil {
		return nil, err
	}
	if copied.ips, err = deepCopy(e.ips); err != nil {
		return nil, err
	}
	if copied.images, err = deepCopy(e.images); err != nil {
		return nil, err
	}
	if copied.switches, err = deepCopy(e.switches); err != nil {
		return nil, err
	}
	if copied.switchStatuses, err = deepCopy(e.switchStatuses); err != nil {
		return nil, err
	}
	if copied.machines, err = deepCopy(e.machines); err != nil {
		return nil, err
	}

	return copied, nil
}

func deepCopy[T any](in T) (T, error) {
	var out T
	bytes, err := json.Marshal(in)
	if err != nil {
		return out, err
	}
	err = json.Unmarshal(bytes, &out)
	if err != nil {
		return out, err
	}
	return out, nil
}

func getCurrentEntities(ctx context.Context, store *testStore) (*entities, error) {
	e := &entities{}

	tenants, err := store.Tenant().List(ctx, &apiv2.TenantServiceListRequest{})
	if err != nil {
		return nil, err
	}
	e.tenants = map[string]*apiv2.Tenant{}
	for _, t := range tenants {
		e.tenants[t.Login] = t
	}
	projects, err := store.UnscopedProject().List(ctx, &apiv2.ProjectServiceListRequest{})
	if err != nil {
		return nil, err
	}
	e.projects = map[string][]*apiv2.Project{}
	for _, p := range projects {
		e.projects[p.Tenant] = append(e.projects[p.Tenant], p)
		slices.SortStableFunc(e.projects[p.Tenant], func(p1, p2 *apiv2.Project) int {
			return strings.Compare(p1.Uuid, p2.Uuid)
		})
	}
	partitions, err := store.Partition().List(ctx, &apiv2.PartitionQuery{})
	if err != nil {
		return nil, err
	}
	e.partitions = map[string]*apiv2.Partition{}
	for _, p := range partitions {
		e.partitions[p.Id] = p
	}
	sizes, err := store.Size().List(ctx, &apiv2.SizeQuery{})
	if err != nil {
		return nil, err
	}
	e.sizes = map[string]*apiv2.Size{}
	for _, s := range sizes {
		e.sizes[s.Id] = s
	}
	networks, err := store.UnscopedNetwork().List(ctx, &apiv2.NetworkQuery{})
	if err != nil {
		return nil, err
	}
	e.networks = map[string]*apiv2.Network{}
	for _, n := range networks {
		e.networks[n.Id] = n
	}
	ips, err := store.UnscopedIP().List(ctx, &apiv2.IPQuery{})
	if err != nil {
		return nil, err
	}
	e.ips = map[string]*apiv2.IP{}
	for _, ip := range ips {
		e.ips[ip.Ip] = ip
	}
	images, err := store.Image().List(ctx, &apiv2.ImageQuery{})
	if err != nil {
		return nil, err
	}
	e.images = map[string]*apiv2.Image{}
	for _, i := range images {
		e.images[i.Id] = i
	}
	switches, err := store.Switch().List(ctx, &apiv2.SwitchQuery{})
	if err != nil {
		return nil, err
	}
	e.switches = map[string]*apiv2.Switch{}
	e.switchStatuses = map[string]*metal.SwitchStatus{}
	for _, sw := range switches {
		e.switches[sw.Id] = sw

		status, err := store.GetSwitchStatus(sw.Id)
		if err != nil {
			return nil, err
		}
		e.switchStatuses[sw.Id] = status
	}
	machines, err := store.UnscopedMachine().List(ctx, &apiv2.MachineQuery{})
	if err != nil {
		return nil, err
	}
	e.machines = map[string]*apiv2.Machine{}
	for _, m := range machines {
		e.machines[m.Uuid] = m
	}
	return e, nil
}
