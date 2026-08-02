package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/metal-stack/api/go/errorutil"
	"github.com/metal-stack/metal-apiserver/pkg/db/interfaces"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
)

// compile-time check
var _ interfaces.Storage[*metal.IP] = (*storage[*metal.IP])(nil)

type storage[E interfaces.Entity] struct {
	ds        *datastore
	tableName string
}

func newStorage[E interfaces.Entity](ds *datastore, tableName string) *storage[E] {
	ds.tableNames = append(ds.tableNames, tableName)
	return &storage[E]{
		ds:        ds,
		tableName: tableName,
	}
}

func (s *storage[E]) Create(ctx context.Context, e E) (E, error) {
	var (
		now  = time.Now()
		zero E
	)

	if err := setEntityField(e, "Created", now); err != nil {
		return zero, err
	}
	if err := setEntityField(e, "Changed", now); err != nil {
		return zero, err
	}
	if err := setEntityField(e, "Generation", uint64(0)); err != nil {
		return zero, err
	}

	if e.GetID() == "" {
		uid, err := uuid.NewV7()
		if err != nil {
			return zero, err
		}
		e.SetID(uid.String())
	}

	data, err := json.Marshal(e)
	if err != nil {
		return zero, fmt.Errorf("cannot marshal %v for creation: %w", s.tableName, err)
	}

	_, err = s.ds.pool.Exec(ctx,
		`INSERT INTO `+quoteIdent(s.tableName)+` (id, data, created, changed, generation) VALUES ($1, $2, $3, $4, $5)`,
		e.GetID(), data, e.GetCreated(), e.GetChanged(), e.GetGeneration(),
	)
	if err != nil {
		if isPgUniqueViolation(err) {
			return zero, errorutil.Conflict("cannot create %v in database, entity already exists: %s", s.tableName, e.GetID())
		}
		return zero, fmt.Errorf("cannot create %v in database: %w", s.tableName, err)
	}

	return e, nil
}

func (s *storage[E]) Delete(ctx context.Context, e E) error {
	tag, err := s.ds.pool.Exec(ctx,
		`DELETE FROM `+quoteIdent(s.tableName)+` WHERE id = $1`,
		e.GetID(),
	)
	if err != nil {
		return fmt.Errorf("cannot delete %v with id %q from database: %w", s.tableName, e.GetID(), err)
	}
	if tag.RowsAffected() == 0 {
		return errorutil.NotFound("no %v with id %q found", s.tableName, e.GetID())
	}
	return nil
}

func (s *storage[E]) Find(ctx context.Context, queries ...any) (E, error) {
	rows, err := s.ds.pool.Query(ctx,
		`SELECT data FROM `+quoteIdent(s.tableName),
	)
	if err != nil {
		var zero E
		return zero, fmt.Errorf("cannot find %v in database: %w", s.tableName, err)
	}
	defer rows.Close()

	var results []E

	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			var zero E
			return zero, fmt.Errorf("cannot scan %v: %w", s.tableName, err)
		}

		e := new(E)
		if err := json.Unmarshal(raw, e); err != nil {
			var zero E
			return zero, fmt.Errorf("cannot unmarshal %v: %w", s.tableName, err)
		}

		if matchEntity(*e, queries) {
			results = append(results, *e)
		}
	}

	if len(results) == 0 {
		var zero E
		return zero, errorutil.NotFound("no %v found", s.tableName)
	}
	if len(results) > 1 {
		var zero E
		return zero, fmt.Errorf("more than one %v exists", s.tableName)
	}

	return results[0], nil
}

func (s *storage[E]) List(ctx context.Context, queries ...any) ([]E, error) {
	rows, err := s.ds.pool.Query(ctx,
		`SELECT data FROM `+quoteIdent(s.tableName),
	)
	if err != nil {
		return nil, fmt.Errorf("cannot search %v in database: %w", s.tableName, err)
	}
	defer rows.Close()

	var results []E

	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, fmt.Errorf("cannot scan %v: %w", s.tableName, err)
		}

		e := new(E)
		if err := json.Unmarshal(raw, e); err != nil {
			return nil, fmt.Errorf("cannot unmarshal %v: %w", s.tableName, err)
		}

		if matchEntity(*e, queries) {
			results = append(results, *e)
		}
	}

	return results, nil
}

func (s *storage[E]) Get(ctx context.Context, id string) (E, error) {
	var zero E

	var raw []byte
	err := s.ds.pool.QueryRow(ctx,
		`SELECT data FROM `+quoteIdent(s.tableName)+` WHERE id = $1`,
		id,
	).Scan(&raw)
	if err != nil {
		if err == pgx.ErrNoRows {
			return zero, errorutil.NotFound("no %v with id %q found", s.tableName, id)
		}
		return zero, fmt.Errorf("cannot find %v with id %q in database: %w", s.tableName, id, err)
	}

	e := new(E)
	if err := json.Unmarshal(raw, e); err != nil {
		return zero, fmt.Errorf("cannot unmarshal %v: %w", s.tableName, err)
	}

	return *e, nil
}

func (s *storage[E]) Update(ctx context.Context, e E) error {
	if e.GetChanged().IsZero() {
		return fmt.Errorf("cannot update %v (%s): no changed timestamp set on entity", s.tableName, e.GetID())
	}

	changedTimestamp := e.GetChanged()

	if err := setEntityField(e, "Changed", time.Now()); err != nil {
		return err
	}
	newGeneration := e.GetGeneration() + 1
	if err := setEntityField(e, "Generation", newGeneration); err != nil {
		return err
	}

	data, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("cannot marshal %v for update: %w", s.tableName, err)
	}

	// Use changed timestamp for optimistic locking, but also update the new changed value
	tag, err := s.ds.pool.Exec(ctx,
		`UPDATE `+quoteIdent(s.tableName)+` SET data = $1, changed = $2, generation = $3 WHERE id = $4 AND changed = $5`,
		data, e.GetChanged(), newGeneration, e.GetID(), changedTimestamp,
	)
	if err != nil {
		return fmt.Errorf("cannot update %v (%s): %w", s.tableName, e.GetID(), err)
	}
	if tag.RowsAffected() == 0 {
		return errorutil.Aborted("cannot update %v (%s): %s", s.tableName, e.GetID(), entityAlreadyModifiedErrorMessage)
	}

	return nil
}

func (s *storage[E]) Upsert(ctx context.Context, e E) error {
	now := time.Now()

	if e.GetCreated().IsZero() {
		if err := setEntityField(e, "Created", now); err != nil {
			return err
		}
	}

	if err := setEntityField(e, "Changed", now); err != nil {
		return err
	}
	if err := setEntityField(e, "Generation", e.GetGeneration()+1); err != nil {
		return err
	}

	data, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("cannot marshal %v for upsert: %w", s.tableName, err)
	}

	_, err = s.ds.pool.Exec(ctx,
		`INSERT INTO `+quoteIdent(s.tableName)+` (id, data, created, changed, generation) VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (id) DO UPDATE SET data = $2, changed = $4, generation = `+quoteIdent(s.tableName)+`.generation + 1`,
		e.GetID(), data, e.GetCreated(), e.GetChanged(),
	)
	if err != nil {
		return fmt.Errorf("cannot upsert %v (%s) in database: %w", s.tableName, e.GetID(), err)
	}

	return nil
}

// setEntityField sets a named field on an entity using reflection.
func setEntityField[E interfaces.Entity](e E, fieldName string, val any) error {
	v := reflect.ValueOf(e)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	f := v.FieldByName(fieldName)
	if !f.IsValid() {
		return fmt.Errorf("field %s not found on %T", fieldName, e)
	}
	if !f.CanSet() {
		return fmt.Errorf("field %s cannot be set on %T", fieldName, e)
	}
	fv := reflect.ValueOf(val)
	if f.Kind() == reflect.Uint64 && fv.Kind() == reflect.Uint64 {
		f.SetUint(fv.Uint())
	} else if f.Kind() == reflect.Struct && f.Type() == reflect.TypeOf(time.Time{}) {
		f.Set(fv)
	} else {
		return fmt.Errorf("cannot set field %s: type mismatch", fieldName)
	}
	return nil
}

// matchEntity checks if an entity matches all the given query filters.
func matchEntity[E interfaces.Entity](e E, queries []any) bool {
	if len(queries) == 0 {
		return true
	}
	data, err := json.Marshal(e)
	if err != nil {
		return true
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return true
	}
	for _, q := range queries {
		if q == nil {
			continue
		}
		if f, ok := q.(func(map[string]any) bool); ok {
			if !f(m) {
				return false
			}
		}
	}
	return true
}

func quoteIdent(name string) string {
	return `"` + name + `"`
}
