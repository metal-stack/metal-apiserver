package scenarios

import (
	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
)

var (
	DefaultDatacenter = DatacenterSpec{
		Partitions:        []string{Partition1},
		Tenants:           []string{Tenant1},
		ProjectsPerTenant: 1,
		Images: map[string]apiv2.ImageFeature{
			ImageDebian13:    apiv2.ImageFeature_IMAGE_FEATURE_MACHINE,
			ImageDebian12:    apiv2.ImageFeature_IMAGE_FEATURE_MACHINE,
			ImageFirewall3_0: apiv2.ImageFeature_IMAGE_FEATURE_FIREWALL,
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
				Id:       new("internet"),
				Prefixes: []string{"1.2.3.0/24"},
				Type:     apiv2.NetworkType_NETWORK_TYPE_EXTERNAL,
				Vrf:      new(uint32(11)),
			},
			{
				Id:                       new("tenant-super-namespaced"),
				Prefixes:                 []string{"12.100.0.0/16"},
				DestinationPrefixes:      []string{"1.2.3.0/24"},
				DefaultChildPrefixLength: &apiv2.ChildPrefixLength{Ipv4: new(uint32(22))},
				Type:                     apiv2.NetworkType_NETWORK_TYPE_SUPER_NAMESPACED,
			},
		},
		IPs: []*apiv2.IPServiceCreateRequest{
			{
				Network: "internet",
				Project: Tenant1Project1,
			},
		},
		Switches: []*apiv2.Switch{
			SwitchFunc(P01Rack01Switch1, Partition1, P01Rack01, []string{"Ethernet0"}, SwitchOSSonic2021),
			SwitchFunc(P01Rack01Switch2, Partition1, P01Rack01, []string{"Ethernet0"}, SwitchOSSonic2021),
		},
		Machines: []*MachineWithLiveliness{
			MachineFunc(Machine1, Partition1, SizeC1Large, Tenant1Project1, ImageDebian13, metal.MachineLivelinessAlive),
		},
	}
)
