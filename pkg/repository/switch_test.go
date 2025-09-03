package repository

import (
	"reflect"
	"testing"

	"github.com/google/go-cmp/cmp"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-lib/pkg/pointer"
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
	type args struct {
		m        *metal.Machine
		vrf      *string
		networks []*metal.Network
		ips      []*metal.IP
	}
	tests := []struct {
		name    string
		args    args
		want    *apiv2.BGPFilter
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := makeBGPFilter(tt.args.m, tt.args.vrf, tt.args.networks, tt.args.ips)
			if (err != nil) != tt.wantErr {
				t.Errorf("makeBGPFilter() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("makeBGPFilter() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_makeBGPFilterFirewall(t *testing.T) {
	tests := []struct {
		name    string
		m       *metal.Machine
		want    *apiv2.BGPFilter
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := makeBGPFilterFirewall(tt.m)
			if (err != nil) != tt.wantErr {
				t.Errorf("makeBGPFilterFirewall() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("makeBGPFilterFirewall() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_makeBGPFilterMachine(t *testing.T) {
	type args struct {
		m        *metal.Machine
		networks metal.NetworkMap
		ips      metal.IPsMap
	}
	tests := []struct {
		name    string
		args    args
		want    *apiv2.BGPFilter
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := makeBGPFilterMachine(tt.args.m, tt.args.networks, tt.args.ips)
			if (err != nil) != tt.wantErr {
				t.Errorf("makeBGPFilterMachine() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("makeBGPFilterMachine() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_compactCidrs(t *testing.T) {
	type args struct {
		cidrs []string
	}
	tests := []struct {
		name    string
		args    args
		want    []string
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := compactCidrs(tt.args.cidrs)
			if (err != nil) != tt.wantErr {
				t.Errorf("compactCidrs() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("compactCidrs() = %v, want %v", got, tt.want)
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
