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
		entities  *Entities
		testStore *testStore
		t         testing.TB
		closers   []func()
	}

	Entities struct {
		Tenants           map[string]*apiv2.Tenant
		Projects          map[string][]*apiv2.Project
		Partitions        map[string]*apiv2.Partition
		FilesystemLayouts map[string]*apiv2.FilesystemLayout
		Sizes             map[string]*apiv2.Size
		Networks          map[string]*apiv2.Network
		Ips               map[string]*apiv2.IP
		Images            map[string]*apiv2.Image
		Switches          map[string]*apiv2.Switch
		SwitchStatuses    map[string]*metal.SwitchStatus
		Machines          map[string]*apiv2.Machine
	}

	Asserters struct {
		Tenants           func(tenants map[string]*apiv2.Tenant)
		Projects          func(projects map[string][]*apiv2.Project)
		Partitions        func(partitions map[string]*apiv2.Partition)
		Sizes             func(sizes map[string]*apiv2.Size)
		FilesystemLayouts func(filesystemLayouts map[string]*apiv2.FilesystemLayout)
		Networks          func(networks map[string]*apiv2.Network)
		IPs               func(ips map[string]*apiv2.IP)
		Images            func(images map[string]*apiv2.Image)
		Switches          func(switches map[string]*apiv2.Switch)
		SwitchStatuses    func(switchStatuses map[string]*metal.SwitchStatus)
		Machines          func(machines map[string]*apiv2.Machine)
	}
)

func NewDatacenter(t testing.TB, log *slog.Logger, testOpts ...testOpt) *Datacenter {
	testStore, closer := StartRepositoryWithCleanup(t, log, testOpts...)

	dc := &Datacenter{
		t:         t,
		testStore: testStore,
		entities: &Entities{
			Tenants:           make(map[string]*apiv2.Tenant),
			Projects:          make(map[string][]*apiv2.Project),
			Partitions:        make(map[string]*apiv2.Partition),
			Sizes:             make(map[string]*apiv2.Size),
			FilesystemLayouts: make(map[string]*apiv2.FilesystemLayout),
			Networks:          make(map[string]*apiv2.Network),
			Ips:               make(map[string]*apiv2.IP),
			Images:            make(map[string]*apiv2.Image),
			Switches:          make(map[string]*apiv2.Switch),
			SwitchStatuses:    make(map[string]*metal.SwitchStatus),
			Machines:          make(map[string]*apiv2.Machine),
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
	dc.createFilesystemLayouts(spec)
	dc.createSizeImageConstraints(spec)
	dc.createNetworks(spec)
	dc.createIPs(spec)
	dc.createMachines(spec)
	dc.createSwitches(spec)
	dc.createSwitchStatuses(spec)

	// this is done after creating all currentEntities because some currentEntities affect other currentEntities upon creation and we want to start of with a consistent state between database and datacenter
	currentEntities, err := getCurrentEntities(dc.t.Context(), dc.testStore)
	require.NoError(dc.t, err)
	e := &Entities{}

	e.Tenants = currentEntities.Tenants
	e.Projects = currentEntities.Projects
	e.Partitions = currentEntities.Partitions
	e.Sizes = currentEntities.Sizes
	e.FilesystemLayouts = currentEntities.FilesystemLayouts
	e.Networks = currentEntities.Networks
	e.Ips = currentEntities.Ips
	e.Images = currentEntities.Images
	e.Switches = currentEntities.Switches
	e.SwitchStatuses = currentEntities.SwitchStatuses
	e.Machines = currentEntities.Machines
	dc.entities = e
}

func (dc *Datacenter) Snapshot() *Entities {
	copied, err := dc.entities.deepCopy()
	require.NoError(dc.t, err)
	return copied
}

func (dc *Datacenter) GetTenants() map[string]*apiv2.Tenant {
	return dc.entities.Tenants
}

func (dc *Datacenter) GetProjects() map[string][]*apiv2.Project {
	return dc.entities.Projects
}

func (dc *Datacenter) GetPartitions() map[string]*apiv2.Partition {
	return dc.entities.Partitions
}

func (dc *Datacenter) GetSizes() map[string]*apiv2.Size {
	return dc.entities.Sizes
}

func (dc *Datacenter) GetNetworks() map[string]*apiv2.Network {
	return dc.entities.Networks
}

func (dc *Datacenter) GetNetworkByName(name string) *apiv2.Network {
	for _, n := range dc.entities.Networks {
		if n.Name != nil && *n.Name == name {
			return n
		}
	}
	return nil
}

func (dc *Datacenter) GetIPs() map[string]*apiv2.IP {
	return dc.entities.Ips
}

func (dc *Datacenter) GetImages() map[string]*apiv2.Image {
	return dc.entities.Images
}

func (dc *Datacenter) GetSwitches() map[string]*apiv2.Switch {
	return dc.entities.Switches
}

func (dc *Datacenter) GetSwitchStatuses() map[string]*metal.SwitchStatus {
	return dc.entities.SwitchStatuses
}

func (dc *Datacenter) GetMachines() map[string]*apiv2.Machine {
	return dc.entities.Machines
}

func (dc *Datacenter) GetFilesystemLayouts() map[string]*apiv2.FilesystemLayout {
	return dc.entities.FilesystemLayouts
}

func (dc *Datacenter) Close() {
	for _, close := range dc.closers {
		close()
	}
}

func (dc *Datacenter) Cleanup() {
	dc.testStore.Cleanup(dc.t)
}

// AssertSnapshot tests whether all of the intended changes (and no others) were applied to the database as compared to a specific snapshot.
//
// Usage:
//
// Define modifier functions that express what changes you expect the functions you are testing to apply to the database.
// Run the functions you are testing.
// Call dc.AssertSnapshot(snapshot, mods) with the modifiers and a snapshot that you created at some point before calling this func.
// AssertSnapshot will fetch all current entities from the database and apply the modifications to the snapshot you passed.
// After the modifications were applied the snapshot is expected to be equal to the actual current state.
// If the results differ AssertSnapshot will return an error containing the diff.
// A `-` in the diff indicates a field that was expected but is not present in the database.
// A `+` in the diff indicates a field that was unexpectedly present in the database.
func (dc *Datacenter) AssertSnapshot(snapshot *Entities, mods *Asserters, opts ...cmp.Option) error {
	copied, err := snapshot.deepCopy()
	require.NoError(dc.t, err)

	if mods != nil {
		if mods.Tenants != nil {
			mods.Tenants(copied.Tenants)
		}
		if mods.Projects != nil {
			mods.Projects(copied.Projects)
		}
		if mods.Partitions != nil {
			mods.Partitions(copied.Partitions)
		}
		if mods.Sizes != nil {
			mods.Sizes(copied.Sizes)
		}
		if mods.FilesystemLayouts != nil {
			mods.FilesystemLayouts(copied.FilesystemLayouts)
		}
		if mods.Networks != nil {
			mods.Networks(copied.Networks)
		}
		if mods.IPs != nil {
			mods.IPs(copied.Ips)
		}
		if mods.Images != nil {
			mods.Images(copied.Images)
		}
		if mods.Switches != nil {
			mods.Switches(copied.Switches)
		}
		if mods.SwitchStatuses != nil {
			mods.SwitchStatuses(copied.SwitchStatuses)
		}
		if mods.Machines != nil {
			mods.Machines(copied.Machines)
		}
	}

	current, err := getCurrentEntities(dc.t.Context(), dc.testStore)
	if err != nil {
		return err
	}

	options := slices.Concat(opts, []cmp.Option{
		protocmp.Transform(),
		cmp.AllowUnexported(Entities{}),
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

// Assert tests whether all of the intended changes (and no others) were applied to the database as compared to its initial state.
//
// Usage:
//
// Define modifier functions (mods) that express what changes you expect the functions you are testing to apply to the database.
// Run the functions you are testing.
// Call dc.Assert(mods) with the modifiers and a snapshot that you created at some point before calling this func.
// Assert will fetch all current entities from the database and apply the modifications to a deepcopy of its initial state.
// After the modifications were applied the new state is expected to be equal to the actual current state.
// If the results differ Assert will return an error containing the diff.
// A `-` in the diff indicates a field that was expected but is not present in the database.
// A `+` in the diff indicates a field that was unexpectedly present in the database.
func (dc *Datacenter) Assert(mods *Asserters, opts ...cmp.Option) error {
	snapshot := dc.Snapshot()

	if mods != nil {
		if mods.Tenants != nil {
			mods.Tenants(snapshot.Tenants)
		}
		if mods.Projects != nil {
			mods.Projects(snapshot.Projects)
		}
		if mods.Partitions != nil {
			mods.Partitions(snapshot.Partitions)
		}
		if mods.Sizes != nil {
			mods.Sizes(snapshot.Sizes)
		}
		if mods.FilesystemLayouts != nil {
			mods.FilesystemLayouts(snapshot.FilesystemLayouts)
		}
		if mods.Networks != nil {
			mods.Networks(snapshot.Networks)
		}
		if mods.IPs != nil {
			mods.IPs(snapshot.Ips)
		}
		if mods.Images != nil {
			mods.Images(snapshot.Images)
		}
		if mods.Switches != nil {
			mods.Switches(snapshot.Switches)
		}
		if mods.SwitchStatuses != nil {
			mods.SwitchStatuses(snapshot.SwitchStatuses)
		}
		if mods.Machines != nil {
			mods.Machines(snapshot.Machines)
		}
	}

	current, err := getCurrentEntities(dc.t.Context(), dc.testStore)
	if err != nil {
		return err
	}

	options := slices.Concat(opts, []cmp.Option{
		protocmp.Transform(),
		cmp.AllowUnexported(Entities{}),
		cmpopts.IgnoreFields(
			metal.Base{}, "Created", "Changed", "Generation",
		),
		protocmp.IgnoreFields(
			&apiv2.Meta{}, "generation", "updated_at", "created_at",
		),
	})

	diff := cmp.Diff(snapshot, current, options...)
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
				Id:             name,
				Url:            ts.URL,
				Features:       []apiv2.ImageFeature{feature},
				Classification: apiv2.ImageClassification_IMAGE_CLASSIFICATION_SUPPORTED,
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

func (dc *Datacenter) createFilesystemLayouts(spec *scenarios.DatacenterSpec) {
	for _, fsl := range spec.FilesystemLayouts {
		_, err := dc.GetTestStore().FilesystemLayout().Create(dc.t.Context(), fsl)
		require.NoError(dc.t, err)
	}
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

func (dc *Datacenter) createSwitchStatuses(spec *scenarios.DatacenterSpec) {
	CreateSwitchStatuses(dc.t, dc.testStore, spec.SwitchStatuses)
}

func (e *Entities) deepCopy() (*Entities, error) {
	var (
		copied = &Entities{}
		err    error
	)

	if copied.Tenants, err = deepCopy(e.Tenants); err != nil {
		return nil, err
	}
	if copied.Projects, err = deepCopy(e.Projects); err != nil {
		return nil, err
	}
	if copied.Partitions, err = deepCopy(e.Partitions); err != nil {
		return nil, err
	}
	if copied.Sizes, err = deepCopy(e.Sizes); err != nil {
		return nil, err
	}
	if copied.FilesystemLayouts, err = deepCopy(e.FilesystemLayouts); err != nil {
		return nil, err
	}
	if copied.Networks, err = deepCopy(e.Networks); err != nil {
		return nil, err
	}
	if copied.Ips, err = deepCopy(e.Ips); err != nil {
		return nil, err
	}
	if copied.Images, err = deepCopy(e.Images); err != nil {
		return nil, err
	}
	if copied.Switches, err = deepCopy(e.Switches); err != nil {
		return nil, err
	}
	if copied.SwitchStatuses, err = deepCopy(e.SwitchStatuses); err != nil {
		return nil, err
	}
	if copied.Machines, err = deepCopy(e.Machines); err != nil {
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

func getCurrentEntities(ctx context.Context, store *testStore) (*Entities, error) {
	e := &Entities{}

	tenants, err := store.Tenant().List(ctx, &apiv2.TenantServiceListRequest{})
	if err != nil {
		return nil, err
	}
	e.Tenants = map[string]*apiv2.Tenant{}
	for _, t := range tenants {
		e.Tenants[t.Login] = t
	}
	projects, err := store.UnscopedProject().List(ctx, &apiv2.ProjectServiceListRequest{})
	if err != nil {
		return nil, err
	}
	e.Projects = map[string][]*apiv2.Project{}
	for _, p := range projects {
		e.Projects[p.Tenant] = append(e.Projects[p.Tenant], p)
		slices.SortStableFunc(e.Projects[p.Tenant], func(p1, p2 *apiv2.Project) int {
			return strings.Compare(p1.Uuid, p2.Uuid)
		})
	}
	partitions, err := store.Partition().List(ctx, &apiv2.PartitionQuery{})
	if err != nil {
		return nil, err
	}
	e.Partitions = map[string]*apiv2.Partition{}
	for _, p := range partitions {
		e.Partitions[p.Id] = p
	}
	sizes, err := store.Size().List(ctx, &apiv2.SizeQuery{})
	if err != nil {
		return nil, err
	}
	e.Sizes = map[string]*apiv2.Size{}
	for _, s := range sizes {
		e.Sizes[s.Id] = s
	}
	fsls, err := store.FilesystemLayout().List(ctx, &apiv2.FilesystemServiceListRequest{})
	if err != nil {
		return nil, err
	}
	e.FilesystemLayouts = map[string]*apiv2.FilesystemLayout{}
	for _, fsl := range fsls {
		e.FilesystemLayouts[fsl.Id] = fsl
	}
	networks, err := store.UnscopedNetwork().List(ctx, &apiv2.NetworkQuery{})
	if err != nil {
		return nil, err
	}
	e.Networks = map[string]*apiv2.Network{}
	for _, n := range networks {
		e.Networks[n.Id] = n
	}
	ips, err := store.UnscopedIP().List(ctx, &apiv2.IPQuery{})
	if err != nil {
		return nil, err
	}
	e.Ips = map[string]*apiv2.IP{}
	for _, ip := range ips {
		e.Ips[ip.Ip] = ip
	}
	images, err := store.Image().List(ctx, &apiv2.ImageQuery{})
	if err != nil {
		return nil, err
	}
	e.Images = map[string]*apiv2.Image{}
	for _, i := range images {
		e.Images[i.Id] = i
	}
	switches, err := store.Switch().List(ctx, &apiv2.SwitchQuery{})
	if err != nil {
		return nil, err
	}
	e.Switches = map[string]*apiv2.Switch{}
	e.SwitchStatuses = map[string]*metal.SwitchStatus{}
	for _, sw := range switches {
		e.Switches[sw.Id] = sw

		status, err := store.GetSwitchStatus(sw.Id)
		if err != nil {
			return nil, err
		}
		e.SwitchStatuses[sw.Id] = status
	}
	machines, err := store.UnscopedMachine().List(ctx, &apiv2.MachineQuery{})
	if err != nil {
		return nil, err
	}
	e.Machines = map[string]*apiv2.Machine{}
	for _, m := range machines {
		e.Machines[m.Uuid] = m
	}
	return e, nil
}
