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
	pgSize1 = &metal.Size{
		ID: "s1", Name: "s1", Description: "Size 1",
		Labels:      map[string]string{"tier": "standard", "region": "eu"},
		Constraints: []metal.Constraint{{Type: metal.CoreConstraint, Min: 4, Max: 4}},
	}
	pgSize2 = &metal.Size{
		ID: "s2", Name: "s2", Description: "Size 2",
		Labels:      map[string]string{"tier": "premium", "region": "eu"},
		Constraints: []metal.Constraint{{Type: metal.CoreConstraint, Min: 8, Max: 16}},
	}
	pgSize3 = &metal.Size{
		ID: "s3", Name: "s3", Description: "Size 3",
		Labels:      map[string]string{"tier": "standard", "region": "us"},
		Constraints: []metal.Constraint{{Type: metal.CoreConstraint, Min: 4, Max: 8}, {Type: metal.MemoryConstraint, Min: 8192, Max: 16384}},
	}
	pgSize4 = &metal.Size{
		ID: "gpu-size", Name: "gpu-size", Description: "GPU Size",
		Labels:      map[string]string{"tier": "premium", "region": "eu", "gpu": "nvidia"},
		Constraints: []metal.Constraint{{Type: metal.GPUConstraint, Min: 1, Max: 4, Identifier: "a100"}},
	}
	pgSizes = []*metal.Size{pgSize1, pgSize2, pgSize3, pgSize4}
)

// TestSizeFilterPostgres mirrors the RethinkDB-backed TestSizeFilter from
// pkg/db/queries, but runs against the Postgres backend using q.SizeFilter.
func TestSizeFilterPostgres(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	db, closer := test.StartPostgres(t, log)
	defer closer()

	repo, err := pg.NewGenericRepository[*metal.Size](log, db)
	require.NoError(t, err)

	ctx := t.Context()

	for _, size := range pgSizes {
		require.NoError(t, repo.Create(ctx, size.GetID(), size))
	}

	tests := []struct {
		name string
		rq   *apiv2.SizeQuery
		want []*metal.Size
	}{
		{name: "empty request returns unfiltered", rq: nil, want: pgSizes},
		{name: "by id", rq: &apiv2.SizeQuery{Id: &pgSize1.ID}, want: []*metal.Size{pgSize1}},
		{name: "by id 2", rq: &apiv2.SizeQuery{Id: &pgSize2.ID}, want: []*metal.Size{pgSize2}},
		{name: "by name", rq: &apiv2.SizeQuery{Name: &pgSize3.Name}, want: []*metal.Size{pgSize3}},
		{name: "by description", rq: &apiv2.SizeQuery{Description: &pgSize4.Description}, want: []*metal.Size{pgSize4}},
		{name: "by label", rq: &apiv2.SizeQuery{Labels: &apiv2.Labels{Labels: map[string]string{"tier": "standard"}}}, want: []*metal.Size{pgSize1, pgSize3}},
		{name: "by label 2", rq: &apiv2.SizeQuery{Labels: &apiv2.Labels{Labels: map[string]string{"region": "us"}}}, want: []*metal.Size{pgSize3}},
		{name: "by label 3", rq: &apiv2.SizeQuery{Labels: &apiv2.Labels{Labels: map[string]string{"gpu": "nvidia"}}}, want: []*metal.Size{pgSize4}},
		{name: "no result by label", rq: &apiv2.SizeQuery{Labels: &apiv2.Labels{Labels: map[string]string{"tier": "nonexistent"}}}, want: nil},
		{name: "no result by name", rq: &apiv2.SizeQuery{Name: new("nonexistent")}, want: nil},
		{name: "no result by description", rq: &apiv2.SizeQuery{Description: new("nonexistent")}, want: nil},
		{name: "label and name combined", rq: &apiv2.SizeQuery{Name: &pgSize1.Name, Labels: &apiv2.Labels{Labels: map[string]string{"tier": "standard"}}}, want: []*metal.Size{pgSize1}},
		{name: "label and description combined", rq: &apiv2.SizeQuery{Description: &pgSize1.Description, Labels: &apiv2.Labels{Labels: map[string]string{"region": "eu"}}}, want: []*metal.Size{pgSize1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := queryData(t, ctx, repo, q.SizeFilter(tt.rq))
			if diff := cmp.Diff(
				tt.want, got,
				cmpopts.SortSlices(func(a, b *metal.Size) bool { return a.ID < b.ID }),
				cmpopts.IgnoreFields(metal.Size{}, "Created", "Changed", "Generation"),
			); diff != "" {
				t.Errorf("SizeFilter() = %v, want %v diff: %s", got, tt.want, diff)
			}
		})
	}
}
