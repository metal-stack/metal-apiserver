package routing

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"time"

	"uuid"

	"github.com/metal-stack/api/go/errorutil"
	"github.com/metal-stack/metal-apiserver/pkg/db/generic/pg"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-apiserver/pkg/db/storage"
)

var _ storage.Storage[*metal.Machine] = (*postgresStorage[*metal.Machine])(nil)

// postgresStorage adapts a pg.GenericRepository to the neutral Entity Storage
// port.
//
// Identity: the port addresses entities by their metal string id, which is used
// verbatim as the Postgres primary key (the column is TEXT). UUID-keyed entities
// that leave their id empty get a generated UUIDv7; named entities (partition,
// size, image, filesystemlayout, switch, ...) carry their meaningful name.
//
// Concurrency: RethinkDB guards updates with the `changed` timestamp and keeps
// a `generation` counter; Postgres guards updates with an integer `version`.
// The adapter maps them with the invariant `version = generation + 1`: create
// stamps generation 0 (Postgres writes version 1) and update increments
// generation by one, using the pre-increment generation as the expected version.
//
// Errors: the repository returns backend-agnostic sentinels; this adapter
// renders them as the connect errors the Repository layer expects, using the
// entity's legacy name so both backends report identical messages.
type postgresStorage[E storage.Entity] struct {
	repo *pg.GenericRepository[E]
	name string
}

// NewPostgresStorage wraps a Postgres repository in the neutral port. name is
// the entity's legacy (lowercase) name, used in error messages.
func NewPostgresStorage[E storage.Entity](repo *pg.GenericRepository[E], name string) storage.Storage[E] {
	return &postgresStorage[E]{repo: repo, name: name}
}

func (s *postgresStorage[E]) Create(ctx context.Context, e E) (E, error) {
	var zero E

	id := entityID(e)
	if err := stampCreate(e); err != nil {
		return zero, err
	}
	if err := s.repo.Create(ctx, id, e); err != nil {
		if errors.Is(err, pg.ErrAlreadyExists) {
			return zero, errorutil.Conflict("cannot create %s in database, entity already exists: %s", s.name, id)
		}
		return zero, toConnectError(err)
	}

	return e, nil
}

func (s *postgresStorage[E]) Update(ctx context.Context, e E) error {
	id := entityID(e)

	expected := int32(e.GetGeneration()) + 1
	if err := stampUpdate(e); err != nil {
		return err
	}

	err := s.repo.Update(ctx, id, expected, e)
	switch {
	case err == nil:
		return nil
	case errors.Is(err, pg.ErrOptimisticLockConflict):
		return errorutil.Aborted("cannot update %s (%s): %s", s.name, e.GetID(), entityAlreadyModifiedMessage)
	case errors.Is(err, pg.ErrNotFound):
		return errorutil.NotFound("no %s with id %q found", s.name, e.GetID())
	default:
		return toConnectError(err)
	}
}

func (s *postgresStorage[E]) Upsert(ctx context.Context, e E) error {
	id := entityID(e)
	if err := stampUpsert(e); err != nil {
		return err
	}

	err := s.repo.Upsert(ctx, id, e)
	if err != nil && errors.Is(err, pg.ErrAlreadyExists) {
		return errorutil.Conflict("cannot upsert %s (%s) in database: entity already exists", s.name, e.GetID())
	}
	return toConnectError(err)
}

func (s *postgresStorage[E]) Delete(ctx context.Context, e E) error {
	err := s.repo.Delete(ctx, entityID(e))
	if errors.Is(err, pg.ErrNotFound) {
		// deletes are idempotent: deleting a missing entity is a no-op.
		return nil
	}
	return toConnectError(err)
}

func (s *postgresStorage[E]) Get(ctx context.Context, id string) (E, error) {
	var zero E

	ent, err := s.repo.Get(ctx, id)
	if err != nil {
		if errors.Is(err, pg.ErrNotFound) {
			return zero, errorutil.NotFound("no %s with id %q found", s.name, id)
		}
		return zero, toConnectError(err)
	}

	return ent.Data, nil
}

func (s *postgresStorage[E]) Find(ctx context.Context, filters ...storage.Filter) (E, error) {
	var zero E

	entities, err := s.List(ctx, filters...)
	if err != nil {
		return zero, err
	}
	switch len(entities) {
	case 0:
		return zero, errorutil.NotFound("no %s found", s.name)
	case 1:
		return entities[0], nil
	default:
		return zero, fmt.Errorf("more than one %s found by query", s.name)
	}
}

func (s *postgresStorage[E]) List(ctx context.Context, filters ...storage.Filter) ([]E, error) {
	pgFilters, err := postgresFilters(filters)
	if err != nil {
		return nil, err
	}

	entities, err := s.repo.Query(ctx, pgFilters, nil)
	if err != nil {
		return nil, toConnectError(err)
	}

	out := make([]E, 0, len(entities))
	for _, ent := range entities {
		out = append(out, ent.Data)
	}
	return out, nil
}

func (s *postgresStorage[E]) Watch(ctx context.Context, id string) (<-chan struct {
	Old E
	New E
}, error) {
	changes, err := s.repo.Watch(ctx, id)
	if err != nil {
		return nil, toConnectError(err)
	}
	return changes, nil
}

// postgresFilters extracts the Postgres representation from every filter.
func postgresFilters(filters []storage.Filter) ([]pg.QueryFilter, error) {
	if err := requirePostgresFilters(filters); err != nil {
		return nil, err
	}
	result := make([]pg.QueryFilter, 0, len(filters))
	for i, f := range filters {
		pf, ok := f.Postgres.([]pg.QueryFilter)
		if !ok {
			return nil, fmt.Errorf("%w: filter %d has no postgres representation", storage.ErrFilterUnsupported, i)
		}
		result = append(result, pf...)
	}
	return result, nil
}

// entityID returns the entity's primary key. An empty id is generated as a
// UUIDv7 (for UUID-keyed entities that let the server assign one) and written
// back to the entity; named entities supply their meaningful id up front and it
// is used verbatim.
func entityID[E storage.Entity](e E) string {
	id := e.GetID()
	if id == "" {
		id = uuid.NewV7().String()
		e.SetID(id)
	}
	return id
}

// toConnectError maps the postgres sentinels to the connect errors callers
// expect. The message is rendered from the sentinel; per-operation callers
// override it with the entity name where a more specific message is needed.
func toConnectError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, pg.ErrNotFound):
		return errorutil.NotFound("%v", err)
	case errors.Is(err, pg.ErrAlreadyExists):
		return errorutil.Conflict("%v", err)
	case errors.Is(err, pg.ErrOptimisticLockConflict):
		return errorutil.Aborted("%v", err)
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
