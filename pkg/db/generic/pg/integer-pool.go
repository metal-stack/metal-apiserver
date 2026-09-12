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
		id BIGINT NOT NULL,
		is_allocated BOOLEAN NOT NULL DEFAULT FALSE,
		allocated_at TIMESTAMPTZ,
		PRIMARY KEY (pool_type, id)
	);
	CREATE INDEX IF NOT EXISTS idx_integer_pool_type_free ON integer_pool (pool_type, id) WHERE is_allocated = FALSE;
	CREATE TABLE IF NOT EXISTS integer_pool_state (
		pool_type VARCHAR(64) PRIMARY KEY,
		next BIGINT NOT NULL,
		max BIGINT NOT NULL
	);
`
)

var (
	// ErrPoolExhausted is returned by Acquire when a pool has no free integers left.
	ErrPoolExhausted   = errors.New("pool exhausted: no integers available")
	ErrIntegerNotFound = errors.New("was either not found or already released")
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
func (p *IntegerPool) Seed(ctx context.Context, poolType PoolType, startID, endID uint32) error {
	const query = `
		INSERT INTO integer_pool_state (pool_type, next, max)
		VALUES ($1, $2, $3)
		ON CONFLICT (pool_type) DO UPDATE
		SET next = LEAST(integer_pool_state.next, EXCLUDED.next),
		    max = GREATEST(integer_pool_state.max, EXCLUDED.max);
	`
	p.log.Debug("seed", "pool", poolType, "start", startID, "end", endID)
	_, err := p.db.ExecContext(ctx, query, string(poolType), startID, endID)
	return err
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
			return 0, fmt.Errorf("%w: pool '%s'", ErrPoolExhausted, poolType)
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
