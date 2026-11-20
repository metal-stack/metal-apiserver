package queries

import (
	"github.com/samber/lo"

	r "gopkg.in/rethinkdb/rethinkdb-go.v6"
)

// ByIDs selects all entities with the given primary keys using the primary index.
//
// It must be the first query applied to a table because GetAll is only valid on a table.
// Callers should not pass an empty id list.
func ByIDs(ids ...string) func(q r.Term) r.Term {
	keys := lo.Uniq(ids)
	return func(q r.Term) r.Term {
		return q.GetAll(r.Args(keys))
	}
}
