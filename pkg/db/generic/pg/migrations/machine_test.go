package migrations_test

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"uuid"

	_ "github.com/lib/pq"
	"github.com/metal-stack/metal-apiserver/pkg/db/generic"
	"github.com/metal-stack/metal-apiserver/pkg/db/generic/pg"
	"github.com/metal-stack/metal-apiserver/pkg/db/generic/pg/migrations"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-apiserver/pkg/test"
	"github.com/stretchr/testify/require"
)

func TestMigrateMachine(t *testing.T) {
	ctx := t.Context()
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	rdb, _, rethinkCloser := test.StartRethink(t, log)
	defer rethinkCloser()

	pgdb, pgCloser := test.StartPostgres(t, log)
	defer pgCloser()

	seedMachines(t, ctx, rdb)

	require.NoError(t, migrations.MigrateMachine(ctx, log, rdb, pgdb))

	repo, err := pg.NewGenericRepository[*metal.Machine](log, pgdb)
	require.NoError(t, err)

	t.Run("migrates all machines preserving data", func(t *testing.T) {
		for _, m := range testMachines {
			id, err := uuid.Parse(m.id)
			require.NoError(t, err)

			ent, err := repo.Get(ctx, id)
			require.NoError(t, err)
			require.NotNil(t, ent)
			require.Equal(t, m.name, ent.Data.Name)
			require.Equal(t, m.partitionID, ent.Data.PartitionID)
			require.Equal(t, m.sizeID, ent.Data.SizeID)
			require.Equal(t, m.rackID, ent.Data.RackID)
			require.Equal(t, m.state, ent.Data.State.Value)
		}
	})

	t.Run("migrated machines can be queried with the new query format", func(t *testing.T) {
		results, err := repo.Query(ctx, []pg.QueryFilter{
			{Path: "PartitionID", Op: "=", Value: "partition-1"},
		}, nil)
		require.NoError(t, err)
		require.Len(t, results, 2)
	})

	t.Run("migration is idempotent", func(t *testing.T) {
		require.NoError(t, migrations.MigrateMachine(ctx, log, rdb, pgdb))

		all, err := repo.Query(ctx, nil, nil)
		require.NoError(t, err)
		require.Len(t, all, len(testMachines))
	})
}

type machineSeed struct {
	id          string
	name        string
	partitionID string
	sizeID      string
	rackID      string
	state       metal.MState
}

var testMachines = []machineSeed{
	{id: "6b22ccd6-4c93-4a1f-8c8f-2f63b3c6e001", name: "machine-1", partitionID: "partition-1", sizeID: "s1.xlarge", rackID: "rack-1", state: metal.AvailableState},
	{id: "6b22ccd6-4c93-4a1f-8c8f-2f63b3c6e002", name: "machine-2", partitionID: "partition-1", sizeID: "s1.xlarge", rackID: "rack-1", state: metal.TaintedState},
	{id: "6b22ccd6-4c93-4a1f-8c8f-2f63b3c6e003", name: "machine-3", partitionID: "partition-2", sizeID: "s1.2xlarge", rackID: "rack-2", state: metal.LockedState},
}

func seedMachines(t *testing.T, ctx context.Context, ds generic.Datastore) {
	t.Helper()
	for _, s := range testMachines {
		m := &metal.Machine{
			ID:          s.id,
			Name:        s.name,
			Description: "desc-" + s.name,
			PartitionID: s.partitionID,
			SizeID:      s.sizeID,
			RackID:      s.rackID,
			State: metal.MachineState{
				Value: s.state,
			},
			Tags: []string{"foo=bar", "baz=qux"},
			Allocation: &metal.MachineAllocation{
				Project: "project-1",
				UUID:    "6b22ccd6-4c93-4a1f-8c8f-2f63b3c6e0ff",
				Name:    "alloc-" + s.name,
			},
		}
		_, err := ds.Machine().Create(ctx, m)
		require.NoError(t, err)
	}
}
