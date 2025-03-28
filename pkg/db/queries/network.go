package queries

import (
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"

	r "gopkg.in/rethinkdb/rethinkdb-go.v6"
)

func NetworkProjectScoped(project string) func(q r.Term) r.Term {
	return func(q r.Term) r.Term {
		return q.Filter(func(row r.Term) r.Term {
			return row.Field("projectid").Eq(project)
		})
	}
}

func NetworkFilter(rq *apiv2.NetworkQuery) func(q r.Term) r.Term {
	if rq == nil {
		return nil
	}
	return func(q r.Term) r.Term {
		if rq.Project != nil {
			q = q.Filter(func(row r.Term) r.Term {
				return row.Field("projectid").Eq(*rq.Project)
			})
		}

		if rq.Id != nil {
			q = q.Filter(func(row r.Term) r.Term {
				return row.Field("id").Eq(*rq.Id)
			})
		}

		if rq.Name != nil {
			q = q.Filter(func(row r.Term) r.Term {
				return row.Field("name").Eq(*rq.Name)
			})
		}

		if rq.Partition != nil {
			q = q.Filter(func(row r.Term) r.Term {
				return row.Field("partitionid").Eq(*rq.Partition)
			})
		}

		if rq.ParentNetworkId != nil {
			q = q.Filter(func(row r.Term) r.Term {
				return row.Field("parentnetworkid").Eq(*rq.ParentNetworkId)
			})
		}
		if rq.Vrf != nil {
			q = q.Filter(func(row r.Term) r.Term {
				return row.Field("vrf").Eq(*rq.Vrf)
			})
		}
		if rq.Labels != nil {
			for key, value := range rq.Labels.Labels {
				q = q.Filter(func(row r.Term) r.Term {
					return row.Field("labels").Field(key).Eq(value)
				})
			}
		}

		if rq.Options != nil && rq.Options.Underlay != nil {
			q = q.Filter(func(row r.Term) r.Term {
				return row.Field("underlay").Eq(*rq.Options.Underlay)
			})
		}
		if rq.Options != nil && rq.Options.Shared != nil {
			q = q.Filter(func(row r.Term) r.Term {
				return row.Field("shared").Eq(*rq.Options.Shared)
			})
		}

		if rq.Options != nil && rq.Options.PrivateSuper != nil {
			q = q.Filter(func(row r.Term) r.Term {
				return row.Field("privatesuper").Eq(*rq.Options.PrivateSuper)
			})
		}
		if rq.Options != nil && rq.Options.Nat != nil {
			q = q.Filter(func(row r.Term) r.Term {
				return row.Field("nat").Eq(*rq.Options.Nat)
			})
		}
		if rq.AddressFamily != nil {
			var separator string
			switch rq.AddressFamily.String() {
			case apiv2.IPAddressFamily_IP_ADDRESS_FAMILY_V4.String():
				separator = "\\."
			case apiv2.IPAddressFamily_IP_ADDRESS_FAMILY_V6.String():
				separator = ":"
			}

			q = q.Filter(func(row r.Term) r.Term {
				return row.Field("id").Match(separator)
			})
		}

		return q
	}
}
