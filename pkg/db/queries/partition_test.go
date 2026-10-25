package queries_test

import (
	"log/slog"
	"os"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/stretchr/testify/require"

	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/filters"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-apiserver/pkg/test"
)

var (
	part1 = &metal.Partition{
		ID: "part1", Name: "partition 1", Description: "Partition 1",
		MgmtServiceAddress: "mgmt1.example.com",
		Labels:             map[string]string{"region": "eu"},
		DNSServers:         metal.DNSServers{},
		NTPServers:         metal.NTPServers{},
	}
	part2 = &metal.Partition{
		ID: "part2", Name: "partition 2", Description: "Partition 2",
		MgmtServiceAddress: "mgmt2.example.com",
		Labels:             map[string]string{"region": "us"},
		DNSServers:         metal.DNSServers{},
		NTPServers:         metal.NTPServers{},
	}
	part3 = &metal.Partition{
		ID: "part3", Name: "partition 3", Description: "Partition 3",
		MgmtServiceAddress: "mgmt3.example.com",
		Labels:             map[string]string{"region": "eu", "gpu": "true"},
		DNSServers:         metal.DNSServers{},
		NTPServers:         metal.NTPServers{},
	}
	partitions = []*metal.Partition{part1, part2, part3}
)

func TestPartitionFilter(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	ds, _, rethinkCloser := test.StartRethink(t, log)
	defer func() {
		rethinkCloser()
	}()

	ctx := t.Context()

	for _, part := range partitions {
		created, err := ds.Partition().Create(ctx, part)
		require.NoError(t, err)
		require.NotNil(t, created)
		require.Equal(t, part.ID, created.ID)
	}

	tests := []struct {
		name string
		rq   *apiv2.PartitionQuery
		want []*metal.Partition
	}{
		{
			name: "nil request returns unfiltered",
			rq:   nil,
			want: partitions,
		},
		{
			name: "empty request returns unfiltered",
			rq:   &apiv2.PartitionQuery{},
			want: partitions,
		},
		{
			name: "by id",
			rq:   &apiv2.PartitionQuery{Id: new("part1")},
			want: []*metal.Partition{part1},
		},
		{
			name: "by id 2",
			rq:   &apiv2.PartitionQuery{Id: new("part3")},
			want: []*metal.Partition{part3},
		},
		{
			name: "no result by id",
			rq:   &apiv2.PartitionQuery{Id: new("nonexistent")},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ds.Partition().List(ctx, filters.Partition(tt.rq)...)
			require.NoError(t, err)

			if diff := cmp.Diff(
				tt.want, got,
				cmpopts.SortSlices(func(a, b *metal.Partition) bool { return a.ID < b.ID }),
				cmpopts.IgnoreFields(
					metal.Partition{}, "Created", "Changed", "Generation",
				),
			); diff != "" {
				t.Errorf("PartitionFilter() = %v, want %v, diff: %s", got, tt.want, diff)
			}
		})
	}
}
