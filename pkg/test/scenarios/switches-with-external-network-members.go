package scenarios

import (
	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
)

var (
	SwitchesWithExternalNetworkMembers = DatacenterSpec{
		Partitions: []string{Partition1, Partition2},
		Switches: []*apiv2.Switch{
			SwitchFunc(P01Rack01Switch1, Partition1, P01Rack01, []string{"Ethernet0", "Ethernet1"}, SwitchOSSonic2021, apiv2.SwitchReplaceMode_SWITCH_REPLACE_MODE_OPERATIONAL),
			SwitchFunc(P01Rack01Switch2, Partition1, P01Rack01, []string{"Ethernet0", "Ethernet1", "Ethernet120"}, SwitchOSSonic2021, apiv2.SwitchReplaceMode_SWITCH_REPLACE_MODE_OPERATIONAL),
			SwitchFunc(P01Rack02Switch1, Partition1, P01Rack02, []string{"Ethernet0", "Ethernet1"}, SwitchOSSonic2021, apiv2.SwitchReplaceMode_SWITCH_REPLACE_MODE_OPERATIONAL),
			SwitchFunc(P01Rack02Switch2, Partition1, P01Rack02, []string{"Ethernet0", "Ethernet1"}, SwitchOSSonic2021, apiv2.SwitchReplaceMode_SWITCH_REPLACE_MODE_OPERATIONAL),
			SwitchFunc(P02Rack01Switch1, Partition2, P02Rack01, []string{"Ethernet0", "Ethernet1"}, SwitchOSSonic2021, apiv2.SwitchReplaceMode_SWITCH_REPLACE_MODE_OPERATIONAL),
			SwitchFunc(P02Rack01Switch2, Partition2, P02Rack01, []string{"Ethernet0", "Ethernet1"}, SwitchOSSonic2021, apiv2.SwitchReplaceMode_SWITCH_REPLACE_MODE_OPERATIONAL),
		},
		Networks: []*adminv2.NetworkServiceCreateRequest{
			{
				Id:       new(NetworkExternal),
				Vrf:      new(uint32(100)),
				Type:     apiv2.NetworkType_NETWORK_TYPE_EXTERNAL,
				Prefixes: []string{"10.1.0.0/16"},
			},
			{
				Id:        new(NetworkNameTenantPartition1),
				Vrf:       new(uint32(99)),
				Type:      apiv2.NetworkType_NETWORK_TYPE_EXTERNAL,
				Prefixes:  []string{"10.2.0.0/16"},
				Partition: new(Partition1),
			},
			{
				Id:        new(NetworkUnderlayPartition1),
				Type:      apiv2.NetworkType_NETWORK_TYPE_UNDERLAY,
				Prefixes:  []string{"10.3.0.0/16"},
				Partition: new(Partition1),
			},
			{
				Id:                       new(NetworkSuper),
				Type:                     apiv2.NetworkType_NETWORK_TYPE_SUPER,
				DefaultChildPrefixLength: &apiv2.ChildPrefixLength{Ipv4: new(uint32(22))},
				Prefixes:                 []string{"10.4.0.0/16"},
			},
			{
				Id:                       new(NetworkSuperNamespaced),
				Type:                     apiv2.NetworkType_NETWORK_TYPE_SUPER_NAMESPACED,
				DefaultChildPrefixLength: &apiv2.ChildPrefixLength{Ipv4: new(uint32(22))},
				Prefixes:                 []string{"10.5.0.0/16"},
			},
		},
		Machines: []*MachineWithLiveliness{
			MachineFunc(Machine1, Partition1, SizeC1Large, "", "", metal.MachineLivelinessAlive, false),
		},
	}
)
