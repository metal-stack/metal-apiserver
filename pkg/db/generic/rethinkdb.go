package generic

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/metal-stack/api-server/pkg/db/metal"
	r "gopkg.in/rethinkdb/rethinkdb-go.v6"
)

const entityAlreadyModifiedErrorMessage = "the entity was changed from another, please retry"

type (
	// Entity is an interface that allows metal entities to be created and stored
	// into the database with the generic creation and update functions.
	//
	// see https://go.googlesource.com/proposal/+/HEAD/design/43651-type-parameters.md#pointer-method-example for possible solution to prevent slices of pointers.
	Entity interface {
		// GetID returns the entity's id
		GetID() string
		// SetID sets the entity's id
		SetID(id string)
		// GetChanged returns the entity's changed time
		GetChanged() time.Time
		// SetChanged sets the entity's changed time
		SetChanged(changed time.Time)
		// GetCreated sets the entity's creation time
		GetCreated() time.Time
		// SetCreated sets the entity's creation time
		SetCreated(created time.Time)
	}

	EntityQuery func(q r.Term) r.Term

	Storage[E Entity] interface {
		Create(ctx context.Context, e E) (E, error)
		Update(ctx context.Context, new, old E) error
		Upsert(ctx context.Context, e E) error
		Delete(ctx context.Context, e E) error
		Get(ctx context.Context, id string) (E, error)
		Find(ctx context.Context, queries ...EntityQuery) (E, error)
		List(ctx context.Context, queries ...EntityQuery) ([]E, error)
	}

	Datastore struct {
		ip               Storage[*metal.IP]
		partition        Storage[*metal.Partition]
		network          Storage[*metal.Network]
		filesystemlayout Storage[*metal.FilesystemLayout]
		// event               Storage[*metal.ProvisioningEventContainer]
		// image               Storage[*metal.Image]
		// machine             Storage[*metal.Machine]
		// size                Storage[*metal.Size]
		// sizeimageConstraint Storage[*metal.SizeImageConstraint]
		// sw                  Storage[*metal.Switch]
		// switchStatus        Storage[*metal.SwitchStatus]
	}

	rethinkStore[E Entity] struct {
		log           *slog.Logger
		queryExecutor r.QueryExecutor
		dbname        string
		table         r.Term
		tableName     string
	}
)

func New(log *slog.Logger, dbname string, queryExecutor r.QueryExecutor) (*Datastore, error) {
	// Create the database
	err := r.DBList().Contains(dbname).Do(func(row r.Term) r.Term {
		return r.Branch(row, nil, r.DBCreate(dbname))
	}).Exec(queryExecutor)
	if err != nil {
		return nil, fmt.Errorf("cannot create database: %w", err)
	}

	// create tables
	// TODO loop over them
	ip, err := newStorage[*metal.IP](log, dbname, "ip", queryExecutor)
	if err != nil {
		return nil, err
	}
	network, err := newStorage[*metal.Network](log, dbname, "network", queryExecutor)
	if err != nil {
		return nil, err
	}
	partition, err := newStorage[*metal.Partition](log, dbname, "partition", queryExecutor)
	if err != nil {
		return nil, err
	}
	filesystemlayout, err := newStorage[*metal.FilesystemLayout](log, dbname, "filesystemlayout", queryExecutor)
	if err != nil {
		return nil, err
	}
	return &Datastore{
		ip:               ip,
		partition:        partition,
		network:          network,
		filesystemlayout: filesystemlayout,
		// event:               newStorage[*metal.ProvisioningEventContainer](log, dbname, "event", queryExecutor),
		// image:               newStorage[*metal.Image](log, dbname, "image", queryExecutor),
		// machine:             newStorage[*metal.Machine](log, dbname, "machine", queryExecutor),
		// size:                newStorage[*metal.Size](log, dbname, "size", queryExecutor),
		// sizeimageConstraint: newStorage[*metal.SizeImageConstraint](log, dbname, "sizeimageconstraint", queryExecutor),
		// sw:                  newStorage[*metal.Switch](log, dbname, "switch", queryExecutor),
		// switchStatus:        newStorage[*metal.SwitchStatus](log, dbname, "switchstatus", queryExecutor),
	}, nil
}

func (d *Datastore) IP() Storage[*metal.IP] {
	return d.ip
}
func (d *Datastore) Network() Storage[*metal.Network] {
	return d.network
}
func (d *Datastore) Partition() Storage[*metal.Partition] {
	return d.partition
}
func (d *Datastore) FilesystemLayout() Storage[*metal.FilesystemLayout] {
	return d.filesystemlayout
}

// newStorage creates a new Storage which uses the given database abstraction.
func newStorage[E Entity](log *slog.Logger, dbname, tableName string, queryExecutor r.QueryExecutor) (Storage[E], error) {
	ds := &rethinkStore[E]{
		log:           log,
		queryExecutor: queryExecutor,
		dbname:        dbname,
		table:         r.DB(dbname).Table(tableName),
		tableName:     tableName,
	}

	err := ds.initialize()
	if err != nil {
		return nil, err
	}

	return ds, nil
}

// Create creates the given entity in the database. in case it is already present, a conflict error will be returned.
//
// if the ID field of the entity is an empty string, the ID will be generated automatically.
func (rs *rethinkStore[E]) Create(ctx context.Context, e E) (E, error) {
	now := time.Now()
	e.SetCreated(now)
	e.SetChanged(now)

	var zero E
	res, err := rs.table.Insert(e).RunWrite(rs.queryExecutor, r.RunOpts{Context: ctx})
	if err != nil {
		if r.IsConflictErr(err) {
			return zero, Conflict("cannot create %v in database, entity already exists: %s", rs.tableName, e.GetID())
		}
		return zero, fmt.Errorf("cannot create %v in database: %w", rs.tableName, err)
	}

	if e.GetID() == "" && len(res.GeneratedKeys) > 0 {
		e.SetID(res.GeneratedKeys[0])
	}

	return e, nil
}

// Delete deletes the given entity from the database.
func (rs *rethinkStore[E]) Delete(ctx context.Context, e E) error {
	_, err := rs.table.Get(e.GetID()).Delete().RunWrite(rs.queryExecutor, r.RunOpts{Context: ctx})
	if err != nil {
		return fmt.Errorf("cannot delete %v with id %q from database: %w", rs.tableName, e.GetID(), err)
	}

	return nil
}

// Find attempts to find a single entity from the given set of queries.
//
// if either none or more than one entities were found by the query, an error gets returned.
func (rs *rethinkStore[E]) Find(ctx context.Context, queries ...EntityQuery) (E, error) {
	query := rs.table
	for _, q := range queries {
		query = q(query)
	}

	var zero E
	res, err := query.Run(rs.queryExecutor, r.RunOpts{Context: ctx})
	if err != nil {
		return zero, fmt.Errorf("cannot find %v in database: %w", rs.tableName, err)
	}
	defer res.Close()
	if res.IsNil() {
		return zero, NotFound("no %v with found", rs.tableName)
	}

	e := new(E)
	hasResult := res.Next(e)
	if !hasResult {
		return zero, fmt.Errorf("cannot find %v in database: %w", rs.tableName, err)
	}

	next := new(E)
	hasResult = res.Next(&next)
	if hasResult {
		return zero, fmt.Errorf("more than one %v exists", rs.tableName)
	}

	return *e, nil
}

// List returns all entities present in the database, optionally filtered by the given set of queries.
func (rs *rethinkStore[E]) List(ctx context.Context, queries ...EntityQuery) ([]E, error) {
	query := rs.table
	for _, q := range queries {
		query = q(query)
	}

	rs.log.Debug("list", "table", rs.table, "query", query.String())

	res, err := query.Run(rs.queryExecutor, r.RunOpts{Context: ctx})
	if err != nil {
		return nil, fmt.Errorf("cannot search %v in database: %w", rs.tableName, err)
	}
	defer res.Close()

	result := new([]E)

	err = res.All(result)
	if err != nil {
		return nil, fmt.Errorf("cannot fetch all entities: %w", err)
	}

	return *result, nil
}

// Get returns the entity of the given ID  from the database.
func (rs *rethinkStore[E]) Get(ctx context.Context, id string) (E, error) {
	var zero E
	res, err := rs.table.Get(id).Run(rs.queryExecutor, r.RunOpts{Context: ctx})
	if err != nil {
		return zero, fmt.Errorf("cannot find %v with id %q in database: %w", rs.tableName, id, err)
	}

	defer res.Close()
	if res.IsNil() {
		return zero, NotFound("no %v with id %q found", rs.tableName, id)
	}

	e := new(E)
	err = res.One(e)
	if err != nil {
		return zero, fmt.Errorf("more than one %v with same id exists: %w", rs.tableName, err)
	}

	return *e, nil
}

// Update updates the entity to the contents of the new entity.
//
// it uses the "changed" timestamp of the old entity to figure out if it was already modified by some other process.
// if this happens a conflict error will be returned.
func (rs *rethinkStore[E]) Update(ctx context.Context, new, old E) error {
	new.SetChanged(time.Now())

	_, err := rs.table.Get(old.GetID()).Replace(func(row r.Term) r.Term {
		return r.Branch(row.Field("changed").Eq(r.Expr(old.GetChanged())), new, r.Error(entityAlreadyModifiedErrorMessage))
	}).RunWrite(rs.queryExecutor, r.RunOpts{Context: ctx})
	if err != nil {
		if strings.Contains(err.Error(), entityAlreadyModifiedErrorMessage) {
			return Conflict("cannot update %v (%s): %s", rs.tableName, old.GetID(), entityAlreadyModifiedErrorMessage)
		}

		return fmt.Errorf("cannot update %v (%s): %w", rs.tableName, old.GetID(), err)
	}

	return nil
}

// Upsert inserts the given entity into the database, replacing it completely if it is already present.
func (rs *rethinkStore[E]) Upsert(ctx context.Context, e E) error {
	now := time.Now()
	if e.GetCreated().IsZero() {
		e.SetCreated(now)
	}
	e.SetChanged(now)

	res, err := rs.table.Insert(e, r.InsertOpts{
		Conflict: "replace",
	}).RunWrite(rs.queryExecutor)
	if err != nil {
		return fmt.Errorf("cannot upsert %v (%s) in database: %w", rs.tableName, e.GetID(), err)
	}

	if e.GetID() == "" && len(res.GeneratedKeys) > 0 {
		e.SetID(res.GeneratedKeys[0])
	}

	return nil
}

// initialize initializes the database, it should be called before serving the metal-api
// in order to ensure that tables, pools, permissions are properly initialized
func (rs *rethinkStore[E]) initialize() error {
	return rs.initializeTable(r.TableCreateOpts{Shards: 1, Replicas: 1})
}

func (rs *rethinkStore[E]) initializeTable(opts r.TableCreateOpts) error {
	rs.log.Info("starting database init", "table", rs.tableName)

	err := r.DB(rs.dbname).TableList().Contains(rs.tableName).Do(func(row r.Term) r.Term {
		return r.Branch(row, nil, r.DB(rs.dbname).TableCreate(rs.tableName, opts))
	}).Exec(rs.queryExecutor)
	if err != nil {
		return fmt.Errorf("cannot create table %s %w", rs.tableName, err)
	}

	return nil
}
