package routing

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/metal-stack/metal-apiserver/pkg/db/storage"
)

// entityAlreadyModifiedMessage is the user-facing text for an optimistic-lock
// conflict. It matches the RethinkDB adapter so both backends report the same
// message.
const entityAlreadyModifiedMessage = "the entity was already modified, please retry"

// ErrPartialWrite is returned when a dual write succeeded on the read
// backend but failed on the other one. The data is now inconsistent and the
// failed backend needs to be reconciled.
var ErrPartialWrite = errors.New("partial write: backend mirror failed")

// Router is a storage.Storage implementation that dispatches to a backend
// according to the configured Mode:
//
//   - ModeRethink:  only the RethinkDB adapter is used
//   - ModePostgres: only the Postgres adapter is used
//   - ModeBoth:     writes go to both (RethinkDB first), reads go to ReadFrom
//
// It is the configuration-driven seam described in Option 4 of the pg README.
type Router[E storage.Entity] struct {
	log      *slog.Logger
	entity   string
	mode     Mode
	readFrom Mode

	rethink  storage.Storage[E]
	postgres storage.Storage[E]
}

// NewRouter builds a routed storage for a single entity type. The backends
// required by the configured mode must be non-nil.
func NewRouter[E storage.Entity](log *slog.Logger, cfg Config, entity string, rethink, postgres storage.Storage[E]) (*Router[E], error) {
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

func (r *Router[E]) Find(ctx context.Context, filters ...storage.Filter) (E, error) {
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

func (r *Router[E]) List(ctx context.Context, filters ...storage.Filter) ([]E, error) {
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

func (r *Router[E]) Watch(ctx context.Context, id string) (<-chan struct {
	Old E
	New E
}, error) {
	if r.readFrom == ModePostgres {
		return r.postgres.Watch(ctx, id)
	}
	return r.rethink.Watch(ctx, id)
}

func requireRethinkFilters(filters []storage.Filter) error {
	for i, f := range filters {
		if f.Rethink == nil {
			return fmt.Errorf("%w: filter %d has no rethinkdb representation", storage.ErrFilterUnsupported, i)
		}
	}
	return nil
}

func requirePostgresFilters(filters []storage.Filter) error {
	for i, f := range filters {
		if f.Postgres == nil {
			return fmt.Errorf("%w: filter %d has no postgres representation", storage.ErrFilterUnsupported, i)
		}
	}
	return nil
}
