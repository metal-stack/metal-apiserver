package pg_test

import (
	"log/slog"
	"strconv"
	"testing"

	"uuid"

	_ "github.com/lib/pq"
	"github.com/metal-stack/metal-apiserver/pkg/db/generic"
	"github.com/metal-stack/metal-apiserver/pkg/db/generic/pg"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-apiserver/pkg/test"
	"github.com/stretchr/testify/require"
	r "gopkg.in/rethinkdb/rethinkdb-go.v6"
)

// ipTypeFilter builds a rethinkdb query that filters the ip table on its type
// field, mirroring the postgres query on the same entity.
func ipTypeFilter(t metal.IPType) generic.EntityQuery {
	return func(q r.Term) r.Term {
		return q.Filter(func(row r.Term) r.Term {
			return row.Field("type").Eq(string(t))
		})
	}
}

// BenchmarkPgVsRethink compares query performance of postgres and rethinkdb
// while storing the exact same entities (metal.IP) in both databases and
// executing the same logical queries against each.
func BenchmarkPgVsRethink(b *testing.B) {
	ctx := b.Context()
	log := slog.Default()

	pgDB, pgCloser := test.StartPostgres(b, log)
	defer pgCloser()

	pgRepo, err := pg.NewGenericRepository[*metal.IP](log, pgDB)
	require.NoError(b, err)

	ds, _, rethinkCloser := test.StartRethink(b, log)
	defer rethinkCloser()
	rethinkRepo := ds.IP()

	const numIPs = 1000

	// Seed the same entities into both databases.
	for i := range numIPs {
		id := uuid.NewV7()
		valStr := strconv.Itoa(i)

		ip := &metal.IP{
			IPAddress:        id.String(),
			AllocationUUID:   "alloc-" + valStr,
			Name:             "ip-" + valStr,
			ProjectID:        "project",
			NetworkID:        "network",
			Type:             metal.Ephemeral,
			Tags:             []string{"benchmark"},
			ParentPrefixCidr: "10.0.0.0/16",
		}

		require.NoError(b, pgRepo.Create(ctx, id, ip))
		_, err := rethinkRepo.Create(ctx, ip)
		require.NoError(b, err)
	}

	b.ResetTimer()

	b.Run("Point_Get_By_ID_Postgres", func(b *testing.B) {
		target := uuid.NewV7()
		_ = pgRepo.Create(ctx, target, &metal.IP{IPAddress: target.String(), Type: metal.Ephemeral})
		b.ResetTimer()
		for b.Loop() {
			_, err := pgRepo.Get(ctx, target)
			require.NoError(b, err)
		}
	})

	b.Run("Point_Get_By_ID_Rethink", func(b *testing.B) {
		target := &metal.IP{IPAddress: uuid.NewV7().String(), Type: metal.Ephemeral}
		_, _ = rethinkRepo.Create(ctx, target)
		b.ResetTimer()
		for b.Loop() {
			_, err := rethinkRepo.Get(ctx, target.IPAddress)
			require.NoError(b, err)
		}
	})

	b.Run("Filtered_Query_Postgres", func(b *testing.B) {
		for b.Loop() {
			_, err := pgRepo.Query(ctx, []pg.QueryFilter{
				{Path: "Type", Op: "=", Value: string(metal.Ephemeral)},
			}, nil)
			require.NoError(b, err)
		}
	})

	b.Run("Filtered_Query_Rethink", func(b *testing.B) {
		for b.Loop() {
			_, err := rethinkRepo.List(ctx, ipTypeFilter(metal.Ephemeral))
			require.NoError(b, err)
		}
	})
}
