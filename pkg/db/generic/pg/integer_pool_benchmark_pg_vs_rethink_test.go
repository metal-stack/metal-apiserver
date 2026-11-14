package pg_test

import (
	"log/slog"
	"testing"

	_ "github.com/lib/pq"
	"github.com/metal-stack/metal-apiserver/pkg/db/generic/pg"
	"github.com/metal-stack/metal-apiserver/pkg/test"
	"github.com/stretchr/testify/require"
)

// BenchmarkIntegerPoolPgVsRethink compares the throughput of the Postgres-based
// integer pool (pg.IntegerPool) with the legacy RethinkDB-based pool
// (generic.VrfPool). Both pools are seeded with the same range and execute the
// same logical operations: acquire an integer and release it again.
func BenchmarkIntegerPoolPgVsRethink(b *testing.B) {
	ctx := b.Context()
	log := slog.Default()

	const poolSize = 50000

	pgDB, pgCloser := test.StartPostgres(b, log)
	defer pgCloser()

	pgPool, err := pg.NewIntegerPool(log, pgDB)
	require.NoError(b, err)
	require.NoError(b, pgPool.Seed(ctx, pg.PoolTypeVRF, uint32(1), uint32(poolSize)))

	ds, _, rethinkCloser := test.StartRethinkWithPoolRanges(b, log, 1, poolSize, 1, poolSize)
	defer rethinkCloser()
	rethinkPool := ds.VrfPool()

	b.ResetTimer()

	b.Run("AcquireAndRelease_Sequential_Postgres", func(b *testing.B) {
		for b.Loop() {
			id, err := pgPool.Acquire(ctx, pg.PoolTypeVRF)
			require.NoError(b, err)
			require.NoError(b, pgPool.Release(ctx, pg.PoolTypeVRF, id))
		}
	})

	b.Run("AcquireAndRelease_Sequential_Rethink", func(b *testing.B) {
		for b.Loop() {
			id, err := rethinkPool.AcquireRandomUniqueInteger(ctx)
			require.NoError(b, err)
			require.NoError(b, rethinkPool.ReleaseUniqueInteger(ctx, id))
		}
	})

	b.Run("AcquireAndRelease_Parallel_Postgres", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				id, err := pgPool.Acquire(ctx, pg.PoolTypeVRF)
				require.NoError(b, err)
				require.NoError(b, pgPool.Release(ctx, pg.PoolTypeVRF, id))
			}
		})
	})

	b.Run("AcquireAndRelease_Parallel_Rethink", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				id, err := rethinkPool.AcquireRandomUniqueInteger(ctx)
				require.NoError(b, err)
				require.NoError(b, rethinkPool.ReleaseUniqueInteger(ctx, id))
			}
		})
	})
}
