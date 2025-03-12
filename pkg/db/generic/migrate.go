package generic

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"sort"

	r "gopkg.in/rethinkdb/rethinkdb-go.v6"
)

type (
	// Migration defines a database migration
	Migration struct {
		Name    string
		Version int
		Up      migrateFunc
	}

	// migrationVersionEntry is a version entry in the migration database
	migrationVersionEntry struct {
		Version int    `rethinkdb:"id"`
		Name    string `rethinkdb:"name"`
	}

	// migrateFunc is a function that contains database migration logic
	migrateFunc func(ctx context.Context, db *r.Term, session r.QueryExecutor, ds Datastore) error

	// migrations is a list of migrations
	migrations []Migration
)

const (
	migrationTableName = "migration"
)

var (
	ms                    migrations
	migrationRegisterLock sync.Mutex
)

// MustRegisterMigration registers a migration and panics when a problem occurs
func MustRegisterMigration(m Migration) {
	if m.Version < 1 {
		panic(fmt.Sprintf("migrations should start from version number '1', but found version %q", m.Version))
	}
	migrationRegisterLock.Lock()
	defer migrationRegisterLock.Unlock()
	for _, migration := range ms {
		if migration.Version == m.Version {
			panic(fmt.Sprintf("migration with version %d is defined multiple times", m.Version))
		}
	}
	ms = append(ms, m)
}

// Migrate runs database migrations and puts the database into read only mode for demoted runtime users.
func Migrate(ctx context.Context, opts r.ConnectOpts, log *slog.Logger, targetVersion *int, dry bool) error {
	migrationTable := r.DB(opts.Database).Table(migrationTableName)

	session, err := r.Connect(opts)
	if err != nil {
		return err
	}

	ds, err := New(log, opts)
	if err != nil {
		return err
	}

	ds.queryExecutor = session // the metal user cannot create tables

	_, err = migrationTable.Insert(migrationVersionEntry{Version: 0}, r.InsertOpts{
		Conflict: "replace",
	}).RunWrite(session, r.RunOpts{Context: ctx})
	if err != nil {
		return err
	}

	results, err := migrationTable.Max().Run(session, r.RunOpts{Context: ctx})
	if err != nil {
		return err
	}
	defer results.Close()

	var current migrationVersionEntry
	err = results.One(&current)
	if err != nil {
		return err
	}

	if targetVersion != nil && *targetVersion < current.Version {
		return fmt.Errorf("target version (=%d) smaller than current version (=%d) and down migrations not supported", *targetVersion, current.Version)
	}
	ms, err := ms.between(current.Version, targetVersion)
	if err != nil {
		return err
	}

	if len(ms) == 0 {
		log.Info("no database migration required", "current-version", current.Version)
		return nil
	}

	log.Info("database migration required", "current-version", current.Version, "newer-versions", len(ms), "target-version", ms[len(ms)-1].Version)

	if dry {
		for _, m := range ms {
			log.Info("database migration dry run", "version", m.Version, "name", m.Name)
		}
		return nil
	}

	db := r.DB(opts.Database)

	log.Info("setting demoted runtime user to read only", "user", demotedUser)
	_, err = db.Grant(demotedUser, map[string]interface{}{"read": true, "write": false}).RunWrite(ds.queryExecutor)
	if err != nil {
		return err
	}
	defer func() {
		log.Info("removing read only", "user", demotedUser)
		_, err = db.Grant(demotedUser, map[string]interface{}{"read": true, "write": true}).RunWrite(ds.queryExecutor)
		if err != nil {
			log.Error("error giving back write permissions", "user", demotedUser)
		}
	}()

	for _, m := range ms {
		log.Info("running database migration", "version", m.Version, "name", m.Name)
		err = m.Up(ctx, &db, session, ds)
		if err != nil {
			return fmt.Errorf("error running database migration: %w", err)
		}

		_, err := migrationTable.Insert(migrationVersionEntry{Version: m.Version, Name: m.Name}, r.InsertOpts{
			Conflict: "replace",
		}).RunWrite(ds.queryExecutor)
		if err != nil {
			return fmt.Errorf("error updating database migration version: %w", err)
		}
	}

	ds.log.Info("database migration succeeded")

	return nil
}

// between returns a sorted slice of migrations that are between the given current version
// and target version (target version contained). If target version is nil all newer versions
// than current are contained in the slice.
func (ms migrations) between(current int, target *int) (migrations, error) {
	var result migrations
	targetFound := false
	for _, m := range ms {
		if target != nil {
			if m.Version > *target {
				continue
			}
			if m.Version == *target {
				targetFound = true
			}
		}

		if m.Version <= current {
			continue
		}

		result = append(result, m)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Version < result[j].Version
	})

	if target != nil && !targetFound {
		return nil, errors.New("target version not found")
	}

	return result, nil
}
