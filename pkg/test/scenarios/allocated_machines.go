package scenarios

import (
	"testing"

	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/stretchr/testify/require"
)

var (
	DatacenterWithAllocations = DatacenterSpec{
		Partitions:        []string{Partition1},
		Tenants:           []string{Tenant1, Tenant2},
		ProjectsPerTenant: 2,
		Images: map[string]apiv2.ImageFeature{
			ImageDebian13:    apiv2.ImageFeature_IMAGE_FEATURE_MACHINE,
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
				Id:                       new(NetworkTenantSuperPartition1),
				Partition:                new(Partition1),
				Prefixes:                 []string{"12.110.0.0/16"},
				DestinationPrefixes:      []string{"1.2.3.0/24"},
				DefaultChildPrefixLength: &apiv2.ChildPrefixLength{Ipv4: new(uint32(22))},
				Type:                     apiv2.NetworkType_NETWORK_TYPE_SUPER,
			},
			{
				Partition: new(Partition1),
				Project:   new(Tenant1Project1),
				Type:      apiv2.NetworkType_NETWORK_TYPE_CHILD,
				Name:      new(NetworkNameTenantPartition1),
			},
		},

		IPs: []*apiv2.IPServiceCreateRequest{
			{
				Ip:      new("1.2.3.1"),
				Network: NetworkInternet,
				Project: Tenant1Project1,
				Name:    new("static internet"),
				Type:    apiv2.IPType_IP_TYPE_STATIC.Enum(),
			},
			{
				Ip:      new("1.2.3.2"),
				Network: NetworkInternet,
				Project: Tenant1Project1,
				Name:    new("ephemeral internet"),
				Type:    apiv2.IPType_IP_TYPE_EPHEMERAL.Enum(),
			},
		},

		Switches: []*apiv2.Switch{
			SwitchFunc(P01Rack01Switch1, Partition1, P01Rack01, []string{"Ethernet0"}, SwitchOSSonic2021, apiv2.SwitchReplaceMode_SWITCH_REPLACE_MODE_OPERATIONAL),
			SwitchFunc(P01Rack01Switch2, Partition1, P01Rack01, []string{"Ethernet0"}, SwitchOSSonic2021, apiv2.SwitchReplaceMode_SWITCH_REPLACE_MODE_OPERATIONAL),
		},

		IpFns: func(t testing.TB, nws map[string]*apiv2.Network) []*apiv2.IPServiceCreateRequest {
			var tenantNetwork *apiv2.Network

			for _, nw := range nws {
				if nw.Name != nil && *nw.Name == NetworkNameTenantPartition1 {
					tenantNetwork = nw
					break
				}
			}

			require.NotNil(t, tenantNetwork, "tenant network was not created")

			return []*apiv2.IPServiceCreateRequest{
				{
					Ip:      new("12.110.0.1"),
					Network: tenantNetwork.Id,
					Project: Tenant1Project1,
					Name:    new("ephemeral node"),
					Type:    apiv2.IPType_IP_TYPE_EPHEMERAL.Enum(),
					// FIXME: why can a user set an arbitrary machine association? this is dangerous?
					Machine: new(Machine1),
				},
			}
		},

		Machines: []*MachineWithLiveliness{
			MachineFunc(Machine2, Partition1, SizeC1Large, "Tenant1Project1", "", metal.MachineLivelinessAlive, true),
		},

		MachineFns: func(t testing.TB, nws map[string]*apiv2.Network) []*MachineWithLiveliness {
			var tenantNetwork *apiv2.Network

			for _, nw := range nws {
				if nw.Name != nil && *nw.Name == NetworkNameTenantPartition1 {
					tenantNetwork = nw
					break
				}
			}

			require.NotNil(t, tenantNetwork, "tenant network was not created")

			return []*MachineWithLiveliness{
				AllocatedMachineFunc(Machine1, Partition1, SizeC1Large, Tenant1Project1, ImageDebian13, metal.MachineLivelinessAlive, []*metal.MachineNetwork{
					{
						NetworkID:           NetworkInternet,
						Prefixes:            []string{},
						IPs:                 []string{"1.2.3.1", "1.2.3.2"},
						DestinationPrefixes: []string{},
						Vrf:                 104009,
						PrivatePrimary:      false,
						Private:             false,
						ASN:                 4210000001,
						Nat:                 true,
						Underlay:            false,
						Shared:              false,
						ProjectID:           "",
						NetworkType:         metal.NetworkTypeExternal,
						NATType:             metal.NATTypeIPv4Masquerade,
					},
					{
						NetworkID:           tenantNetwork.Id,
						Prefixes:            []string{},
						IPs:                 []string{"12.110.0.1"},
						DestinationPrefixes: []string{},
						Vrf:                 uint(*tenantNetwork.Vrf),
						PrivatePrimary:      true,
						Private:             true,
						ASN:                 4210000001,
						Nat:                 false,
						Underlay:            false,
						Shared:              false,
						ProjectID:           *tenantNetwork.Project,
						NetworkType:         metal.NetworkTypeChild,
						NATType:             metal.NATTypeNone,
					},
				}),
			}
		},
	}
)
