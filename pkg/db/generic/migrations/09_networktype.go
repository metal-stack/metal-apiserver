package migrations

import (
	"context"
	"slices"

	"github.com/metal-stack/metal-lib/pkg/pointer"

	"github.com/metal-stack/metal-apiserver/pkg/db/generic"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	r "gopkg.in/rethinkdb/rethinkdb-go.v6"
)

func init() {

	generic.MustRegisterMigration(generic.Migration{
		Name:    "migrate networks to have proper networkType set",
		Version: 9,
		Up: func(ctx context.Context, db *r.Term, session r.QueryExecutor, ds generic.Datastore) error {
			nws, err := ds.Network().List(ctx)
			if err != nil {
				return err
			}
			// first detect all shared vrf ids
			// maps vrfid > count
			var (
				vrfCount   = make(map[uint]int)
				sharedVrfs []uint
			)
			for _, nw := range nws {
				if nw.Vrf == 0 {
					continue
				}
				count, ok := vrfCount[nw.Vrf]
				if !ok {
					vrfCount[nw.Vrf] = 1
				} else {
					vrfCount[nw.Vrf] = count + 1
				}
			}
			for vrf, count := range vrfCount {
				if count > 1 {
					sharedVrfs = append(sharedVrfs, vrf)
				}
			}

			// No convert all networks
			for _, old := range nws {
				new := *old
				if slices.Contains(sharedVrfs, old.Vrf) {
					if old.ParentNetworkID != "" {
						new.NetworkType = pointer.Pointer(metal.VrfSharedNetworkType)
					} else {
						new.NetworkType = pointer.Pointer(metal.SuperVrfSharedNetworkType)
					}
				}
				if old.Shared && old.ParentNetworkID != "" {
					new.NetworkType = pointer.Pointer(metal.PrivateSharedNetworkType)
				}
				if old.Shared && old.ParentNetworkID == "" {
					new.NetworkType = pointer.Pointer(metal.SharedNetworkType)
				}
				if !old.Shared && old.ParentNetworkID != "" && !slices.Contains(sharedVrfs, old.Vrf) {
					new.NetworkType = pointer.Pointer(metal.PrivateNetworkType)
				}
				// TODO: This is weird in the current metal-api implementation, internet is not shared ?
				if old.ProjectID == "" && old.ParentNetworkID == "" && !slices.Contains(sharedVrfs, old.Vrf) {
					new.NetworkType = pointer.Pointer(metal.SharedNetworkType)
				}
				if old.PrivateSuper {
					new.NetworkType = pointer.Pointer(metal.PrivateSuperNetworkType)
				}
				if old.Underlay {
					new.NetworkType = pointer.Pointer(metal.UnderlayNetworkType)
				}

				if old.Nat {
					new.NATType = pointer.Pointer(metal.IPv4MasqueradeNATType)
				} else {
					new.NATType = pointer.Pointer(metal.NoneNATType)
				}

				err := ds.Network().Update(ctx, &new)
				if err != nil {
					return err
				}
			}

			return nil
		},
	})
}
