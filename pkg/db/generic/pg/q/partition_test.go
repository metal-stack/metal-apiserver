package q_test

import (
	"log/slog"
	"os"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/generic/pg"
	"github.com/metal-stack/metal-apiserver/pkg/db/generic/pg/q"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-apiserver/pkg/test"
	"github.com/stretchr/testify/require"
)

var (
	pgPartition1 = &metal.Partition{
		ID: "part1", Name: "partition 1", Description: "Partition 1",
		MgmtServiceAddress: "mgmt1.example.com",
		Labels:             map[string]string{"region": "eu"},
		DNSServers:         metal.DNSServers{},
		NTPServers:         metal.NTPServers{},
	}
	pgPartition2 = &metal.Partition{
		ID: "part2", Name: "partition 2", Description: "Partition 2",
		MgmtServiceAddress: "mgmt2.example.com",
		Labels:             map[string]string{"region": "us"},
		DNSServers:         metal.DNSServers{},
		NTPServers:         metal.NTPServers{},
	}
	pgPartition3 = &metal.Partition{
		ID: "part3", Name: "partition 3", Description: "Partition 3",
		MgmtServiceAddress: "mgmt3.example.com",
		Labels:             map[string]string{"region": "eu", "gpu": "true"},
		DNSServers:         metal.DNSServers{},
		NTPServers:         metal.NTPServers{},
	}
	pgPartitions = []*metal.Partition{pgPartition1, pgPartition2, pgPartition3}
)

// TestPartitionFilterPostgres mirrors the RethinkDB-backed TestPartitionFilter
// from pkg/db/queries, but runs against the Postgres backend using
// q.PartitionFilter.
func TestPartitionFilterPostgres(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	db, closer := test.StartPostgres(t, log)
	defer closer()

	repo, err := pg.NewGenericRepository[*metal.Partition](log, db)
	require.NoError(t, err)

	ctx := t.Context()

	for _, part := range pgPartitions {
		require.NoError(t, repo.Create(ctx, part.GetID(), part))
	}

	tests := []struct {
		name string
		rq   *apiv2.PartitionQuery
		want []*metal.Partition
	}{
		{name: "nil request returns unfiltered", rq: nil, want: pgPartitions},
		{name: "empty request returns unfiltered", rq: &apiv2.PartitionQuery{}, want: pgPartitions},
		{name: "by id", rq: &apiv2.PartitionQuery{Id: new("part1")}, want: []*metal.Partition{pgPartition1}},
		{name: "by id 2", rq: &apiv2.PartitionQuery{Id: new("part3")}, want: []*metal.Partition{pgPartition3}},
		{name: "no result by id", rq: &apiv2.PartitionQuery{Id: new("nonexistent")}, want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := queryData(t, ctx, repo, q.PartitionFilter(tt.rq))
			if diff := cmp.Diff(
				tt.want, got,
				cmpopts.SortSlices(func(a, b *metal.Partition) bool { return a.ID < b.ID }),
				cmpopts.IgnoreFields(metal.Partition{}, "Created", "Changed", "Generation"),
			); diff != "" {
				t.Errorf("PartitionFilter() = %v, want %v diff: %s", got, tt.want, diff)
			}
		})
	}
}
