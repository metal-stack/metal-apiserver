package q_test

import (
	"fmt"
	"log/slog"
	"os"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/api/go/tag"
	"github.com/metal-stack/metal-apiserver/pkg/db/generic/pg"
	"github.com/metal-stack/metal-apiserver/pkg/db/generic/pg/q"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-apiserver/pkg/test"
	"github.com/stretchr/testify/require"
)

var (
	pgIPProjectID = "project-1"
	pgIPNamespace = "namespace-1"

	pgIP1 = &metal.IP{
		IPAddress:        "10.0.0.1",
		AllocationUUID:   "uuid-1",
		Namespace:        nil,
		ParentPrefixCidr: "10.0.0.0/8",
		Name:             "ip-one",
		Description:      "IP 1",
		ProjectID:        pgIPProjectID,
		NetworkID:        "n1",
		Type:             metal.Ephemeral,
		Tags:             []string{"color=red", "size=small", fmt.Sprintf("%s=m1", tag.MachineID)},
	}
	pgIP2 = &metal.IP{
		IPAddress:        "10.0.0.2",
		AllocationUUID:   "uuid-2",
		Namespace:        nil,
		ParentPrefixCidr: "10.0.0.0/8",
		Name:             "ip-two",
		Description:      "IP 2",
		ProjectID:        pgIPProjectID,
		NetworkID:        "n2",
		Type:             metal.Static,
		Tags:             []string{"color=green", "size=medium"},
	}
	pgIP3 = &metal.IP{
		IPAddress:        "10.0.0.3",
		AllocationUUID:   "uuid-3",
		Namespace:        &pgIPNamespace,
		ParentPrefixCidr: "10.0.1.0/24",
		Name:             "ip-three",
		Description:      "IP 3",
		ProjectID:        pgIPProjectID,
		NetworkID:        "n3",
		Type:             metal.Ephemeral,
		Tags:             []string{"color=blue"},
	}
	pgIP4 = &metal.IP{
		IPAddress:        "10.0.0.4",
		AllocationUUID:   "uuid-4",
		Namespace:        nil,
		ParentPrefixCidr: "10.0.0.0/8",
		Name:             "ip-four",
		Description:      "IP 4",
		ProjectID:        "project-2",
		NetworkID:        "n1",
		Type:             metal.Static,
		Tags:             []string{"color=yellow"},
	}
	pgIP5 = &metal.IP{
		IPAddress:        "fd12:34:56:78::1",
		AllocationUUID:   "uuid-5",
		Namespace:        nil,
		ParentPrefixCidr: "fd12:34:56:78::/64",
		Name:             "ip-v6",
		Description:      "IPv6 IP",
		ProjectID:        pgIPProjectID,
		NetworkID:        "n4",
		Type:             metal.Ephemeral,
		Tags:             []string{},
	}
	pgIPs = []*metal.IP{pgIP1, pgIP2, pgIP3, pgIP4, pgIP5}
)

// TestIPFilterPostgres mirrors the RethinkDB-backed TestIPFilter from
// pkg/db/queries, but runs against the Postgres backend using q.IpFilter.
func TestIPFilterPostgres(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	db, closer := test.StartPostgres(t, log)
	defer closer()

	repo, err := pg.NewGenericRepository[*metal.IP](log, db)
	require.NoError(t, err)

	ctx := t.Context()

	for _, ip := range pgIPs {
		require.NoError(t, repo.Create(ctx, ip.GetID(), ip))
	}

	tests := []struct {
		name string
		rq   *apiv2.IPQuery
		want []*metal.IP
	}{
		{name: "empty request returns unfiltered", rq: nil, want: pgIPs},
		{name: "by ip", rq: &apiv2.IPQuery{Ip: &pgIP1.IPAddress}, want: []*metal.IP{pgIP1}},
		{name: "by ip 2", rq: &apiv2.IPQuery{Ip: &pgIP2.IPAddress}, want: []*metal.IP{pgIP2}},
		{name: "by uuid", rq: &apiv2.IPQuery{Uuid: &pgIP1.AllocationUUID}, want: []*metal.IP{pgIP1}},
		{name: "by name", rq: &apiv2.IPQuery{Name: &pgIP1.Name}, want: []*metal.IP{pgIP1}},
		{name: "by name 2", rq: &apiv2.IPQuery{Name: &pgIP5.Name}, want: []*metal.IP{pgIP5}},
		{name: "by project", rq: &apiv2.IPQuery{Project: &pgIP1.ProjectID}, want: []*metal.IP{pgIP1, pgIP2, pgIP3, pgIP5}},
		{name: "by project different", rq: &apiv2.IPQuery{Project: &pgIP4.ProjectID}, want: []*metal.IP{pgIP4}},
		{name: "by namespace", rq: &apiv2.IPQuery{Namespace: pgIP3.Namespace}, want: []*metal.IP{pgIP3}},
		{name: "by network", rq: &apiv2.IPQuery{Network: &pgIP1.NetworkID}, want: []*metal.IP{pgIP1, pgIP4}},
		{name: "by network 2", rq: &apiv2.IPQuery{Network: &pgIP3.NetworkID}, want: []*metal.IP{pgIP3}},
		{name: "by parent prefix cidr", rq: &apiv2.IPQuery{ParentPrefixCidr: &pgIP1.ParentPrefixCidr}, want: []*metal.IP{pgIP1, pgIP2, pgIP4}},
		{name: "by parent prefix cidr 2", rq: &apiv2.IPQuery{ParentPrefixCidr: &pgIP3.ParentPrefixCidr}, want: []*metal.IP{pgIP3}},
		{name: "by label", rq: &apiv2.IPQuery{Labels: &apiv2.Labels{Labels: map[string]string{"color": "red"}}}, want: []*metal.IP{pgIP1}},
		{name: "by label 2", rq: &apiv2.IPQuery{Labels: &apiv2.Labels{Labels: map[string]string{"size": "small"}}}, want: []*metal.IP{pgIP1}},
		{name: "by label 3", rq: &apiv2.IPQuery{Labels: &apiv2.Labels{Labels: map[string]string{"color": "green"}}}, want: []*metal.IP{pgIP2}},
		{name: "by machine", rq: &apiv2.IPQuery{Machine: new("m1")}, want: []*metal.IP{pgIP1}},
		{name: "by machine empty result", rq: &apiv2.IPQuery{Machine: new("nonexistent-machine")}, want: nil},
		{name: "by type ephemeral", rq: &apiv2.IPQuery{Type: apiv2.IPType_IP_TYPE_EPHEMERAL.Enum()}, want: []*metal.IP{pgIP1, pgIP3, pgIP5}},
		{name: "by type static", rq: &apiv2.IPQuery{Type: apiv2.IPType_IP_TYPE_STATIC.Enum()}, want: []*metal.IP{pgIP2, pgIP4}},
		{name: "by address family v4", rq: &apiv2.IPQuery{AddressFamily: apiv2.IPAddressFamily_IP_ADDRESS_FAMILY_V4.Enum()}, want: []*metal.IP{pgIP1, pgIP2, pgIP3, pgIP4}},
		{name: "by address family v6", rq: &apiv2.IPQuery{AddressFamily: apiv2.IPAddressFamily_IP_ADDRESS_FAMILY_V6.Enum()}, want: []*metal.IP{pgIP5}},
		{name: "combined by project and type", rq: &apiv2.IPQuery{Project: &pgIP1.ProjectID, Type: apiv2.IPType_IP_TYPE_STATIC.Enum()}, want: []*metal.IP{pgIP2}},
		{name: "combined by ip and project", rq: &apiv2.IPQuery{Ip: &pgIP1.IPAddress, Project: &pgIP1.ProjectID}, want: []*metal.IP{pgIP1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := queryData(t, ctx, repo, q.IpFilter(tt.rq))
			if diff := cmp.Diff(
				tt.want, got,
				cmpopts.IgnoreFields(metal.IP{}, "Created", "Changed"),
				cmpopts.SortSlices(func(a, b *metal.IP) bool { return a.IPAddress < b.IPAddress }),
			); diff != "" {
				t.Errorf("IpFilter() = %v, want %v diff: %s", got, tt.want, diff)
			}
		})
	}
}

// TestIpProjectScopedPostgres mirrors the RethinkDB-backed TestIpProjectScoped
// from pkg/db/queries, but runs against the Postgres backend using
// q.IpProjectScoped.
func TestIpProjectScopedPostgres(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	db, closer := test.StartPostgres(t, log)
	defer closer()

	repo, err := pg.NewGenericRepository[*metal.IP](log, db)
	require.NoError(t, err)

	ctx := t.Context()

	for _, ip := range pgIPs {
		require.NoError(t, repo.Create(ctx, ip.GetID(), ip))
	}

	tests := []struct {
		name     string
		project  string
		expected []string
	}{
		{name: "project-1 returns matching ips", project: "project-1", expected: []string{pgIP1.IPAddress, pgIP2.IPAddress, pgIP3.IPAddress, pgIP5.IPAddress}},
		{name: "project-2 returns matching ips", project: "project-2", expected: []string{pgIP4.IPAddress}},
		{name: "nonexistent project returns empty", project: "project-nonexistent", expected: []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := queryData(t, ctx, repo, q.IpProjectScoped(tt.project))

			var gotIDs []string
			for _, ip := range got {
				gotIDs = append(gotIDs, ip.IPAddress)
			}

			require.ElementsMatch(t, tt.expected, gotIDs)
		})
	}
}
