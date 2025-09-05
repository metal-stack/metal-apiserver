package repository

import (
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	"github.com/metal-stack/metal-lib/pkg/pointer"
	"google.golang.org/protobuf/testing/protocmp"
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
					Vrf:        pointer.Pointer("Vrf100"),
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
					Vrf:        pointer.Pointer("Vrf100"),
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
		vrf      *string
		networks []*metal.Network
		ips      []*metal.IP
		want     *apiv2.BGPFilter
		wantErr  bool
	}{
		{
			name:     "no allocation",
			m:        &metal.Machine{},
			vrf:      new(string),
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
			vrf:      pointer.Pointer("default"),
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
			got, err := makeBGPFilter(tt.m, tt.vrf, tt.networks, tt.ips)
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
		name     string
		m        *metal.Machine
		networks metal.NetworkMap
		ips      metal.IPsMap
		want     *apiv2.BGPFilter
		wantErr  error
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
			name: "add cidrs",
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
						IPAddress: "5.5.5.5",
					},
				},
			},
			want: &apiv2.BGPFilter{
				Cidrs: []string{"1.1.1.1/32", "1.1.2.0/24", "2.2.2.0/30", "4.4.4.4/32"},
				Vnis:  []string{},
			},
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := makeBGPFilterMachine(tt.m, tt.networks, tt.ips)
			if diff := cmp.Diff(tt.wantErr, err, errorutil.ErrorComparer()); diff != "" {
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
