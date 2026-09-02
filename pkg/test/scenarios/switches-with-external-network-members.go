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
			{
				Id:        P01Rack01Switch1,
				Rack:      new(P01Rack01),
				Partition: Partition1,
				Nics: []*apiv2.SwitchNic{
					{
						Name:       "Ethernet0",
						Identifier: "Ethernet0",
						State: &apiv2.NicState{
							Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
						},
						Vrf: new("Vrf100"),
					},
					{
						Name:       "Ethernet1",
						Identifier: "Ethernet1",
						State: &apiv2.NicState{
							Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
						},
						Vrf: new("Vrf100"),
					},
				},
				Os: SwitchOSSonic2021,
				MachineConnections: []*apiv2.MachineConnection{
					{
						MachineId: Machine1,
						Nic:       &apiv2.SwitchNic{Name: "Ethernet0"},
					},
				},
			},
			{
				Id:        P01Rack01Switch2,
				Rack:      new(P01Rack01),
				Partition: Partition1,
				Nics: []*apiv2.SwitchNic{
					{
						Name:       "Ethernet0",
						Identifier: "Ethernet0",
						State: &apiv2.NicState{
							Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
						},
						Vrf: new("Vrf100"),
					},
					{
						Name:       "Ethernet1",
						Identifier: "Ethernet1",
						State: &apiv2.NicState{
							Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
						},
						Vrf: new("Vrf100"),
					},
					{
						Name:       "Ethernet120",
						Identifier: "Ethernet120",
						State: &apiv2.NicState{
							Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
						},
					},
				},
				Os: SwitchOSSonic2021,
				MachineConnections: []*apiv2.MachineConnection{
					{
						MachineId: Machine1,
						Nic:       &apiv2.SwitchNic{Name: "Ethernet0"},
					},
				},
			},
			{
				Id:        P01Rack02Switch1,
				Rack:      new(P01Rack02),
				Partition: Partition1,
				Nics: []*apiv2.SwitchNic{
					{
						Name:       "Ethernet0",
						Identifier: "Ethernet0",
						State: &apiv2.NicState{
							Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
						},
						Vrf: new("Vrf100"),
					},
					{
						Name:       "Ethernet1",
						Identifier: "Ethernet1",
						State: &apiv2.NicState{
							Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
						},
						Vrf: new("Vrf100"),
					},
					{
						Name:       "Ethernet2",
						Identifier: "Ethernet2",
						State: &apiv2.NicState{
							Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
						},
					},
					{
						Name:       "Ethernet3",
						Identifier: "Ethernet3",
						State: &apiv2.NicState{
							Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
						},
					},
				},
				Os: SwitchOSSonic2021,
			},
			{
				Id:        P01Rack02Switch2,
				Rack:      new(P01Rack02),
				Partition: Partition1,
				Nics: []*apiv2.SwitchNic{
					{
						Name:       "Ethernet0",
						Identifier: "Ethernet0",
						State: &apiv2.NicState{
							Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
						},
						Vrf: new("Vrf100"),
					},
					{
						Name:       "Ethernet1",
						Identifier: "Ethernet1",
						State: &apiv2.NicState{
							Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
						},
						Vrf: new("Vrf100"),
					},
					{
						Name:       "Ethernet2",
						Identifier: "Ethernet2",
						State: &apiv2.NicState{
							Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
						},
					},
					{
						Name:       "Ethernet3",
						Identifier: "Ethernet3",
						State: &apiv2.NicState{
							Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
						},
					},
				},
				Os: SwitchOSSonic2021,
			},
			{
				Id:        P02Rack01Switch1,
				Rack:      new(P02Rack01),
				Partition: Partition2,
				Nics: []*apiv2.SwitchNic{
					{
						Name:       "Ethernet0",
						Identifier: "Ethernet0",
						State: &apiv2.NicState{
							Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
						},
						Vrf: new("Vrf99"),
					},
					{
						Name:       "Ethernet1",
						Identifier: "Ethernet1",
						State: &apiv2.NicState{
							Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
						},
					},
				},
				Os: SwitchOSSonic2021,
			},
			{
				Id:        P02Rack01Switch2,
				Rack:      new(P02Rack01),
				Partition: Partition2,
				Nics: []*apiv2.SwitchNic{
					{
						Name:       "Ethernet0",
						Identifier: "Ethernet0",
						State: &apiv2.NicState{
							Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
						},
						Vrf: new("Vrf99"),
					},
					{
						Name:       "Ethernet1",
						Identifier: "Ethernet1",
						State: &apiv2.NicState{
							Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
						},
					},
				},
				Os: SwitchOSSonic2021,
			},
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
		},
		Machines: []*MachineWithLiveliness{
			MachineFunc(Machine1, Partition1, SizeC1Large, "", "", metal.MachineLivelinessAlive, false),
		},
	}
)
