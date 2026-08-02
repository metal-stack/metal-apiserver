package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/metal-stack/api/go/errorutil"
	"github.com/metal-stack/metal-apiserver/pkg/db/interfaces"
)

// compile-time check
var _ interfaces.IntegerPool = (*integerPool)(nil)

type integerPool struct {
	ds        *datastore
	tableName string
	min, max  uint
}

func newIntegerPool(ds *datastore, tableName string, min, max uint) *integerPool {
	ds.tableNames = append(ds.tableNames, tableName)
	return &integerPool{
		ds:        ds,
		tableName: tableName,
		min:       min,
		max:       max,
	}
}

func (ip *integerPool) AcquireRandomUniqueInteger(ctx context.Context) (uint, error) {
	// Delete one row and return its value
	var id uint
	err := ip.ds.pool.QueryRow(ctx,
		`DELETE FROM `+quoteIdent(ip.tableName)+` WHERE id = (
			SELECT id FROM `+quoteIdent(ip.tableName)+` ORDER BY random() LIMIT 1
		) RETURNING id`,
	).Scan(&id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, errorutil.Internal("acquisition of a value failed for exhausted pool")
		}
		return 0, fmt.Errorf("cannot acquire random integer: %w", err)
	}
	return id, nil
}

func (ip *integerPool) AcquireUniqueInteger(ctx context.Context, value uint) (uint, error) {
	err := ip.verifyRange(value)
	if err != nil {
		return 0, err
	}

	var id uint
	err = ip.ds.pool.QueryRow(ctx,
		`DELETE FROM `+quoteIdent(ip.tableName)+` WHERE id = $1 RETURNING id`,
		value,
	).Scan(&id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Check if pool is empty
			var count int64
			err2 := ip.ds.pool.QueryRow(ctx,
				`SELECT count(*) FROM `+quoteIdent(ip.tableName),
			).Scan(&count)
			if err2 != nil {
				return 0, fmt.Errorf("cannot check pool count: %w", err2)
			}
			if count <= 0 {
				return 0, errorutil.Internal("acquisition of a value failed for exhausted pool")
			}
			return 0, errorutil.Conflict("integer %d is already acquired by another", value)
		}
		return 0, fmt.Errorf("cannot acquire integer %d: %w", value, err)
	}

	return id, nil
}

func (ip *integerPool) ReleaseUniqueInteger(ctx context.Context, id uint) error {
	err := ip.verifyRange(id)
	if err != nil {
		return err
	}

	_, err = ip.ds.pool.Exec(ctx,
		`INSERT INTO `+quoteIdent(ip.tableName)+` (id) VALUES ($1) ON CONFLICT (id) DO NOTHING`,
		id,
	)
	if err != nil {
		return fmt.Errorf("cannot release integer %d: %w", id, err)
	}

	return nil
}

func (ip *integerPool) verifyRange(value uint) error {
	if value < ip.min || value > ip.max {
		return fmt.Errorf("value '%d' is outside of the allowed range '%d - %d'", value, ip.min, ip.max)
	}
	return nil
}
