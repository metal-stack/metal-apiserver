package queries

import (
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	r "gopkg.in/rethinkdb/rethinkdb-go.v6"
)

func SizeImageConstraintFilter(rq *apiv2.SizeImageConstraintQuery) func(q r.Term) r.Term {
	if rq == nil {
		return nil
	}
	return func(q r.Term) r.Term {
		if rq.Size != nil {
			q = q.Filter(func(row r.Term) r.Term {
				return row.Field("id").Eq(*rq.Size)
			})
		}
		if rq.Name != nil {
			q = q.Filter(func(row r.Term) r.Term {
				return row.Field("name").Eq(*rq.Name)
			})
		}
		if rq.Description != nil {
			q = q.Filter(func(row r.Term) r.Term {
				return row.Field("description").Eq(*rq.Description)
			})
		}
		return q
	}
}
