package scenarios

import (
	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
)

const (
	m1 = "00000000-0000-0000-0000-000000000001"
)

var (
	DefaultDatacenter = &DatacenterSpec{
		Tenants:           []string{"john.doe"},
		ProjectsPerTenant: 1,
		Images: map[string]apiv2.ImageFeature{
			"debian-13.0.20260131":         apiv2.ImageFeature_IMAGE_FEATURE_MACHINE,
			"debian-12.0.20251220":         apiv2.ImageFeature_IMAGE_FEATURE_MACHINE,
			"firewall-ubuntu-3.0.20260201": apiv2.ImageFeature_IMAGE_FEATURE_FIREWALL,
		},
		Sizes: []*apiv2.Size{
			{
				Id: "n1-medium-x86", Name: new("n1-medium-x86"),
				Constraints: []*apiv2.SizeConstraint{
					{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_CORES, Min: 2, Max: 2},
					{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_MEMORY, Min: 1024, Max: 1024},
					{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_STORAGE, Min: 1024, Max: 1024},
				},
			},
			{
				Id: "c1-large-x86",
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
					Project:     "john.doe-project-0",
					Size:        "n1-medium-x86",
					Partitions:  []string{"partition-1"},
					Amount:      2,
				},
			},
		},
		Partitions: map[string]Partition{
			"partition-1": {
				Racks: map[string]Rack{
					"rack-1": {
						Machines: []*metal.Machine{
							{Base: metal.Base{ID: m1}, PartitionID: "partition-1", SizeID: "c1-large-x86"},
						},
						Switches: switchPairFunc("partition-1", "rack-1", 2),
					},
				},
			},
		},
		Networks: []*adminv2.NetworkServiceCreateRequest{
			{
				Id: new("internet"), Prefixes: []string{"1.2.3.0/24"}, Type: apiv2.NetworkType_NETWORK_TYPE_EXTERNAL, Vrf: new(uint32(11)),
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
				Project: "john.doe-project-0",
			},
		},
	}
)
