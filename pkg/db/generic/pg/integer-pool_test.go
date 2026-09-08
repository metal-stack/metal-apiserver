package pg_test

import (
	"database/sql"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/metal-stack/metal-apiserver/pkg/db/generic/pg"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestIntegerPoolService(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := t.Context()
	log := slog.Default()
	service := pg.NewIntegerPool(log, db)

	t.Run("Seed and Pool Isolation", func(t *testing.T) {
		// Seed ASN (64512-64514) and VRFId (100-102)
		err := service.Seed(ctx, pg.PoolTypeASN, 64512, 64514)
		require.NoError(t, err)
		err = service.Seed(ctx, pg.PoolTypeVRF, 100, 102)
		require.NoError(t, err)

		// Acquire from ASN
		asn1, err := service.Acquire(ctx, pg.PoolTypeASN)
		require.NoError(t, err)
		require.Equal(t, 64512, asn1)

		// Acquire from VRFId (should not impact or be impacted by ASN)
		vrf1, err := service.Acquire(ctx, pg.PoolTypeVRF)
		require.NoError(t, err)
		require.Equal(t, 100, vrf1)

		// Acquire next ASN
		asn2, err := service.Acquire(ctx, pg.PoolTypeASN)
		require.NoError(t, err)
		require.Equal(t, 64513, asn2)

	})

	t.Run("Release and Reuse", func(t *testing.T) {
		// Release ASN 64512 back to pool
		err := service.Release(ctx, pg.PoolTypeASN, 64512)
		require.NoError(t, err)

		// Acquire should return 64512 because it is ordered ASC
		asn, err := service.Acquire(ctx, pg.PoolTypeASN)
		require.NoError(t, err)
		require.Equal(t, 64512, asn)

		// Releasing an unallocated or non-existent ID should error
		err = service.Release(ctx, pg.PoolTypeASN, 99999)
		require.EqualError(t, err, "integer 99999 in pool 'ASN' was either not found or already released")
	})

	t.Run("Pool Exhaustion", func(t *testing.T) {
		// Empty remaining items from VRFId pool
		_, _ = service.Acquire(ctx, pg.PoolTypeVRF) // 101
		_, _ = service.Acquire(ctx, pg.PoolTypeVRF) // 102

		// Next acquire should fail
		_, err := service.Acquire(ctx, pg.PoolTypeVRF)
		require.ErrorIs(t, err, pg.ErrPoolExhausted)
		require.EqualError(t, err, "pool exhausted: no integers available: pool 'VRF'")
	})

	t.Run("Concurrent Acquisition Safety", func(t *testing.T) {
		// Seed a fresh custom pool with 50 items
		const (
			poolSize               = 50
			customPool pg.PoolType = "CONCURRENT_TEST"
		)

		err := service.Seed(ctx, customPool, 1, poolSize)
		require.NoError(t, err)

		results := make(chan int, poolSize)
		var wg sync.WaitGroup

		// Spawn 50 concurrent workers attempting to acquire at the exact same time
		for range poolSize {
			wg.Go(func() {
				id, err := service.Acquire(ctx, customPool)
				if err == nil {
					results <- id
				}
			})
		}

		wg.Wait()
		close(results)

		// Assert that all 50 workers got unique IDs without collisions
		seen := make(map[int]bool)
		for id := range results {
			require.False(t, seen[id])
			seen[id] = true
		}

		require.Len(t, seen, poolSize)
	})
}

// setupTestDB provisions an isolated Postgres container running the schema.
func setupTestDB(t *testing.T) (*sql.DB, func()) {
	t.Helper()
	ctx := t.Context()

	pgContainer, err := postgres.Run(ctx,
		"postgres:18-alpine",
		postgres.WithDatabase("testdb"),
		postgres.WithUsername("testuser"),
		postgres.WithPassword("testpass"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(5*time.Second),
		),
	)
	if err != nil {
		t.Fatalf("failed to start postgres container: %s", err)
	}

	connectionString, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("failed to get connection string: %s", err)
	}

	db, err := sql.Open("postgres", connectionString)
	if err != nil {
		t.Fatalf("failed to connect to db: %s", err)
	}

	// Schema creation
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
	if _, err := db.ExecContext(ctx, schema); err != nil {
		t.Fatalf("failed to create schema: %s", err)
	}

	cleanup := func() {
		_ = db.Close()
		_ = pgContainer.Terminate(ctx)
	}

	return db, cleanup
}
