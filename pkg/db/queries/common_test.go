package queries_test

import (
	"log/slog"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/metal-stack/metal-apiserver/pkg/db/generic"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-apiserver/pkg/db/queries"
	"github.com/metal-stack/metal-apiserver/pkg/test"
	"github.com/stretchr/testify/require"

	r "gopkg.in/rethinkdb/rethinkdb-go.v6"
)

func TestByIDs(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	ds, _, rethinkCloser := test.StartRethink(t, log)
	defer rethinkCloser()

	ctx := t.Context()

	for _, id := range []string{"a", "b", "c", "d"} {
		_, err := ds.Machine().Create(ctx, &metal.Machine{Base: metal.Base{ID: id}, PartitionID: "p-" + id})
		require.NoError(t, err)
	}

	ids := func(ms []*metal.Machine) []string {
		var res []string
		for _, m := range ms {
			res = append(res, m.ID)
		}
		slices.SortFunc(res, strings.Compare)
		return res
	}

	tests := []struct {
		name    string
		queries []generic.EntityQuery
		want    []string
	}{
		{
			name:    "single id",
			queries: []generic.EntityQuery{queries.ByIDs("a")},
			want:    []string{"a"},
		},
		{
			name:    "multiple ids, duplicates and missing ids",
			queries: []generic.EntityQuery{queries.ByIDs("c", "a", "a", "x")},
			want:    []string{"a", "c"},
		},
		{
			name: "combined with a subsequent filter",
			queries: []generic.EntityQuery{queries.ByIDs("a", "b", "c"), func(q r.Term) r.Term {
				return q.Filter(func(row r.Term) r.Term { return row.Field("partitionid").Eq("p-b") })
			}},
			want: []string{"b"},
		},
		{
			name:    "empty ids",
			queries: []generic.EntityQuery{queries.ByIDs()},
			want:    nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ds.Machine().List(ctx, tt.queries...)
			require.NoError(t, err)
			require.Equal(t, tt.want, ids(got))
		})
	}
}
