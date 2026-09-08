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
)

// ErrPoolExhausted is returned by Acquire when a pool has no free integers left.
var ErrPoolExhausted = errors.New("pool exhausted: no integers available")

type IntegerPool struct {
	log *slog.Logger
	db  *sql.DB
}

func NewIntegerPool(log *slog.Logger, db *sql.DB) *IntegerPool {
	return &IntegerPool{
		log: log.WithGroup("integer-pool"),
		db:  db,
	}
}

// Seed ensures a specific pool contains integers from startID up to endID.
func (p *IntegerPool) Seed(ctx context.Context, poolType PoolType, startID, endID int) error {
	const query = `
		INSERT INTO integer_pool (pool_type, id, is_allocated)
		SELECT $1, g, false
		FROM generate_series($2::int, $3::int) AS g
		ON CONFLICT (pool_type, id) DO NOTHING;
	`
	p.log.Debug("seed", "pool", poolType, "start", startID, "end", endID)
	_, err := p.db.ExecContext(ctx, query, string(poolType), startID, endID)
	return err
}

// Acquire gets the next available integer for a given pool atomically.
func (p *IntegerPool) Acquire(ctx context.Context, poolType PoolType) (int, error) {
	const query = `
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

	var acquiredID int
	p.log.Debug("acquire", "pool", poolType)
	err := p.db.QueryRowContext(ctx, query, string(poolType)).Scan(&acquiredID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, fmt.Errorf("%w: pool '%s'", ErrPoolExhausted, poolType)
		}
		return 0, err
	}

	return acquiredID, nil
}

// Release makes an integer in a specific pool available again.
func (p *IntegerPool) Release(ctx context.Context, poolType PoolType, id int) error {
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
		return fmt.Errorf("integer %d in pool '%s' was either not found or already released", id, poolType)
	}

	return nil
}
