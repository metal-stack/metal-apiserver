package routing_test

import (
	"log/slog"
	"os"
	"testing"
	"time"

	"connectrpc.com/connect"
	_ "github.com/lib/pq"
	"github.com/metal-stack/metal-apiserver/pkg/db/generic/pg"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-apiserver/pkg/db/routing"
	"github.com/metal-stack/metal-apiserver/pkg/test"
	"github.com/stretchr/testify/require"
)

// TestPostgresStorageUpdateChangedLock checks that the postgres storage uses
// the changed timestamp as optimistic lock token like the RethinkDB storage:
// the API sets it from the updated_at a client sent.
func TestPostgresStorageUpdateChangedLock(t *testing.T) {
	ctx := t.Context()
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	db, closer := test.StartPostgres(t, log)
	defer closer()

	repo, err := pg.NewGenericRepository[*metal.Partition](log, db)
	require.NoError(t, err)
	s := routing.NewPostgresStorage(repo, "partition")

	_, err = s.Create(ctx, &metal.Partition{Base: metal.Base{ID: "p1"}})
	require.NoError(t, err)

	for _, tt := range []struct {
		name    string
		changed func(stored time.Time) time.Time
	}{
		{name: "changed in the past", changed: func(time.Time) time.Time { return time.Date(2002, 2, 12, 12, 0, 0, 0, time.UTC) }},
		{name: "changed in the future", changed: func(time.Time) time.Time { return time.Now().Add(10 * time.Minute) }},
		{name: "changed off by a millisecond", changed: func(stored time.Time) time.Time { return stored.Add(time.Millisecond) }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			p, err := s.Get(ctx, "p1")
			require.NoError(t, err)

			p.Changed = tt.changed(p.Changed)
			p.Description = "stale"
			err = s.Update(ctx, p)
			require.Equal(t, connect.CodeAborted, connect.CodeOf(err), "got %v", err)

			p, err = s.Get(ctx, "p1")
			require.NoError(t, err)
			require.Empty(t, p.Description)
			require.Zero(t, p.Generation)
		})
	}

	t.Run("matching changed succeeds, also in another time zone", func(t *testing.T) {
		p, err := s.Get(ctx, "p1")
		require.NoError(t, err)

		// The API hands updated_at to clients as UTC timestamp.
		p.Changed = p.Changed.UTC()
		p.Description = "updated"
		require.NoError(t, s.Update(ctx, p))

		p, err = s.Get(ctx, "p1")
		require.NoError(t, err)
		require.Equal(t, "updated", p.Description)
		require.Equal(t, uint64(1), p.Generation)
	})

	t.Run("zero changed is rejected", func(t *testing.T) {
		p, err := s.Get(ctx, "p1")
		require.NoError(t, err)

		p.Changed = time.Time{}
		require.ErrorContains(t, s.Update(ctx, p), "no changed timestamp set")
	})

	t.Run("missing entity is not found", func(t *testing.T) {
		err := s.Update(ctx, &metal.Partition{Base: metal.Base{ID: "p2", Changed: time.Now()}})
		require.Equal(t, connect.CodeNotFound, connect.CodeOf(err), "got %v", err)
	})
}
