package routing

import (
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/metal-stack/metal-apiserver/pkg/db/generic"
	"github.com/metal-stack/metal-apiserver/pkg/db/generic/pg"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-apiserver/pkg/db/storage"
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

	ip                  storage.Storage[*metal.IP]
	machine             storage.Storage[*metal.Machine]
	size                storage.Storage[*metal.Size]
	sizeImageConstraint storage.Storage[*metal.SizeImageConstraint]
	sizeReservation     storage.Storage[*metal.SizeReservation]
	partition           storage.Storage[*metal.Partition]
	network             storage.Storage[*metal.Network]
	filesystemLayout    storage.Storage[*metal.FilesystemLayout]
	image               storage.Storage[*metal.Image]
	sw                  storage.Storage[*metal.Switch]
	switchStatus        storage.Storage[*metal.SwitchStatus]
	event               storage.Storage[*metal.ProvisioningEventContainer]
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

func newEntityStorage[E storage.Entity](log *slog.Logger, cfg Config, entity string, rethink storage.Storage[E], pgDB *sql.DB, watcher *pg.Watcher) (storage.Storage[E], error) {
	var postgres storage.Storage[E]
	if cfg.ModeFor(entity) != ModeRethink {
		if pgDB == nil {
			return nil, fmt.Errorf("entity %q is configured for postgres but no postgres database was provided", entity)
		}
		repo, err := pg.NewGenericRepository[E](log, pgDB, pg.WithWatcher(watcher))
		if err != nil {
			return nil, fmt.Errorf("unable to create postgres repository for %q: %w", entity, err)
		}
		postgres = NewPostgresStorage(repo, strings.ToLower(entity))
	}

	return NewRouter(log, cfg, entity, rethink, postgres)
}

func (d *Datastore) IP() storage.Storage[*metal.IP] {
	return d.ip
}
func (d *Datastore) Machine() storage.Storage[*metal.Machine] {
	return d.machine
}
func (d *Datastore) Size() storage.Storage[*metal.Size] {
	return d.size
}
func (d *Datastore) SizeImageConstraint() storage.Storage[*metal.SizeImageConstraint] {
	return d.sizeImageConstraint
}
func (d *Datastore) SizeReservation() storage.Storage[*metal.SizeReservation] {
	return d.sizeReservation
}
func (d *Datastore) Partition() storage.Storage[*metal.Partition] {
	return d.partition
}
func (d *Datastore) Network() storage.Storage[*metal.Network] {
	return d.network
}
func (d *Datastore) FilesystemLayout() storage.Storage[*metal.FilesystemLayout] {
	return d.filesystemLayout
}
func (d *Datastore) Image() storage.Storage[*metal.Image] {
	return d.image
}
func (d *Datastore) Switch() storage.Storage[*metal.Switch] {
	return d.sw
}
func (d *Datastore) SwitchStatus() storage.Storage[*metal.SwitchStatus] {
	return d.switchStatus
}
func (d *Datastore) Event() storage.Storage[*metal.ProvisioningEventContainer] {
	return d.event
}
