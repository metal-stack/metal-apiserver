package q_test

import (
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/metal-stack/metal-apiserver/pkg/db/generic/pg"
	"github.com/metal-stack/metal-apiserver/pkg/db/generic/pg/q"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-apiserver/pkg/test"
	"github.com/stretchr/testify/require"
)

var (
	pgEvent1 = &metal.ProvisioningEventContainer{
		ID:        "m-ev1",
		Events:    metal.ProvisioningEvents{},
		CrashLoop: true,
	}
	pgEvent2 = &metal.ProvisioningEventContainer{
		ID: "m-ev2",
		Events: metal.ProvisioningEvents{{
			Time:    time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC),
			Event:   metal.ProvisioningEventPhonedHome,
			Message: "machine phoned home",
		}},
	}
	pgEvent3 = &metal.ProvisioningEventContainer{
		ID:                   "m-ev3",
		Events:               metal.ProvisioningEvents{},
		FailedMachineReclaim: true,
	}
	pgEvents = []*metal.ProvisioningEventContainer{pgEvent1, pgEvent2, pgEvent3}
)

// TestEventFilterPostgres mirrors the RethinkDB-backed TestEventFilter from
// pkg/db/queries, but runs against the Postgres backend using q.EventFilter.
func TestEventFilterPostgres(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	db, closer := test.StartPostgres(t, log)
	defer closer()

	repo, err := pg.NewGenericRepository[*metal.ProvisioningEventContainer](log, db)
	require.NoError(t, err)

	ctx := t.Context()

	for _, pec := range pgEvents {
		require.NoError(t, repo.Create(ctx, pec.GetID(), pec))
	}

	tests := []struct {
		name      string
		machineID string
		want      []*metal.ProvisioningEventContainer
	}{
		{name: "empty machine id returns all", machineID: "", want: pgEvents},
		{name: "by machine id", machineID: "m-ev1", want: []*metal.ProvisioningEventContainer{pgEvent1}},
		{name: "by machine id 2", machineID: "m-ev2", want: []*metal.ProvisioningEventContainer{pgEvent2}},
		{name: "no result by machine id", machineID: "nonexistent", want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := queryData(t, ctx, repo, q.EventFilter(tt.machineID))
			if diff := cmp.Diff(
				tt.want, got,
				cmpopts.SortSlices(func(a, b *metal.ProvisioningEventContainer) bool { return a.ID < b.ID }),
				cmpopts.IgnoreFields(metal.ProvisioningEventContainer{}, "Created", "Changed", "Generation"),
			); diff != "" {
				t.Errorf("EventFilter() = %v, want %v diff: %s", got, tt.want, diff)
			}
		})
	}
}
