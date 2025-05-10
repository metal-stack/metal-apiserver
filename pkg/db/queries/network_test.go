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
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	"github.com/metal-stack/metal-apiserver/pkg/test"
	"github.com/metal-stack/metal-lib/pkg/pointer"
)

var (
	n1 = &metal.Network{
		Base:                       metal.Base{ID: "n1", Name: "n1", Description: "Network 1"},
		Prefixes:                   metal.Prefixes{{IP: "10.0.0.0", Length: "8"}},
		DestinationPrefixes:        metal.Prefixes{{IP: "0.0.0.0", Length: "0"}},
		DefaultChildPrefixLength:   metal.ChildPrefixLength{metal.IPv4AddressFamily: 22},
		MinChildPrefixLength:       metal.ChildPrefixLength{metal.IPv4AddressFamily: 14},
		PartitionID:                "partition-1",
		ProjectID:                  "p1",
		ParentNetworkID:            "parent-network",
		Vrf:                        uint(42),
		PrivateSuper:               true,
		Nat:                        true,
		Underlay:                   true,
		Shared:                     true,
		Labels:                     map[string]string{"color": "red", "size": "small"},
		AdditionalAnnouncableCIDRs: []string{"10.240.0.0/12"},
		NetworkType:                pointer.Pointer(metal.ExternalNetworkType),
		NATType:                    pointer.Pointer(metal.IPv4MasqueradeNATType),
	}
	n2 = &metal.Network{
		Base:                       metal.Base{ID: "n2", Name: "n2", Description: "Network 2"},
		Prefixes:                   metal.Prefixes{{IP: "2001:db8", Length: "96"}},
		DestinationPrefixes:        metal.Prefixes{{IP: "0:0", Length: "0"}},
		DefaultChildPrefixLength:   metal.ChildPrefixLength{metal.IPv6AddressFamily: 64},
		MinChildPrefixLength:       metal.ChildPrefixLength{metal.IPv6AddressFamily: 56},
		PartitionID:                "partition-2",
		ProjectID:                  "p2",
		ParentNetworkID:            "parent-network-2",
		Vrf:                        uint(43),
		PrivateSuper:               false,
		Nat:                        false,
		Underlay:                   false,
		Shared:                     false,
		Labels:                     map[string]string{"color": "green", "size": "medium"},
		AdditionalAnnouncableCIDRs: []string{"10.241.0.0/12"},
		NetworkType:                pointer.Pointer(metal.ChildNetworkType),
		NATType:                    pointer.Pointer(metal.NoneNATType),
	}
	n3 = &metal.Network{
		Base:                       metal.Base{ID: "n3", Name: "n3", Description: "Network 3"},
		Prefixes:                   metal.Prefixes{{IP: "2001:db8", Length: "96"}, {IP: "13.0.0.0", Length: "8"}},
		DestinationPrefixes:        metal.Prefixes{{IP: "0:0", Length: "0"}, {IP: "0.0.0.0", Length: "0"}},
		DefaultChildPrefixLength:   metal.ChildPrefixLength{metal.IPv6AddressFamily: 64, metal.IPv4AddressFamily: 22},
		MinChildPrefixLength:       metal.ChildPrefixLength{metal.IPv6AddressFamily: 56, metal.IPv4AddressFamily: 14},
		PartitionID:                "partition-3",
		ProjectID:                  "p3",
		ParentNetworkID:            "parent-network-3",
		Vrf:                        uint(44),
		PrivateSuper:               false,
		Nat:                        false,
		Underlay:                   false,
		Shared:                     false,
		Labels:                     map[string]string{"color": "blue", "size": "large"},
		AdditionalAnnouncableCIDRs: []string{"10.241.0.0/12"},
		NetworkType:                pointer.Pointer(metal.ExternalNetworkType),
		NATType:                    pointer.Pointer(metal.NoneNATType),
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
		name    string
		rq      *apiv2.NetworkQuery
		want    []*metal.Network
		wantErr error
	}{
		{
			name:    "by id",
			rq:      &apiv2.NetworkQuery{Id: pointer.Pointer("n1")},
			want:    []*metal.Network{n1},
			wantErr: nil,
		},
		{
			name:    "by id 2",
			rq:      &apiv2.NetworkQuery{Id: pointer.Pointer("n2")},
			want:    []*metal.Network{n2},
			wantErr: nil,
		},
		{
			name:    "by label",
			rq:      &apiv2.NetworkQuery{Labels: &apiv2.Labels{Labels: map[string]string{"color": "red"}}},
			want:    []*metal.Network{n1},
			wantErr: nil,
		},
		{
			name:    "by label 2",
			rq:      &apiv2.NetworkQuery{Labels: &apiv2.Labels{Labels: map[string]string{"size": "medium"}}},
			want:    []*metal.Network{n2},
			wantErr: nil,
		},
		{
			name:    "by project",
			rq:      &apiv2.NetworkQuery{Project: pointer.Pointer("p1")},
			want:    []*metal.Network{n1},
			wantErr: nil,
		},
		{
			name:    "by parent network",
			rq:      &apiv2.NetworkQuery{ParentNetworkId: pointer.Pointer("parent-network-2")},
			want:    []*metal.Network{n2},
			wantErr: nil,
		},
		{
			name:    "by partition",
			rq:      &apiv2.NetworkQuery{Partition: pointer.Pointer("partition-1")},
			want:    []*metal.Network{n1},
			wantErr: nil,
		},
		{
			name:    "by vrf",
			rq:      &apiv2.NetworkQuery{Vrf: pointer.Pointer(uint32(42))},
			want:    []*metal.Network{n1},
			wantErr: nil,
		},
		{
			name:    "by nattype",
			rq:      &apiv2.NetworkQuery{NatType: apiv2.NATType_NAT_TYPE_IPV4_MASQUERADE.Enum()},
			want:    []*metal.Network{n1},
			wantErr: nil,
		},
		{
			name:    "by nattype 2",
			rq:      &apiv2.NetworkQuery{NatType: apiv2.NATType_NAT_TYPE_NONE.Enum()},
			want:    []*metal.Network{n2, n3},
			wantErr: nil,
		},
		{
			name:    "by networktype",
			rq:      &apiv2.NetworkQuery{Type: apiv2.NetworkType_NETWORK_TYPE_CHILD.Enum()},
			want:    []*metal.Network{n2},
			wantErr: nil,
		},
		{
			name:    "by wrong networktype",
			rq:      &apiv2.NetworkQuery{Type: apiv2.NetworkType_NETWORK_TYPE_UNDERLAY.Enum()},
			want:    nil,
			wantErr: nil,
		},
		{
			name:    "by addressfamily",
			rq:      &apiv2.NetworkQuery{AddressFamily: apiv2.IPAddressFamily_IP_ADDRESS_FAMILY_V4.Enum()},
			want:    []*metal.Network{n1, n3},
			wantErr: nil,
		},
		{
			name:    "by addressfamily 2",
			rq:      &apiv2.NetworkQuery{AddressFamily: apiv2.IPAddressFamily_IP_ADDRESS_FAMILY_V6.Enum()},
			want:    []*metal.Network{n2, n3},
			wantErr: nil,
		},
		{
			name: "by addressfamily 3, with no result",
			rq: &apiv2.NetworkQuery{
				AddressFamily: apiv2.IPAddressFamily_IP_ADDRESS_FAMILY_V6.Enum(),
				Id:            pointer.Pointer("n1"),
			},
			want:    nil,
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ds.Network().List(ctx, queries.NetworkFilter(tt.rq))

			if diff := cmp.Diff(err, tt.wantErr, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("diff = %s", diff)
			}

			if diff := cmp.Diff(
				tt.want, got,
				cmpopts.IgnoreFields(
					metal.Network{}, "Created", "Changed",
				),
			); diff != "" {
				t.Errorf("networkServiceServer.Create() = %v, want %vņdiff: %s", got, tt.want, diff)
			}

		})
	}
}
