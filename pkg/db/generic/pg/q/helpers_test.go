package q_test

import (
	"context"
	"testing"

	_ "github.com/lib/pq"
	"github.com/metal-stack/metal-apiserver/pkg/db/generic/pg"
	"github.com/stretchr/testify/require"
)

// queryData runs a pg query and returns the decoded entity data, or nil when
// there are no matches, so it can be compared against the reference test
// expectations directly.
func queryData[T any](t *testing.T, ctx context.Context, repo *pg.GenericRepository[T], filters []pg.QueryFilter) []T {
	t.Helper()
	ents, err := repo.Query(ctx, filters, nil)
	require.NoError(t, err)
	got := make([]T, 0, len(ents))
	for _, e := range ents {
		got = append(got, e.Data)
	}
	if len(got) == 0 {
		return nil
	}
	return got
}
