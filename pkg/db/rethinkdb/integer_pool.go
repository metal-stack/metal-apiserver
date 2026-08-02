package rethinkdb

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/avast/retry-go/v4"
	"github.com/metal-stack/api/go/errorutil"
	"github.com/metal-stack/metal-apiserver/pkg/db/interfaces"
	r "gopkg.in/rethinkdb/rethinkdb-go.v6"
)

// compile-time check
var _ interfaces.IntegerPool = (*integerPool)(nil)

// integerPoolType defines the name of the IntegerPool
type integerPoolType string

const (
	// vrfIntegerPool defines the name of the pool for VRFs
	// this also defines the name of the tables
	// FIXME, must be renamed to vrfpool later
	vrfIntegerPool integerPoolType = "integerpool"
	// asnIntegerPool defines the name of the pool for ASNs
	asnIntegerPool integerPoolType = "asnpool"
)

// integerPool manages unique integers
type integerPool struct {
	r *datastore

	poolType  integerPoolType
	table     r.Term
	tableName string

	min, max uint
}

type integer struct {
	ID uint `rethinkdb:"id"`
}

type integerinfo struct {
	ID            string `rethinkdb:"id"`
	IsInitialized bool   `rethinkdb:"isInitialized"`
}

func newIntegerPool(d *datastore, poolType integerPoolType, tableName string, min, max uint) *integerPool {
	d.tableNames = append(d.tableNames, tableName)
	return &integerPool{
		r:         d,
		poolType:  poolType,
		table:     r.Table(tableName),
		tableName: tableName,
		min:       min,
		max:       max,
	}
}

func (ip *integerPool) initialize() error {
	if err := ip.r.createTable(context.Background(), ip.tableName); err != nil {
		return err
	}
	if err := ip.r.createTable(context.Background(), ip.tableName+"info"); err != nil {
		return err
	}

	infoTable := r.DB(ip.r.dbname).Table(ip.tableName + "info")

	var info integerinfo
	err := infoTable.ReadOne(&info, ip.r.queryExecutor)
	if err != nil && !errors.Is(err, r.ErrEmptyResult) {
		return err
	}

	if info.IsInitialized {
		ip.r.log.Info("integer pool already initialized", "table", ip.tableName)
		return nil
	}

	ip.r.log.Info("initializing integer pool", "table", ip.tableName, "RangeMin", ip.min, "RangeMax", ip.max)

	intRange := makeRange(ip.min, ip.max)

	_, err = ip.table.Insert(intRange).RunWrite(ip.r.queryExecutor, r.RunOpts{ArrayLimit: ip.max})
	if err != nil {
		return err
	}

	_, err = infoTable.Insert(integerinfo{
		ID:            string(ip.poolType),
		IsInitialized: true,
	}).RunWrite(ip.r.queryExecutor)
	if err != nil {
		return err
	}

	return nil
}

// AcquireRandomUniqueInteger returns a random unique integer from the pool.
func (ip *integerPool) AcquireRandomUniqueInteger(ctx context.Context) (uint, error) {
	t := ip.table.Limit(1)

	var integer uint
	err := retry.Do(
		func() error {
			var err2 error
			integer, err2 = ip.genericAcquire(ctx, &t)
			return err2
		},
		retry.Attempts(10),
		retry.MaxDelay(100*time.Millisecond),
		retry.LastErrorOnly(true),
	)

	return integer, err
}

// AcquireUniqueInteger returns a unique integer from the pool.
func (ip *integerPool) AcquireUniqueInteger(ctx context.Context, value uint) (uint, error) {
	err := ip.verifyRange(value)
	if err != nil {
		return 0, err
	}

	t := ip.table.Get(value)

	return ip.genericAcquire(ctx, &t)
}

// ReleaseUniqueInteger returns a unique integer to the pool.
func (ip *integerPool) ReleaseUniqueInteger(ctx context.Context, id uint) error {
	err := ip.verifyRange(id)
	if err != nil {
		return err
	}

	i := integer{
		ID: id,
	}

	_, err = ip.table.Insert(i, r.InsertOpts{Conflict: "replace"}).RunWrite(ip.r.queryExecutor, r.RunOpts{Context: ctx})
	if err != nil {
		return err
	}

	return nil
}

func (ip *integerPool) genericAcquire(ctx context.Context, term *r.Term) (uint, error) {
	res, err := term.Delete(r.DeleteOpts{ReturnChanges: true}).RunWrite(ip.r.queryExecutor, r.RunOpts{Context: ctx})
	if err != nil {
		return 0, err
	}

	if len(res.Changes) == 0 {
		res, err := ip.table.Count().Run(ip.r.queryExecutor, r.RunOpts{Context: ctx})
		if err != nil {
			return 0, err
		}

		var count int64
		err = res.One(&count)
		if err != nil {
			return 0, err
		}

		if count <= 0 {
			return 0, errorutil.Internal("acquisition of a value failed for exhausted pool")
		}

		return 0, errorutil.Conflict("integer is already acquired by another")
	}

	result := uint(res.Changes[0].OldValue.(map[string]any)["id"].(float64))

	return result, nil
}

func makeRange(min, max uint) []integer {
	a := make([]integer, max-min+1)

	for i := range a {
		a[i] = integer{
			ID: min + uint(i),
		}
	}

	return a
}

func (ip *integerPool) verifyRange(value uint) error {
	if value < ip.min || value > ip.max {
		return fmt.Errorf("value '%d' is outside of the allowed range '%d - %d'", value, ip.min, ip.max)
	}

	return nil
}
