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
	sic1 = &metal.SizeImageConstraint{
		Base:        metal.Base{ID: "size-1", Name: "size-1", Description: "Size 1 Constraint"},
		Images:      map[string]string{"debian": ">= 11.0.0", "ubuntu": ">= 22.04.0"},
	}
	sic2 = &metal.SizeImageConstraint{
		Base:        metal.Base{ID: "size-2", Name: "size-2", Description: "Size 2 Constraint"},
		Images:      map[string]string{"debian": ">= 12.0.0", "fedora": ">= 38.0"},
	}
	sic3 = &metal.SizeImageConstraint{
		Base:        metal.Base{ID: "size-3", Name: "size-3", Description: "Size 3 Constraint"},
		Images:      map[string]string{"ubuntu": ">= 20.04.0"},
	}
	sic4 = &metal.SizeImageConstraint{
		Base:        metal.Base{ID: "firewall-size", Name: "firewall-size", Description: "Firewall Size Constraint"},
		Images:      map[string]string{"metalstack": ">= 1.0.0"},
	}
	sizeImageConstraints = []*metal.SizeImageConstraint{sic1, sic2, sic3, sic4}
)

func TestSizeImageConstraintFilter(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	ds, _, rethinkCloser := test.StartRethink(t, log)
	defer func() {
		rethinkCloser()
	}()

	ctx := t.Context()

	for _, sic := range sizeImageConstraints {
		_, err := ds.SizeImageConstraint().Create(ctx, sic)
		require.NoError(t, err)
	}

	tests := []struct {
		name string
		rq   *apiv2.SizeImageConstraintQuery
		want []*metal.SizeImageConstraint
	}{
		{
			name: "empty request returns unfiltered",
			rq:   nil,
			want: []*metal.SizeImageConstraint{sic1, sic2, sic3, sic4},
		},
		{
			name: "by size",
			rq:   &apiv2.SizeImageConstraintQuery{Size: &sic1.ID},
			want: []*metal.SizeImageConstraint{sic1},
		},
		{
			name: "by size 2",
			rq:   &apiv2.SizeImageConstraintQuery{Size: &sic2.ID},
			want: []*metal.SizeImageConstraint{sic2},
		},
		{
			name: "by name",
			rq:   &apiv2.SizeImageConstraintQuery{Name: &sic3.Name},
			want: []*metal.SizeImageConstraint{sic3},
		},
		{
			name: "by description",
			rq:   &apiv2.SizeImageConstraintQuery{Description: &sic4.Description},
			want: []*metal.SizeImageConstraint{sic4},
		},
		{
			name: "no result by size",
			rq:   &apiv2.SizeImageConstraintQuery{Size: new("nonexistent")},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ds.SizeImageConstraint().List(ctx, queries.SizeImageConstraintFilter(tt.rq))
			require.NoError(t, err)

			if diff := cmp.Diff(
				tt.want, got,
				cmpopts.SortSlices(func(a, b *metal.SizeImageConstraint) bool { return a.ID < b.ID }),
				cmpopts.IgnoreFields(
					metal.SizeImageConstraint{}, "Created", "Changed", "Generation",
				),
			); diff != "" {
				t.Errorf("SizeImageConstraintFilter() = %v, want %v, diff: %s", got, tt.want, diff)
			}

		})
	}
}
