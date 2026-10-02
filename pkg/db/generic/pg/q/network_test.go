package q_test

import (
	"log/slog"
	"os"
	"slices"
	"strings"
	"testing"

	"uuid"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	_ "github.com/lib/pq"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/generic/pg"
	"github.com/metal-stack/metal-apiserver/pkg/db/generic/pg/q"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-apiserver/pkg/test"
	"github.com/stretchr/testify/require"
)

var (
	pgN1 = &metal.Network{
		ID: "6b22ccd6-4c93-4a1f-8c8f-2f63b3c6e011", Name: "n1", Description: "Network 1",
		Prefixes:                   metal.Prefixes{{IP: "10.0.0.0", Length: "8"}},
		DestinationPrefixes:        metal.Prefixes{{IP: "0.0.0.0", Length: "0"}},
		DefaultChildPrefixLength:   metal.ChildPrefixLength{metal.AddressFamilyIPv4: 22},
		MinChildPrefixLength:       metal.ChildPrefixLength{metal.AddressFamilyIPv4: 14},
		PartitionID:                "partition-1",
		ProjectID:                  "p1",
		ParentNetworkID:            "parent-network",
		Vrf:                        uint(42),
		PrivateSuper:               true, // nolint:staticcheck
		Nat:                        true, // nolint:staticcheck
		Underlay:                   true, // nolint:staticcheck
		Shared:                     true, // nolint:staticcheck
		Labels:                     map[string]string{"color": "red", "size": "small"},
		AdditionalAnnouncableCIDRs: []string{"10.240.0.0/12"},
		NetworkType:                metal.NetworkTypeExternal,
		NATType:                    metal.NATTypeIPv4Masquerade,
	}
	pgN2 = &metal.Network{
		ID: "6b22ccd6-4c93-4a1f-8c8f-2f63b3c6e012", Name: "n2", Description: "Network 2",
		Prefixes:                   metal.Prefixes{{IP: "2001:db8::", Length: "96"}},
		DestinationPrefixes:        metal.Prefixes{{IP: "::", Length: "0"}},
		DefaultChildPrefixLength:   metal.ChildPrefixLength{metal.AddressFamilyIPv6: 64},
		MinChildPrefixLength:       metal.ChildPrefixLength{metal.AddressFamilyIPv6: 56},
		PartitionID:                "partition-2",
		ProjectID:                  "p2",
		ParentNetworkID:            "parent-network-2",
		Vrf:                        uint(43),
		PrivateSuper:               false, // nolint:staticcheck
		Nat:                        false, // nolint:staticcheck
		Underlay:                   false, // nolint:staticcheck
		Shared:                     false, // nolint:staticcheck
		Labels:                     map[string]string{"color": "green", "size": "medium"},
		AdditionalAnnouncableCIDRs: []string{"10.241.0.0/12"},
		NetworkType:                metal.NetworkTypeChild,
		NATType:                    metal.NATTypeNone,
	}
	pgN3 = &metal.Network{
		ID: "6b22ccd6-4c93-4a1f-8c8f-2f63b3c6e013", Name: "n3", Description: "Network 3",
		Prefixes:                   metal.Prefixes{{IP: "2001:db8::", Length: "96"}, {IP: "13.0.0.0", Length: "8"}},
		DestinationPrefixes:        metal.Prefixes{{IP: "::", Length: "0"}, {IP: "0.0.0.0", Length: "0"}},
		DefaultChildPrefixLength:   metal.ChildPrefixLength{metal.AddressFamilyIPv6: 64, metal.AddressFamilyIPv4: 22},
		MinChildPrefixLength:       metal.ChildPrefixLength{metal.AddressFamilyIPv6: 56, metal.AddressFamilyIPv4: 14},
		PartitionID:                "partition-3",
		ProjectID:                  "p3",
		Namespace:                  new("p3"),
		ParentNetworkID:            "parent-network-3",
		Vrf:                        uint(44),
		PrivateSuper:               false, // nolint:staticcheck
		Nat:                        false, // nolint:staticcheck
		Underlay:                   false, // nolint:staticcheck
		Shared:                     false, // nolint:staticcheck
		Labels:                     map[string]string{"color": "blue", "size": "large"},
		AdditionalAnnouncableCIDRs: []string{"10.241.0.0/12"},
		NetworkType:                metal.NetworkTypeExternal,
		NATType:                    metal.NATTypeNone,
	}
	pgNetworks = []*metal.Network{pgN1, pgN2, pgN3}
)

// TestNetworkFilterPostgres mirrors the RethinkDB-backed TestNetworkFilter from
// pkg/db/queries/network_test.go, but runs against the Postgres backend using
// the pg.GenericRepository and q.NetworkFilter.
func TestNetworkFilterPostgres(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	db, closer := test.StartPostgres(t, log)
	defer closer()

	repo, err := pg.NewGenericRepository[*metal.Network](log, db)
	require.NoError(t, err)

	ctx := t.Context()

	for _, n := range pgNetworks {
		id, err := uuid.Parse(n.ID)
		require.NoError(t, err)
		require.NoError(t, repo.Create(ctx, id, n))
	}

	// helper returns the data of all entities matching the given network query
	list := func(rq *apiv2.NetworkQuery) []*metal.Network {
		t.Helper()
		ents, err := repo.Query(ctx, q.NetworkFilter(rq), nil)
		require.NoError(t, err)
		got := make([]*metal.Network, 0, len(ents))
		for _, e := range ents {
			got = append(got, e.Data)
		}
		if len(got) == 0 {
			return nil
		}
		return got
	}

	tests := []struct {
		name string
		rq   *apiv2.NetworkQuery
		want []*metal.Network
	}{
		{
			name: "empty request returns unfiltered",
			rq:   nil,
			want: []*metal.Network{pgN1, pgN2, pgN3},
		},
		{
			name: "by id",
			rq:   &apiv2.NetworkQuery{Id: &pgN1.ID},
			want: []*metal.Network{pgN1},
		},
		{
			name: "by id 2",
			rq:   &apiv2.NetworkQuery{Id: &pgN2.ID},
			want: []*metal.Network{pgN2},
		},
		{
			name: "by name",
			rq:   &apiv2.NetworkQuery{Name: &pgN1.Name},
			want: []*metal.Network{pgN1},
		},
		{
			name: "by description",
			rq:   &apiv2.NetworkQuery{Description: &pgN1.Description},
			want: []*metal.Network{pgN1},
		},
		{
			name: "by label",
			rq:   &apiv2.NetworkQuery{Labels: &apiv2.Labels{Labels: map[string]string{"color": "red"}}},
			want: []*metal.Network{pgN1},
		},
		{
			name: "by label 2",
			rq:   &apiv2.NetworkQuery{Labels: &apiv2.Labels{Labels: map[string]string{"size": "medium"}}},
			want: []*metal.Network{pgN2},
		},
		{
			name: "by project",
			rq:   &apiv2.NetworkQuery{Project: &pgN1.ProjectID},
			want: []*metal.Network{pgN1},
		},
		{
			name: "by namespace",
			rq:   &apiv2.NetworkQuery{Namespace: pgN3.Namespace},
			want: []*metal.Network{pgN3},
		},
		{
			name: "by parent network",
			rq:   &apiv2.NetworkQuery{ParentNetwork: &pgN2.ParentNetworkID},
			want: []*metal.Network{pgN2},
		},
		{
			name: "by partition",
			rq:   &apiv2.NetworkQuery{Partition: &pgN1.PartitionID},
			want: []*metal.Network{pgN1},
		},
		{
			name: "by vrf",
			rq:   &apiv2.NetworkQuery{Vrf: new(uint32(pgN1.Vrf))},
			want: []*metal.Network{pgN1},
		},
		{
			name: "by nattype",
			rq:   &apiv2.NetworkQuery{NatType: apiv2.NATType_NAT_TYPE_IPV4_MASQUERADE.Enum()},
			want: []*metal.Network{pgN1},
		},
		{
			name: "by nattype 2",
			rq:   &apiv2.NetworkQuery{NatType: apiv2.NATType_NAT_TYPE_NONE.Enum()},
			want: []*metal.Network{pgN2, pgN3},
		},
		{
			name: "by networktype",
			rq:   &apiv2.NetworkQuery{Type: apiv2.NetworkType_NETWORK_TYPE_CHILD.Enum()},
			want: []*metal.Network{pgN2},
		},
		{
			name: "by wrong networktype",
			rq:   &apiv2.NetworkQuery{Type: apiv2.NetworkType_NETWORK_TYPE_UNDERLAY.Enum()},
			want: nil,
		},
		{
			name: "by addressfamily",
			rq:   &apiv2.NetworkQuery{AddressFamily: apiv2.NetworkAddressFamily_NETWORK_ADDRESS_FAMILY_V4.Enum()},
			want: []*metal.Network{pgN1, pgN3},
		},
		{
			name: "by addressfamily 2",
			rq:   &apiv2.NetworkQuery{AddressFamily: apiv2.NetworkAddressFamily_NETWORK_ADDRESS_FAMILY_V6.Enum()},
			want: []*metal.Network{pgN2, pgN3},
		},
		{
			name: "by addressfamily 3, with no result",
			rq: &apiv2.NetworkQuery{
				AddressFamily: apiv2.NetworkAddressFamily_NETWORK_ADDRESS_FAMILY_V6.Enum(),
				Id:            new("n1"),
			},
			want: nil,
		},
		{
			name: "by addressfamily 4 (dual stack)",
			rq:   &apiv2.NetworkQuery{AddressFamily: apiv2.NetworkAddressFamily_NETWORK_ADDRESS_FAMILY_DUAL_STACK.Enum()},
			want: []*metal.Network{pgN3},
		},
		{
			name: "by prefixes",
			rq:   &apiv2.NetworkQuery{Prefixes: []string{pgN3.Prefixes[1].String()}},
			want: []*metal.Network{pgN3},
		},
		{
			name: "by two prefixes",
			rq:   &apiv2.NetworkQuery{Prefixes: []string{pgN3.Prefixes[0].String(), pgN3.Prefixes[1].String()}},
			want: []*metal.Network{pgN3},
		},
		{
			name: "by prefixes empty result",
			rq:   &apiv2.NetworkQuery{Prefixes: []string{pgN3.Prefixes[0].String(), pgN3.Prefixes[1].String(), "1.2.3.4/32"}},
			want: nil,
		},
		{
			name: "by prefixes in different networks",
			rq:   &apiv2.NetworkQuery{Prefixes: []string{"2001:db8::/96"}},
			want: []*metal.Network{pgN2, pgN3},
		},
		{
			name: "by destination prefixes",
			rq:   &apiv2.NetworkQuery{DestinationPrefixes: []string{pgN3.DestinationPrefixes[1].String()}},
			want: []*metal.Network{pgN1, pgN3},
		},
		{
			name: "by two destination prefixes",
			rq:   &apiv2.NetworkQuery{DestinationPrefixes: []string{pgN3.DestinationPrefixes[0].String(), pgN3.DestinationPrefixes[1].String()}},
			want: []*metal.Network{pgN3},
		},
		{
			name: "by destination prefixes empty result",
			rq:   &apiv2.NetworkQuery{DestinationPrefixes: []string{pgN3.DestinationPrefixes[0].String(), pgN3.DestinationPrefixes[1].String(), "1.2.3.4/32"}},
			want: nil,
		},
		{
			name: "by destination prefixes in different networks",
			rq:   &apiv2.NetworkQuery{DestinationPrefixes: []string{"::/0"}},
			want: []*metal.Network{pgN2, pgN3},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := list(tt.rq)

			slices.SortFunc(got, func(a, b *metal.Network) int {
				return strings.Compare(a.ID, b.ID)
			})

			if diff := cmp.Diff(
				tt.want, got,
				cmpopts.IgnoreFields(
					metal.Network{}, "Created", "Changed",
				),
			); diff != "" {
				t.Errorf("network list = %v, want %v diff: %s", got, tt.want, diff)
			}
		})
	}
}

func TestNetworkFilterNilQuery(t *testing.T) {
	require.Nil(t, q.NetworkFilter(nil))
}
