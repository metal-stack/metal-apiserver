package test

import (
	"fmt"
	"log/slog"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/repository"
	"github.com/metal-stack/metal-lib/pkg/pointer"
	"github.com/stretchr/testify/require"
)

type (
	DatacenterConfig struct {
		Partitions        *uint
		Racks             *uint
		SwitchesPerRack   *uint
		MachinesPerRack   *uint
		Tenants           *uint
		ProjectsPerTenant *uint
	}

	Datacenter struct {
		Tenants    []string
		Projects   map[string]string
		Partitions map[string]*Partition
		Sizes      map[string]*apiv2.Size
		Networks   map[string]*apiv2.Network
		IPs        map[string]*apiv2.IP
		Images     map[string]*apiv2.Image

		testStore *testStore
		t         testing.TB
		closers   []func()
	}

	Partition struct {
		Racks map[string]*Rack
	}

	Rack struct {
		Switches map[string]*apiv2.Switch
		Machines map[string]*apiv2.Machine
	}
)

func NewDatacenter(t testing.TB, config *DatacenterConfig) *Datacenter {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	testStore, closer := StartRepositoryWithCleanup(t, log, WithPostgres(true))

	dc := &Datacenter{
		testStore:  testStore,
		t:          t,
		Projects:   make(map[string]string),
		Partitions: make(map[string]*Partition),
		Sizes:      make(map[string]*apiv2.Size),
		Networks:   make(map[string]*apiv2.Network),
		IPs:        make(map[string]*apiv2.IP),
		Images:     make(map[string]*apiv2.Image),
	}

	dc.closers = append(dc.closers, closer)

	dc.createTenantsProjectsAndMembers()
	dc.createPartitions(config.Partitions)
	dc.createImagesAndSizes()
	dc.createIPsAndNetworks()
	dc.createMachines()
	dc.createSwitches()

	return dc
}

func (dc *Datacenter) Close() {
	for _, close := range dc.closers {
		close()
	}
}

func (dc *Datacenter) createTenantsProjectsAndMembers() {
	dc.Tenants = CreateTenants(dc.t, dc.testStore, []*apiv2.TenantServiceCreateRequest{
		{Name: "john.doe@github.com"},
		{Name: "foo.bar@github.com"},
		{Name: "viewer@github.com"},
		{Name: "ansible"},
		{Name: "metal-image-cache-sync"},
		{Name: "metal-hammer"},
		{Name: "pixiecore"},
	})
	CreateTenantMemberships(dc.t, dc.testStore, "john.doe@github.com", []*repository.TenantMemberCreateRequest{
		{MemberID: "john.doe@github.com", Role: apiv2.TenantRole_TENANT_ROLE_OWNER},
		{MemberID: "viewer@github.com", Role: apiv2.TenantRole_TENANT_ROLE_VIEWER},
	})
	CreateTenantMemberships(dc.t, dc.testStore, "viewer@github.com", []*repository.TenantMemberCreateRequest{
		{MemberID: "viewer@github.com", Role: apiv2.TenantRole_TENANT_ROLE_OWNER},
	})
	CreateTenantMemberships(dc.t, dc.testStore, "foo.bar@github.com", []*repository.TenantMemberCreateRequest{
		{MemberID: "foo.bar@github.com", Role: apiv2.TenantRole_TENANT_ROLE_OWNER},
	})
	CreateTenantMemberships(dc.t, dc.testStore, "metal-image-cache-sync", []*repository.TenantMemberCreateRequest{
		{MemberID: "metal-image-cache-sync", Role: apiv2.TenantRole_TENANT_ROLE_OWNER},
	})
	CreateTenantMemberships(dc.t, dc.testStore, "metal-hammer", []*repository.TenantMemberCreateRequest{
		{MemberID: "metal-hammer", Role: apiv2.TenantRole_TENANT_ROLE_OWNER},
	})
	CreateTenantMemberships(dc.t, dc.testStore, "pixiecore", []*repository.TenantMemberCreateRequest{
		{MemberID: "pixiecore", Role: apiv2.TenantRole_TENANT_ROLE_OWNER},
	})
	CreateTenantMemberships(dc.t, dc.testStore, "ansible", []*repository.TenantMemberCreateRequest{
		{MemberID: "ansible", Role: apiv2.TenantRole_TENANT_ROLE_OWNER},
	})
	dc.Projects = CreateProjects(dc.t, dc.testStore, []*apiv2.ProjectServiceCreateRequest{
		{Login: "john.doe@github.com"},
	})
	CreateProjectMemberships(dc.t, dc.testStore, dc.Projects["john.doe@github.com"], []*repository.ProjectMemberCreateRequest{
		{TenantId: "foo.bar@github.com", Role: apiv2.ProjectRole_PROJECT_ROLE_VIEWER},
	})
}

func (dc *Datacenter) createPartitions(count *uint) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintln(w, "a image")
	}))
	dc.closers = append(dc.closers, ts.Close)

	validURL := ts.URL

	if count == nil {
		count = pointer.Pointer(uint(1))
	}

	var partitions []*adminv2.PartitionServiceCreateRequest
	for i := range *count {
		id := fmt.Sprintf("partition-%d", i)
		partitions = append(partitions, &adminv2.PartitionServiceCreateRequest{
			Partition: &apiv2.Partition{
				Id:          id,
				Description: id,
				BootConfiguration: &apiv2.PartitionBootConfiguration{
					ImageUrl:  validURL,
					KernelUrl: validURL,
				},
			},
		})
		dc.Partitions[id] = &Partition{}
	}

	CreatePartitions(dc.t, dc.testStore, partitions)
}

func (dc *Datacenter) createImagesAndSizes() {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintln(w, "a image")
	}))
	dc.closers = append(dc.closers, ts.Close)

	dc.Images = CreateImages(dc.t, dc.testStore, []*adminv2.ImageServiceCreateRequest{
		{
			Image: &apiv2.Image{Id: "debian-12.0.20241231", Url: ts.URL, Features: []apiv2.ImageFeature{apiv2.ImageFeature_IMAGE_FEATURE_MACHINE}},
		},
		{
			Image: &apiv2.Image{Id: "firewall-ubuntu-3.0.20241231", Url: ts.URL, Features: []apiv2.ImageFeature{apiv2.ImageFeature_IMAGE_FEATURE_FIREWALL}},
		},
		{
			Image: &apiv2.Image{Id: "debian-13.0.20241231", Url: ts.URL, Features: []apiv2.ImageFeature{apiv2.ImageFeature_IMAGE_FEATURE_MACHINE}},
		},
	})

	dc.Sizes = CreateSizes(dc.t, dc.testStore, []*adminv2.SizeServiceCreateRequest{
		{
			Size: &apiv2.Size{
				Id: "n1-medium-x86", Name: pointer.Pointer("n1-medium-x86"),
				Constraints: []*apiv2.SizeConstraint{
					{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_CORES, Min: 4, Max: 4},
					{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_MEMORY, Min: 1024 * 1024, Max: 1024 * 1024},
					{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_STORAGE, Min: 10 * 1024 * 1024, Max: 10 * 1024 * 1024},
				},
			},
		},
		{
			Size: &apiv2.Size{
				Id: "c1-large-x86", Name: pointer.Pointer("c1-large-x86"),
				Constraints: []*apiv2.SizeConstraint{
					{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_CORES, Min: 8, Max: 8},
					{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_MEMORY, Min: 1024 * 1024, Max: 1024 * 1024},
					{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_STORAGE, Min: 10 * 1024 * 1024, Max: 10 * 1024 * 1024},
				},
			},
		},
	})
}

func (dc *Datacenter) createIPsAndNetworks() {

	var startOctet = 10
	for partition := range dc.Partitions {

		networks := CreateNetworks(dc.t, dc.testStore, []*adminv2.NetworkServiceCreateRequest{
			{
				Id:                       pointer.Pointer("tenant-super-network-" + partition),
				Prefixes:                 []string{fmt.Sprintf("%d.100.0.0/14", startOctet)},
				DefaultChildPrefixLength: &apiv2.ChildPrefixLength{Ipv4: pointer.Pointer(uint32(22))},
				Type:                     apiv2.NetworkType_NETWORK_TYPE_SUPER,
				Partition:                &partition,
			},
			{
				Id:        pointer.Pointer("underlay-" + partition),
				Name:      pointer.Pointer("Underlay Network"),
				Partition: &partition,
				Prefixes:  []string{fmt.Sprintf("%d.0.0.0/24", startOctet)},
				Type:      apiv2.NetworkType_NETWORK_TYPE_UNDERLAY,
			},
		})
		maps.Copy(dc.Networks, networks)
		startOctet++
	}

	internet := CreateNetworks(dc.t, dc.testStore, []*adminv2.NetworkServiceCreateRequest{
		{

			Id:                  pointer.Pointer("internet"),
			Prefixes:            []string{"1.2.3.0/24"},
			DestinationPrefixes: []string{"0.0.0.0/0"},
			Vrf:                 pointer.Pointer(uint32(11)),
			Type:                apiv2.NetworkType_NETWORK_TYPE_EXTERNAL,
		},
	})
	maps.Copy(dc.Networks, internet)

	for partition := range dc.Partitions {
		for _, project := range dc.Projects {
			allocatedNetworks := AllocateNetworks(dc.t, dc.testStore, []*apiv2.NetworkServiceCreateRequest{
				{Name: pointer.Pointer(project + "-network-a-" + partition), Description: pointer.Pointer(project + "-network-a-" + partition), Project: project, Partition: &partition},
				{Name: pointer.Pointer(project + "-network-b-" + partition), Description: pointer.Pointer(project + "-network-b-" + partition), Project: project, Partition: &partition},
			})
			maps.Copy(dc.Networks, allocatedNetworks)
		}
	}
}

func (dc *Datacenter) createMachines() {

}

func (dc *Datacenter) createSwitches() {

}

type Asserters struct {
	Partition func(t testing.TB, partition *apiv2.Partition)
}

func (dc *Datacenter) Assert(asserters *Asserters) {
	require.NotNil(dc.t, asserters)

	if asserters.Partition != nil {

		for partition := range dc.Partitions {
			require.NotNil(dc.t, asserters.Partition)

			resp, err := dc.testStore.Partition().Get(dc.t.Context(), partition)
			require.NoError(dc.t, err)
			asserters.Partition(dc.t, resp)
		}

	} // else compare if partition did not change
}
