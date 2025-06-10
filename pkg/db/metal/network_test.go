package metal_test

import (
	"reflect"
	"testing"

	"github.com/google/go-cmp/cmp"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-lib/pkg/pointer"
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
			af:   metal.AddressFamilyIPv4,
			p:    metal.Prefixes{},
			want: nil,
		},
		{
			name: "prefixes filtered by ipv4",
			af:   metal.AddressFamilyIPv4,
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
			af:   metal.AddressFamilyIPv6,
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
			af:   metal.AddressFamilyIPv6,
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
			want: metal.AddressFamilies{metal.AddressFamilyIPv4},
		},
		{
			name: "only ipv6",
			p: metal.Prefixes{
				{IP: "fe80::", Length: "64"},
			},
			want: metal.AddressFamilies{metal.AddressFamilyIPv6},
		},
		{
			name: "both afs",
			p: metal.Prefixes{
				{IP: "1.2.3.0", Length: "28"},
				{IP: "fe80::", Length: "64"},
			},
			want: metal.AddressFamilies{metal.AddressFamilyIPv4, metal.AddressFamilyIPv6},
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
			var copyExisting metal.Prefixes
			copyExisting = append(copyExisting, tt.existing...)

			if diff := cmp.Diff(tt.want, metal.Prefixes(tt.existing.SubtractPrefixes(tt.subtract...))); diff != "" {
				t.Errorf("diff = %s", diff)
			}

			// check existing not modified
			if diff := cmp.Diff(tt.existing, copyExisting); diff != "" {
				t.Errorf("modified prefixes = %s", diff)
			}
		})
	}
}

func TestToNetworkType(t *testing.T) {
	tests := []struct {
		name    string
		nwt     apiv2.NetworkType
		want    metal.NetworkType
		wantErr bool
	}{
		{
			name:    "child network type",
			nwt:     apiv2.NetworkType_NETWORK_TYPE_CHILD,
			want:    metal.NetworkTypeChild,
			wantErr: false,
		},
		{
			name:    "child shared network type",
			nwt:     apiv2.NetworkType_NETWORK_TYPE_CHILD_SHARED,
			want:    metal.NetworkTypeChildShared,
			wantErr: false,
		},
		{
			name:    "super network type",
			nwt:     apiv2.NetworkType_NETWORK_TYPE_SUPER,
			want:    metal.NetworkTypeSuper,
			wantErr: false,
		},
		{
			name:    "super namespaced network type",
			nwt:     apiv2.NetworkType_NETWORK_TYPE_SUPER_NAMESPACED,
			want:    metal.NetworkTypeSuperNamespaced,
			wantErr: false,
		},
		{
			name:    "external network type",
			nwt:     apiv2.NetworkType_NETWORK_TYPE_EXTERNAL,
			want:    metal.NetworkTypeExternal,
			wantErr: false,
		},
		{
			name:    "underlay network type",
			nwt:     apiv2.NetworkType_NETWORK_TYPE_UNDERLAY,
			want:    metal.NetworkTypeUnderlay,
			wantErr: false,
		},
		{
			name:    "unspecified network type",
			nwt:     apiv2.NetworkType_NETWORK_TYPE_UNSPECIFIED,
			want:    metal.NetworkType(""),
			wantErr: true,
		},
		{
			name:    "unspecified network type",
			nwt:     42,
			want:    metal.NetworkType(""),
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := metal.ToNetworkType(tt.nwt)
			if (err != nil) != tt.wantErr {
				t.Errorf("ToNetworkType() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ToNetworkType() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFromNetworkType(t *testing.T) {
	tests := []struct {
		name    string
		nwt     metal.NetworkType
		want    apiv2.NetworkType
		wantErr bool
	}{

		{
			name:    "child network type",
			nwt:     metal.NetworkTypeChild,
			want:    apiv2.NetworkType_NETWORK_TYPE_CHILD,
			wantErr: false,
		},
		{
			name:    "child shared network type",
			nwt:     metal.NetworkTypeChildShared,
			want:    apiv2.NetworkType_NETWORK_TYPE_CHILD_SHARED,
			wantErr: false,
		},
		{
			name:    "super network type",
			nwt:     metal.NetworkTypeSuper,
			want:    apiv2.NetworkType_NETWORK_TYPE_SUPER,
			wantErr: false,
		},
		{
			name:    "super namespaced network type",
			nwt:     metal.NetworkTypeSuperNamespaced,
			want:    apiv2.NetworkType_NETWORK_TYPE_SUPER_NAMESPACED,
			wantErr: false,
		},
		{
			name:    "external network type",
			nwt:     metal.NetworkTypeExternal,
			want:    apiv2.NetworkType_NETWORK_TYPE_EXTERNAL,
			wantErr: false,
		},
		{
			name:    "underlay network type",
			nwt:     metal.NetworkTypeUnderlay,
			want:    apiv2.NetworkType_NETWORK_TYPE_UNDERLAY,
			wantErr: false,
		},
		{
			name:    "unspecified network type",
			nwt:     metal.NetworkType(""),
			want:    apiv2.NetworkType_NETWORK_TYPE_UNSPECIFIED,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := metal.FromNetworkType(tt.nwt)
			if (err != nil) != tt.wantErr {
				t.Errorf("FromNetworkType() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("FromNetworkType() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestToNATType(t *testing.T) {
	tests := []struct {
		name    string
		nt      apiv2.NATType
		want    metal.NATType
		wantErr bool
	}{
		{
			name:    "none nattype",
			nt:      apiv2.NATType_NAT_TYPE_NONE,
			want:    metal.NATTypeNone,
			wantErr: false,
		},
		{
			name:    "masq nattype",
			nt:      apiv2.NATType_NAT_TYPE_IPV4_MASQUERADE,
			want:    metal.NATTypeIPv4Masquerade,
			wantErr: false,
		},
		{
			name:    "unspecified nattype",
			nt:      apiv2.NATType_NAT_TYPE_UNSPECIFIED,
			want:    metal.NATTypeInvalid,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := metal.ToNATType(tt.nt)
			if (err != nil) != tt.wantErr {
				t.Errorf("ToNATType() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ToNATType() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFromNATType(t *testing.T) {
	tests := []struct {
		name    string
		nt      metal.NATType
		want    apiv2.NATType
		wantErr bool
	}{
		{
			name:    "none nattype",
			nt:      metal.NATTypeNone,
			want:    apiv2.NATType_NAT_TYPE_NONE,
			wantErr: false,
		},
		{
			name:    "masq nattype",
			nt:      metal.NATTypeIPv4Masquerade,
			want:    apiv2.NATType_NAT_TYPE_IPV4_MASQUERADE,
			wantErr: false,
		},
		{
			name:    "unspecified nattype",
			nt:      metal.NATTypeInvalid,
			want:    apiv2.NATType_NAT_TYPE_UNSPECIFIED,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := metal.FromNATType(tt.nt)
			if (err != nil) != tt.wantErr {
				t.Errorf("FromNATType() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("FromNATType() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestToAddressFamily(t *testing.T) {
	tests := []struct {
		name    string
		af      apiv2.IPAddressFamily
		want    metal.AddressFamily
		wantErr bool
	}{
		{
			name:    "ipv4",
			af:      apiv2.IPAddressFamily_IP_ADDRESS_FAMILY_V4,
			want:    metal.AddressFamilyIPv4,
			wantErr: false,
		},
		{
			name:    "ipv6",
			af:      apiv2.IPAddressFamily_IP_ADDRESS_FAMILY_V6,
			want:    metal.AddressFamilyIPv6,
			wantErr: false,
		},
		{
			name:    "invalid",
			af:      apiv2.IPAddressFamily_IP_ADDRESS_FAMILY_UNSPECIFIED,
			want:    metal.AddressFamily(""),
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := metal.ToAddressFamily(tt.af)
			if (err != nil) != tt.wantErr {
				t.Errorf("ToAddressFamily() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ToAddressFamily() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsSuperNetwork(t *testing.T) {
	tests := []struct {
		name string
		nt   *metal.NetworkType
		want bool
	}{
		{
			name: "super",
			nt:   pointer.Pointer(metal.NetworkTypeSuper),
			want: true,
		},
		{
			name: "super namespaced",
			nt:   pointer.Pointer(metal.NetworkTypeSuperNamespaced),
			want: true,
		},
		{
			name: "underlay",
			nt:   pointer.Pointer(metal.NetworkTypeUnderlay),
			want: false,
		},
		{
			name: "nil",
			nt:   nil,
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := metal.IsSuperNetwork(tt.nt); got != tt.want {
				t.Errorf("IsChildNetwork() = %v, want %v", got, tt.want)
			}
		})
	}
}
func TestIsChildNetwork(t *testing.T) {
	tests := []struct {
		name string
		nt   *metal.NetworkType
		want bool
	}{
		{
			name: "child",
			nt:   pointer.Pointer(metal.NetworkTypeChild),
			want: true,
		},
		{
			name: "child shared",
			nt:   pointer.Pointer(metal.NetworkTypeChildShared),
			want: true,
		},
		{
			name: "underlay",
			nt:   pointer.Pointer(metal.NetworkTypeUnderlay),
			want: false,
		},
		{
			name: "nil",
			nt:   nil,
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := metal.IsChildNetwork(tt.nt); got != tt.want {
				t.Errorf("IsChildNetwork() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestToChildPrefixLength(t *testing.T) {
	tests := []struct {
		name string
		cpl  *apiv2.ChildPrefixLength
		want metal.ChildPrefixLength
	}{
		{
			name: "nil results into nil",
			cpl:  nil,
			want: nil,
		},
		{
			name: "zero results into zero",
			cpl:  &apiv2.ChildPrefixLength{},
			want: metal.ChildPrefixLength{},
		},
		{
			name: "ipv4 and ipv6 are properly mapped",
			cpl: &apiv2.ChildPrefixLength{
				Ipv4: pointer.Pointer(uint32(28)),
				Ipv6: pointer.Pointer(uint32(56)),
			},
			want: metal.ChildPrefixLength{
				metal.AddressFamilyIPv4: 28,
				metal.AddressFamilyIPv6: 56,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := metal.ToChildPrefixLength(tt.cpl)
			if diff := cmp.Diff(got, tt.want); diff != "" {
				t.Errorf("diff = %s", diff)
			}
		})
	}
}
