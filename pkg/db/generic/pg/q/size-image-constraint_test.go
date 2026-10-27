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
	pgSIC1 = &metal.SizeImageConstraint{
		ID: "size-1", Name: "size-1", Description: "Size 1 Constraint",
		Images: map[string]string{"debian": ">= 11.0.0", "ubuntu": ">= 22.04.0"},
	}
	pgSIC2 = &metal.SizeImageConstraint{
		ID: "size-2", Name: "size-2", Description: "Size 2 Constraint",
		Images: map[string]string{"debian": ">= 12.0.0", "fedora": ">= 38.0"},
	}
	pgSIC3 = &metal.SizeImageConstraint{
		ID: "size-3", Name: "size-3", Description: "Size 3 Constraint",
		Images: map[string]string{"ubuntu": ">= 20.04.0"},
	}
	pgSIC4 = &metal.SizeImageConstraint{
		ID: "firewall-size", Name: "firewall-size", Description: "Firewall Size Constraint",
		Images: map[string]string{"metalstack": ">= 1.0.0"},
	}
	pgSICs = []*metal.SizeImageConstraint{pgSIC1, pgSIC2, pgSIC3, pgSIC4}
)

// TestSizeImageConstraintFilterPostgres mirrors the RethinkDB-backed
// TestSizeImageConstraintFilter from pkg/db/queries, but runs against the
// Postgres backend using q.SizeImageConstraintFilter.
func TestSizeImageConstraintFilterPostgres(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	db, closer := test.StartPostgres(t, log)
	defer closer()

	repo, err := pg.NewGenericRepository[*metal.SizeImageConstraint](log, db)
	require.NoError(t, err)

	ctx := t.Context()

	for _, sic := range pgSICs {
		require.NoError(t, repo.Create(ctx, sic.GetID(), sic))
	}

	tests := []struct {
		name string
		rq   *apiv2.SizeImageConstraintQuery
		want []*metal.SizeImageConstraint
	}{
		{name: "empty request returns unfiltered", rq: nil, want: pgSICs},
		{name: "by size", rq: &apiv2.SizeImageConstraintQuery{Size: &pgSIC1.ID}, want: []*metal.SizeImageConstraint{pgSIC1}},
		{name: "by size 2", rq: &apiv2.SizeImageConstraintQuery{Size: &pgSIC2.ID}, want: []*metal.SizeImageConstraint{pgSIC2}},
		{name: "by name", rq: &apiv2.SizeImageConstraintQuery{Name: &pgSIC3.Name}, want: []*metal.SizeImageConstraint{pgSIC3}},
		{name: "by description", rq: &apiv2.SizeImageConstraintQuery{Description: &pgSIC4.Description}, want: []*metal.SizeImageConstraint{pgSIC4}},
		{name: "no result by size", rq: &apiv2.SizeImageConstraintQuery{Size: new("nonexistent")}, want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := queryData(t, ctx, repo, q.SizeImageConstraintFilter(tt.rq))
			if diff := cmp.Diff(
				tt.want, got,
				cmpopts.SortSlices(func(a, b *metal.SizeImageConstraint) bool { return a.ID < b.ID }),
				cmpopts.IgnoreFields(metal.SizeImageConstraint{}, "Created", "Changed", "Generation"),
			); diff != "" {
				t.Errorf("SizeImageConstraintFilter() = %v, want %v diff: %s", got, tt.want, diff)
			}
		})
	}
}
