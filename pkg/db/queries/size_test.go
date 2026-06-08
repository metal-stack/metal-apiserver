package queries_test

import (
	"log/slog"
	"os"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/stretchr/testify/require"

	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-apiserver/pkg/db/queries"
	"github.com/metal-stack/metal-apiserver/pkg/test"
)

var (
	s1 = &metal.Size{
		Base:        metal.Base{ID: "s1", Name: "s1", Description: "Size 1"},
		Labels:      map[string]string{"tier": "standard", "region": "eu"},
		Constraints: []metal.Constraint{{Type: metal.CoreConstraint, Min: 4, Max: 4}},
	}
	s2 = &metal.Size{
		Base:        metal.Base{ID: "s2", Name: "s2", Description: "Size 2"},
		Labels:      map[string]string{"tier": "premium", "region": "eu"},
		Constraints: []metal.Constraint{{Type: metal.CoreConstraint, Min: 8, Max: 16}},
	}
	s3 = &metal.Size{
		Base:        metal.Base{ID: "s3", Name: "s3", Description: "Size 3"},
		Labels:      map[string]string{"tier": "standard", "region": "us"},
		Constraints: []metal.Constraint{{Type: metal.CoreConstraint, Min: 4, Max: 8}, {Type: metal.MemoryConstraint, Min: 8192, Max: 16384}},
	}
	s4 = &metal.Size{
		Base:        metal.Base{ID: "gpu-size", Name: "gpu-size", Description: "GPU Size"},
		Labels:      map[string]string{"tier": "premium", "region": "eu", "gpu": "nvidia"},
		Constraints: []metal.Constraint{{Type: metal.GPUConstraint, Min: 1, Max: 4, Identifier: "a100"}},
	}
	sizes = []*metal.Size{s1, s2, s3, s4}
)

func TestSizeFilter(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	ds, _, rethinkCloser := test.StartRethink(t, log)
	defer func() {
		rethinkCloser()
	}()

	ctx := t.Context()

	for _, size := range sizes {
		_, err := ds.Size().Create(ctx, size)
		require.NoError(t, err)
	}

	tests := []struct {
		name string
		rq   *apiv2.SizeQuery
		want []*metal.Size
	}{
		{
			name: "empty request returns unfiltered",
			rq:   nil,
			want: []*metal.Size{s1, s2, s3, s4},
		},
		{
			name: "by id",
			rq:   &apiv2.SizeQuery{Id: &s1.ID},
			want: []*metal.Size{s1},
		},
		{
			name: "by id 2",
			rq:   &apiv2.SizeQuery{Id: &s2.ID},
			want: []*metal.Size{s2},
		},
		{
			name: "by name",
			rq:   &apiv2.SizeQuery{Name: &s3.Name},
			want: []*metal.Size{s3},
		},
		{
			name: "by description",
			rq:   &apiv2.SizeQuery{Description: &s4.Description},
			want: []*metal.Size{s4},
		},
		{
			name: "by label",
			rq:   &apiv2.SizeQuery{Labels: &apiv2.Labels{Labels: map[string]string{"tier": "standard"}}},
			want: []*metal.Size{s1, s3},
		},
		{
			name: "by label 2",
			rq:   &apiv2.SizeQuery{Labels: &apiv2.Labels{Labels: map[string]string{"region": "us"}}},
			want: []*metal.Size{s3},
		},
		{
			name: "by label 3",
			rq:   &apiv2.SizeQuery{Labels: &apiv2.Labels{Labels: map[string]string{"gpu": "nvidia"}}},
			want: []*metal.Size{s4},
		},
		{
			name: "no result by label",
			rq:   &apiv2.SizeQuery{Labels: &apiv2.Labels{Labels: map[string]string{"tier": "nonexistent"}}},
			want: nil,
		},
		{
			name: "no result by name",
			rq:   &apiv2.SizeQuery{Name: new("nonexistent")},
			want: nil,
		},
		{
			name: "no result by description",
			rq:   &apiv2.SizeQuery{Description: new("nonexistent")},
			want: nil,
		},
		{
			name: "label and name combined",
			rq:   &apiv2.SizeQuery{Name: &s1.Name, Labels: &apiv2.Labels{Labels: map[string]string{"tier": "standard"}}},
			want: []*metal.Size{s1},
		},
		{
			name: "label and description combined",
			rq:   &apiv2.SizeQuery{Description: &s1.Description, Labels: &apiv2.Labels{Labels: map[string]string{"region": "eu"}}},
			want: []*metal.Size{s1},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ds.Size().List(ctx, queries.SizeFilter(tt.rq))
			require.NoError(t, err)

			if diff := cmp.Diff(
				tt.want, got,
				cmpopts.SortSlices(func(a, b *metal.Size) bool { return a.ID < b.ID }),
				cmpopts.IgnoreFields(
					metal.Size{}, "Created", "Changed", "Generation",
				),
			); diff != "" {
				t.Errorf("SizeFilter() = %v, want %v, diff: %s", got, tt.want, diff)
			}

		})
	}
}
