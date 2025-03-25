package queries

import (
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	r "gopkg.in/rethinkdb/rethinkdb-go.v6"
)

func PartitionFilter(rq *apiv2.PartitionQuery) func(q r.Term) r.Term {
	if rq == nil {
		return nil
	}
	return func(q r.Term) r.Term {
		if rq.Id != nil {
			q = q.Filter(func(row r.Term) r.Term {
				return row.Field("id").Eq(*rq.Id)
			})
		}
		return q
	}
}
