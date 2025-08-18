package queries

import (
	"net/netip"
	"strconv"

	"github.com/metal-stack/api/go/enum"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"

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

		if rq.Namespace != nil {
			q = q.Filter(func(row r.Term) r.Term {
				return row.Field("namespace").Eq(*rq.Namespace)
			})
		}

		if rq.Description != nil {
			q = q.Filter(func(row r.Term) r.Term {
				return row.Field("description").Eq(*rq.Description)
			})
		}

		if rq.Partition != nil {
			q = q.Filter(func(row r.Term) r.Term {
				return row.Field("partitionid").Eq(*rq.Partition)
			})
		}

		if rq.ParentNetwork != nil {
			q = q.Filter(func(row r.Term) r.Term {
				return row.Field("ParentNetwork").Eq(*rq.ParentNetwork)
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

		if rq.Type != nil {
			stringValue, err := enum.GetStringValue(rq.Type)
			if err == nil {
				q = q.Filter(func(row r.Term) r.Term {
					return row.Field("networktype").Eq(stringValue)
				})
			}
		}

		if rq.NatType != nil {
			nt, err := metal.ToNATType(*rq.NatType)
			if err == nil {
				q = q.Filter(func(row r.Term) r.Term {
					return row.Field("nattype").Eq(string(nt))
				})
			}
		}

		for _, prefix := range rq.Prefixes {
			pfx := netip.MustParsePrefix(prefix)
			ip := pfx.Addr()
			length := pfx.Bits()

			q = q.Filter(func(row r.Term) r.Term {
				return row.Field("prefixes").Map(func(p r.Term) r.Term {
					return p.Field("ip")
				}).Contains(r.Expr(ip.String()))
			})

			q = q.Filter(func(row r.Term) r.Term {
				return row.Field("prefixes").Map(func(p r.Term) r.Term {
					return p.Field("length")
				}).Contains(r.Expr(strconv.Itoa(length)))
			})
		}

		for _, destPrefix := range rq.DestinationPrefixes {
			pfx := netip.MustParsePrefix(destPrefix)
			ip := pfx.Addr()
			length := pfx.Bits()

			q = q.Filter(func(row r.Term) r.Term {
				return row.Field("destinationprefixes").Map(func(dp r.Term) r.Term {
					return dp.Field("ip")
				}).Contains(r.Expr(ip.String()))
			})

			q = q.Filter(func(row r.Term) r.Term {
				return row.Field("destinationprefixes").Map(func(dp r.Term) r.Term {
					return dp.Field("length")
				}).Contains(r.Expr(strconv.Itoa(length)))
			})
		}

		if rq.AddressFamily != nil {
			const (
				ipv4Separator = "\\."
				ipv6Separator = ":"
			)

			var separator string

			switch rq.AddressFamily.String() {
			case apiv2.NetworkAddressFamily_NETWORK_ADDRESS_FAMILY_V4.String():
				separator = ipv4Separator
			case apiv2.NetworkAddressFamily_NETWORK_ADDRESS_FAMILY_V6.String():
				separator = ipv6Separator
			case apiv2.NetworkAddressFamily_NETWORK_ADDRESS_FAMILY_DUAL_STACK.String():
				q = q.Filter(func(row r.Term) r.Term {
					return row.Field("prefixes").Contains(func(p r.Term) r.Term {
						return p.Field("ip").Match(ipv4Separator)
					}).And(row.Field("prefixes").Contains(func(p r.Term) r.Term {
						return p.Field("ip").Match(ipv6Separator)
					}))
				})
			}

			if separator != "" {
				q = q.Filter(func(row r.Term) r.Term {
					return row.Field("prefixes").Contains(func(p r.Term) r.Term {
						return p.Field("ip").Match(separator)
					})
				})
			}
		}

		return q
	}
}
