package metal_test

import (
	"reflect"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/metal-stack/api-server/pkg/db/metal"
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
