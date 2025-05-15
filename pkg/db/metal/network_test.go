package metal_test

import (
	"reflect"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
)

func TestPrefixes_OfFamily(t *testing.T) {
	tests := []struct {
		name string
		af   metal.AddressFamily
		p    metal.Prefixes
		want metal.Prefixes
	}{
		{
			name: "no prefixes filtered by ipv4",
			af:   metal.IPv4AddressFamily,
			p:    metal.Prefixes{},
			want: nil,
		},
		{
			name: "prefixes filtered by ipv4",
			af:   metal.IPv4AddressFamily,
			p: metal.Prefixes{
				{IP: "1.2.3.0", Length: "28"},
				{IP: "fe80::", Length: "64"},
			},
			want: metal.Prefixes{
				{IP: "1.2.3.0", Length: "28"},
			},
		},
		{
			name: "prefixes filtered by ipv6",
			af:   metal.IPv6AddressFamily,
			p: metal.Prefixes{
				{IP: "1.2.3.0", Length: "28"},
				{IP: "fe80::", Length: "64"},
			},
			want: metal.Prefixes{
				{IP: "fe80::", Length: "64"},
			},
		},
		{
			name: "malformed prefixes are skipped",
			af:   metal.IPv6AddressFamily,
			p: metal.Prefixes{
				{IP: "1.2.3.0", Length: "28"},
				{IP: "fe80::", Length: "metal-stack-rulez"},
			},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.p.OfFamily(tt.af)
			if diff := cmp.Diff(got, tt.want); diff != "" {
				t.Errorf("diff = %s", diff)
			}
		})
	}
}

func TestPrefixes_AddressFamilies(t *testing.T) {
	tests := []struct {
		name string
		p    metal.Prefixes
		want metal.AddressFamilies
	}{
		{
			name: "only ipv4",
			p: metal.Prefixes{
				{IP: "1.2.3.0", Length: "28"},
			},
			want: metal.AddressFamilies{metal.IPv4AddressFamily},
		},
		{
			name: "only ipv6",
			p: metal.Prefixes{
				{IP: "fe80::", Length: "64"},
			},
			want: metal.AddressFamilies{metal.IPv6AddressFamily},
		},
		{
			name: "both afs",
			p: metal.Prefixes{
				{IP: "1.2.3.0", Length: "28"},
				{IP: "fe80::", Length: "64"},
			},
			want: metal.AddressFamilies{metal.IPv4AddressFamily, metal.IPv6AddressFamily},
		},
		{
			name: "nil prefixes",
			p:    nil,
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.p.AddressFamilies(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Prefixes.AddressFamilies() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNetwork_SubtractPrefixes(t *testing.T) {
	tests := []struct {
		name     string
		existing metal.Prefixes
		subtract metal.Prefixes
		want     metal.Prefixes
	}{
		{
			name: "subtract single prefix from existing prefixes",
			existing: metal.Prefixes{
				{IP: "1.2.3.4", Length: "32"},
				{IP: "2.3.4.5", Length: "32"},
				{IP: "3.4.5.6", Length: "32"},
				{IP: "10.0.0.0", Length: "8"},
			},
			subtract: metal.Prefixes{
				{IP: "2.3.4.5", Length: "32"},
			},
			want: metal.Prefixes{
				{IP: "1.2.3.4", Length: "32"},
				{IP: "3.4.5.6", Length: "32"},
				{IP: "10.0.0.0", Length: "8"},
			},
		},
		{
			name: "subtract two prefix from existing prefixes",
			existing: metal.Prefixes{
				{IP: "1.2.3.4", Length: "32"},
				{IP: "2.3.4.5", Length: "32"},
				{IP: "3.4.5.6", Length: "32"},
				{IP: "10.0.0.0", Length: "8"},
			},
			subtract: metal.Prefixes{
				{IP: "2.3.4.5", Length: "32"},
				{IP: "10.0.0.0", Length: "8"},
			},
			want: metal.Prefixes{
				{IP: "1.2.3.4", Length: "32"},
				{IP: "3.4.5.6", Length: "32"},
			},
		},
		{
			name: "subtract non existing prefix",
			existing: metal.Prefixes{
				{IP: "1.2.3.4", Length: "32"},
				{IP: "2.3.4.5", Length: "32"},
				{IP: "3.4.5.6", Length: "32"},
				{IP: "10.0.0.0", Length: "8"},
			},
			subtract: metal.Prefixes{
				{IP: "255.255.255.0", Length: "24"},
			},
			want: metal.Prefixes{
				{IP: "1.2.3.4", Length: "32"},
				{IP: "2.3.4.5", Length: "32"},
				{IP: "3.4.5.6", Length: "32"},
				{IP: "10.0.0.0", Length: "8"},
			},
		},
		{
			name:     "subtract from empty",
			existing: nil,
			subtract: metal.Prefixes{
				{IP: "255.255.255.0", Length: "24"},
			},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := &metal.Network{Prefixes: tt.existing}

			if diff := cmp.Diff(tt.want, metal.Prefixes(n.SubtractPrefixes(tt.subtract...))); diff != "" {
				t.Errorf("diff = %s", diff)
			}
		})
	}
}
