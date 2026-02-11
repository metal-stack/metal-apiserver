package queries_test

import (
	"fmt"
	"log/slog"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-apiserver/pkg/db/queries"
	"github.com/metal-stack/metal-apiserver/pkg/test"
	"github.com/stretchr/testify/require"
)

var (
	sw1 = &metal.Switch{
		Base:      metal.Base{ID: "sw1"},
		Rack:      "rack01",
		Partition: "partition-a",
		OS: &metal.SwitchOS{
			Vendor:  metal.SwitchOSVendorCumulus,
			Version: "5.9",
		},
		Nics: metal.Nics{},
	}
	sw2 = &metal.Switch{
		Base:      metal.Base{ID: "sw2"},
		Rack:      "rack01",
		Partition: "partition-b",
		OS: &metal.SwitchOS{
			Vendor:  metal.SwitchOSVendorCumulus,
			Version: "5.6",
		},
		Nics: metal.Nics{},
	}
	sw3 = &metal.Switch{
		Base:      metal.Base{ID: "sw3"},
		Rack:      "rack02",
		Partition: "partition-a",
		OS: &metal.SwitchOS{
			Vendor:  metal.SwitchOSVendorSonic,
			Version: "ec202111.11",
		},
		Nics: metal.Nics{},
	}
	switches = []*metal.Switch{sw1, sw2, sw3}
)

func TestSwitchFilter(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	ds, _, rethinkCloser := test.StartRethink(t, log)
	defer func() {
		rethinkCloser()
	}()

	ctx := t.Context()

	for _, sw := range switches {
		createdSwitch, err := ds.Switch().Create(ctx, sw)
		require.NoError(t, err)
		require.NotNil(t, createdSwitch)
		require.Equal(t, sw.ID, createdSwitch.ID)
	}

	tests := []struct {
		name string
		rq   *apiv2.SwitchQuery
		want []*metal.Switch
	}{
		{
			name: "empty query returns all",
			rq:   &apiv2.SwitchQuery{},
			want: switches,
		},
		{
			name: "query by id",
			rq: &apiv2.SwitchQuery{
				Id: new("sw1"),
			},
			want: []*metal.Switch{sw1},
		},
		{
			name: "query by partition",
			rq: &apiv2.SwitchQuery{
				Partition: new("partition-a"),
			},
			want: []*metal.Switch{sw1, sw3},
		},
		{
			name: "query by rack",
			rq: &apiv2.SwitchQuery{
				Rack: new("rack01"),
			},
			want: []*metal.Switch{sw1, sw2},
		},
		{
			name: "query by os vendor",
			rq: &apiv2.SwitchQuery{
				Os: &apiv2.SwitchOSQuery{
					Vendor: new(apiv2.SwitchOSVendor_SWITCH_OS_VENDOR_SONIC),
				},
			},
			want: []*metal.Switch{sw3},
		},
		{
			name: "query by os version",
			rq: &apiv2.SwitchQuery{
				Os: &apiv2.SwitchOSQuery{
					Version: new("5.6"),
				},
			},
			want: []*metal.Switch{sw2},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ds.Switch().List(ctx, queries.SwitchFilter(tt.rq))
			require.NoError(t, err)

			slices.SortFunc(got, func(a, b *metal.Switch) int {
				return strings.Compare(a.ID, b.ID)
			})

			fmt.Print(got)
			if diff := cmp.Diff(
				tt.want, got,
				cmpopts.IgnoreFields(
					metal.Switch{}, "Created", "Changed",
				),
			); diff != "" {
				t.Errorf("switchServiceServer.List() diff = %s", diff)
			}
		})
	}
}
