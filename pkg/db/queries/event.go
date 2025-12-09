package queries

import (
	r "gopkg.in/rethinkdb/rethinkdb-go.v6"
)

func EventFilter(machineID string) func(q r.Term) r.Term {
	if machineID == "" {
		return nil
	}
	return func(q r.Term) r.Term {
		q = q.Filter(func(row r.Term) r.Term {
			return row.Field("id").Eq(machineID)
		})
		return q
	}
}
