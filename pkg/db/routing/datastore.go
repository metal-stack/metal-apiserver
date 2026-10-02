package routing

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"

	"github.com/metal-stack/api/go/errorutil"
	"github.com/metal-stack/metal-apiserver/pkg/db/generic"
	"github.com/metal-stack/metal-apiserver/pkg/db/generic/pg"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
)

// Datastore wires per-entity Routers into the generic.Datastore interface, so
// the repository and the test harness can run against RethinkDB, Postgres, or
// both, purely by configuration.
//
// It embeds the RethinkDB datastore for everything that is not entity storage
// (integer pools, the shared mutex, version reporting, table names). The
// RethinkDB backend is therefore required, which matches the transition period:
// the router's default read backend is RethinkDB and RethinkDB stays
// authoritative until an entity is flipped.
type Datastore struct {
	generic.Datastore

	ip                  Storage[*metal.IP]
	machine             Storage[*metal.Machine]
	size                Storage[*metal.Size]
	sizeImageConstraint Storage[*metal.SizeImageConstraint]
	sizeReservation     Storage[*metal.SizeReservation]
	partition           Storage[*metal.Partition]
	network             Storage[*metal.Network]
	filesystemLayout    Storage[*metal.FilesystemLayout]
	image               Storage[*metal.Image]
	sw                  Storage[*metal.Switch]
	switchStatus        Storage[*metal.SwitchStatus]
	event               Storage[*metal.ProvisioningEventContainer]
}

// DatastoreOption customizes how a routing Datastore is built.
type DatastoreOption func(*datastoreOptions)

type datastoreOptions struct {
	watcher *pg.Watcher
}

// WithPostgresWatcher attaches a PostgreSQL watcher to the postgres-backed
// entities, enabling Watch for them. The watcher must be created with
// pg.NewWatcher and be closed by the caller.
func WithPostgresWatcher(w *pg.Watcher) DatastoreOption {
	return func(o *datastoreOptions) {
		o.watcher = w
	}
}

// NewDatastore builds a configuration-driven generic.Datastore. pgDB is only
// required (and may only be nil) when at least one entity is configured for
// "postgres" or "both".
func NewDatastore(log *slog.Logger, cfg Config, rethink generic.Datastore, pgDB *sql.DB, opts ...DatastoreOption) (*Datastore, error) {
	if rethink == nil {
		return nil, errors.New("routing datastore requires a rethinkdb datastore")
	}

	options := &datastoreOptions{}
	for _, opt := range opts {
		opt(options)
	}

	d := &Datastore{Datastore: rethink}

	var err error
	if d.ip, err = newEntityStorage(log, cfg, "IP", rethink.IP(), pgDB, options.watcher); err != nil {
		return nil, err
	}
	if d.machine, err = newEntityStorage(log, cfg, "Machine", rethink.Machine(), pgDB, options.watcher); err != nil {
		return nil, err
	}
	if d.size, err = newEntityStorage(log, cfg, "Size", rethink.Size(), pgDB, options.watcher); err != nil {
		return nil, err
	}
	if d.sizeImageConstraint, err = newEntityStorage(log, cfg, "SizeImageConstraint", rethink.SizeImageConstraint(), pgDB, options.watcher); err != nil {
		return nil, err
	}
	if d.sizeReservation, err = newEntityStorage(log, cfg, "SizeReservation", rethink.SizeReservation(), pgDB, options.watcher); err != nil {
		return nil, err
	}
	if d.partition, err = newEntityStorage(log, cfg, "Partition", rethink.Partition(), pgDB, options.watcher); err != nil {
		return nil, err
	}
	if d.network, err = newEntityStorage(log, cfg, "Network", rethink.Network(), pgDB, options.watcher); err != nil {
		return nil, err
	}
	if d.filesystemLayout, err = newEntityStorage(log, cfg, "FilesystemLayout", rethink.FilesystemLayout(), pgDB, options.watcher); err != nil {
		return nil, err
	}
	if d.image, err = newEntityStorage(log, cfg, "Image", rethink.Image(), pgDB, options.watcher); err != nil {
		return nil, err
	}
	if d.sw, err = newEntityStorage(log, cfg, "Switch", rethink.Switch(), pgDB, options.watcher); err != nil {
		return nil, err
	}
	if d.switchStatus, err = newEntityStorage(log, cfg, "SwitchStatus", rethink.SwitchStatus(), pgDB, options.watcher); err != nil {
		return nil, err
	}
	if d.event, err = newEntityStorage(log, cfg, "Event", rethink.Event(), pgDB, options.watcher); err != nil {
		return nil, err
	}

	return d, nil
}

func newEntityStorage[E generic.Entity](log *slog.Logger, cfg Config, entity string, rethink generic.Storage[E], pgDB *sql.DB, watcher *pg.Watcher) (Storage[E], error) {
	var postgres Storage[E]
	if cfg.ModeFor(entity) != ModeRethink {
		if pgDB == nil {
			return nil, fmt.Errorf("entity %q is configured for postgres but no postgres database was provided", entity)
		}
		repo, err := pg.NewGenericRepository[E](log, pgDB, pg.WithWatcher(watcher))
		if err != nil {
			return nil, fmt.Errorf("unable to create postgres repository for %q: %w", entity, err)
		}
		postgres = NewPostgresStorage(repo)
	}

	return NewRouter[E](log, cfg, entity, NewRethinkStorage(rethink), postgres)
}

func (d *Datastore) IP() generic.Storage[*metal.IP] {
	return genericStorage[*metal.IP]{port: d.ip, name: "ip"}
}
func (d *Datastore) Machine() generic.Storage[*metal.Machine] {
	return genericStorage[*metal.Machine]{port: d.machine, name: "machine"}
}
func (d *Datastore) Size() generic.Storage[*metal.Size] {
	return genericStorage[*metal.Size]{port: d.size, name: "size"}
}
func (d *Datastore) SizeImageConstraint() generic.Storage[*metal.SizeImageConstraint] {
	return genericStorage[*metal.SizeImageConstraint]{port: d.sizeImageConstraint, name: "sizeimageconstraint"}
}
func (d *Datastore) SizeReservation() generic.Storage[*metal.SizeReservation] {
	return genericStorage[*metal.SizeReservation]{port: d.sizeReservation, name: "sizereservation"}
}
func (d *Datastore) Partition() generic.Storage[*metal.Partition] {
	return genericStorage[*metal.Partition]{port: d.partition, name: "partition"}
}
func (d *Datastore) Network() generic.Storage[*metal.Network] {
	return genericStorage[*metal.Network]{port: d.network, name: "network"}
}
func (d *Datastore) FilesystemLayout() generic.Storage[*metal.FilesystemLayout] {
	return genericStorage[*metal.FilesystemLayout]{port: d.filesystemLayout, name: "filesystemlayout"}
}
func (d *Datastore) Image() generic.Storage[*metal.Image] {
	return genericStorage[*metal.Image]{port: d.image, name: "image"}
}
func (d *Datastore) Switch() generic.Storage[*metal.Switch] {
	return genericStorage[*metal.Switch]{port: d.sw, name: "switch"}
}
func (d *Datastore) SwitchStatus() generic.Storage[*metal.SwitchStatus] {
	return genericStorage[*metal.SwitchStatus]{port: d.switchStatus, name: "switchstatus"}
}
func (d *Datastore) Event() generic.Storage[*metal.ProvisioningEventContainer] {
	return genericStorage[*metal.ProvisioningEventContainer]{port: d.event, name: "event"}
}

// genericStorage adapts the routing port back to the generic.Storage interface
// the repository consumes. Query closures are the RethinkDB representation, so a
// filtered read only works while the entity reads from RethinkDB; unfiltered
// reads and id-based operations work in any mode.
type genericStorage[E generic.Entity] struct {
	port Storage[E]
	// name is the legacy rethinkdb table name. It is used to reproduce the
	// exact not-found error messages of the generic rethinkdb storage.
	name string
}

var (
	_ generic.Storage[*metal.Machine] = genericStorage[*metal.Machine]{}
	_ FilteredStorage[*metal.Machine] = genericStorage[*metal.Machine]{}
)

func (g genericStorage[E]) Create(ctx context.Context, e E) (E, error) {
	created, err := g.port.Create(ctx, e)
	return created, toGenericError(err)
}

func (g genericStorage[E]) Update(ctx context.Context, e E) error {
	return toGenericError(g.port.Update(ctx, e))
}

func (g genericStorage[E]) Upsert(ctx context.Context, e E) error {
	return toGenericError(g.port.Upsert(ctx, e))
}

func (g genericStorage[E]) Delete(ctx context.Context, e E) error {
	return toGenericError(g.port.Delete(ctx, e))
}

func (g genericStorage[E]) Get(ctx context.Context, id string) (E, error) {
	e, err := g.port.Get(ctx, id)
	if errors.Is(err, ErrNotFound) {
		var zero E
		return zero, errorutil.NotFound("no %v with id %q found", g.name, id)
	}
	return e, toGenericError(err)
}

func (g genericStorage[E]) Find(ctx context.Context, queries ...generic.EntityQuery) (E, error) {
	e, err := g.port.Find(ctx, toFilters(queries)...)
	if errors.Is(err, ErrNotFound) {
		var zero E
		return zero, errorutil.NotFound("no %v found", g.name)
	}
	return e, toGenericError(err)
}

func (g genericStorage[E]) List(ctx context.Context, queries ...generic.EntityQuery) ([]E, error) {
	entities, err := g.port.List(ctx, toFilters(queries)...)
	return entities, toGenericError(err)
}

func (g genericStorage[E]) Watch(ctx context.Context, id string) (<-chan struct {
	Old E
	New E
}, error) {
	changes, err := g.port.Watch(ctx, id)
	return changes, toGenericError(err)
}

// FindFiltered and ListFiltered implement FilteredStorage: the caller supplies
// filters that carry a representation for both backends, so the read works on
// either.
func (g genericStorage[E]) FindFiltered(ctx context.Context, filters ...Filter) (E, error) {
	e, err := g.port.Find(ctx, filters...)
	if errors.Is(err, ErrNotFound) {
		var zero E
		return zero, errorutil.NotFound("no %v found", g.name)
	}
	return e, toGenericError(err)
}

func (g genericStorage[E]) ListFiltered(ctx context.Context, filters ...Filter) ([]E, error) {
	entities, err := g.port.List(ctx, filters...)
	return entities, toGenericError(err)
}

func toFilters(queries []generic.EntityQuery) []Filter {
	filters := make([]Filter, 0, len(queries))
	for _, q := range queries {
		if q == nil {
			continue
		}
		filters = append(filters, Filter{Rethink: q})
	}
	return filters
}

// toGenericError maps the backend-agnostic port errors back to the connect
// errors the repository and services expect.
func toGenericError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ErrNotFound):
		return errorutil.NotFound("%v", err)
	case errors.Is(err, ErrAlreadyExists):
		return errorutil.Conflict("%v", err)
	case errors.Is(err, ErrConflict):
		return errorutil.Aborted("%v", err)
	default:
		return err
	}
}
