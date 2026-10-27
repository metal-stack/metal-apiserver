package q_test

import (
	"log/slog"
	"os"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/generic/pg"
	"github.com/metal-stack/metal-apiserver/pkg/db/generic/pg/q"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-apiserver/pkg/test"
	"github.com/stretchr/testify/require"
)

var (
	pgSwitch1 = &metal.Switch{
		ID:        "sw1",
		Rack:      "rack01",
		Room:      "room01",
		Partition: "partition-a",
		OS: &metal.SwitchOS{
			Vendor:  metal.SwitchOSVendorCumulus,
			Version: "5.9",
		},
		Nics: metal.Nics{},
		MachineConnections: metal.ConnectionMap{
			"m1": []metal.Connection{
				{
					Nic:       metal.Nic{Neighbors: metal.Nics{}},
					MachineID: "m1",
				},
			},
		},
	}
	pgSwitch2 = &metal.Switch{
		ID:        "sw2",
		Rack:      "rack01",
		Partition: "partition-b",
		OS: &metal.SwitchOS{
			Vendor:  metal.SwitchOSVendorCumulus,
			Version: "5.6",
		},
		Nics: metal.Nics{},
		MachineConnections: metal.ConnectionMap{
			"m1": []metal.Connection{
				{
					Nic:       metal.Nic{Neighbors: metal.Nics{}},
					MachineID: "m1",
				},
			},
		},
	}
	pgSwitch3 = &metal.Switch{
		ID:        "sw3",
		Rack:      "rack02",
		Partition: "partition-a",
		OS: &metal.SwitchOS{
			Vendor:  metal.SwitchOSVendorSonic,
			Version: "ec202111.11",
		},
		Nics: metal.Nics{},
	}
	pgSwitches = []*metal.Switch{pgSwitch1, pgSwitch2, pgSwitch3}
)

// TestSwitchFilterPostgres mirrors the RethinkDB-backed TestSwitchFilter from
// pkg/db/queries, but runs against the Postgres backend using q.SwitchFilter.
func TestSwitchFilterPostgres(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	db, closer := test.StartPostgres(t, log)
	defer closer()

	repo, err := pg.NewGenericRepository[*metal.Switch](log, db)
	require.NoError(t, err)

	ctx := t.Context()

	for _, sw := range pgSwitches {
		require.NoError(t, repo.Create(ctx, sw.GetID(), sw))
	}

	tests := []struct {
		name string
		rq   *apiv2.SwitchQuery
		want []*metal.Switch
	}{
		{name: "empty query returns all", rq: &apiv2.SwitchQuery{}, want: pgSwitches},
		{name: "query by id", rq: &apiv2.SwitchQuery{Id: new("sw1")}, want: []*metal.Switch{pgSwitch1}},
		{name: "query by partition", rq: &apiv2.SwitchQuery{Partition: new("partition-a")}, want: []*metal.Switch{pgSwitch1, pgSwitch3}},
		{name: "query by rack", rq: &apiv2.SwitchQuery{Rack: new("rack01")}, want: []*metal.Switch{pgSwitch1, pgSwitch2}},
		{name: "query by room", rq: &apiv2.SwitchQuery{Room: new("room01")}, want: []*metal.Switch{pgSwitch1}},
		{name: "query by os vendor", rq: &apiv2.SwitchQuery{Os: &apiv2.SwitchOSQuery{Vendor: new(apiv2.SwitchOSVendor_SWITCH_OS_VENDOR_SONIC)}}, want: []*metal.Switch{pgSwitch3}},
		{name: "query by os version", rq: &apiv2.SwitchQuery{Os: &apiv2.SwitchOSQuery{Version: new("5.6")}}, want: []*metal.Switch{pgSwitch2}},
		{name: "query by connected machine id", rq: &apiv2.SwitchQuery{ConnectedMachineId: new("m1")}, want: []*metal.Switch{pgSwitch1, pgSwitch2}},
		{name: "query by connected machine id no result", rq: &apiv2.SwitchQuery{ConnectedMachineId: new("nonexistent")}, want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := queryData(t, ctx, repo, q.SwitchFilter(tt.rq))
			if diff := cmp.Diff(
				tt.want, got,
				cmpopts.SortSlices(func(a, b *metal.Switch) bool { return a.ID < b.ID }),
				cmpopts.IgnoreFields(metal.Switch{}, "Created", "Changed"),
			); diff != "" {
				t.Errorf("SwitchFilter() = %v, want %v diff: %s", got, tt.want, diff)
			}
		})
	}
}
