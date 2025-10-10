package metal

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-lib/pkg/pointer"
	"google.golang.org/protobuf/types/known/durationpb"
)

func TestToMetalNics(t *testing.T) {
	tests := []struct {
		name       string
		switchNics []*apiv2.SwitchNic
		want       Nics
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
						Desired: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UNSPECIFIED,
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
						Desired: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
						Actual:  apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_DOWN,
					},
				},
				{
					Name:       "Ethernet1",
					Identifier: "Eth1/2",
					Mac:        "22:22:22:22:22:22",
					Vrf:        pointer.Pointer("Vrf100"),
					State: &apiv2.NicState{
						Desired: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
						Actual:  apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
					},
					BgpPortState: &apiv2.SwitchBGPPortState{
						Neighbor:              "lan0",
						PeerGroup:             "external",
						VrfName:               "Vrf200",
						BgpState:              apiv2.BGPState_BGP_STATE_ESTABLISHED,
						BgpTimerUpEstablished: durationpb.New(time.Hour),
						SentPrefixCounter:     200,
						AcceptedPrefixCounter: 1,
					},
				},
			},
			want: Nics{
				{
					MacAddress: "11:11:11:11:11:11",
					Name:       "Ethernet0",
					Identifier: "Eth1/1",
					State: &NicState{
						Desired: SwitchPortStatusUp,
						Actual:  SwitchPortStatusDown,
					},
				},
				{
					MacAddress: "22:22:22:22:22:22",
					Name:       "Ethernet1",
					Identifier: "Eth1/2",
					Vrf:        "Vrf100",
					State: &NicState{
						Desired: SwitchPortStatusUp,
						Actual:  SwitchPortStatusUp,
					},
					BGPPortState: &SwitchBGPPortState{
						Neighbor:              "lan0",
						PeerGroup:             "external",
						VrfName:               "Vrf200",
						BgpState:              BGPStateEstablished,
						BgpTimerUpEstablished: uint64(time.Hour),
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
			got, err := ToMetalNics(tt.switchNics)
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
		want        ConnectionMap
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
			want: ConnectionMap{
				"machine-a": {
					{
						Nic: Nic{
							Identifier: "Eth1/1",
						},
						MachineID: "machine-a",
					},
				},
				"machine-b": {
					{
						Nic: Nic{
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
			want: ConnectionMap{
				"machine-a": {
					{
						Nic: Nic{
							Identifier: "Eth1/1",
						},
						MachineID: "machine-a",
					},
				},
				"machine-b": {
					{
						Nic: Nic{
							Identifier: "Eth1/2",
						},
						MachineID: "machine-b",
					},
					{
						Nic: Nic{
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
			got, err := ToMachineConnections(tt.connections)
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

func TestToReplaceMode(t *testing.T) {
	tests := []struct {
		name    string
		mode    apiv2.SwitchReplaceMode
		want    SwitchReplaceMode
		wantErr bool
	}{
		{
			name:    "unspecified",
			mode:    apiv2.SwitchReplaceMode_SWITCH_REPLACE_MODE_UNSPECIFIED,
			want:    "",
			wantErr: true,
		},
		{
			name:    "valid",
			mode:    apiv2.SwitchReplaceMode_SWITCH_REPLACE_MODE_OPERATIONAL,
			want:    SwitchReplaceModeOperational,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ToReplaceMode(tt.mode)
			if (err != nil) != tt.wantErr {
				t.Errorf("ToReplaceMode() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ToReplaceMode() = %v, want %v", got, tt.want)
			}
		})
	}
}
