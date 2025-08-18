package queries_test

import (
	"log/slog"
	"os"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/stretchr/testify/require"

	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-apiserver/pkg/db/queries"
	"github.com/metal-stack/metal-apiserver/pkg/test"
	"github.com/metal-stack/metal-lib/pkg/pointer"
)

var (
	n1 = &metal.Network{
		Base:                       metal.Base{ID: "n1", Name: "n1", Description: "Network 1"},
		Prefixes:                   metal.Prefixes{{IP: "10.0.0.0", Length: "8"}},
		DestinationPrefixes:        metal.Prefixes{{IP: "0.0.0.0", Length: "0"}},
		DefaultChildPrefixLength:   metal.ChildPrefixLength{metal.AddressFamilyIPv4: 22},
		MinChildPrefixLength:       metal.ChildPrefixLength{metal.AddressFamilyIPv4: 14},
		PartitionID:                "partition-1",
		ProjectID:                  "p1",
		ParentNetwork:              "parent-network",
		Vrf:                        uint(42),
		PrivateSuper:               true,
		Nat:                        true,
		Underlay:                   true,
		Shared:                     true,
		Labels:                     map[string]string{"color": "red", "size": "small"},
		AdditionalAnnouncableCIDRs: []string{"10.240.0.0/12"},
		NetworkType:                pointer.Pointer(metal.NetworkTypeExternal),
		NATType:                    pointer.Pointer(metal.NATTypeIPv4Masquerade),
	}
	n2 = &metal.Network{
		Base:                       metal.Base{ID: "n2", Name: "n2", Description: "Network 2"},
		Prefixes:                   metal.Prefixes{{IP: "2001:db8::", Length: "96"}},
		DestinationPrefixes:        metal.Prefixes{{IP: "::", Length: "0"}},
		DefaultChildPrefixLength:   metal.ChildPrefixLength{metal.AddressFamilyIPv6: 64},
		MinChildPrefixLength:       metal.ChildPrefixLength{metal.AddressFamilyIPv6: 56},
		PartitionID:                "partition-2",
		ProjectID:                  "p2",
		ParentNetwork:              "parent-network-2",
		Vrf:                        uint(43),
		PrivateSuper:               false,
		Nat:                        false,
		Underlay:                   false,
		Shared:                     false,
		Labels:                     map[string]string{"color": "green", "size": "medium"},
		AdditionalAnnouncableCIDRs: []string{"10.241.0.0/12"},
		NetworkType:                pointer.Pointer(metal.NetworkTypeChild),
		NATType:                    pointer.Pointer(metal.NATTypeNone),
	}
	n3 = &metal.Network{
		Base:                       metal.Base{ID: "n3", Name: "n3", Description: "Network 3"},
		Prefixes:                   metal.Prefixes{{IP: "2001:db8::", Length: "96"}, {IP: "13.0.0.0", Length: "8"}},
		DestinationPrefixes:        metal.Prefixes{{IP: "::", Length: "0"}, {IP: "0.0.0.0", Length: "0"}},
		DefaultChildPrefixLength:   metal.ChildPrefixLength{metal.AddressFamilyIPv6: 64, metal.AddressFamilyIPv4: 22},
		MinChildPrefixLength:       metal.ChildPrefixLength{metal.AddressFamilyIPv6: 56, metal.AddressFamilyIPv4: 14},
		PartitionID:                "partition-3",
		ProjectID:                  "p3",
		Namespace:                  pointer.Pointer("p3"),
		ParentNetwork:              "parent-network-3",
		Vrf:                        uint(44),
		PrivateSuper:               false,
		Nat:                        false,
		Underlay:                   false,
		Shared:                     false,
		Labels:                     map[string]string{"color": "blue", "size": "large"},
		AdditionalAnnouncableCIDRs: []string{"10.241.0.0/12"},
		NetworkType:                pointer.Pointer(metal.NetworkTypeExternal),
		NATType:                    pointer.Pointer(metal.NATTypeNone),
	}
	networks = []*metal.Network{n1, n2, n3}
)

func TestNetworkFilter(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	ds, _, rethinkCloser := test.StartRethink(t, log)
	defer func() {
		rethinkCloser()
	}()

	ctx := t.Context()

	for _, n := range networks {
		_, err := ds.Network().Create(ctx, n)
		require.NoError(t, err)
	}

	tests := []struct {
		name string
		rq   *apiv2.NetworkQuery
		want []*metal.Network
	}{
		{
			name: "empty request returns unfiltered",
			rq:   nil,
			want: []*metal.Network{n1, n2, n3},
		},
		{
			name: "by id",
			rq:   &apiv2.NetworkQuery{Id: &n1.ID},
			want: []*metal.Network{n1},
		},
		{
			name: "by id 2",
			rq:   &apiv2.NetworkQuery{Id: &n2.ID},
			want: []*metal.Network{n2},
		},
		{
			name: "by name",
			rq:   &apiv2.NetworkQuery{Name: &n1.Name},
			want: []*metal.Network{n1},
		},
		{
			name: "by description",
			rq:   &apiv2.NetworkQuery{Description: &n1.Description},
			want: []*metal.Network{n1},
		},
		{
			name: "by label",
			rq:   &apiv2.NetworkQuery{Labels: &apiv2.Labels{Labels: map[string]string{"color": "red"}}},
			want: []*metal.Network{n1},
		},
		{
			name: "by label 2",
			rq:   &apiv2.NetworkQuery{Labels: &apiv2.Labels{Labels: map[string]string{"size": "medium"}}},
			want: []*metal.Network{n2},
		},
		{
			name: "by project",
			rq:   &apiv2.NetworkQuery{Project: &n1.ProjectID},
			want: []*metal.Network{n1},
		},
		{
			name: "by namespace",
			rq:   &apiv2.NetworkQuery{Namespace: n3.Namespace},
			want: []*metal.Network{n3},
		},
		{
			name: "by parent network",
			rq:   &apiv2.NetworkQuery{ParentNetwork: &n2.ParentNetwork},
			want: []*metal.Network{n2},
		},
		{
			name: "by partition",
			rq:   &apiv2.NetworkQuery{Partition: &n1.PartitionID},
			want: []*metal.Network{n1},
		},
		{
			name: "by vrf",
			rq:   &apiv2.NetworkQuery{Vrf: pointer.Pointer(uint32(n1.Vrf))},
			want: []*metal.Network{n1},
		},
		{
			name: "by nattype",
			rq:   &apiv2.NetworkQuery{NatType: apiv2.NATType_NAT_TYPE_IPV4_MASQUERADE.Enum()},
			want: []*metal.Network{n1},
		},
		{
			name: "by nattype 2",
			rq:   &apiv2.NetworkQuery{NatType: apiv2.NATType_NAT_TYPE_NONE.Enum()},
			want: []*metal.Network{n2, n3},
		},
		{
			name: "by networktype",
			rq:   &apiv2.NetworkQuery{Type: apiv2.NetworkType_NETWORK_TYPE_CHILD.Enum()},
			want: []*metal.Network{n2},
		},
		{
			name: "by wrong networktype",
			rq:   &apiv2.NetworkQuery{Type: apiv2.NetworkType_NETWORK_TYPE_UNDERLAY.Enum()},
			want: nil,
		},
		{
			name: "by addressfamily",
			rq:   &apiv2.NetworkQuery{AddressFamily: apiv2.NetworkAddressFamily_NETWORK_ADDRESS_FAMILY_V4.Enum()},
			want: []*metal.Network{n1, n3},
		},
		{
			name: "by addressfamily 2",
			rq:   &apiv2.NetworkQuery{AddressFamily: apiv2.NetworkAddressFamily_NETWORK_ADDRESS_FAMILY_V6.Enum()},
			want: []*metal.Network{n2, n3},
		},
		{
			name: "by addressfamily 3, with no result",
			rq: &apiv2.NetworkQuery{
				AddressFamily: apiv2.NetworkAddressFamily_NETWORK_ADDRESS_FAMILY_V6.Enum(),
				Id:            pointer.Pointer("n1"),
			},
			want: nil,
		},
		{
			name: "by addressfamily 4 (dual stack)",
			rq:   &apiv2.NetworkQuery{AddressFamily: apiv2.NetworkAddressFamily_NETWORK_ADDRESS_FAMILY_DUAL_STACK.Enum()},
			want: []*metal.Network{n3},
		},
		{
			name: "by prefixes",
			rq:   &apiv2.NetworkQuery{Prefixes: []string{n3.Prefixes[1].String()}},
			want: []*metal.Network{n3},
		},
		{
			name: "by two prefixes",
			rq:   &apiv2.NetworkQuery{Prefixes: []string{n3.Prefixes[0].String(), n3.Prefixes[1].String()}},
			want: []*metal.Network{n3},
		},
		{
			name: "by prefixes empty result",
			rq:   &apiv2.NetworkQuery{Prefixes: []string{n3.Prefixes[0].String(), n3.Prefixes[1].String(), "1.2.3.4/32"}},
			want: nil,
		},
		{
			name: "by prefixes in different networks",
			rq:   &apiv2.NetworkQuery{Prefixes: []string{"2001:db8::/96"}},
			want: []*metal.Network{n2, n3},
		},
		{
			name: "by destination prefixes",
			rq:   &apiv2.NetworkQuery{DestinationPrefixes: []string{n3.DestinationPrefixes[1].String()}},
			want: []*metal.Network{n1, n3},
		},
		{
			name: "by two destination prefixes",
			rq:   &apiv2.NetworkQuery{DestinationPrefixes: []string{n3.DestinationPrefixes[0].String(), n3.DestinationPrefixes[1].String()}},
			want: []*metal.Network{n3},
		},
		{
			name: "by destination prefixes empty result",
			rq:   &apiv2.NetworkQuery{DestinationPrefixes: []string{n3.DestinationPrefixes[0].String(), n3.DestinationPrefixes[1].String(), "1.2.3.4/32"}},
			want: nil,
		},
		{
			name: "by destination prefixes in different networks",
			rq:   &apiv2.NetworkQuery{DestinationPrefixes: []string{"::/0"}},
			want: []*metal.Network{n2, n3},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ds.Network().List(ctx, queries.NetworkFilter(tt.rq))
			require.NoError(t, err)

			if diff := cmp.Diff(
				tt.want, got,
				cmpopts.IgnoreFields(
					metal.Network{}, "Created", "Changed",
				),
			); diff != "" {
				t.Errorf("networkServiceServer.List() = %v, want %vņdiff: %s", got, tt.want, diff)
			}

		})
	}
}
