package queries

import (
	"github.com/metal-stack/api/go/enum"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	r "gopkg.in/rethinkdb/rethinkdb-go.v6"
)

func SwitchFilter(query *apiv2.SwitchQuery) func(q r.Term) r.Term {
	if query == nil {
		return nil
	}
	return func(q r.Term) r.Term {
		if query.Id != nil {
			q = q.Filter(func(row r.Term) r.Term {
				return row.Field("id").Eq(*query.Id)
			})
		}

		if query.Partition != nil {
			q = q.Filter(func(row r.Term) r.Term {
				return row.Field("partitionid").Eq(*query.Partition)
			})
		}

		if query.Rack != nil {
			q = q.Filter(func(row r.Term) r.Term {
				return row.Field("rackid").Eq(*query.Rack)
			})
		}

		if query.Os != nil {
			if query.Os.Vendor != nil {
				stringValue, err := enum.GetStringValue(query.Os.Vendor)
				if err == nil {
					q = q.Filter(func(row r.Term) r.Term {
						return row.Field("os").Field("vendor").Eq(stringValue)
					})
				}
			}

			if query.Os.Version != nil {
				q = q.Filter(func(row r.Term) r.Term {
					return row.Field("os").Field("version").Eq(*query.Os.Version)
				})
			}
		}

		return q
	}
}
