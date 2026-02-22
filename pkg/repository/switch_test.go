package repository

import (
	"fmt"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	"github.com/metal-stack/metal-lib/pkg/testcommon"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/durationpb"
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
			got := updateNicNames(tt.old, tt.new)
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
			want: []*apiv2.MachineConnection{},
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
				"machine02": metal.Connections{
					{
						MachineID: "machine02",
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
					MachineId: "machine02",
					Nic: &apiv2.SwitchNic{
						Identifier: "Eth1/2",
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
							Name:       "Ethernet0",
							Identifier: "Eth1/1",
						},
					},
					{
						MachineID: "machine01",
						Nic: metal.Nic{
							Name:       "Ethernet1",
							Identifier: "Eth1/2",
						},
					},
					{
						MachineID: "machine01",
						Nic: metal.Nic{
							Name:       "Ethernet2",
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
			wantErr: errorutil.InvalidArgument("nics [Ethernet1 Ethernet2] could not be found but are connected to machines"),
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
		hostname   string
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
					Vrf:        new("Vrf100"),
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
						Desired: new(metal.SwitchPortStatusUp),
						Actual:  metal.SwitchPortStatusDown,
					},
				},
				{
					MacAddress: "22:22:22:22:22:22",
					Name:       "Ethernet1",
					Identifier: "Eth1/2",
					Vrf:        "Vrf100",
					State: &metal.NicState{
						Desired: new(metal.SwitchPortStatusUp),
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
			got, err := toMetalNics(tt.switchNics, tt.hostname)
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
		hostname    string
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
			got, err := toMachineConnections(tt.connections, tt.hostname)
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
				Desired: new(apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP),
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
				Desired: new(apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_DOWN),
				Actual:  apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
			},
			status: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
			want: &apiv2.NicState{
				Desired: new(apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_DOWN),
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
				Desired: new(apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP),
				Actual:  apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_DOWN,
			},
			status: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UNKNOWN,
			want: &apiv2.NicState{
				Desired: new(apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP),
				Actual:  apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UNKNOWN,
			},
			wantChanged: true,
		},
		{
			name: "state changed and matches desired",
			current: &apiv2.NicState{
				Desired: new(apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP),
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

func Test_toMetalSwitchSync(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name string
		sync *apiv2.SwitchSync
		want *metal.SwitchSync
	}{
		{
			name: "sync nil",
			sync: nil,
			want: nil,
		},
		{
			name: "time nil",
			sync: &apiv2.SwitchSync{
				Duration: durationpb.New(time.Second),
			},
			want: &metal.SwitchSync{
				Duration: time.Second,
			},
		},
		{
			name: "duration nil",
			sync: &apiv2.SwitchSync{
				Time: timestamppb.New(now),
			},
			want: &metal.SwitchSync{
				Time: now,
			},
		},
		{
			name: "error occurred",
			sync: &apiv2.SwitchSync{
				Time:     timestamppb.New(now),
				Duration: durationpb.New(2 * time.Second),
				Error:    new("fail"),
			},
			want: &metal.SwitchSync{
				Time:     now,
				Duration: 2 * time.Second,
				Error:    new("fail"),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toMetalSwitchSync(tt.sync)
			if diff := cmp.Diff(tt.want, got, protocmp.Transform()); diff != "" {
				t.Errorf("toMetalSwitchSync() diff = %v", diff)
			}
		})
	}
}

func Test_switchRepository_updateAllButNics(t *testing.T) {
	tests := []struct {
		name    string
		sw      *metal.Switch
		req     *adminv2.SwitchServiceUpdateRequest
		want    *metal.Switch
		wantErr bool
	}{
		{
			name: "update everything",
			sw: &metal.Switch{
				Base: metal.Base{
					ID: "sw1",
				},
				Rack:           "rack01",
				Partition:      "partition-a",
				ReplaceMode:    metal.SwitchReplaceModeOperational,
				ManagementIP:   "1.1.1.1",
				ManagementUser: "admin",
				ConsoleCommand: "tty",
				OS: &metal.SwitchOS{
					Vendor:           metal.SwitchOSVendorSonic,
					Version:          "ec202211",
					MetalCoreVersion: "v0.13.1",
				},
				Nics: metal.Nics{
					{
						MacAddress: "11:11:11:11:11:11",
						Name:       "Ethernet0",
						Identifier: "Eth1/1",
						Vrf:        "Vrf100",
						Hostname:   "sw1",
					},
				},
				MachineConnections: metal.ConnectionMap{
					"m1": metal.Connections{
						{
							Nic: metal.Nic{
								MacAddress: "11:11:11:11:11:11",
								Name:       "Ethernet0",
								Identifier: "Eth1/1",
								Vrf:        "Vrf100",
								Hostname:   "sw1",
							},
							MachineID: "m1",
						},
					},
				},
			},
			req: &adminv2.SwitchServiceUpdateRequest{
				Id:             "sw1",
				Description:    new("new description"),
				ReplaceMode:    apiv2.SwitchReplaceMode_SWITCH_REPLACE_MODE_REPLACE.Enum(),
				ManagementIp:   new("1.2.3.4"),
				ManagementUser: new("metal"),
				ConsoleCommand: new("ssh"),
				Nics: []*apiv2.SwitchNic{
					{
						Name:       "Ethernet2",
						Identifier: "Eth1/1",
						Mac:        "11:11:11:11:11:11",
						State: &apiv2.NicState{
							Desired: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP.Enum(),
							Actual:  apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_DOWN,
						},
						BgpFilter:    &apiv2.BGPFilter{},
						BgpPortState: &apiv2.SwitchBGPPortState{},
					},
				},
				Os: &apiv2.SwitchOS{
					Vendor:           apiv2.SwitchOSVendor_SWITCH_OS_VENDOR_CUMULUS,
					Version:          "ec202105",
					MetalCoreVersion: "v0.14.0",
				},
			},
			want: &metal.Switch{
				Base: metal.Base{
					ID:          "sw1",
					Description: "new description",
				},
				Rack:           "rack01",
				Partition:      "partition-a",
				ReplaceMode:    metal.SwitchReplaceModeReplace,
				ManagementIP:   "1.2.3.4",
				ManagementUser: "metal",
				ConsoleCommand: "ssh",
				OS: &metal.SwitchOS{
					Vendor:           metal.SwitchOSVendorCumulus,
					Version:          "ec202105",
					MetalCoreVersion: "v0.14.0",
				},
				Nics: metal.Nics{
					{
						MacAddress: "11:11:11:11:11:11",
						Name:       "Ethernet0",
						Identifier: "Eth1/1",
						Vrf:        "Vrf100",
						Hostname:   "sw1",
					},
				},
				MachineConnections: metal.ConnectionMap{
					"m1": metal.Connections{
						{
							Nic: metal.Nic{
								MacAddress: "11:11:11:11:11:11",
								Name:       "Ethernet0",
								Identifier: "Eth1/1",
								Vrf:        "Vrf100",
								Hostname:   "sw1",
							},
							MachineID: "m1",
						},
					},
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := updateAllButNics(tt.sw, tt.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("switchRepository.updateAllButNics() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("switchRepository.updateAllButNics() diff = %v", diff)
			}
		})
	}
}

func Test_adoptFromTwin(t *testing.T) {
	type args struct {
		old       *metal.Switch
		twin      *metal.Switch
		newSwitch *metal.Switch
	}
	tests := []struct {
		name    string
		args    args
		want    *metal.Switch
		wantErr bool
	}{
		{
			name: "adopt machine connections and nic configuration from twin",
			args: args{
				old: &metal.Switch{
					ReplaceMode: metal.SwitchReplaceModeReplace,
				},
				twin: &metal.Switch{
					OS: &metal.SwitchOS{Vendor: metal.SwitchOSVendorCumulus},
					Nics: metal.Nics{
						metal.Nic{
							Name:       "swp1s0",
							MacAddress: "aa:aa:aa:aa:aa:a1",
							Vrf:        "1",
						},
						metal.Nic{
							Name:       "swp1s1",
							MacAddress: "aa:aa:aa:aa:aa:a2",
						},
						metal.Nic{
							Name:       "swp1s2",
							MacAddress: "aa:aa:aa:aa:aa:a3",
						},
					},
					MachineConnections: metal.ConnectionMap{
						"m1": metal.Connections{
							metal.Connection{
								Nic: metal.Nic{
									Name:       "swp1s0",
									MacAddress: "aa:aa:aa:aa:aa:a1",
								},
							},
						},
						"fw1": metal.Connections{
							metal.Connection{
								Nic: metal.Nic{
									Name:       "swp1s1",
									MacAddress: "aa:aa:aa:aa:aa:a2",
								},
							},
						},
					},
				},
				newSwitch: &metal.Switch{
					OS: &metal.SwitchOS{Vendor: metal.SwitchOSVendorCumulus},
					Nics: metal.Nics{
						metal.Nic{
							Name:       "swp1s0",
							MacAddress: "bb:bb:bb:bb:bb:b1",
						},
						metal.Nic{
							Name:       "swp1s1",
							MacAddress: "bb:bb:bb:bb:bb:b2",
						},
						metal.Nic{
							Name:       "swp1s2",
							MacAddress: "bb:bb:bb:bb:bb:b3",
						},
						metal.Nic{
							Name:       "swp1s3",
							MacAddress: "bb:bb:bb:bb:bb:b4",
						},
					},
				},
			},
			want: &metal.Switch{
				ReplaceMode: metal.SwitchReplaceModeOperational,
				OS:          &metal.SwitchOS{Vendor: metal.SwitchOSVendorCumulus},
				Nics: metal.Nics{
					metal.Nic{
						Name:       "swp1s0",
						MacAddress: "bb:bb:bb:bb:bb:b1",
						Vrf:        "1",
					},
					metal.Nic{
						Name:       "swp1s1",
						MacAddress: "bb:bb:bb:bb:bb:b2",
					},
					metal.Nic{
						Name:       "swp1s2",
						MacAddress: "bb:bb:bb:bb:bb:b3",
					},
					metal.Nic{
						Name:       "swp1s3",
						MacAddress: "bb:bb:bb:bb:bb:b4",
					},
				},
				MachineConnections: metal.ConnectionMap{
					"m1": metal.Connections{
						metal.Connection{
							Nic: metal.Nic{
								Name:       "swp1s0",
								MacAddress: "bb:bb:bb:bb:bb:b1",
							},
						},
					},
					"fw1": metal.Connections{
						metal.Connection{
							Nic: metal.Nic{
								Name:       "swp1s1",
								MacAddress: "bb:bb:bb:bb:bb:b2",
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "fail if partition differs",
			args: args{
				old: &metal.Switch{
					ReplaceMode: metal.SwitchReplaceModeReplace,
					Partition:   "1",
				},
				newSwitch: &metal.Switch{
					Partition: "2",
				},
			},
			wantErr: true,
		},
		{
			name: "fail if rack differs",
			args: args{
				old: &metal.Switch{
					ReplaceMode: metal.SwitchReplaceModeReplace,
					Partition:   "1",
					Rack:        "1",
				},
				newSwitch: &metal.Switch{
					Partition: "1",
					Rack:      "2",
				},
			},
			wantErr: true,
		},
		{
			name: "fail if twin switch is also in replace mode",
			args: args{
				old: &metal.Switch{
					ReplaceMode: metal.SwitchReplaceModeReplace,
					Partition:   "1",
					Rack:        "1",
				},
				twin: &metal.Switch{
					ReplaceMode: metal.SwitchReplaceModeReplace,
					Partition:   "1",
					Rack:        "1",
				},
				newSwitch: &metal.Switch{
					Partition: "1",
					Rack:      "1",
				},
			},
			wantErr: true,
		},
		{
			name: "new switch is directly useable if twin has no machine connections",
			args: args{
				old: &metal.Switch{
					ReplaceMode: metal.SwitchReplaceModeReplace,
					Partition:   "1",
					Rack:        "1",
				},
				twin: &metal.Switch{
					OS:        &metal.SwitchOS{Vendor: metal.SwitchOSVendorCumulus},
					Partition: "1",
					Rack:      "1",
				},
				newSwitch: &metal.Switch{
					Partition: "1",
					Rack:      "1",
				},
			},
			want: &metal.Switch{
				Partition:   "1",
				Rack:        "1",
				ReplaceMode: metal.SwitchReplaceModeOperational,
			},
			wantErr: false,
		},
		{
			name: "adopt machine connections and nic configuration from twin with different switch os",
			args: args{
				old: &metal.Switch{
					OS: &metal.SwitchOS{
						Vendor: metal.SwitchOSVendorCumulus,
					},
					ReplaceMode: metal.SwitchReplaceModeReplace,
				},
				twin: &metal.Switch{
					OS: &metal.SwitchOS{
						Vendor: metal.SwitchOSVendorCumulus,
					},
					Nics: metal.Nics{
						metal.Nic{
							Name:       "swp1s0",
							MacAddress: "aa:aa:aa:aa:aa:a1",
							Vrf:        "1",
						},
						metal.Nic{
							Name:       "swp1s1",
							MacAddress: "aa:aa:aa:aa:aa:a2",
						},
						metal.Nic{
							Name:       "swp1s2",
							MacAddress: "aa:aa:aa:aa:aa:a3",
						},
					},
					MachineConnections: metal.ConnectionMap{
						"m1": metal.Connections{
							metal.Connection{
								Nic: metal.Nic{
									Name:       "swp1s0",
									MacAddress: "aa:aa:aa:aa:aa:a1",
								},
							},
						},
						"fw1": metal.Connections{
							metal.Connection{
								Nic: metal.Nic{
									Name:       "swp1s1",
									MacAddress: "aa:aa:aa:aa:aa:a2",
								},
							},
						},
					},
				},
				newSwitch: &metal.Switch{
					OS: &metal.SwitchOS{
						Vendor: metal.SwitchOSVendorSonic,
					},
					Nics: metal.Nics{
						metal.Nic{
							Name:       "Ethernet0",
							MacAddress: "bb:bb:bb:bb:bb:b1",
						},
						metal.Nic{
							Name:       "Ethernet1",
							MacAddress: "bb:bb:bb:bb:bb:b2",
						},
						metal.Nic{
							Name:       "Ethernet2",
							MacAddress: "bb:bb:bb:bb:bb:b3",
						},
						metal.Nic{
							Name:       "Ethernet3",
							MacAddress: "bb:bb:bb:bb:bb:b4",
						},
					},
				},
			},
			want: &metal.Switch{
				ReplaceMode: metal.SwitchReplaceModeOperational,
				OS: &metal.SwitchOS{
					Vendor: metal.SwitchOSVendorSonic,
				},
				Nics: metal.Nics{
					metal.Nic{
						Name:       "Ethernet0",
						MacAddress: "bb:bb:bb:bb:bb:b1",
						Vrf:        "1",
					},
					metal.Nic{
						Name:       "Ethernet1",
						MacAddress: "bb:bb:bb:bb:bb:b2",
					},
					metal.Nic{
						Name:       "Ethernet2",
						MacAddress: "bb:bb:bb:bb:bb:b3",
					},
					metal.Nic{
						Name:       "Ethernet3",
						MacAddress: "bb:bb:bb:bb:bb:b4",
					},
				},
				MachineConnections: metal.ConnectionMap{
					"m1": metal.Connections{
						metal.Connection{
							Nic: metal.Nic{
								Name:       "Ethernet0",
								MacAddress: "bb:bb:bb:bb:bb:b1",
							},
						},
					},
					"fw1": metal.Connections{
						metal.Connection{
							Nic: metal.Nic{
								Name:       "Ethernet1",
								MacAddress: "bb:bb:bb:bb:bb:b2",
							},
						},
					},
				},
			},
			wantErr: false,
		},
	}

	for i := range tests {
		tt := tests[i]
		t.Run(tt.name, func(t *testing.T) {
			got, err := adoptFromTwin(tt.args.old, tt.args.twin, tt.args.newSwitch)
			if (err != nil) != tt.wantErr {
				t.Errorf("adoptFromTwin() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !cmp.Equal(got, tt.want) {
				t.Errorf("adoptFromTwin() = %v", cmp.Diff(got, tt.want))
			}
		})
	}
}

func Test_adoptNics(t *testing.T) {
	type args struct {
		twin      *metal.Switch
		newSwitch *metal.Switch
	}
	tests := []struct {
		name    string
		args    args
		want    metal.Nics
		wantErr bool
	}{
		{
			name: "adopt vrf configuration, leave underlay ports untouched, newSwitch might have additional ports",
			args: args{
				twin: &metal.Switch{
					OS: &metal.SwitchOS{Vendor: metal.SwitchOSVendorCumulus},
					Nics: metal.Nics{
						metal.Nic{
							Name:       "swp1s0",
							MacAddress: "aa:aa:aa:aa:aa:a1",
							Vrf:        "vrf1",
						},
						metal.Nic{
							Name:       "swp1s1",
							MacAddress: "aa:aa:aa:aa:aa:a2",
							Vrf:        "",
						},
					},
				},
				newSwitch: &metal.Switch{
					OS: &metal.SwitchOS{Vendor: metal.SwitchOSVendorCumulus},
					Nics: metal.Nics{
						metal.Nic{
							Name:       "swp1s0",
							MacAddress: "bb:bb:bb:bb:bb:b1",
						},
						metal.Nic{
							Name:       "swp1s1",
							MacAddress: "bb:bb:bb:bb:bb:b2",
						},
						metal.Nic{
							Name:       "swp99",
							MacAddress: "bb:bb:bb:bb:bb:b3",
						},
					},
				},
			},
			want: metal.Nics{
				metal.Nic{
					Name:       "swp1s0",
					MacAddress: "bb:bb:bb:bb:bb:b1",
					Vrf:        "vrf1",
				},
				metal.Nic{
					Name:       "swp1s1",
					MacAddress: "bb:bb:bb:bb:bb:b2",
					Vrf:        "",
				},
				metal.Nic{
					Name:       "swp99",
					MacAddress: "bb:bb:bb:bb:bb:b3",
				},
			},
			wantErr: false,
		},
		{
			name: "new switch misses nic",
			args: args{
				twin: &metal.Switch{
					OS: &metal.SwitchOS{Vendor: metal.SwitchOSVendorCumulus},
					Nics: metal.Nics{
						metal.Nic{
							Name:       "swp1s0",
							MacAddress: "aa:aa:aa:aa:aa:a1",
							Vrf:        "vrf1",
						},
					},
				},
				newSwitch: &metal.Switch{
					OS: &metal.SwitchOS{Vendor: metal.SwitchOSVendorCumulus},
					Nics: metal.Nics{
						metal.Nic{
							Name:       "swp1s1",
							MacAddress: "bb:bb:bb:bb:bb:b2",
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "switch os from cumulus to sonic",
			args: args{
				twin: &metal.Switch{
					OS: &metal.SwitchOS{
						Vendor: metal.SwitchOSVendorCumulus,
					},
					Nics: metal.Nics{
						metal.Nic{
							Name:       "swp1s0",
							MacAddress: "aa:aa:aa:aa:aa:a1",
							Vrf:        "vrf1",
						},
						metal.Nic{
							Name:       "swp1s1",
							MacAddress: "aa:aa:aa:aa:aa:a2",
							Vrf:        "",
						},
						metal.Nic{
							Name:       "swp99",
							MacAddress: "aa:aa:aa:aa:aa:a3",
						},
					},
				},
				newSwitch: &metal.Switch{
					OS: &metal.SwitchOS{
						Vendor: metal.SwitchOSVendorSonic,
					},
					Nics: metal.Nics{
						metal.Nic{
							Name:       "Ethernet0",
							MacAddress: "bb:bb:bb:bb:bb:b2",
						},
						metal.Nic{
							Name:       "Ethernet1",
							MacAddress: "bb:bb:bb:bb:bb:b3",
						},
						metal.Nic{
							Name:       "Ethernet392",
							MacAddress: "bb:bb:bb:bb:bb:b4",
						},
					},
				},
			},
			want: metal.Nics{
				metal.Nic{
					Name:       "Ethernet0",
					MacAddress: "bb:bb:bb:bb:bb:b2",
					Vrf:        "vrf1",
				},
				metal.Nic{
					Name:       "Ethernet1",
					MacAddress: "bb:bb:bb:bb:bb:b3",
					Vrf:        "",
				},
				metal.Nic{
					Name:       "Ethernet392",
					MacAddress: "bb:bb:bb:bb:bb:b4",
				},
			},
			wantErr: false,
		},
	}

	for i := range tests {
		tt := tests[i]
		t.Run(tt.name, func(t *testing.T) {
			got, err := adoptNics(tt.args.twin, tt.args.newSwitch)
			if (err != nil) != tt.wantErr {
				t.Errorf("adoptNics() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if diff := cmp.Diff(got, tt.want); diff != "" {
				t.Errorf("diff %v", diff)
			}
		})
	}
}

func Test_adoptMachineConnections(t *testing.T) {
	type args struct {
		twin      *metal.Switch
		newSwitch *metal.Switch
	}
	tests := []struct {
		name    string
		args    args
		want    metal.ConnectionMap
		wantErr bool
	}{
		{
			name: "adopt machine connections from twin",
			args: args{
				twin: &metal.Switch{
					OS: &metal.SwitchOS{Vendor: metal.SwitchOSVendorCumulus},
					MachineConnections: metal.ConnectionMap{
						"m1": metal.Connections{
							metal.Connection{
								Nic: metal.Nic{
									Name:       "swp1s0",
									MacAddress: "aa:aa:aa:aa:aa:a1",
								},
							},
						},
						"m2": metal.Connections{
							metal.Connection{
								Nic: metal.Nic{
									Name:       "swp1s1",
									MacAddress: "aa:aa:aa:aa:aa:a2",
								},
							},
						},
					},
				},
				newSwitch: &metal.Switch{
					OS: &metal.SwitchOS{Vendor: metal.SwitchOSVendorCumulus},
					Nics: metal.Nics{
						metal.Nic{
							Name:       "swp1s0",
							MacAddress: "bb:bb:bb:bb:bb:b1",
						},
						metal.Nic{
							Name:       "swp1s1",
							MacAddress: "bb:bb:bb:bb:bb:b2",
						},
					},
				},
			},
			want: metal.ConnectionMap{
				"m1": metal.Connections{
					metal.Connection{
						Nic: metal.Nic{
							Name:       "swp1s0",
							MacAddress: "bb:bb:bb:bb:bb:b1",
						},
					},
				},
				"m2": metal.Connections{
					metal.Connection{
						Nic: metal.Nic{
							Name:       "swp1s1",
							MacAddress: "bb:bb:bb:bb:bb:b2",
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "new switch misses nic for existing machine connection at twin",
			args: args{
				twin: &metal.Switch{
					OS: &metal.SwitchOS{Vendor: metal.SwitchOSVendorCumulus},
					MachineConnections: metal.ConnectionMap{
						"m1": metal.Connections{
							metal.Connection{
								Nic: metal.Nic{
									Name:       "swp1s0",
									MacAddress: "aa:aa:aa:aa:aa:a1",
								},
							},
						},
					},
				},
				newSwitch: &metal.Switch{
					OS: &metal.SwitchOS{Vendor: metal.SwitchOSVendorCumulus},
					Nics: metal.Nics{
						metal.Nic{
							Name:       "swp1s1",
							MacAddress: "bb:bb:bb:bb:bb:b2",
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "adopt from twin with different switch os",
			args: args{
				twin: &metal.Switch{
					OS: &metal.SwitchOS{Vendor: metal.SwitchOSVendorCumulus},
					MachineConnections: metal.ConnectionMap{
						"m1": metal.Connections{
							metal.Connection{
								Nic: metal.Nic{
									Name:       "swp1s0",
									MacAddress: "aa:aa:aa:aa:aa:a1",
								},
							},
						},
						"m2": metal.Connections{
							metal.Connection{
								Nic: metal.Nic{
									Name:       "swp1s1",
									MacAddress: "aa:aa:aa:aa:aa:a2",
								},
							},
						},
					},
				},
				newSwitch: &metal.Switch{
					OS: &metal.SwitchOS{Vendor: metal.SwitchOSVendorSonic},
					Nics: metal.Nics{
						metal.Nic{
							Name:       "Ethernet0",
							MacAddress: "bb:bb:bb:bb:bb:b1",
						},
						metal.Nic{
							Name:       "Ethernet1",
							MacAddress: "bb:bb:bb:bb:bb:b2",
						},
					},
				},
			},
			want: metal.ConnectionMap{
				"m1": metal.Connections{
					metal.Connection{
						Nic: metal.Nic{
							Name:       "Ethernet0",
							MacAddress: "bb:bb:bb:bb:bb:b1",
						},
					},
				},
				"m2": metal.Connections{
					metal.Connection{
						Nic: metal.Nic{
							Name:       "Ethernet1",
							MacAddress: "bb:bb:bb:bb:bb:b2",
						},
					},
				},
			},
			wantErr: false,
		},
	}

	for i := range tests {
		tt := tests[i]
		t.Run(tt.name, func(t *testing.T) {
			got, err := adoptMachineConnections(tt.args.twin, tt.args.newSwitch)
			if (err != nil) != tt.wantErr {
				t.Errorf("adoptMachineConnections() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("adoptMachineConnections() diff = %s", diff)
			}
		})
	}
}

func Test_adjustMachineNics(t *testing.T) {
	tests := []struct {
		name        string
		nics        metal.Nics
		connections metal.Connections
		nicMap      metal.NicMap
		want        metal.Nics
		wantErr     bool
	}{
		{
			name: "nothing to adjust",
			nics: []metal.Nic{
				{
					Name:       "eth0",
					MacAddress: "11:11:11:11:11:11",
					Neighbors: []metal.Nic{
						{
							Name:       "swp1",
							MacAddress: "aa:aa:aa:aa:aa:aa",
						},
					},
				},
				{
					Name:       "eth1",
					MacAddress: "11:11:11:11:11:22",
					Neighbors: []metal.Nic{
						{
							Name:       "swp1",
							MacAddress: "aa:aa:aa:aa:aa:bb",
						},
					},
				},
			},
			connections: []metal.Connection{
				{
					Nic: metal.Nic{
						Name:       "swp1",
						MacAddress: "cc:cc:cc:cc:cc:cc",
					},
				},
			},
			want: []metal.Nic{
				{
					Name:       "eth0",
					MacAddress: "11:11:11:11:11:11",
					Neighbors: []metal.Nic{
						{
							Name:       "swp1",
							MacAddress: "aa:aa:aa:aa:aa:aa",
						},
					},
				},
				{
					Name:       "eth1",
					MacAddress: "11:11:11:11:11:22",
					Neighbors: []metal.Nic{
						{
							Name:       "swp1",
							MacAddress: "aa:aa:aa:aa:aa:bb",
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "unrealistic error case",
			nics: []metal.Nic{
				{
					Name:       "eth0",
					MacAddress: "11:11:11:11:11:11",
					Neighbors: []metal.Nic{
						{
							Name:       "swp2",
							MacAddress: "aa:aa:aa:aa:aa:aa",
						},
					},
				},
				{
					Name:       "eth1",
					MacAddress: "11:11:11:11:11:22",
					Neighbors: []metal.Nic{
						{
							Name:       "swp2",
							MacAddress: "aa:aa:aa:aa:aa:bb",
						},
					},
				},
			},
			connections: []metal.Connection{
				{
					Nic: metal.Nic{
						Name:       "swp2",
						MacAddress: "aa:aa:aa:aa:aa:aa",
					},
				},
			},
			nicMap: map[string]*metal.Nic{
				"swp1": {
					Name:       "Ethernet0",
					MacAddress: "dd:dd:dd:dd:dd:dd",
				},
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "adjust nics from cumulus to sonic",
			nics: []metal.Nic{
				{
					Name:       "eth0",
					MacAddress: "11:11:11:11:11:11",
					Neighbors: []metal.Nic{
						{
							Name:       "swp1",
							MacAddress: "aa:aa:aa:aa:aa:aa",
						},
					},
				},
				{
					Name:       "eth1",
					MacAddress: "11:11:11:11:11:22",
					Neighbors: []metal.Nic{
						{
							Name:       "swp1",
							MacAddress: "aa:aa:aa:aa:aa:bb",
						},
					},
				},
			},
			connections: []metal.Connection{
				{
					Nic: metal.Nic{
						Name:       "swp1",
						MacAddress: "aa:aa:aa:aa:aa:aa",
					},
				},
			},
			nicMap: map[string]*metal.Nic{
				"swp1": {
					Name:       "Ethernet0",
					MacAddress: "dd:dd:dd:dd:dd:dd",
				},
			},
			want: []metal.Nic{
				{
					Name:       "eth0",
					MacAddress: "11:11:11:11:11:11",
					Neighbors: []metal.Nic{
						{
							Name:       "Ethernet0",
							MacAddress: "dd:dd:dd:dd:dd:dd",
						},
					},
				},
				{
					Name:       "eth1",
					MacAddress: "11:11:11:11:11:22",
					Neighbors: []metal.Nic{
						{
							Name:       "swp1",
							MacAddress: "aa:aa:aa:aa:aa:bb",
						},
					},
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := adjustMachineNics(tt.nics, tt.connections, tt.nicMap)
			if (err != nil) != tt.wantErr {
				t.Errorf("adjustMachineNics() error = %v, wantErr %v", err, tt.wantErr)
			}
			if diff := cmp.Diff(got, tt.want); diff != "" {
				t.Errorf("adjustMachineNics() diff = %v", diff)
			}
		})
	}
}
