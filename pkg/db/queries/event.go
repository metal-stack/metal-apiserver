package queries

import (
	r "gopkg.in/rethinkdb/rethinkdb-go.v6"
)

func EventFilter(machineId *string) func(q r.Term) r.Term {
	return func(q r.Term) r.Term {
		if machineId != nil {
			q = q.Filter(func(row r.Term) r.Term {
				return row.Field("id").Eq(*machineId)
			})
		}
		return q
	}
}
