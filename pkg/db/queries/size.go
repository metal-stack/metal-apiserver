package queries

import (
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"

	r "gopkg.in/rethinkdb/rethinkdb-go.v6"
)

func SizeFilter(rq *apiv2.SizeQuery) func(q r.Term) r.Term {
	if rq == nil {
		return nil
	}
	return func(q r.Term) r.Term {
		if rq.Id != nil {
			q = q.Filter(func(row r.Term) r.Term {
				return row.Field("id").Eq(*rq.Id)
			})
		}
		if rq.Description != nil {
			q = q.Filter(func(row r.Term) r.Term {
				return row.Field("description").Eq(*rq.Description)
			})
		}
		if rq.Name != nil {
			q = q.Filter(func(row r.Term) r.Term {
				return row.Field("name").Eq(*rq.Name)
			})
		}
		return q
	}
}
