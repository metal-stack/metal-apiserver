package routing

import (
	"context"
	"fmt"

	"github.com/metal-stack/api/go/errorutil"
	"github.com/metal-stack/metal-apiserver/pkg/db/generic"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
)

var _ Storage[*metal.Machine] = (*rethinkStorage[*metal.Machine])(nil)

// rethinkStorage adapts a generic RethinkDB Storage to the routing port. It is a
// thin delegate: the underlying storage still owns id generation, timestamps and
// optimistic locking.
type rethinkStorage[E generic.Entity] struct {
	storage generic.Storage[E]
}

// NewRethinkStorage wraps a RethinkDB-backed storage in the routing port.
func NewRethinkStorage[E generic.Entity](storage generic.Storage[E]) Storage[E] {
	return &rethinkStorage[E]{storage: storage}
}

func (s *rethinkStorage[E]) Create(ctx context.Context, e E) (E, error) {
	created, err := s.storage.Create(ctx, e)
	return created, mapRethinkError(err)
}

func (s *rethinkStorage[E]) Update(ctx context.Context, e E) error {
	return mapRethinkError(s.storage.Update(ctx, e))
}

func (s *rethinkStorage[E]) Upsert(ctx context.Context, e E) error {
	return mapRethinkError(s.storage.Upsert(ctx, e))
}

func (s *rethinkStorage[E]) Delete(ctx context.Context, e E) error {
	return mapRethinkError(s.storage.Delete(ctx, e))
}

func (s *rethinkStorage[E]) Get(ctx context.Context, id string) (E, error) {
	e, err := s.storage.Get(ctx, id)
	return e, mapRethinkError(err)
}

func (s *rethinkStorage[E]) Find(ctx context.Context, filters ...Filter) (E, error) {
	queries, err := rethinkQueries(filters)
	if err != nil {
		var zero E
		return zero, err
	}
	e, err := s.storage.Find(ctx, queries...)
	return e, mapRethinkError(err)
}

func (s *rethinkStorage[E]) List(ctx context.Context, filters ...Filter) ([]E, error) {
	queries, err := rethinkQueries(filters)
	if err != nil {
		return nil, err
	}
	entities, err := s.storage.List(ctx, queries...)
	return entities, mapRethinkError(err)
}

func (s *rethinkStorage[E]) Watch(ctx context.Context, id string) (<-chan struct {
	Old E
	New E
}, error) {
	changes, err := s.storage.Watch(ctx, id)
	return changes, mapRethinkError(err)
}

func rethinkQueries(filters []Filter) ([]generic.EntityQuery, error) {
	if err := requireRethinkFilters(filters); err != nil {
		return nil, err
	}
	queries := make([]generic.EntityQuery, 0, len(filters))
	for _, f := range filters {
		queries = append(queries, f.Rethink)
	}
	return queries, nil
}

func mapRethinkError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errorutil.IsNotFound(err):
		return fmt.Errorf("%w: %v", ErrNotFound, err)
	case errorutil.IsConflict(err):
		return fmt.Errorf("%w: %v", ErrAlreadyExists, err)
	case errorutil.IsAborted(err):
		return fmt.Errorf("%w: %v", ErrConflict, err)
	default:
		return err
	}
}
