// Package storage defines the backend-agnostic Entity Storage port: the seam a
// Repository uses to persist and retrieve Entities, independent of which
// database serves it.
//
// A Storage implementation returns the connect errors callers expect (the same
// contract the historic generic.Storage had), naming the entity in the message.
// Each backend adapter maps its own error vocabulary to them. A Filter carries
// the backend-specific representation(s) of a query, so the port itself stays
// free of any database query language.
package storage

import (
	"context"
	"errors"
	"time"
)

var (
	// ErrFilterUnsupported is returned when a query does not carry a filter
	// representation for the backend that serves the read.
	ErrFilterUnsupported = errors.New("filter is not available for the read backend")
)

// Entity is the persistence contract an Entity satisfies so that it can be
// stored and retrieved generically.
type Entity interface {
	// GetID returns the entity's id.
	GetID() string
	// SetID sets the entity's id.
	SetID(id string)
	// GetChanged returns the entity's changed time.
	GetChanged() time.Time
	// GetCreated returns the entity's creation time.
	GetCreated() time.Time
	// GetGeneration returns the entity's generation.
	GetGeneration() uint64
}

// Filter carries the backend-specific representation of a query. A caller
// builds the side(s) it needs; a routed read requires the representation of the
// backend that serves reads. The payloads are opaque to this package: the
// backend adapters type-assert the representation they understand.
//
// The representations are produced by the backend-specific query packages:
//
//	Filter{
//	    Rethink:  generic.EntityQuery(queries.MachineFilter(rq)), // pkg/db/queries
//	    Postgres: q.MachineFilter(rq),                            // pkg/db/generic/pg/q
//	}
//
// The Rethink payload is a generic.EntityQuery and the Postgres payload is a
// []pg.QueryFilter; both are held as any so this package depends on neither
// backend.
type Filter struct {
	Rethink  any
	Postgres any
}

// Storage is the backend-agnostic port for metal Entities. Every backend
// implements it and a Router dispatches to one of them.
type Storage[E Entity] interface {
	Create(ctx context.Context, e E) (E, error)
	Update(ctx context.Context, e E) error
	Upsert(ctx context.Context, e E) error
	Delete(ctx context.Context, e E) error
	Get(ctx context.Context, id string) (E, error)
	Find(ctx context.Context, filters ...Filter) (E, error)
	List(ctx context.Context, filters ...Filter) ([]E, error)
	Watch(ctx context.Context, id string) (<-chan struct {
		Old E
		New E
	}, error)
}
