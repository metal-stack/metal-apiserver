// Package migrations contains migrations that read entities from the legacy
// RethinkDB datastore and write them into the new Postgres datastore.
//
// Each entity is migrated separately so that a migration can be rolled out
// (and validated) independently of the others. Migrations are intended to be
// idempotent: they read the current set of entities from RethinkDB and upsert
// them into Postgres, so re-running them on a partially migrated datastore
// converges to the same end state.
package migrations

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"

	"uuid"

	"github.com/metal-stack/metal-apiserver/pkg/db/generic"
	"github.com/metal-stack/metal-apiserver/pkg/db/generic/pg"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
)

// MigrateMachine reads every metal.Machine from the RethinkDB datastore and
// stores it in the Postgres datastore using the same machine UUID as the
// Postgres row id.
func MigrateMachine(ctx context.Context, log *slog.Logger, rdb generic.Datastore, pgdb *sql.DB) error {
	repo, err := pg.NewGenericRepository[*metal.Machine](log, pgdb)
	if err != nil {
		return fmt.Errorf("unable to create postgres machine repository: %w", err)
	}

	machines, err := rdb.Machine().List(ctx)
	if err != nil {
		return fmt.Errorf("unable to list machines from rethinkdb: %w", err)
	}

	for _, m := range machines {
		if err := storeMachine(ctx, repo, m); err != nil {
			return fmt.Errorf("unable to migrate machine %q: %w", m.GetID(), err)
		}
	}

	log.Info("migrated machines", "count", len(machines))
	return nil
}

func storeMachine(ctx context.Context, repo *pg.GenericRepository[*metal.Machine], m *metal.Machine) error {
	id, err := uuid.Parse(m.GetID())
	if err != nil {
		return fmt.Errorf("machine id %q is not a valid uuid: %w", m.GetID(), err)
	}

	if err := repo.Create(ctx, id, m); err != nil {
		return err
	}
	return nil
}
