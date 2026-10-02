package test

import (
	"testing"

	"uuid"

	"github.com/metal-stack/api/go/errorutil"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/generic/pg"
	pgq "github.com/metal-stack/metal-apiserver/pkg/db/generic/pg/q"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-apiserver/pkg/db/routing"
	"github.com/stretchr/testify/require"
)

// TestRoutingDatastoreBoth verifies the configuration-driven storage router
// end-to-end: a Network configured for "both" is written to RethinkDB and
// Postgres, and reads can be served from either backend by configuration.
func TestRoutingDatastoreBoth(t *testing.T) {
	ctx := t.Context()

	s, closer := StartRepositoryWithCleanup(t, WithRoutingConfig(routing.Config{
		Default: routing.ModeRethink,
		Entities: map[string]routing.Mode{
			"Network": routing.ModeBoth,
		},
	}))
	defer closer()

	pgRepo, err := pg.NewGenericRepository[*metal.Network](s.log, s.routingPgDB)
	require.NoError(t, err)

	network := &metal.Network{Name: "n1"}
	network.ProjectID = "p1"

	created, err := s.ds.Network().Create(ctx, network)
	require.NoError(t, err)
	require.NotEmpty(t, created.GetID())

	id, err := uuid.Parse(created.GetID())
	require.NoError(t, err)

	t.Run("written to rethinkdb", func(t *testing.T) {
		fromRethink, err := s.rethink.Network().Get(ctx, created.GetID())
		require.NoError(t, err)
		require.Equal(t, "n1", fromRethink.Name)
	})

	t.Run("mirrored to postgres", func(t *testing.T) {
		fromPostgres, err := pgRepo.Get(ctx, id)
		require.NoError(t, err)
		require.Equal(t, "n1", fromPostgres.Data.Name)
	})

	t.Run("default read backend is rethinkdb", func(t *testing.T) {
		list, err := s.ds.Network().List(ctx)
		require.NoError(t, err)
		require.Len(t, list, 1)
		require.Equal(t, "n1", list[0].Name)
	})

	t.Run("read backend can be flipped to postgres", func(t *testing.T) {
		routed, err := routing.NewDatastore(s.log, routing.Config{
			Default:  routing.ModeRethink,
			ReadFrom: routing.ModePostgres,
			Entities: map[string]routing.Mode{
				"Network": routing.ModeBoth,
			},
		}, s.rethink, s.routingPgDB)
		require.NoError(t, err)

		fromPostgres, err := routed.Network().Get(ctx, created.GetID())
		require.NoError(t, err)
		require.Equal(t, "n1", fromPostgres.Name)
	})

	t.Run("not found is mapped to a connect error", func(t *testing.T) {
		_, err := s.ds.Network().Get(ctx, uuid.NewV7().String())
		require.Error(t, err)
		require.True(t, errorutil.IsNotFound(err), "expected a connect NotFound error, got: %v", err)
	})
}

// TestRoutingDatastoreNotFoundMessage verifies that a not-found coming from the
// postgres-backed storage reproduces the exact error message of the legacy
// rethinkdb storage ("no <table> with id \"<id>\" found"), both at the datastore
// and at the repository layer.
func TestRoutingDatastoreNotFoundMessage(t *testing.T) {
	ctx := t.Context()

	s, closer := StartRepositoryWithCleanup(t, WithRoutingConfig(routing.Config{
		Default:  routing.ModeRethink,
		Entities: map[string]routing.Mode{"Machine": routing.ModePostgres},
	}))
	defer closer()

	missing := uuid.NewV7().String()
	want := errorutil.NotFound(`no machine with id %q found`, missing).Error()

	t.Run("datastore", func(t *testing.T) {
		_, err := s.GetDatastore().Machine().Get(ctx, missing)
		require.Error(t, err)
		require.True(t, errorutil.IsNotFound(err))
		require.Equal(t, want, err.Error())
	})

	t.Run("repository", func(t *testing.T) {
		// mirrors the machine service call path: repo.Machine(project).Get(...)
		_, err := s.Machine("").Get(ctx, missing)
		require.Error(t, err)
		require.True(t, errorutil.IsNotFound(err))
		require.Equal(t, want, err.Error())
	})
}

// TestRoutingMachineListFilteredFromPostgres guards the migration-aware read
// path: a filtered machine query is served from Postgres by supplying the
// Postgres filter representation (routing.FilteredStorage), including for
// non-UUID ids which are keyed by a derived UUID.
func TestRoutingMachineListFilteredFromPostgres(t *testing.T) {
	ctx := t.Context()

	s, closer := StartRepositoryWithCleanup(t, WithRoutingConfig(routing.Config{
		Default:  routing.ModeRethink,
		Entities: map[string]routing.Mode{"Machine": routing.ModePostgres},
	}))
	defer closer()

	ds := s.GetDatastore()

	const project = "project-1"
	uuidID := uuid.NewV7().String()
	for _, m := range []*metal.Machine{
		{ID: "m1", Allocation: &metal.MachineAllocation{Project: project}},
		{ID: uuidID, Allocation: &metal.MachineAllocation{Project: project}},
		{ID: "m2", Allocation: &metal.MachineAllocation{Project: "other"}},
	} {
		_, err := ds.Machine().Create(ctx, m)
		require.NoError(t, err)
	}

	fs, ok := ds.Machine().(routing.FilteredStorage[*metal.Machine])
	require.True(t, ok, "routed machine storage must support filtered reads")

	got, err := fs.ListFiltered(ctx, routing.Filter{
		Postgres: pgq.MachineFilter(&apiv2.MachineQuery{
			Allocation: &apiv2.MachineAllocationQuery{Project: new(project)},
		}),
	})
	require.NoError(t, err)

	ids := make([]string, 0, len(got))
	for _, m := range got {
		ids = append(ids, m.ID)
	}
	require.ElementsMatch(t, []string{"m1", uuidID}, ids)
}
