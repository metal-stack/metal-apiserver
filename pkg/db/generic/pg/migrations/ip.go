package migrations

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"

	"github.com/metal-stack/metal-apiserver/pkg/db/generic"
	"github.com/metal-stack/metal-apiserver/pkg/db/generic/pg"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
)

// MigrateIP reads every metal.IP from the RethinkDB datastore and upserts it
// into the Postgres datastore using the same ip address as the Postgres row id.
// Re-running it converges ips whose data changed in RethinkDB in between;
// unchanged ips keep their version.
func MigrateIP(ctx context.Context, log *slog.Logger, rdb generic.Datastore, pgdb *sql.DB) error {
	repo, err := pg.NewGenericRepository[*metal.IP](log, pgdb)
	if err != nil {
		return fmt.Errorf("unable to create postgres ip repository: %w", err)
	}

	ips, err := rdb.IP().List(ctx)
	if err != nil {
		return fmt.Errorf("unable to list ips from rethinkdb: %w", err)
	}

	for _, ip := range ips {
		if err := repo.Upsert(ctx, ip.GetID(), ip); err != nil {
			return fmt.Errorf("unable to migrate ip %q: %w", ip.GetID(), err)
		}
	}

	log.Info("migrated ips", "count", len(ips))
	return nil
}
