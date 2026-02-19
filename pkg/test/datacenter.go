package test

import (
	"fmt"
	"log/slog"
	"maps"
	"net/http"
	"net/http/httptest"
	"testing"

	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-apiserver/pkg/repository"
	"github.com/metal-stack/metal-apiserver/pkg/test/scenarios"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"
)

type (
	Datacenter struct {
		Tenants    []string
		Projects   map[string]string
		Partitions map[string]*apiv2.Partition
		Sizes      map[string]*apiv2.Size
		Networks   map[string]*apiv2.Network
		IPs        map[string]*apiv2.IP
		Images     map[string]*apiv2.Image
		Switches   map[string]*apiv2.Switch
		Machines   map[string]*metal.Machine

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
		Projects:   make(map[string]string),
		Partitions: make(map[string]*apiv2.Partition),
		Sizes:      make(map[string]*apiv2.Size),
		Networks:   make(map[string]*apiv2.Network),
		IPs:        make(map[string]*apiv2.IP),
		Images:     make(map[string]*apiv2.Image),
		Switches:   make(map[string]*apiv2.Switch),
		Machines:   make(map[string]*metal.Machine),
	}

	dc.closers = append(dc.closers, closer)
	return dc
}

func (dc *Datacenter) Create(spec *scenarios.DatacenterSpec) {
	dc.createPartitions(spec)
	dc.createTenantsAndMembers(spec)
	dc.createImages(spec)
	dc.createSizes(spec)
	dc.createSizeReservations(spec)
	dc.createNetworks(spec)
	dc.createIPs(spec)
	dc.createSwitches(spec)
	dc.createMachines(spec)
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
	dc.Partitions = CreatePartitions(dc.t, dc.TestStore, req)
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

	dc.Tenants = CreateTenants(dc.t, dc.TestStore, tenantCreateReq)
	dc.Projects = CreateProjects(dc.t, dc.TestStore, projectCreateReq)

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
	dc.Images = CreateImages(dc.t, dc.TestStore, req)
}

func (dc *Datacenter) createSizes(spec *scenarios.DatacenterSpec) {
	var req []*adminv2.SizeServiceCreateRequest
	for _, size := range spec.Sizes {
		req = append(req, &adminv2.SizeServiceCreateRequest{
			Size: size,
		})
	}

	dc.Sizes = CreateSizes(dc.t, dc.TestStore, req)
}

func (dc *Datacenter) createSizeReservations(spec *scenarios.DatacenterSpec) {
	CreateSizeReservations(dc.t, dc.TestStore, spec.SizeReservations)
}

func (dc *Datacenter) createNetworks(spec *scenarios.DatacenterSpec) {
	networks := CreateNetworks(dc.t, dc.TestStore, spec.Networks)
	maps.Copy(dc.Networks, networks)
}

func (dc *Datacenter) createIPs(spec *scenarios.DatacenterSpec) {
	ips := CreateIPs(dc.t, dc.TestStore, spec.IPs)
	maps.Copy(dc.IPs, ips)
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

		dc.Machines[m.ID] = m
	}
}

func (dc *Datacenter) createSwitches(spec *scenarios.DatacenterSpec) {
	for _, sw := range spec.Switches {
		switches := CreateSwitches(dc.t, dc.TestStore, []*repository.SwitchServiceCreateRequest{{Switch: sw}})
		for _, s := range switches {
			dc.Switches[s.Id] = s
		}
	}
}
