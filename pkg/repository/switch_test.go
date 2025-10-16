package repository

import (
	"fmt"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	"github.com/metal-stack/metal-lib/pkg/pointer"
	"github.com/metal-stack/metal-lib/pkg/testcommon"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func Test_updateNics(t *testing.T) {
	tests := []struct {
		name string
		old  metal.Nics
		new  metal.Nics
		want metal.Nics
	}{
		{
			name: "new nics just get added",
			old: metal.Nics{
				{
					Identifier: "Eth1/1",
					Name:       "Ethernet0",
				},
			},
			new: metal.Nics{
				{
					Identifier: "Eth1/2",
					Name:       "Ethernet1",
				},
			},
			want: metal.Nics{
				{
					Identifier: "Eth1/2",
					Name:       "Ethernet1",
				},
			},
		},
		{
			name: "existing nics can only be renamed",
			old: metal.Nics{
				{
					Identifier: "Eth1/1",
					Name:       "Ethernet0",
					Vrf:        "Vrf100",
				},
			},
			new: metal.Nics{
				{
					Identifier: "Eth1/1",
					Name:       "Ethernet2",
				},
				{
					Identifier: "Eth1/2",
					Name:       "Ethernet1",
				},
			},
			want: metal.Nics{
				{
					Identifier: "Eth1/1",
					Name:       "Ethernet2",
					Vrf:        "Vrf100",
				},
				{
					Identifier: "Eth1/2",
					Name:       "Ethernet1",
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := updateNics(tt.old, tt.new)
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("updateNics() diff = %s", diff)
			}
		})
	}
}

func Test_makeBGPFilter(t *testing.T) {
	tests := []struct {
		name     string
		m        *metal.Machine
		vrf      string
		networks []*metal.Network
		ips      []*metal.IP
		want     *apiv2.BGPFilter
		wantErr  bool
	}{
		{
			name:     "no allocation",
			m:        &metal.Machine{},
			networks: []*metal.Network{},
			ips:      []*metal.IP{},
			want:     &apiv2.BGPFilter{},
			wantErr:  false,
		},
		{
			name: "firewall with no vrf",
			m: &metal.Machine{
				Allocation: &metal.MachineAllocation{
					Role: metal.RoleFirewall,
				},
			},
			want:    &apiv2.BGPFilter{},
			wantErr: false,
		},
		{
			name: "firewall in default vrf",
			m: &metal.Machine{
				Allocation: &metal.MachineAllocation{
					Role: metal.RoleFirewall,
					MachineNetworks: []*metal.MachineNetwork{
						{
							Underlay: true,
							IPs:      []string{"1.1.1.1", "2.2.2.2"},
						},
						{
							Vrf: 200,
						},
					},
				},
			},
			vrf:      "default",
			networks: []*metal.Network{},
			ips:      []*metal.IP{},
			want: &apiv2.BGPFilter{
				Cidrs: []string{"1.1.1.1/32", "2.2.2.2/32"},
				Vnis:  []string{"200"},
			},
			wantErr: false,
		},
		{
			name: "machine filter",
			m: &metal.Machine{
				Allocation: &metal.MachineAllocation{
					Role: metal.RoleMachine,
					MachineNetworks: []*metal.MachineNetwork{
						{
							Private:   true,
							Prefixes:  []string{"1.1.1.1/32", "2.2.2.0/24"},
							NetworkID: "private-1",
						},
					},
				},
			},
			networks: []*metal.Network{
				{
					Base:            metal.Base{ID: "private-1"},
					ParentNetworkID: "private",
				},
				{
					Base: metal.Base{ID: "private"},
				},
			},
			ips: []*metal.IP{},
			want: &apiv2.BGPFilter{
				Cidrs: []string{"1.1.1.1/32", "2.2.2.0/24"},
				Vnis:  []string{},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := makeBGPFilter(tt.m, nil, tt.vrf, tt.networks, tt.ips)
			if (err != nil) != tt.wantErr {
				t.Errorf("makeBGPFilter() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if diff := cmp.Diff(tt.want, got, protocmp.Transform()); diff != "" {
				t.Errorf("makeBGPFilter() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_makeBGPFilterFirewall(t *testing.T) {
	tests := []struct {
		name            string
		machineNetworks []*metal.MachineNetwork
		want            *apiv2.BGPFilter
		wantErr         bool
	}{
		{
			name:    "machine networks empty",
			want:    &apiv2.BGPFilter{},
			wantErr: false,
		},
		{
			name: "add cidrs and vnis",
			machineNetworks: []*metal.MachineNetwork{
				{
					Underlay: true,
					Vrf:      100,
					IPs:      []string{"1.1.1.1", "2.2.2.2"},
				},
				{
					Vrf: 200,
					IPs: []string{"3.3.3.3", "4.4.4.4"},
				},
				{
					Vrf: 0,
					IPs: []string{"5.5.5.5", "6.6.6.6"},
				},
			},
			want: &apiv2.BGPFilter{
				Cidrs: []string{"1.1.1.1/32", "2.2.2.2/32"},
				Vnis:  []string{"200"},
			},
			wantErr: false,
		},
		{
			name: "error",
			machineNetworks: []*metal.MachineNetwork{
				{
					Underlay: true,
					IPs:      []string{"1.1.1.1", "2.2.2"},
				},
			},
			want:    nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := makeBGPFilterFirewall(tt.machineNetworks)
			if (err != nil) != tt.wantErr {
				t.Errorf("makeBGPFilterFirewall() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if diff := cmp.Diff(tt.want, got, protocmp.Transform()); diff != "" {
				t.Errorf("makeBGPFilterFirewall() diff = %s", diff)
			}
		})
	}
}

func Test_makeBGPFilterMachine(t *testing.T) {
	tests := []struct {
		name            string
		m               *metal.Machine
		projectMachines []*metal.Machine
		networks        metal.NetworkMap
		ips             metal.IPsMap
		want            *apiv2.BGPFilter
		wantErr         error
	}{
		{
			name:     "no allocation",
			m:        &metal.Machine{},
			networks: metal.NetworkMap{},
			ips:      metal.IPsMap{},
			want:     &apiv2.BGPFilter{},
			wantErr:  nil,
		},
		{
			name: "no private network",
			m: &metal.Machine{
				Allocation: &metal.MachineAllocation{
					MachineNetworks: []*metal.MachineNetwork{
						{
							Private:   true,
							NetworkID: "private-1",
						},
					},
				},
			},
			networks: metal.NetworkMap{
				"private": &metal.Network{},
			},
			ips:     metal.IPsMap{},
			want:    nil,
			wantErr: fmt.Errorf("no private network found for id:private-1"),
		},
		{
			name: "no parent network for private network",
			m: &metal.Machine{
				Allocation: &metal.MachineAllocation{
					MachineNetworks: []*metal.MachineNetwork{
						{
							Private:   true,
							NetworkID: "private-1",
						},
					},
				},
			},
			networks: metal.NetworkMap{
				"private-1": &metal.Network{
					ParentNetworkID: "private",
				},
			},
			ips:     metal.IPsMap{},
			want:    nil,
			wantErr: fmt.Errorf("parent network private not found for id:private-1"),
		},
		{
			name: "add cidrs, skip firewall ips",
			m: &metal.Machine{
				Allocation: &metal.MachineAllocation{
					MachineNetworks: []*metal.MachineNetwork{
						{
							Private:   true,
							Prefixes:  []string{"1.1.1.1/32", "1.1.2.0/24"},
							NetworkID: "private-1",
						},
						{
							Underlay: true,
							Prefixes: []string{"3.3.3.0/30"},
						},
					},
					Project: "project-a",
				},
			},
			projectMachines: []*metal.Machine{
				{
					Allocation: &metal.MachineAllocation{
						Role: metal.RoleFirewall,
						MachineNetworks: []*metal.MachineNetwork{
							{
								IPs: []string{"4.4.4.4"},
							},
						},
					},
				},
			},
			networks: metal.NetworkMap{
				"private-1": &metal.Network{
					ParentNetworkID: "private",
				},
				"private": &metal.Network{
					AdditionalAnnouncableCIDRs: []string{"2.2.2.0/30"},
				},
			},
			ips: metal.IPsMap{
				"project-a": metal.IPs{
					{
						IPAddress: "1.1.2.1",
					},
					{
						IPAddress: "2.2.2.2",
					},
					{
						IPAddress: "3.3.3.3",
					},
					{
						IPAddress: "4.4.4.4",
					},
				},
				"project-b": metal.IPs{
					{
						IPAddress: "6.6.6.6",
					},
				},
			},
			want: &apiv2.BGPFilter{
				Cidrs: []string{"1.1.1.1/32", "1.1.2.0/24", "2.2.2.0/30"},
				Vnis:  []string{},
			},
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := makeBGPFilterMachine(tt.m, tt.projectMachines, tt.networks, tt.ips)
			if diff := cmp.Diff(tt.wantErr, err, testcommon.ErrorStringComparer()); diff != "" {
				t.Errorf("makeBGPFilterMachine() error diff = %s", diff)
				return
			}
			if diff := cmp.Diff(tt.want, got, protocmp.Transform()); diff != "" {
				t.Errorf("makeBGPFilterMachine() diff = %s", diff)
			}
		})
	}
}

func Test_compactCidrs(t *testing.T) {
	tests := []struct {
		name    string
		cidrs   []string
		want    []string
		wantErr bool
	}{
		{
			name:    "error",
			cidrs:   []string{"1.1.1.1", "2.2.2.2/34"},
			want:    nil,
			wantErr: true,
		},
		{
			name:    "compact cidrs",
			cidrs:   []string{"1.1.1.1/32", "1.1.1.0/30", "2.2.2.0/24", "2.2.1.0/16"},
			want:    []string{"1.1.1.0/30", "2.2.0.0/16"},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := compactCidrs(tt.cidrs)
			if (err != nil) != tt.wantErr {
				t.Errorf("compactCidrs() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("compactCidrs() diff = %s", diff)
			}
		})
	}
}

func Test_ipWithMask(t *testing.T) {
	tests := []struct {
		name    string
		ip      string
		want    string
		wantErr bool
	}{
		{
			name:    "valid ipv4 address",
			ip:      "1.1.1.1",
			want:    "1.1.1.1/32",
			wantErr: false,
		},
		{
			name:    "valid ipv6 address",
			ip:      "::1",
			want:    "::1/128",
			wantErr: false,
		},
		{
			name:    "invalid address",
			ip:      "1.1.1",
			want:    "",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ipWithMask(tt.ip)
			if (err != nil) != tt.wantErr {
				t.Errorf("ipWithMask() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ipWithMask() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_convertMachineConnections(t *testing.T) {
	tests := []struct {
		name               string
		machineConnections metal.ConnectionMap
		nics               []*apiv2.SwitchNic
		want               []*apiv2.MachineConnection
		wantErr            error
	}{
		{
			name: "ignore malformed empty connections",
			machineConnections: metal.ConnectionMap{
				"machine01": metal.Connections{},
			},
			nics: []*apiv2.SwitchNic{
				{
					Identifier: "Eth1/1",
				},
			},
			want: nil,
		},
		{
			name: "convert connections",
			machineConnections: metal.ConnectionMap{
				"machine01": metal.Connections{
					{
						MachineID: "machine01",
						Nic: metal.Nic{
							Identifier: "Eth1/1",
						},
					},
				},
			},
			nics: []*apiv2.SwitchNic{
				{
					Identifier: "Eth1/1",
				},
			},
			want: []*apiv2.MachineConnection{
				{
					MachineId: "machine01",
					Nic: &apiv2.SwitchNic{
						Identifier: "Eth1/1",
					},
				},
			},
		},
		{
			name: "convert connections with a machine connected multiple times",
			machineConnections: metal.ConnectionMap{
				"machine01": metal.Connections{
					{
						MachineID: "machine01",
						Nic: metal.Nic{
							Identifier: "Eth1/1",
						},
					},
					{
						MachineID: "machine01",
						Nic: metal.Nic{
							Identifier: "Eth1/2",
						},
					},
				},
			},
			nics: []*apiv2.SwitchNic{
				{
					Identifier: "Eth1/1",
				},
				{
					Identifier: "Eth1/2",
				},
			},
			want: []*apiv2.MachineConnection{
				{
					MachineId: "machine01",
					Nic: &apiv2.SwitchNic{
						Identifier: "Eth1/1",
					},
				},
				{
					MachineId: "machine01",
					Nic: &apiv2.SwitchNic{
						Identifier: "Eth1/2",
					},
				},
			},
		},
		{
			name: "connected nics not found",
			machineConnections: metal.ConnectionMap{
				"machine01": metal.Connections{
					{
						MachineID: "machine01",
						Nic: metal.Nic{
							Identifier: "Eth1/1",
						},
					},
					{
						MachineID: "machine02",
						Nic: metal.Nic{
							Identifier: "Eth1/2",
						},
					},
					{
						MachineID: "machine03",
						Nic: metal.Nic{
							Identifier: "Eth1/3",
						},
					},
				},
			},
			nics: []*apiv2.SwitchNic{
				{
					Identifier: "Eth1/1",
				},
			},
			want:    nil,
			wantErr: errorutil.InvalidArgument("nics [Eth1/2 Eth1/3] could not be found but are connected to machines"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := convertMachineConnections(tt.machineConnections, tt.nics)
			if diff := cmp.Diff(tt.wantErr, err, testcommon.ErrorStringComparer()); diff != "" {
				t.Errorf("convertMachineConnections() error diff = %s", diff)
			}

			if diff := cmp.Diff(tt.want, got, protocmp.Transform()); diff != "" {
				t.Errorf("convertMachineConnections() diff = %s", diff)
			}
		})
	}
}

func Test_isFirewallIP(t *testing.T) {
	tests := []struct {
		name     string
		ip       string
		machines []*metal.Machine
		want     bool
	}{
		{
			name: "no firewalls",
			ip:   "2.2.2.2",
			machines: []*metal.Machine{
				{
					Allocation: &metal.MachineAllocation{
						MachineNetworks: []*metal.MachineNetwork{
							{
								IPs: []string{"2.2.2.2"},
							},
						},
						Role: metal.RoleMachine,
					},
				},
				{
					Allocation: nil,
				},
			},
			want: false,
		},
		{
			name: "firewall includes ip",
			ip:   "2.2.2.2",
			machines: []*metal.Machine{
				{
					Allocation: &metal.MachineAllocation{
						MachineNetworks: []*metal.MachineNetwork{
							{
								IPs: []string{"2.2.2.2"},
							},
						},
						Role: metal.RoleFirewall,
					},
				},
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isFirewallIP(tt.ip, tt.machines); got != tt.want {
				t.Errorf("checkIfFirewallIP() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestToMetalNics(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name       string
		switchNics []*apiv2.SwitchNic
		want       metal.Nics
		wantErr    bool
	}{
		{
			name:       "empty nics",
			switchNics: nil,
			want:       nil,
			wantErr:    false,
		},
		{
			name: "bgp state unknown",
			switchNics: []*apiv2.SwitchNic{
				{
					BgpPortState: &apiv2.SwitchBGPPortState{
						BgpState: apiv2.BGPState_BGP_STATE_UNSPECIFIED,
					},
				},
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "port desired state invalid",
			switchNics: []*apiv2.SwitchNic{
				{
					State: &apiv2.NicState{
						Desired: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UNSPECIFIED.Enum(),
					},
				},
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "port actual state invalid",
			switchNics: []*apiv2.SwitchNic{
				{
					State: &apiv2.NicState{
						Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UNSPECIFIED,
					},
				},
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "successfully convert",
			switchNics: []*apiv2.SwitchNic{
				{
					Name:       "Ethernet0",
					Identifier: "Eth1/1",
					Mac:        "11:11:11:11:11:11",
					State: &apiv2.NicState{
						Desired: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP.Enum(),
						Actual:  apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_DOWN,
					},
				},
				{
					Name:       "Ethernet1",
					Identifier: "Eth1/2",
					Mac:        "22:22:22:22:22:22",
					Vrf:        pointer.Pointer("Vrf100"),
					State: &apiv2.NicState{
						Desired: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP.Enum(),
						Actual:  apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
					},
					BgpPortState: &apiv2.SwitchBGPPortState{
						Neighbor:              "lan0",
						PeerGroup:             "external",
						VrfName:               "Vrf200",
						BgpState:              apiv2.BGPState_BGP_STATE_ESTABLISHED,
						BgpTimerUpEstablished: timestamppb.New(now.Add(time.Hour)),
						SentPrefixCounter:     200,
						AcceptedPrefixCounter: 1,
					},
				},
			},
			want: metal.Nics{
				{
					MacAddress: "11:11:11:11:11:11",
					Name:       "Ethernet0",
					Identifier: "Eth1/1",
					State: &metal.NicState{
						Desired: pointer.Pointer(metal.SwitchPortStatusUp),
						Actual:  metal.SwitchPortStatusDown,
					},
				},
				{
					MacAddress: "22:22:22:22:22:22",
					Name:       "Ethernet1",
					Identifier: "Eth1/2",
					Vrf:        "Vrf100",
					State: &metal.NicState{
						Desired: pointer.Pointer(metal.SwitchPortStatusUp),
						Actual:  metal.SwitchPortStatusUp,
					},
					BGPPortState: &metal.SwitchBGPPortState{
						Neighbor:              "lan0",
						PeerGroup:             "external",
						VrfName:               "Vrf200",
						BgpState:              metal.BGPStateEstablished,
						BgpTimerUpEstablished: uint64(now.Add(time.Hour).Unix()),
						SentPrefixCounter:     200,
						AcceptedPrefixCounter: 1,
					},
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := toMetalNics(tt.switchNics)
			if (err != nil) != tt.wantErr {
				t.Errorf("ToMetalNics() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("ToMetalNics() diff = %s", diff)
			}
		})
	}
}

func TestToMachineConnections(t *testing.T) {
	tests := []struct {
		name        string
		connections []*apiv2.MachineConnection
		want        metal.ConnectionMap
		wantErr     bool
	}{
		{
			name: "connections without multiple occurrences of the same machine",
			connections: []*apiv2.MachineConnection{
				{
					MachineId: "machine-a",
					Nic: &apiv2.SwitchNic{
						Identifier: "Eth1/1",
					},
				},
				{
					MachineId: "machine-b",
					Nic: &apiv2.SwitchNic{
						Identifier: "Eth1/2",
					},
				},
			},
			want: metal.ConnectionMap{
				"machine-a": {
					{
						Nic: metal.Nic{
							Identifier: "Eth1/1",
						},
						MachineID: "machine-a",
					},
				},
				"machine-b": {
					{
						Nic: metal.Nic{
							Identifier: "Eth1/2",
						},
						MachineID: "machine-b",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "connections with multiple occurrences of the same machine",
			connections: []*apiv2.MachineConnection{
				{
					MachineId: "machine-a",
					Nic: &apiv2.SwitchNic{
						Identifier: "Eth1/1",
					},
				},
				{
					MachineId: "machine-b",
					Nic: &apiv2.SwitchNic{
						Identifier: "Eth1/2",
					},
				},
				{
					MachineId: "machine-b",
					Nic: &apiv2.SwitchNic{
						Identifier: "Eth1/3",
					},
				},
			},
			want: metal.ConnectionMap{
				"machine-a": {
					{
						Nic: metal.Nic{
							Identifier: "Eth1/1",
						},
						MachineID: "machine-a",
					},
				},
				"machine-b": {
					{
						Nic: metal.Nic{
							Identifier: "Eth1/2",
						},
						MachineID: "machine-b",
					},
					{
						Nic: metal.Nic{
							Identifier: "Eth1/3",
						},
						MachineID: "machine-b",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "cannot connect multiple machines to the same nic",
			connections: []*apiv2.MachineConnection{
				{
					MachineId: "machine-a",
					Nic: &apiv2.SwitchNic{
						Identifier: "Eth1/1",
					},
				},
				{
					MachineId: "machine-b",
					Nic: &apiv2.SwitchNic{
						Identifier: "Eth1/1",
					},
				},
			},
			want:    nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := toMachineConnections(tt.connections)
			if (err != nil) != tt.wantErr {
				t.Errorf("ToMachineConnections() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("ToMachineConnections() diff = %s", diff)
			}
		})
	}
}

func TestGetNewNicState(t *testing.T) {
	tests := []struct {
		name        string
		current     *apiv2.NicState
		status      apiv2.SwitchPortStatus
		want        *apiv2.NicState
		wantChanged bool
	}{
		{
			name:    "current is nil",
			current: nil,
			status:  apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
			want: &apiv2.NicState{
				Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
			},
			wantChanged: true,
		},
		{
			name: "state unchanged and matches desired",
			current: &apiv2.NicState{
				Desired: pointer.Pointer(apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP),
				Actual:  apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
			},
			status: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
			want: &apiv2.NicState{
				Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
			},
			wantChanged: true,
		},
		{
			name: "state unchanged and does not match desired",
			current: &apiv2.NicState{
				Desired: pointer.Pointer(apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_DOWN),
				Actual:  apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
			},
			status: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
			want: &apiv2.NicState{
				Desired: pointer.Pointer(apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_DOWN),
				Actual:  apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
			},
			wantChanged: false,
		},
		{
			name: "state changed and desired empty",
			current: &apiv2.NicState{
				Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
			},
			status: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UNKNOWN,
			want: &apiv2.NicState{
				Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UNKNOWN,
			},
			wantChanged: true,
		},
		{
			name: "state changed and does not match desired",
			current: &apiv2.NicState{
				Desired: pointer.Pointer(apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP),
				Actual:  apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_DOWN,
			},
			status: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UNKNOWN,
			want: &apiv2.NicState{
				Desired: pointer.Pointer(apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP),
				Actual:  apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UNKNOWN,
			},
			wantChanged: true,
		},
		{
			name: "state changed and matches desired",
			current: &apiv2.NicState{
				Desired: pointer.Pointer(apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP),
				Actual:  apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_DOWN,
			},
			status: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
			want: &apiv2.NicState{
				Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
			},
			wantChanged: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, gotChanged := GetNewNicState(tt.current, tt.status)
			if diff := cmp.Diff(tt.want, got, protocmp.Transform()); diff != "" {
				t.Errorf("GetNewNicState() diff = %v", diff)
			}
			if gotChanged != tt.wantChanged {
				t.Errorf("GetNewNicState() changed = %v, want %v", gotChanged, tt.wantChanged)
			}
		})
	}
}
