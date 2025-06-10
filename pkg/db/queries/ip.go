package queries

import (
	"fmt"

	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-lib/pkg/tag"

	r "gopkg.in/rethinkdb/rethinkdb-go.v6"
)

func IpProjectScoped(project string) func(q r.Term) r.Term {
	return func(q r.Term) r.Term {
		return q.Filter(func(row r.Term) r.Term {
			return row.Field("projectid").Eq(project)
		})
	}
}

func IpFilter(rq *apiv2.IPQuery) func(q r.Term) r.Term {
	if rq == nil {
		return nil
	}
	return func(q r.Term) r.Term {
		if rq.Project != nil {
			q = q.Filter(func(row r.Term) r.Term {
				return row.Field("projectid").Eq(*rq.Project)
			})
		}

		if rq.Ip != nil {
			q = q.Filter(func(row r.Term) r.Term {
				return row.Field("id").Eq(*rq.Ip)
			})
		}

		if rq.Uuid != nil {
			q = q.Filter(func(row r.Term) r.Term {
				return row.Field("allocationuuid").Eq(*rq.Uuid)
			})
		}

		if rq.Name != nil {
			q = q.Filter(func(row r.Term) r.Term {
				return row.Field("name").Eq(*rq.Name)
			})
		}

		if rq.Namespace != nil {
			q = q.Filter(func(row r.Term) r.Term {
				return row.Field("namespace").Eq(*rq.Namespace)
			})
		}

		if rq.Network != nil {
			q = q.Filter(func(row r.Term) r.Term {
				return row.Field("networkid").Eq(*rq.Network)
			})
		}

		if rq.ParentPrefixCidr != nil {
			q = q.Filter(func(row r.Term) r.Term {
				return row.Field("prefix").Eq(*rq.ParentPrefixCidr)
			})
		}

		if rq.MachineId != nil {
			tag := fmt.Sprintf("%s=%s", tag.MachineID, *rq.MachineId)
			q = q.Filter(func(row r.Term) r.Term {
				return row.Field("tags").Contains(r.Expr(tag))
			})
		}

		if rq.Labels != nil {
			for key, value := range rq.Labels.Labels {
				tag := fmt.Sprintf("%s=%s", key, value)
				q = q.Filter(func(row r.Term) r.Term {
					return row.Field("tags").Contains(r.Expr(tag))
				})
			}
		}

		if rq.Type != nil {
			q = q.Filter(func(row r.Term) r.Term {
				return row.Field("type").Eq(rq.Type.String())
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
