package queries

import (
	"net/netip"
	"strconv"

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

		if rq.Type != nil && rq.Type == apiv2.NetworkType_NETWORK_TYPE_UNDERLAY.Enum() {
			q = q.Filter(func(row r.Term) r.Term {
				return row.Field("underlay").Eq(true)
			})
		}
		if rq.Type != nil && rq.Type == apiv2.NetworkType_NETWORK_TYPE_SHARED.Enum() {
			q = q.Filter(func(row r.Term) r.Term {
				return row.Field("shared").Eq(true)
			})
		}

		if rq.Type != nil && (rq.Type == apiv2.NetworkType_NETWORK_TYPE_PRIVATE_SUPER.Enum() || rq.Type == apiv2.NetworkType_NETWORK_TYPE_SUPER_VRF_SHARED.Enum()) {
			q = q.Filter(func(row r.Term) r.Term {
				return row.Field("privatesuper").Eq(true)
			})
		}
		if rq.NatType != nil && rq.NatType == apiv2.NATType_NAT_TYPE_IPV4_MASQUERADE.Enum() {
			q = q.Filter(func(row r.Term) r.Term {
				return row.Field("nat").Eq(true)
			})
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
			var separator string
			switch rq.AddressFamily.String() {
			case apiv2.IPAddressFamily_IP_ADDRESS_FAMILY_V4.String():
				separator = "\\."
			case apiv2.IPAddressFamily_IP_ADDRESS_FAMILY_V6.String():
				separator = ":"
			}

			q = q.Filter(func(row r.Term) r.Term {
				return row.Field("prefixes").Contains(func(p r.Term) r.Term {
					return p.Field("ip").Match(separator)
				})
			})
		}

		return q
	}
}
