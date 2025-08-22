package queries

import (
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	r "gopkg.in/rethinkdb/rethinkdb-go.v6"
)

func SwitchFilter(query *apiv2.SwitchQuery) func(q r.Term) r.Term {
	if query == nil {
		return nil
	}
	return func(q r.Term) r.Term {
		// TODO: implement
		return q
	}
}
