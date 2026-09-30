package routing

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"time"

	"uuid"

	"github.com/metal-stack/metal-apiserver/pkg/db/generic"
	"github.com/metal-stack/metal-apiserver/pkg/db/generic/pg"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
)

var _ Storage[*metal.Machine] = (*postgresStorage[*metal.Machine])(nil)

// postgresStorage adapts a pg.GenericRepository to the routing port.
//
// Identity: the port addresses entities by their metal string id, so the
// adapter requires that id to be a UUID (Postgres stores entities keyed by
// uuidv7). Entities with natural, non-UUID ids (IP, Size, Partition, Image,
// FilesystemLayout) are therefore not supported yet — the README calls out that
// they need a re-keying or a name-lookup strategy.
//
// Concurrency: RethinkDB guards updates with the `changed` timestamp and keeps
// a `generation` counter; Postgres guards updates with an integer `version`.
// The adapter maps them with the invariant `version = generation + 1`: create
// stamps generation 0 (Postgres writes version 1) and update increments
// generation by one, using the pre-increment generation as the expected version.
type postgresStorage[E generic.Entity] struct {
	repo *pg.GenericRepository[E]
}

// NewPostgresStorage wraps a Postgres repository in the routing port.
func NewPostgresStorage[E generic.Entity](repo *pg.GenericRepository[E]) Storage[E] {
	return &postgresStorage[E]{repo: repo}
}

func (s *postgresStorage[E]) Create(ctx context.Context, e E) (E, error) {
	var zero E

	id, err := entityUUID(e)
	if err != nil {
		return zero, err
	}
	if err := stampCreate(e); err != nil {
		return zero, err
	}
	if err := s.repo.Create(ctx, id, e); err != nil {
		return zero, mapPostgresError(err)
	}

	return e, nil
}

func (s *postgresStorage[E]) Update(ctx context.Context, e E) error {
	id, err := entityUUID(e)
	if err != nil {
		return err
	}

	expected := int32(e.GetGeneration()) + 1
	if err := stampUpdate(e); err != nil {
		return err
	}

	return mapPostgresError(s.repo.Update(ctx, id, expected, e))
}

func (s *postgresStorage[E]) Upsert(ctx context.Context, e E) error {
	id, err := entityUUID(e)
	if err != nil {
		return err
	}
	if err := stampUpsert(e); err != nil {
		return err
	}

	return mapPostgresError(s.repo.Upsert(ctx, id, e))
}

func (s *postgresStorage[E]) Delete(ctx context.Context, e E) error {
	id, err := entityUUID(e)
	if err != nil {
		return err
	}

	return mapPostgresError(s.repo.Delete(ctx, id))
}

func (s *postgresStorage[E]) Get(ctx context.Context, id string) (E, error) {
	var zero E

	parsed, err := parseEntityID[E](id)
	if err != nil {
		return zero, err
	}
	ent, err := s.repo.Get(ctx, parsed)
	if err != nil {
		return zero, mapPostgresError(err)
	}

	return ent.Data, nil
}

func (s *postgresStorage[E]) Find(ctx context.Context, filters ...Filter) (E, error) {
	var zero E

	entities, err := s.List(ctx, filters...)
	if err != nil {
		return zero, err
	}
	switch len(entities) {
	case 0:
		return zero, fmt.Errorf("%w: no %s found", ErrNotFound, EntityName[E]())
	case 1:
		return entities[0], nil
	default:
		return zero, fmt.Errorf("more than one %s found by query", EntityName[E]())
	}
}

func (s *postgresStorage[E]) List(ctx context.Context, filters ...Filter) ([]E, error) {
	pgFilters, err := postgresFilters(filters)
	if err != nil {
		return nil, err
	}

	entities, err := s.repo.Query(ctx, pgFilters, nil)
	if err != nil {
		return nil, mapPostgresError(err)
	}

	out := make([]E, 0, len(entities))
	for _, ent := range entities {
		out = append(out, ent.Data)
	}
	return out, nil
}

func postgresFilters(filters []Filter) ([]pg.QueryFilter, error) {
	if err := requirePostgresFilters(filters); err != nil {
		return nil, err
	}
	result := make([]pg.QueryFilter, 0, len(filters))
	for _, f := range filters {
		result = append(result, f.Postgres...)
	}
	return result, nil
}

func entityUUID[E generic.Entity](e E) (uuid.UUID, error) {
	id := e.GetID()
	if id == "" {
		id = uuid.NewV7().String()
		e.SetID(id)
	}
	return parseEntityID[E](id)
}

func parseEntityID[E generic.Entity](id string) (uuid.UUID, error) {
	parsed, err := uuid.Parse(id)
	if err != nil {
		return uuid.UUID{}, fmt.Errorf("%s has a non-uuid id %q, which the postgres backend does not support yet: %w", EntityName[E](), id, err)
	}
	return parsed, nil
}

func mapPostgresError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, pg.ErrNotFound):
		return fmt.Errorf("%w: %v", ErrNotFound, err)
	case errors.Is(err, pg.ErrAlreadyExists):
		return fmt.Errorf("%w: %v", ErrAlreadyExists, err)
	case errors.Is(err, pg.ErrOptimisticLockConflict):
		return fmt.Errorf("%w: %v", ErrConflict, err)
	default:
		return err
	}
}

func stampCreate(e any) error {
	now := time.Now()
	if err := setField(e, "Created", now); err != nil {
		return err
	}
	if err := setField(e, "Changed", now); err != nil {
		return err
	}
	return setField(e, "Generation", uint64(0))
}

func stampUpdate(e any) error {
	if err := setField(e, "Changed", time.Now()); err != nil {
		return err
	}
	return setField(e, "Generation", getGeneration(e)+1)
}

func stampUpsert(e any) error {
	if created, ok := getTimeField(e, "Created"); ok && created.IsZero() {
		if err := setField(e, "Created", time.Now()); err != nil {
			return err
		}
	}
	if err := setField(e, "Changed", time.Now()); err != nil {
		return err
	}
	return setField(e, "Generation", getGeneration(e)+1)
}

func getGeneration(e any) uint64 {
	rv := reflect.ValueOf(e)
	if rv.Kind() != reflect.Pointer || rv.IsNil() {
		return 0
	}
	f := rv.Elem().FieldByName("Generation")
	if !f.IsValid() || f.Kind() != reflect.Uint64 {
		return 0
	}
	return f.Uint()
}

func getTimeField(e any, name string) (time.Time, bool) {
	rv := reflect.ValueOf(e)
	if rv.Kind() != reflect.Pointer || rv.IsNil() {
		return time.Time{}, false
	}
	f := rv.Elem().FieldByName(name)
	if !f.IsValid() || f.Type() != reflect.TypeFor[time.Time]() {
		return time.Time{}, false
	}
	return f.Interface().(time.Time), true
}

func setField(e any, name string, value any) error {
	rv := reflect.ValueOf(e)
	if rv.Kind() != reflect.Pointer || rv.IsNil() {
		return fmt.Errorf("cannot set %s on non-pointer entity %T", name, e)
	}
	f := rv.Elem().FieldByName(name)
	if !f.IsValid() || !f.CanSet() {
		return fmt.Errorf("entity %T has no settable %s field", e, name)
	}
	v := reflect.ValueOf(value)
	if !v.Type().AssignableTo(f.Type()) {
		return fmt.Errorf("entity %T field %s is %s, cannot assign %T", e, name, f.Type(), value)
	}
	f.Set(v)
	return nil
}
