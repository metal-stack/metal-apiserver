package queries_test

import (
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/stretchr/testify/require"

	"github.com/metal-stack/metal-apiserver/pkg/db/filters"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-apiserver/pkg/test"
)

var (
	ev1 = &metal.ProvisioningEventContainer{
		ID:        "m-ev1",
		Events:    metal.ProvisioningEvents{},
		CrashLoop: true,
	}
	ev2 = &metal.ProvisioningEventContainer{
		ID: "m-ev2",
		Events: metal.ProvisioningEvents{{
			Time:    time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC),
			Event:   metal.ProvisioningEventPhonedHome,
			Message: "machine phoned home",
		}},
	}
	ev3 = &metal.ProvisioningEventContainer{
		ID:                   "m-ev3",
		Events:               metal.ProvisioningEvents{},
		FailedMachineReclaim: true,
	}
	eventContainers = []*metal.ProvisioningEventContainer{ev1, ev2, ev3}
)

func TestEventFilter(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	ds, _, rethinkCloser := test.StartRethink(t, log)
	defer func() {
		rethinkCloser()
	}()

	ctx := t.Context()

	for _, pec := range eventContainers {
		created, err := ds.Event().Create(ctx, pec)
		require.NoError(t, err)
		require.NotNil(t, created)
		require.Equal(t, pec.ID, created.ID)
	}

	tests := []struct {
		name      string
		machineID string
		want      []*metal.ProvisioningEventContainer
	}{
		{
			name:      "empty machine id returns all",
			machineID: "",
			want:      eventContainers,
		},
		{
			name:      "by machine id",
			machineID: "m-ev1",
			want:      []*metal.ProvisioningEventContainer{ev1},
		},
		{
			name:      "by machine id 2",
			machineID: "m-ev2",
			want:      []*metal.ProvisioningEventContainer{ev2},
		},
		{
			name:      "no result by machine id",
			machineID: "nonexistent",
			want:      nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ds.Event().List(ctx, filters.Event(tt.machineID)...)
			require.NoError(t, err)

			if diff := cmp.Diff(
				tt.want, got,
				cmpopts.SortSlices(func(a, b *metal.ProvisioningEventContainer) bool { return a.ID < b.ID }),
				cmpopts.IgnoreFields(
					metal.ProvisioningEventContainer{}, "Created", "Changed", "Generation",
				),
			); diff != "" {
				t.Errorf("EventFilter() = %v, want %v, diff: %s", got, tt.want, diff)
			}
		})
	}
}
