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
	sr1 = &metal.SizeReservation{
		ID: "sr1", Name: "reservation 1", Description: "Size reservation 1",
		SizeID:       "s1",
		Amount:       10,
		ProjectID:    "project-a",
		PartitionIDs: []string{"part1"},
		Labels:       map[string]string{"purpose": "dev"},
	}
	sr2 = &metal.SizeReservation{
		ID: "sr2", Name: "reservation 2", Description: "Size reservation 2",
		SizeID:       "s2",
		Amount:       5,
		ProjectID:    "project-b",
		PartitionIDs: []string{"part1", "part2"},
		Labels:       map[string]string{"purpose": "prod"},
	}
	sr3 = &metal.SizeReservation{
		ID: "sr3", Name: "reservation 3", Description: "Size reservation 3",
		SizeID:       "s1",
		Amount:       1,
		ProjectID:    "project-a",
		PartitionIDs: []string{"part3"},
		Labels:       map[string]string{"purpose": "prod", "region": "eu"},
	}
	sizeReservations = []*metal.SizeReservation{sr1, sr2, sr3}
)

func TestSizeReservationFilter(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	ds, _, rethinkCloser := test.StartRethink(t, log)
	defer func() {
		rethinkCloser()
	}()

	ctx := t.Context()

	for _, sr := range sizeReservations {
		created, err := ds.SizeReservation().Create(ctx, sr)
		require.NoError(t, err)
		require.NotNil(t, created)
		require.Equal(t, sr.ID, created.ID)
	}

	tests := []struct {
		name string
		rq   *apiv2.SizeReservationQuery
		want []*metal.SizeReservation
	}{
		{
			name: "nil request returns unfiltered",
			rq:   nil,
			want: sizeReservations,
		},
		{
			name: "empty request returns unfiltered",
			rq:   &apiv2.SizeReservationQuery{},
			want: sizeReservations,
		},
		{
			name: "by id",
			rq:   &apiv2.SizeReservationQuery{Id: new("sr1")},
			want: []*metal.SizeReservation{sr1},
		},
		{
			name: "by size",
			rq:   &apiv2.SizeReservationQuery{Size: new("s1")},
			want: []*metal.SizeReservation{sr1, sr3},
		},
		{
			name: "by name",
			rq:   &apiv2.SizeReservationQuery{Name: new("reservation 2")},
			want: []*metal.SizeReservation{sr2},
		},
		{
			name: "by description",
			rq:   &apiv2.SizeReservationQuery{Description: new("Size reservation 3")},
			want: []*metal.SizeReservation{sr3},
		},
		{
			name: "by project",
			rq:   &apiv2.SizeReservationQuery{Project: new("project-a")},
			want: []*metal.SizeReservation{sr1, sr3},
		},
		{
			name: "by partition matches reservations containing the partition",
			rq:   &apiv2.SizeReservationQuery{Partition: new("part1")},
			want: []*metal.SizeReservation{sr1, sr2},
		},
		{
			name: "by label",
			rq:   &apiv2.SizeReservationQuery{Labels: &apiv2.Labels{Labels: map[string]string{"purpose": "prod"}}},
			want: []*metal.SizeReservation{sr2, sr3},
		},
		{
			name: "by label 2",
			rq:   &apiv2.SizeReservationQuery{Labels: &apiv2.Labels{Labels: map[string]string{"region": "eu"}}},
			want: []*metal.SizeReservation{sr3},
		},
		{
			name: "project and label combined",
			rq: &apiv2.SizeReservationQuery{
				Project: new("project-a"),
				Labels:  &apiv2.Labels{Labels: map[string]string{"purpose": "prod"}},
			},
			want: []*metal.SizeReservation{sr3},
		},
		{
			name: "no result by id",
			rq:   &apiv2.SizeReservationQuery{Id: new("nonexistent")},
			want: nil,
		},
		{
			name: "no result by project",
			rq:   &apiv2.SizeReservationQuery{Project: new("nonexistent")},
			want: nil,
		},
		{
			name: "no result by partition",
			rq:   &apiv2.SizeReservationQuery{Partition: new("nonexistent")},
			want: nil,
		},
		{
			name: "no result by label",
			rq:   &apiv2.SizeReservationQuery{Labels: &apiv2.Labels{Labels: map[string]string{"purpose": "nonexistent"}}},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ds.SizeReservation().List(ctx, filters.SizeReservation(tt.rq)...)
			require.NoError(t, err)

			if diff := cmp.Diff(
				tt.want, got,
				cmpopts.SortSlices(func(a, b *metal.SizeReservation) bool { return a.ID < b.ID }),
				cmpopts.IgnoreFields(
					metal.SizeReservation{}, "Created", "Changed", "Generation",
				),
			); diff != "" {
				t.Errorf("SizeReservationFilter() = %v, want %v, diff: %s", got, tt.want, diff)
			}
		})
	}
}
