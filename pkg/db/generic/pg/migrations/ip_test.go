package migrations_test

import (
	"context"
	"log/slog"
	"os"
	"testing"

	_ "github.com/lib/pq"
	"github.com/metal-stack/metal-apiserver/pkg/db/generic"
	"github.com/metal-stack/metal-apiserver/pkg/db/generic/pg"
	"github.com/metal-stack/metal-apiserver/pkg/db/generic/pg/migrations"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-apiserver/pkg/test"
	"github.com/stretchr/testify/require"
)

func TestMigrateIP(t *testing.T) {
	ctx := t.Context()
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	rdb, _, rethinkCloser := test.StartRethink(t, log)
	defer rethinkCloser()

	pgdb, pgCloser := test.StartPostgres(t, log)
	defer pgCloser()

	seedIPs(t, ctx, rdb)

	require.NoError(t, migrations.MigrateIP(ctx, log, rdb, pgdb))

	repo, err := pg.NewGenericRepository[*metal.IP](log, pgdb)
	require.NoError(t, err)

	t.Run("migrates all ips preserving data", func(t *testing.T) {
		for _, s := range testIPs {
			ent, err := repo.Get(ctx, s.address)
			require.NoError(t, err)
			require.NotNil(t, ent)
			require.Equal(t, s.projectID, ent.Data.ProjectID)
			require.Equal(t, s.networkID, ent.Data.NetworkID)
			require.Equal(t, s.ipType, ent.Data.Type)
			require.Equal(t, s.tags, ent.Data.Tags)
		}
	})

	t.Run("migrated ips can be queried with the new query format", func(t *testing.T) {
		results, err := repo.Query(ctx, []pg.QueryFilter{
			{Path: "project_id", Op: "=", Value: "project-1"},
		}, nil)
		require.NoError(t, err)
		require.Len(t, results, 2)
	})

	t.Run("migration is idempotent", func(t *testing.T) {
		require.NoError(t, migrations.MigrateIP(ctx, log, rdb, pgdb))

		all, err := repo.Query(ctx, nil, nil)
		require.NoError(t, err)
		require.Len(t, all, len(testIPs))
	})

	t.Run("migration converges changed data on re-run", func(t *testing.T) {
		const ipAddress = "10.0.0.1"

		ip, err := rdb.IP().Get(ctx, ipAddress)
		require.NoError(t, err)
		ip.Name = "ip-1-renamed"
		require.NoError(t, rdb.IP().Update(ctx, ip))

		require.NoError(t, migrations.MigrateIP(ctx, log, rdb, pgdb))

		ent, err := repo.Get(ctx, ipAddress)
		require.NoError(t, err)
		require.Equal(t, "ip-1-renamed", ent.Data.Name)
	})
}

type ipSeed struct {
	address   string
	name      string
	projectID string
	networkID string
	ipType    metal.IPType
	tags      []string
}

var testIPs = []ipSeed{
	{address: "10.0.0.1", name: "ip-1", projectID: "project-1", networkID: "network-1", ipType: metal.Ephemeral, tags: []string{"color=red"}},
	{address: "10.0.0.2", name: "ip-2", projectID: "project-1", networkID: "network-1", ipType: metal.Static, tags: []string{"color=blue"}},
	{address: "2001:db8::1", name: "ip-3", projectID: "project-2", networkID: "network-2", ipType: metal.Ephemeral, tags: []string{}},
}

func seedIPs(t *testing.T, ctx context.Context, ds generic.Datastore) {
	t.Helper()
	for _, s := range testIPs {
		ip := &metal.IP{
			IPAddress:        s.address,
			AllocationUUID:   "uuid-" + s.name,
			Name:             s.name,
			Description:      "desc-" + s.name,
			ProjectID:        s.projectID,
			NetworkID:        s.networkID,
			ParentPrefixCidr: "10.0.0.0/8",
			Type:             s.ipType,
			Tags:             s.tags,
		}
		_, err := ds.IP().Create(ctx, ip)
		require.NoError(t, err)
	}
}
