package pg

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
)

// PoolType defines supported pool categories
type PoolType string

const (
	PoolTypeASN PoolType = "ASN"
	PoolTypeVRF PoolType = "VRF"

	IntegerPoolschema = `
	CREATE TABLE IF NOT EXISTS integer_pool (
		pool_type VARCHAR(64) NOT NULL,
		id INTEGER NOT NULL,
		is_allocated BOOLEAN NOT NULL DEFAULT FALSE,
		allocated_at TIMESTAMPTZ,
		PRIMARY KEY (pool_type, id)
	);
	CREATE INDEX IF NOT EXISTS idx_integer_pool_type_free ON integer_pool (pool_type, id) WHERE is_allocated = FALSE;
	CREATE TABLE IF NOT EXISTS integer_pool_state (
		pool_type VARCHAR(64) PRIMARY KEY,
		next INTEGER NOT NULL,
		max INTEGER NOT NULL
	);
`
)

var (
	// ErrPoolExhausted is returned by Acquire when a pool has no free integers left.
	ErrPoolExhausted = errors.New("pool exhausted: no integers available")
	// ErrIntegerNotFound is returned by Release when the integer was not found or already released.
	ErrIntegerNotFound = errors.New("was either not found or already released")
	// ErrIntegerAlreadyAcquired is returned by AcquireUniqueInteger when the requested integer is already in use.
	ErrIntegerAlreadyAcquired = errors.New("integer is already acquired")
	// ErrPoolAlreadySeeded is returned by Seed when the pool has already been seeded.
	ErrPoolAlreadySeeded = errors.New("pool is already seeded")
	// ErrPoolNotSeeded is returned by Acquire and AcquireUniqueInteger when the
	// pool has no configured range yet.
	ErrPoolNotSeeded = errors.New("pool not seeded")
)

type IntegerPool struct {
	log *slog.Logger
	db  *sql.DB
}

func NewIntegerPool(log *slog.Logger, db *sql.DB) (*IntegerPool, error) {
	_, err := db.ExecContext(context.Background(), IntegerPoolschema)
	if err != nil {
		return nil, err
	}

	return &IntegerPool{
		log: log.WithGroup("integer-pool"),
		db:  db,
	}, nil
}

// Seed configures the range of a pool. It does not precompute the individual
// integers of the range; instead it records the range bounds and the pool
// grows on demand when integers are acquired.
//
// A pool can only be seeded once: Seed returns ErrPoolAlreadySeeded if the
// pool was seeded before. This keeps the growth counter monotonic, so an
// already-allocated integer can never be handed out again.
func (p *IntegerPool) Seed(ctx context.Context, poolType PoolType, startID, endID uint32) error {
	if startID > endID {
		return fmt.Errorf("invalid range for pool '%s': start %d must not exceed end %d", poolType, startID, endID)
	}

	const query = `
		INSERT INTO integer_pool_state (pool_type, next, max)
		VALUES ($1, $2, $3)
		ON CONFLICT (pool_type) DO NOTHING;
	`
	p.log.Debug("seed", "pool", poolType, "start", startID, "end", endID)

	res, err := p.db.ExecContext(ctx, query, string(poolType), startID, endID)
	if err != nil {
		return err
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return fmt.Errorf("%w: pool '%s'", ErrPoolAlreadySeeded, poolType)
	}

	return nil
}

// Acquire gets the next available integer for a given pool atomically.
// Released integers are reused first (ordered ASC); if none are free the pool
// grows by taking the next integer from its configured range.
func (p *IntegerPool) Acquire(ctx context.Context, poolType PoolType) (uint32, error) {
	const takeFree = `
		WITH next_num AS (
			SELECT pool_type, id
			FROM integer_pool
			WHERE pool_type = $1 AND is_allocated = FALSE
			ORDER BY id ASC
			LIMIT 1
			FOR UPDATE SKIP LOCKED
		)
		UPDATE integer_pool
		SET is_allocated = TRUE, allocated_at = NOW()
		FROM next_num
		WHERE integer_pool.pool_type = next_num.pool_type
		  AND integer_pool.id = next_num.id
		RETURNING integer_pool.id;
	`

	var acquiredID uint32
	p.log.Debug("acquire", "pool", poolType)

	err := p.db.QueryRowContext(ctx, takeFree, string(poolType)).Scan(&acquiredID)
	if err == nil {
		return acquiredID, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}

	// No released integer available, grow the pool from its range counter.
	// The UPDATE gates on next <= max and is atomic, so concurrent acquires
	// receive distinct, monotonically increasing values.
	const grow = `
		UPDATE integer_pool_state
		SET next = next + 1
		WHERE pool_type = $1 AND next <= max
		RETURNING next - 1;
	`
	err = p.db.QueryRowContext(ctx, grow, string(poolType)).Scan(&acquiredID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, p.poolUnavailableError(ctx, poolType)
		}
		return 0, err
	}

	// Record the grown integer as allocated so Release can validate it.
	const record = `
		INSERT INTO integer_pool (pool_type, id, is_allocated)
		VALUES ($1, $2, TRUE)
		ON CONFLICT (pool_type, id) DO NOTHING;
	`
	if _, err := p.db.ExecContext(ctx, record, string(poolType), acquiredID); err != nil {
		return 0, err
	}

	return acquiredID, nil
}

// AcquireUniqueInteger acquires a specific integer from a pool atomically.
// The value must lie within the pool's configured range and must not already be
// in use. Unlike Acquire, it does not grow the pool: if the requested integer is
// not currently free (either already allocated, or never within the range) an
// error is returned.
func (p *IntegerPool) AcquireUniqueInteger(ctx context.Context, poolType PoolType, value uint32) (uint32, error) {
	// Ensure the requested value lies within the pool's configured range.
	const maxQuery = `
		SELECT max
		FROM integer_pool_state
		WHERE pool_type = $1;
	`
	var maxID uint32
	err := p.db.QueryRowContext(ctx, maxQuery, string(poolType)).Scan(&maxID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("%w: pool '%s'", ErrPoolNotSeeded, poolType)
	}
	if err != nil {
		return 0, err
	}
	if value > maxID {
		return 0, fmt.Errorf("value %d is outside of the allowed range 0 - %d for pool '%s'", value, maxID, poolType)
	}

	p.log.Debug("acquire-unique", "pool", poolType, "id", value)

	// Single atomic statement: insert the row if it does not exist yet (rows are
	// created lazily by Acquire, so a specific value may not have a row yet), or
	// flip an existing free row to allocated. Zero returned rows mean the value
	// is already allocated. Under concurrency only one caller can win.
	const claim = `
		INSERT INTO integer_pool (pool_type, id, is_allocated, allocated_at)
		VALUES ($1, $2, TRUE, NOW())
		ON CONFLICT (pool_type, id) DO UPDATE
		SET is_allocated = TRUE, allocated_at = NOW()
		WHERE integer_pool.is_allocated = FALSE
		RETURNING id;
	`
	var claimed uint32
	err = p.db.QueryRowContext(ctx, claim, string(poolType), value).Scan(&claimed)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("%w: %d in pool '%s'", ErrIntegerAlreadyAcquired, value, poolType)
	}
	if err != nil {
		return 0, err
	}

	return claimed, nil
}

// poolUnavailableError reports why no integer could be acquired: either the
// pool has no configured range at all (ErrPoolNotSeeded) or its range is fully
// allocated (ErrPoolExhausted). It is only called on the failure path of Acquire.
func (p *IntegerPool) poolUnavailableError(ctx context.Context, poolType PoolType) error {
	const query = `SELECT 1 FROM integer_pool_state WHERE pool_type = $1`

	var exists int
	err := p.db.QueryRowContext(ctx, query, string(poolType)).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%w: pool '%s'", ErrPoolNotSeeded, poolType)
	}
	if err != nil {
		return err
	}

	return fmt.Errorf("%w: pool '%s'", ErrPoolExhausted, poolType)
}

// Release makes an integer in a specific pool available again.
func (p *IntegerPool) Release(ctx context.Context, poolType PoolType, id uint32) error {
	const query = `
		UPDATE integer_pool
		SET is_allocated = FALSE, allocated_at = NULL
		WHERE pool_type = $1 AND id = $2 AND is_allocated = TRUE;
	`

	p.log.Debug("release", "pool", poolType, "id", id)
	res, err := p.db.ExecContext(ctx, query, string(poolType), id)
	if err != nil {
		return err
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		return fmt.Errorf("integer %d in pool '%s' %w", id, poolType, ErrIntegerNotFound)
	}

	return nil
}
