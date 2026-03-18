package test

import (
	"context"
	"encoding/json"
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
	"google.golang.org/protobuf/testing/protocmp"
)

type (
	Datacenter struct {
		tenants        []string
		projects       map[string][]string
		partitions     map[string]*apiv2.Partition
		sizes          map[string]*apiv2.Size
		networks       map[string]*apiv2.Network
		ips            map[string]*apiv2.IP
		images         map[string]*apiv2.Image
		switches       map[string]*apiv2.Switch
		switchStatuses map[string]*metal.SwitchStatus
		machines       map[string]*apiv2.Machine

		testStore *testStore
		t         testing.TB
		closers   []func()
	}

	Asserters struct {
		Tenants        func(tenants []string)
		Projects       func(projects map[string][]string)
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
		t:              t,
		testStore:      testStore,
		projects:       make(map[string][]string),
		partitions:     make(map[string]*apiv2.Partition),
		sizes:          make(map[string]*apiv2.Size),
		networks:       make(map[string]*apiv2.Network),
		ips:            make(map[string]*apiv2.IP),
		images:         make(map[string]*apiv2.Image),
		switches:       make(map[string]*apiv2.Switch),
		switchStatuses: make(map[string]*metal.SwitchStatus),
		machines:       make(map[string]*apiv2.Machine),
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
	CreateSizeReservations(dc.t, dc.testStore, spec.SizeReservations)
	CreateSizeImageConstraints(dc.t, dc.testStore, spec.SizeImageConstraints)
	CreateNetworks(dc.t, dc.testStore, spec.Networks)
	CreateIPs(dc.t, dc.testStore, spec.IPs)
	dc.createMachines(spec)
	dc.createSwitchesAndStatuses(spec)

	// this is done after creating all entities because some entities affect other entities upon creation and we want to start of with a consistent state between database and datacenter
	entities, err := getCurrentEntities(dc.t.Context(), dc.testStore)
	require.NoError(dc.t, err)

	dc.tenants = entities.tenants
	dc.projects = entities.projects
	dc.partitions = entities.partitions
	dc.sizes = entities.sizes
	dc.networks = entities.networks
	dc.ips = entities.ips
	dc.images = entities.images
	dc.switches = entities.switches
	dc.switchStatuses = entities.switchStatuses
	dc.machines = entities.machines
}

func (dc *Datacenter) GetTenants() []string {
	return dc.tenants
}

func (dc *Datacenter) GetProjects() map[string][]string {
	return dc.projects
}

func (dc *Datacenter) GetPartitions() map[string]*apiv2.Partition {
	return dc.partitions
}

func (dc *Datacenter) GetSizes() map[string]*apiv2.Size {
	return dc.sizes
}

func (dc *Datacenter) GetNetworks() map[string]*apiv2.Network {
	return dc.networks
}

func (dc *Datacenter) GetIPs() map[string]*apiv2.IP {
	return dc.ips
}

func (dc *Datacenter) GetImages() map[string]*apiv2.Image {
	return dc.images
}

func (dc *Datacenter) GetSwitches() map[string]*apiv2.Switch {
	return dc.switches
}

func (dc *Datacenter) GetSwitchStatuses() map[string]*metal.SwitchStatus {
	return dc.switchStatuses
}

func (dc *Datacenter) GetMachines() map[string]*apiv2.Machine {
	return dc.machines
}

func (dc *Datacenter) Close() {
	for _, close := range dc.closers {
		close()
	}
}

func (dc *Datacenter) Cleanup() {
	dc.testStore.CleanUp(dc.t)
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
func (dc *Datacenter) Assert(mods *Asserters, opts ...cmp.Option) error {
	copied, err := dc.copyEntities()
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
		cmp.AllowUnexported(Datacenter{}),
		cmpopts.IgnoreFields(
			Datacenter{}, "testStore", "t", "closers",
		),
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

	CreateTenants(dc.t, dc.testStore, tenantCreateReq)
	CreateProjects(dc.t, dc.testStore, projectCreateReq)

	for _, tenant := range spec.Tenants {
		tenantMemberCreateReq = append(tenantMemberCreateReq, &repository.TenantMemberCreateRequest{
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

func (dc *Datacenter) createSwitchesAndStatuses(spec *scenarios.DatacenterSpec) {
	reqs := lo.Map(spec.Switches, func(sw *apiv2.Switch, _ int) *repository.SwitchServiceCreateRequest {
		return &repository.SwitchServiceCreateRequest{Switch: sw}
	})
	CreateSwitches(dc.t, dc.testStore, reqs)

	statuses := lo.Map(spec.Switches, func(sw *apiv2.Switch, _ int) *repository.SwitchStatus {
		return &repository.SwitchStatus{
			ID: sw.Id,
		}
	})
	CreateSwitchStatuses(dc.t, dc.testStore, statuses)
}

func (dc *Datacenter) copyEntities() (*Datacenter, error) {
	var (
		copied = &Datacenter{
			tenants:  []string{},
			projects: map[string][]string{},
		}
		err error
	)

	copied.tenants = append([]string{}, dc.tenants...)
	for tenant, projects := range dc.projects {
		copied.projects[tenant] = append([]string{}, projects...)
	}
	if copied.partitions, err = deepCopy(dc.partitions); err != nil {
		return nil, err
	}
	if copied.sizes, err = deepCopy(dc.sizes); err != nil {
		return nil, err
	}
	if copied.networks, err = deepCopy(dc.networks); err != nil {
		return nil, err
	}
	if copied.ips, err = deepCopy(dc.ips); err != nil {
		return nil, err
	}
	if copied.images, err = deepCopy(dc.images); err != nil {
		return nil, err
	}
	if copied.switches, err = deepCopy(dc.switches); err != nil {
		return nil, err
	}
	if copied.switchStatuses, err = deepCopy(dc.switchStatuses); err != nil {
		return nil, err
	}
	if copied.machines, err = deepCopy(dc.machines); err != nil {
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

func getCurrentEntities(ctx context.Context, store *testStore) (*Datacenter, error) {
	current := &Datacenter{}

	tenants, err := store.Tenant().List(ctx, &apiv2.TenantServiceListRequest{})
	if err != nil {
		return nil, err
	}
	current.tenants = lo.Map(tenants, func(t *apiv2.Tenant, _ int) string {
		return t.Login
	})
	projects, err := store.UnscopedProject().List(ctx, &apiv2.ProjectServiceListRequest{})
	if err != nil {
		return nil, err
	}
	current.projects = map[string][]string{}
	for _, p := range projects {
		current.projects[p.Tenant] = append(current.projects[p.Tenant], p.Uuid)
		slices.Sort(current.projects[p.Tenant])
	}
	partitions, err := store.Partition().List(ctx, &apiv2.PartitionQuery{})
	if err != nil {
		return nil, err
	}
	current.partitions = map[string]*apiv2.Partition{}
	for _, p := range partitions {
		current.partitions[p.Id] = p
	}
	sizes, err := store.Size().List(ctx, &apiv2.SizeQuery{})
	if err != nil {
		return nil, err
	}
	current.sizes = map[string]*apiv2.Size{}
	for _, s := range sizes {
		current.sizes[s.Id] = s
	}
	networks, err := store.UnscopedNetwork().List(ctx, &apiv2.NetworkQuery{})
	if err != nil {
		return nil, err
	}
	current.networks = map[string]*apiv2.Network{}
	for _, n := range networks {
		current.networks[n.Id] = n
	}
	ips, err := store.UnscopedIP().List(ctx, &apiv2.IPQuery{})
	if err != nil {
		return nil, err
	}
	current.ips = map[string]*apiv2.IP{}
	for _, ip := range ips {
		current.ips[ip.Ip] = ip
	}
	images, err := store.Image().List(ctx, &apiv2.ImageQuery{})
	if err != nil {
		return nil, err
	}
	current.images = map[string]*apiv2.Image{}
	for _, i := range images {
		current.images[i.Id] = i
	}
	switches, err := store.Switch().List(ctx, &apiv2.SwitchQuery{})
	if err != nil {
		return nil, err
	}
	current.switches = map[string]*apiv2.Switch{}
	current.switchStatuses = map[string]*metal.SwitchStatus{}
	for _, sw := range switches {
		current.switches[sw.Id] = sw

		status, err := store.GetSwitchStatus(sw.Id)
		if err != nil {
			return nil, err
		}
		current.switchStatuses[sw.Id] = status
	}
	machines, err := store.UnscopedMachine().List(ctx, &apiv2.MachineQuery{})
	if err != nil {
		return nil, err
	}
	current.machines = map[string]*apiv2.Machine{}
	for _, m := range machines {
		current.machines[m.Uuid] = m
	}
	return current, nil
}
