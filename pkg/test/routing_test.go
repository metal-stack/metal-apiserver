package test

import (
	"testing"

	"uuid"

	"github.com/metal-stack/api/go/errorutil"
	"github.com/metal-stack/metal-apiserver/pkg/db/generic/pg"
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
