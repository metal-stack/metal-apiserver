package scenarios

import (
	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
)

const (
	Machine1 = "00000000-0000-0000-0000-000000000001"
	Machine2 = "00000000-0000-0000-0000-000000000002"
	Machine3 = "00000000-0000-0000-0000-000000000003"
	Machine4 = "00000000-0000-0000-0000-000000000004"
	Machine5 = "00000000-0000-0000-0000-000000000005"

	Switch1 = "p01-r01leaf01"
	Switch2 = "p01-r01leaf02"

	Partition1 = "partition-1"
	Partition2 = "partition-2"

	SizeN1Medium = "n1-medium-x86"
	SizeC1Large  = "c1-large-x86"

	Tenant1 = "john.doe"
	// Project UUIDs are generated be counting the first digit for every tenant, last digit for every project of this tenant
	Tenant1Project1 = "10000000-0000-0000-0000-000000000001"
	Tenant1Project2 = "10000000-0000-0000-0000-000000000002"

	NetworkInternet              = "internet"
	NetworkUnderlayPartition1    = "underlay-partition-1"
	NetworkTenantSuperNamespaced = "tenant-super-namespaced"
	NetworkTenantSuperPartition1 = "tenant-super-partition-1"

	ImageDebian13    = "debian-13.0.20260131"
	ImageDebian12    = "debian-12.0.20251220"
	ImageDebian11    = "debian-11.0.20241220"
	ImageFirewall3_0 = "firewall-ubuntu-3.0.20260201"
)

var (
	DefaultDatacenter = DatacenterSpec{
		Partitions:        []string{Partition1},
		Tenants:           []string{Tenant1},
		ProjectsPerTenant: 1,
		Images: map[string]apiv2.ImageFeature{
			ImageDebian13:    apiv2.ImageFeature_IMAGE_FEATURE_MACHINE,
			ImageDebian12:    apiv2.ImageFeature_IMAGE_FEATURE_MACHINE,
			ImageDebian11:    apiv2.ImageFeature_IMAGE_FEATURE_MACHINE,
			ImageFirewall3_0: apiv2.ImageFeature_IMAGE_FEATURE_FIREWALL,
		},
		FilesystemLayouts: []*adminv2.FilesystemServiceCreateRequest{
			{
				FilesystemLayout: &apiv2.FilesystemLayout{
					Id: "debian",
					Constraints: &apiv2.FilesystemLayoutConstraints{
						Sizes: []string{SizeC1Large, SizeN1Medium},
						Images: map[string]string{
							"debian": ">= 12.0",
						},
					},
					Disks: []*apiv2.Disk{
						{
							Device: "/dev/sda",
							Partitions: []*apiv2.DiskPartition{
								{
									Number: 0,
									Size:   1024,
								},
							},
						},
					},
				},
			},
			{
				FilesystemLayout: &apiv2.FilesystemLayout{
					Id: "firewall",
					Constraints: &apiv2.FilesystemLayoutConstraints{
						Sizes: []string{SizeN1Medium},
						Images: map[string]string{
							"firewall-ubuntu": ">= 3.0",
						},
					},
				},
			},
		},
		Sizes: []*apiv2.Size{
			{
				Id:   SizeN1Medium,
				Name: new(SizeN1Medium),
				Constraints: []*apiv2.SizeConstraint{
					{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_CORES, Min: 2, Max: 2},
					{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_MEMORY, Min: 1024, Max: 1024},
					{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_STORAGE, Min: 1024, Max: 1024},
				},
			},
			{
				Id:   SizeC1Large,
				Name: new(SizeC1Large),
				Constraints: []*apiv2.SizeConstraint{
					{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_CORES, Min: 4, Max: 4},
					{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_MEMORY, Min: 1024, Max: 1024},
					{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_STORAGE, Min: 1024, Max: 1024},
				},
			},
		},
		SizeReservations: []*adminv2.SizeReservationServiceCreateRequest{
			{
				SizeReservation: &apiv2.SizeReservation{
					Name:        "sz-n1",
					Description: "N1 Reservation for project-1 in partition-4",
					Project:     Tenant1Project1,
					Size:        SizeN1Medium,
					Partitions:  []string{Partition1},
					Amount:      1,
				},
			},
		},

		Networks: []*adminv2.NetworkServiceCreateRequest{
			{
				Id:       new(NetworkInternet),
				Prefixes: []string{"1.2.3.0/24"},
				Type:     apiv2.NetworkType_NETWORK_TYPE_EXTERNAL,
				NatType:  apiv2.NATType_NAT_TYPE_IPV4_MASQUERADE.Enum(),
				Vrf:      new(uint32(11)),
			},
			{
				Id:        new(NetworkUnderlayPartition1),
				Prefixes:  []string{"10.253.0.0/16"},
				Type:      apiv2.NetworkType_NETWORK_TYPE_UNDERLAY,
				Partition: new(Partition1),
			},
			{
				Id:                       new(NetworkTenantSuperNamespaced),
				Prefixes:                 []string{"12.100.0.0/16"},
				DestinationPrefixes:      []string{"1.2.3.0/24"},
				DefaultChildPrefixLength: &apiv2.ChildPrefixLength{Ipv4: new(uint32(22))},
				Type:                     apiv2.NetworkType_NETWORK_TYPE_SUPER_NAMESPACED,
			},
			{
				Id:                       new(NetworkTenantSuperPartition1),
				Partition:                new(Partition1),
				Prefixes:                 []string{"12.110.0.0/16"},
				DestinationPrefixes:      []string{"1.2.3.0/24"},
				DefaultChildPrefixLength: &apiv2.ChildPrefixLength{Ipv4: new(uint32(22))},
				Type:                     apiv2.NetworkType_NETWORK_TYPE_SUPER,
			},
		},
		IPs: []*apiv2.IPServiceCreateRequest{
			{
				Network: "internet",
				Project: Tenant1Project1,
			},
		},
		Switches: SwitchPairFunc([2]string{Switch1, Switch2}, Partition1, "rack-1", 2),
		Machines: []*MachineWithLiveliness{
			MachineFunc(Machine1, Partition1, SizeC1Large, Tenant1Project1, ImageDebian13, metal.MachineLivelinessAlive),
		},
	}
)
