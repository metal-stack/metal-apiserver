package routing

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/metal-stack/metal-apiserver/pkg/db/generic"
	"github.com/metal-stack/metal-apiserver/pkg/db/generic/pg"
)

var (
	// ErrPartialWrite is returned when a dual write succeeded on the read
	// backend but failed on the other one. The data is now inconsistent and the
	// failed backend needs to be reconciled.
	ErrPartialWrite = errors.New("partial write: backend mirror failed")

	// ErrFilterUnsupported is returned when a query does not carry a filter
	// representation for the backend that serves the read.
	ErrFilterUnsupported = errors.New("filter is not available for the read backend")

	// ErrNotFound is returned when an entity does not exist.
	ErrNotFound = errors.New("entity not found")
	// ErrAlreadyExists is returned when a create conflicts with an existing
	// entity.
	ErrAlreadyExists = errors.New("entity already exists")
	// ErrConflict is returned when an optimistic-lock conflict was detected.
	ErrConflict = errors.New("entity was modified concurrently")
)

// Filter carries the backend-specific representation of a query. A caller
// builds the side(s) it needs; a routed read requires the representation of the
// backend that serves reads. This keeps the port free of a ReQL interpreter
// while still allowing a single call to serve either backend.
//
// The representations are produced by the existing, backend-specific query
// packages:
//
//	Filter{
//	    Rethink:  queries.MachineFilter(rq),   // pkg/db/queries
//	    Postgres: q.MachineFilter(rq),         // pkg/db/generic/pg/q
//	}
type Filter struct {
	Rethink  generic.EntityQuery
	Postgres []pg.QueryFilter
}

// Storage is the backend-agnostic port for metal entities. Both the RethinkDB
// and the Postgres backend implement it, and Router dispatches to one of them.
type Storage[E generic.Entity] interface {
	Create(ctx context.Context, e E) (E, error)
	Update(ctx context.Context, e E) error
	Upsert(ctx context.Context, e E) error
	Delete(ctx context.Context, e E) error
	Get(ctx context.Context, id string) (E, error)
	Find(ctx context.Context, filters ...Filter) (E, error)
	List(ctx context.Context, filters ...Filter) ([]E, error)
}

// Router is a Storage implementation that dispatches to a backend according to
// the configured Mode:
//
//   - ModeRethink:  only the RethinkDB adapter is used
//   - ModePostgres: only the Postgres adapter is used
//   - ModeBoth:     writes go to both (RethinkDB first), reads go to ReadFrom
//
// It is the configuration-driven seam described in Option 4 of the pg README.
type Router[E generic.Entity] struct {
	log      *slog.Logger
	entity   string
	mode     Mode
	readFrom Mode

	rethink  Storage[E]
	postgres Storage[E]
}

// NewRouter builds a routed storage for a single entity type. The backends
// required by the configured mode must be non-nil.
func NewRouter[E generic.Entity](log *slog.Logger, cfg Config, entity string, rethink, postgres Storage[E]) (*Router[E], error) {
	if log == nil {
		log = slog.Default()
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	mode := cfg.ModeFor(entity)
	switch mode {
	case ModeRethink:
		if rethink == nil {
			return nil, fmt.Errorf("entity %q is configured for %q but no rethinkdb backend was provided", entity, mode)
		}
	case ModePostgres:
		if postgres == nil {
			return nil, fmt.Errorf("entity %q is configured for %q but no postgres backend was provided", entity, mode)
		}
	case ModeBoth:
		if rethink == nil || postgres == nil {
			return nil, fmt.Errorf("entity %q is configured for %q but both backends are required", entity, mode)
		}
	default:
		return nil, fmt.Errorf("entity %q has invalid mode %q", entity, mode)
	}

	return &Router[E]{
		log:      log.With("entity", entity),
		entity:   entity,
		mode:     mode,
		readFrom: cfg.readBackend(mode),
		rethink:  rethink,
		postgres: postgres,
	}, nil
}

// Mode returns the configured mode of this router.
func (r *Router[E]) Mode() Mode { return r.mode }

// ReadFrom returns the backend reads are served from.
func (r *Router[E]) ReadFrom() Mode { return r.readFrom }

func (r *Router[E]) Create(ctx context.Context, e E) (E, error) {
	switch r.mode {
	case ModePostgres:
		return r.postgres.Create(ctx, e)
	case ModeBoth:
		created, err := r.rethink.Create(ctx, e)
		if err != nil {
			return created, err
		}
		// Mirror with the entity returned by rethink so a generated id is
		// carried over to postgres.
		if _, err := r.postgres.Create(ctx, created); err != nil {
			r.log.Error("postgres mirror failed after rethinkdb write, backends diverged", "id", created.GetID(), "error", err)
			return created, fmt.Errorf("%w (entity %s, create, postgres): %v", ErrPartialWrite, r.entity, err)
		}
		return created, nil
	default:
		return r.rethink.Create(ctx, e)
	}
}

func (r *Router[E]) Update(ctx context.Context, e E) error {
	switch r.mode {
	case ModePostgres:
		return r.postgres.Update(ctx, e)
	case ModeBoth:
		// Hand postgres a copy of the pre-update entity: the rethinkdb adapter
		// mutates the entity (changed/generation) in place, and the postgres
		// adapter derives its optimistic-lock version from those fields.
		mirror, err := clone(e)
		if err != nil {
			return err
		}
		if err := r.rethink.Update(ctx, e); err != nil {
			return err
		}
		if err := r.postgres.Update(ctx, mirror); err != nil {
			r.log.Error("postgres mirror failed after rethinkdb write, backends diverged", "id", e.GetID(), "error", err)
			return fmt.Errorf("%w (entity %s, update, postgres): %v", ErrPartialWrite, r.entity, err)
		}
		return nil
	default:
		return r.rethink.Update(ctx, e)
	}
}

func (r *Router[E]) Upsert(ctx context.Context, e E) error {
	switch r.mode {
	case ModePostgres:
		return r.postgres.Upsert(ctx, e)
	case ModeBoth:
		mirror, err := clone(e)
		if err != nil {
			return err
		}
		if err := r.rethink.Upsert(ctx, e); err != nil {
			return err
		}
		if err := r.postgres.Upsert(ctx, mirror); err != nil {
			r.log.Error("postgres mirror failed after rethinkdb write, backends diverged", "id", e.GetID(), "error", err)
			return fmt.Errorf("%w (entity %s, upsert, postgres): %v", ErrPartialWrite, r.entity, err)
		}
		return nil
	default:
		return r.rethink.Upsert(ctx, e)
	}
}

func (r *Router[E]) Delete(ctx context.Context, e E) error {
	switch r.mode {
	case ModePostgres:
		return r.postgres.Delete(ctx, e)
	case ModeBoth:
		if err := r.rethink.Delete(ctx, e); err != nil {
			return err
		}
		if err := r.postgres.Delete(ctx, e); err != nil {
			r.log.Error("postgres mirror failed after rethinkdb write, backends diverged", "id", e.GetID(), "error", err)
			return fmt.Errorf("%w (entity %s, delete, postgres): %v", ErrPartialWrite, r.entity, err)
		}
		return nil
	default:
		return r.rethink.Delete(ctx, e)
	}
}

func (r *Router[E]) Get(ctx context.Context, id string) (E, error) {
	if r.readFrom == ModePostgres {
		return r.postgres.Get(ctx, id)
	}
	return r.rethink.Get(ctx, id)
}

func (r *Router[E]) Find(ctx context.Context, filters ...Filter) (E, error) {
	if r.readFrom == ModePostgres {
		if err := requirePostgresFilters(filters); err != nil {
			var zero E
			return zero, err
		}
		return r.postgres.Find(ctx, filters...)
	}
	if err := requireRethinkFilters(filters); err != nil {
		var zero E
		return zero, err
	}
	return r.rethink.Find(ctx, filters...)
}

func (r *Router[E]) List(ctx context.Context, filters ...Filter) ([]E, error) {
	if r.readFrom == ModePostgres {
		if err := requirePostgresFilters(filters); err != nil {
			return nil, err
		}
		return r.postgres.List(ctx, filters...)
	}
	if err := requireRethinkFilters(filters); err != nil {
		return nil, err
	}
	return r.rethink.List(ctx, filters...)
}

func requireRethinkFilters(filters []Filter) error {
	for i, f := range filters {
		if f.Rethink == nil {
			return fmt.Errorf("%w: filter %d has no rethinkdb representation", ErrFilterUnsupported, i)
		}
	}
	return nil
}

func requirePostgresFilters(filters []Filter) error {
	for i, f := range filters {
		if len(f.Postgres) == 0 {
			return fmt.Errorf("%w: filter %d has no postgres representation", ErrFilterUnsupported, i)
		}
	}
	return nil
}
