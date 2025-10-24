package generic

import (
	"context"
	"fmt"
	"log/slog"

	r "gopkg.in/rethinkdb/rethinkdb-go.v6"
)

type dataStoreOption any

type vrfPoolRange struct {
	min, max uint
}

func VrfPoolRange(min, max uint) dataStoreOption {
	return &vrfPoolRange{
		min: min,
		max: max,
	}
}

type asnPoolRange struct {
	min, max uint
}

func AsnPoolRange(min, max uint) dataStoreOption {
	return &asnPoolRange{
		min: min,
		max: max,
	}
}

func Initialize(ctx context.Context, log *slog.Logger, opts r.ConnectOpts, dsOpts ...dataStoreOption) error {
	db := r.DB(opts.Database)

	session, err := r.Connect(opts)
	if err != nil {
		return fmt.Errorf("unable to connect to database: %w", err)
	}

	log.Info("creating / updating runtime user metal")

	_, err = r.DB("rethinkdb").Table("users").Insert(map[string]any{"id": demotedUser, "password": opts.Password}, r.InsertOpts{
		Conflict: "update",
	}).RunWrite(session, r.RunOpts{Context: ctx})
	if err != nil {
		return fmt.Errorf("unable to ensure runtime user metal: %w", err)
	}

	log.Info("initializing database", "database", opts.Database)

	err = r.DBList().Contains(opts.Database).Do(func(row r.Term) r.Term {
		return r.Branch(row, nil, r.DBCreate(opts.Database))
	}).Exec(session, r.ExecOpts{Context: ctx})
	if err != nil {
		return fmt.Errorf("cannot create database: %w", err)
	}

	log.Info("ensuring demoted user can read and write")

	_, err = db.Grant(demotedUser, map[string]any{"read": true, "write": true}).RunWrite(session, r.RunOpts{Context: ctx})
	if err != nil {
		return fmt.Errorf("unable to grant read / write permissions to metal user on database %s: %w", opts.Database, err)
	}
	_, err = r.DB("rethinkdb").Grant(demotedUser, map[string]any{"read": true}).RunWrite(session, r.RunOpts{Context: ctx})
	if err != nil {
		return fmt.Errorf("unable to grant read / write permissions to metal user on rethinkdb database: %w", err)
	}

	log.Info("initializing tables")

	ds, err := New(log, opts, dsOpts...)
	if err != nil {
		return fmt.Errorf("unable to create datastore: %w", err)
	}

	ds.queryExecutor = session // the metal user cannot create tables

	err = ds.createTable(ctx, migrationTableName)
	if err != nil {
		return fmt.Errorf("cannot create migration table: %w", err)
	}

	if err := ds.ip.initialize(ctx); err != nil {
		return fmt.Errorf("unable to initialize ip datastore: %w", err)
	}
	if err := ds.machine.initialize(ctx); err != nil {
		return fmt.Errorf("unable to initialize machine datastore: %w", err)
	}
	if err := ds.partition.initialize(ctx); err != nil {
		return fmt.Errorf("unable to initialize partition datastore: %w", err)
	}
	if err := ds.size.initialize(ctx); err != nil {
		return fmt.Errorf("unable to initialize size datastore: %w", err)
	}
	if err := ds.network.initialize(ctx); err != nil {
		return fmt.Errorf("unable to initialize network datastore: %w", err)
	}
	if err := ds.fsl.initialize(ctx); err != nil {
		return fmt.Errorf("unable to initialize fsl datastore: %w", err)
	}
	if err := ds.image.initialize(ctx); err != nil {
		return fmt.Errorf("unable to initialize image datastore: %w", err)
	}
	if err := ds.event.initialize(ctx); err != nil {
		return fmt.Errorf("unable to initialize event datastore: %w", err)
	}
	if err := ds.sw.initialize(ctx); err != nil {
		return fmt.Errorf("unable to initialize switch datastore: %w", err)
	}
	if err := ds.switchStatus.initialize(ctx); err != nil {
		return fmt.Errorf("unable to initialize switch status datastore: %w", err)
	}

	ds.log.Info("waiting for tables to be ready")

	// be graceful after table creation and wait until ready
	res, err := db.Wait().Run(session, r.RunOpts{Context: ctx})
	if err != nil {
		return fmt.Errorf("unable to wait for database creation")
	}
	defer func() {
		if err := res.Close(); err != nil {
			ds.log.Error("unable to close database connection", "error", err)
		}
	}()

	ds.log.Info("initializing pools")

	if err := ds.asnPool.initialize(); err != nil {
		return fmt.Errorf("unable to initialize asn pool: %w", err)
	}
	if err := ds.vrfPool.initialize(); err != nil {
		return fmt.Errorf("unable to initialize vrf pool: %w", err)
	}

	ds.log.Info("database init complete")

	return nil
}

func (ds *datastore) createTable(ctx context.Context, tableName string) error {
	ds.log.Info("init table", "db", ds.dbname, "table", tableName)

	err := r.DB(ds.dbname).TableList().Contains(tableName).Do(func(row r.Term) r.Term {
		return r.Branch(row, nil, r.DB(ds.dbname).TableCreate(tableName, r.TableCreateOpts{Shards: 1, Replicas: 1}))
	}).Exec(ds.queryExecutor, r.ExecOpts{Context: ctx})
	if err != nil {
		return fmt.Errorf("cannot create table %s: %w", tableName, err)
	}

	return nil
}
