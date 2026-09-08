package pg_test

import (
	"database/sql"
	"log/slog"
	"runtime"
	"testing"
	"time"

	"github.com/metal-stack/metal-apiserver/pkg/db/generic/pg"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// setupBenchmarkIntegerPoolDB boots a PostgreSQL container and seeds a large pool.
func setupBenchmarkIntegerPoolDB(b *testing.B, poolSize int) (*sql.DB, func()) {
	b.Helper()
	ctx := b.Context()

	// Spin up Postgres in Docker
	pgContainer, err := postgres.Run(ctx,
		"postgres:18-alpine",
		postgres.WithDatabase("benchdb"),
		postgres.WithUsername("benchuser"),
		postgres.WithPassword("benchpass"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(10*time.Second),
		),
	)
	require.NoError(b, err)

	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	require.NoError(b, err)

	db, err := sql.Open("postgres", connStr)
	require.NoError(b, err)

	// Tune connection pool for benchmark concurrency
	maxConns := runtime.GOMAXPROCS(0) * 4
	db.SetMaxOpenConns(maxConns)
	db.SetMaxIdleConns(maxConns)

	schema := `
		CREATE TABLE integer_pool (
			pool_type VARCHAR(64) NOT NULL,
			id INT NOT NULL,
			is_allocated BOOLEAN NOT NULL DEFAULT FALSE,
			allocated_at TIMESTAMPTZ,
			PRIMARY KEY (pool_type, id)
		);
		CREATE INDEX idx_integer_pool_type_free ON integer_pool (pool_type, id) WHERE is_allocated = FALSE;
	`
	_, err = db.ExecContext(ctx, schema)
	require.NoError(b, err)

	log := slog.Default()
	service := pg.NewIntegerPool(log, db)
	b.Logf("Seeding benchmark database with %d integers...", poolSize)
	err = service.Seed(ctx, pg.PoolTypeASN, 1, poolSize)
	require.NoError(b, err)

	cleanup := func() {
		_ = db.Close()
		_ = pgContainer.Terminate(ctx)
	}

	return db, cleanup
}

func BenchmarkIntegerPool_AcquireAndRelease(b *testing.B) {
	const hugePoolSize = 10000 // 10k Integer Pool
	db, cleanup := setupBenchmarkIntegerPoolDB(b, hugePoolSize)
	defer cleanup()

	log := slog.Default()
	service := pg.NewIntegerPool(log, db)
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
	service := pg.NewIntegerPool(log, db)
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
