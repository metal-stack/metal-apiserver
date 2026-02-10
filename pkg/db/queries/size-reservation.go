package queries

import (
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	r "gopkg.in/rethinkdb/rethinkdb-go.v6"
)

func SizeReservationFilter(rq *apiv2.SizeReservationQuery) func(q r.Term) r.Term {
	if rq == nil {
		return nil
	}
	return func(q r.Term) r.Term {
		if rq.Id != nil {
			q = q.Filter(func(row r.Term) r.Term {
				return row.Field("id").Eq(*rq.Id)
			})
		}

		if rq.Size != nil {
			q = q.Filter(func(row r.Term) r.Term {
				return row.Field("sizeid").Eq(*rq.Size)
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

		if rq.Labels != nil {
			for key, value := range rq.Labels.Labels {
				q = q.Filter(func(row r.Term) r.Term {
					return row.Field("labels").Field(key).Eq(value)
				})
			}
		}

		if rq.Project != nil {
			q = q.Filter(func(row r.Term) r.Term {
				return row.Field("projectid").Eq(*rq.Project)
			})
		}

		if rq.Partition != nil {
			q = q.Filter(func(row r.Term) r.Term {
				return row.Field("partitionids").Contains(r.Expr(*rq.Partition))
			})
		}
		return q
	}
}
