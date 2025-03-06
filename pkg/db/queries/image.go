package queries

import (
	apienum "github.com/metal-stack/api/go/enum"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"

	r "gopkg.in/rethinkdb/rethinkdb-go.v6"
)

func ImageFilter(rq *apiv2.ImageQuery) func(q r.Term) r.Term {
	if rq == nil {
		return nil
	}
	return func(q r.Term) r.Term {
		if rq.Id != nil {
			q = q.Filter(func(row r.Term) r.Term {
				return row.Field("id").Eq(*rq.Id)
			})
		}
		if rq.Os != nil {
			q = q.Filter(func(row r.Term) r.Term {
				return row.Field("os").Eq(*rq.Os)
			})
		}
		if rq.Version != nil {
			q = q.Filter(func(row r.Term) r.Term {
				return row.Field("version").Eq(*rq.Version)
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

		if rq.Feature != nil {
			q = q.Filter(func(row r.Term) r.Term {
				feature, err := apienum.GetStringValue(*rq.Feature)
				if err == nil {
					return row.Field("features").HasFields([]string{feature})
				} else {
					return q
				}

			})
		}

		if rq.Classification != nil {
			q = q.Filter(func(row r.Term) r.Term {
				classification, err := apienum.GetStringValue(*rq.Classification)
				if err == nil {
					return row.Field("classification").Eq(classification)
				} else {
					return q
				}
			})
		}

		return q
	}
}
