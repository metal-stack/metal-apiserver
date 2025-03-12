package migrations

import (
	"context"
	"net/netip"

	"github.com/metal-stack/metal-apiserver/pkg/db/generic"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	r "gopkg.in/rethinkdb/rethinkdb-go.v6"
)

func init() {
	type tmpPartition struct {
		PrivateNetworkPrefixLength uint8 `rethinkdb:"privatenetworkprefixlength"`
	}
	generic.MustRegisterMigration(generic.Migration{
		Name:    "migrate partition.childprefixlength to tenant super network",
		Version: 8,
		Up: func(ctx context.Context, db *r.Term, session r.QueryExecutor, ds generic.Datastore) error {
			nws, err := ds.Network().List(ctx)
			if err != nil {
				return err
			}

			for _, old := range nws {
				cursor, err := db.Table("partition").Get(old.PartitionID).Run(session)
				if err != nil {
					return err
				}
				if cursor.IsNil() {
					_ = cursor.Close()
					continue
				}
				var partition tmpPartition
				err = cursor.One(&partition)
				if err != nil {
					_ = cursor.Close()
					return err
				}
				err = cursor.Close()
				if err != nil {
					return err
				}
				new := *old

				var (
					defaultChildPrefixLength = metal.ChildPrefixLength{}
				)
				for _, prefix := range new.Prefixes {
					parsed, err := netip.ParsePrefix(prefix.String())
					if err != nil {
						return err
					}
					if parsed.Addr().Is4() {
						defaultChildPrefixLength[metal.IPv4AddressFamily] = 22
					}
					if parsed.Addr().Is6() {
						defaultChildPrefixLength[metal.IPv6AddressFamily] = 64
					}
				}

				if new.PrivateSuper {
					if new.DefaultChildPrefixLength == nil {
						new.DefaultChildPrefixLength = metal.ChildPrefixLength{}
					}
					if partition.PrivateNetworkPrefixLength > 0 {
						defaultChildPrefixLength[metal.IPv4AddressFamily] = partition.PrivateNetworkPrefixLength
					}
					new.DefaultChildPrefixLength = defaultChildPrefixLength
				}

				err = ds.Network().Update(ctx, &new, old)
				if err != nil {
					return err
				}
			}

			_, err = db.Table("partition").Replace(r.Row.Without("privatenetworkprefixlength")).RunWrite(session)
			return err
		},
	})
}
