package pg_test

import (
	"database/sql"
	"log/slog"
	"testing"

	"github.com/metal-stack/metal-apiserver/pkg/db/generic/pg"
	"github.com/metal-stack/metal-apiserver/pkg/test"
	"github.com/stretchr/testify/require"
)

// setupBenchmarkIntegerPoolDB boots a PostgreSQL container and seeds a large pool.
func setupBenchmarkIntegerPoolDB(b *testing.B, poolSize uint32) (*sql.DB, func()) {

	log := slog.Default()
	db, closer := test.StartPostgres(b, log)

	service, err := pg.NewIntegerPool(log, db)
	require.NoError(b, err)
	b.Logf("Seeding benchmark database with %d integers...", poolSize)
	err = service.Seed(b.Context(), pg.PoolTypeASN, 1, poolSize)
	require.NoError(b, err)

	return db, closer
}

func BenchmarkIntegerPool_AcquireAndRelease(b *testing.B) {
	const hugePoolSize = 10000 // 10k Integer Pool
	db, cleanup := setupBenchmarkIntegerPoolDB(b, hugePoolSize)
	defer cleanup()

	log := slog.Default()
	service, err := pg.NewIntegerPool(log, db)
	require.NoError(b, err)

	ctx := b.Context()

	// Reset timer to ignore container setup and seeding overhead
	b.ResetTimer()

	// Benchmark parallel Acquire -> Release workflow
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			// 1. Acquire next available integer
			id, err := service.Acquire(ctx, pg.PoolTypeASN)
			require.NoError(b, err)

			// 2. Immediately release it back into the pool
			err = service.Release(ctx, pg.PoolTypeASN, id)
			require.NoError(b, err)
		}
	})
}

func BenchmarkIntegerPool_HighContentionAcquire(b *testing.B) {
	const hugePoolSize = 50000
	db, cleanup := setupBenchmarkIntegerPoolDB(b, hugePoolSize)
	defer cleanup()

	log := slog.Default()
	service, err := pg.NewIntegerPool(log, db)
	require.NoError(b, err)
	ctx := b.Context()

	b.ResetTimer()

	// Runs b.N total Acquisitions spread across multiple worker goroutines
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, err := service.Acquire(ctx, pg.PoolTypeASN)
			require.NoError(b, err)
		}
	})
}
