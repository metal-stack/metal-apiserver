package pg_test

import (
	"log/slog"
	"sync"
	"testing"

	"github.com/metal-stack/metal-apiserver/pkg/db/generic/pg"
	"github.com/metal-stack/metal-apiserver/pkg/test"
	"github.com/stretchr/testify/require"
)

func TestIntegerPoolService(t *testing.T) {
	ctx := t.Context()
	log := slog.Default()
	db, closer := test.StartPostgres(t, log)
	defer closer()
	service, err := pg.NewIntegerPool(log, db)
	require.NoError(t, err)

	t.Run("Seed and Pool Isolation", func(t *testing.T) {
		// Seed ASN (64512-64514) and VRFId (100-102)
		err := service.Seed(ctx, pg.PoolTypeASN, uint32(64512), uint32(64514))
		require.NoError(t, err)
		err = service.Seed(ctx, pg.PoolTypeVRF, uint32(100), uint32(102))
		require.NoError(t, err)

		// Acquire from ASN
		asn1, err := service.Acquire(ctx, pg.PoolTypeASN)
		require.NoError(t, err)
		require.Equal(t, uint32(64512), asn1)

		// Acquire from VRFId (should not impact or be impacted by ASN)
		vrf1, err := service.Acquire(ctx, pg.PoolTypeVRF)
		require.NoError(t, err)
		require.Equal(t, uint32(100), vrf1)

		// Acquire next ASN
		asn2, err := service.Acquire(ctx, pg.PoolTypeASN)
		require.NoError(t, err)
		require.Equal(t, uint32(64513), asn2)

	})

	t.Run("Release and Reuse", func(t *testing.T) {
		// Release ASN 64512 back to pool
		err := service.Release(ctx, pg.PoolTypeASN, uint32(64512))
		require.NoError(t, err)

		// Acquire should return 64512 because it is ordered ASC
		asn, err := service.Acquire(ctx, pg.PoolTypeASN)
		require.NoError(t, err)
		require.Equal(t, uint32(64512), asn)

		// Releasing an unallocated or non-existent ID should error
		err = service.Release(ctx, pg.PoolTypeASN, 99999)
		require.ErrorIs(t, err, pg.ErrIntegerNotFound)
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

	t.Run("AcquireUniqueInteger", func(t *testing.T) {
		// Seed a fresh pool for isolated testing
		err := service.Seed(ctx, pg.PoolTypeVRF, uint32(200), uint32(202))
		require.NoError(t, err)

		// Acquire a specific value
		id, err := service.AcquireUniqueInteger(ctx, pg.PoolTypeVRF, uint32(201))
		require.NoError(t, err)
		require.Equal(t, uint32(201), id)

		// Acquiring the same value again should fail
		_, err = service.AcquireUniqueInteger(ctx, pg.PoolTypeVRF, uint32(201))
		require.ErrorIs(t, err, pg.ErrIntegerAlreadyAcquired)
		require.EqualError(t, err, "integer is already acquired: 201 in pool 'VRF'")

		// A value outside the pool's configured range should fail
		_, err = service.AcquireUniqueInteger(ctx, pg.PoolTypeVRF, uint32(99999))
		require.EqualError(t, err, "value 99999 is outside of the allowed range 0 - 202 for pool 'VRF'")

		// Releasing the specific integer makes it acquirable again
		err = service.Release(ctx, pg.PoolTypeVRF, uint32(201))
		require.NoError(t, err)
		id, err = service.AcquireUniqueInteger(ctx, pg.PoolTypeVRF, uint32(201))
		require.NoError(t, err)
		require.Equal(t, uint32(201), id)

		// A value for a pool that was never seeded should fail
		_, err = service.AcquireUniqueInteger(ctx, "UNSEEDED_POOL", uint32(1))
		require.ErrorIs(t, err, pg.ErrPoolExhausted)
		require.EqualError(t, err, "pool exhausted: no integers available: pool 'UNSEEDED_POOL'")
	})

	t.Run("Concurrent Acquisition Safety", func(t *testing.T) {
		// Seed a fresh custom pool with 50 items
		const (
			poolSize               = 50
			customPool pg.PoolType = "CONCURRENT_TEST"
		)

		err := service.Seed(ctx, customPool, 1, poolSize)
		require.NoError(t, err)

		results := make(chan uint32, poolSize)
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
		seen := make(map[uint32]bool)
		for id := range results {
			require.False(t, seen[id])
			seen[id] = true
		}

		require.Len(t, seen, poolSize)
	})
}
