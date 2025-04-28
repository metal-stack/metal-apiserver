//go:build integration

package migrations_integration

import (
	"log/slog"
	"os"

	"github.com/metal-stack/metal-apiserver/pkg/db/generic"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-apiserver/pkg/test"

	r "gopkg.in/rethinkdb/rethinkdb-go.v6"

	"testing"

	"github.com/stretchr/testify/require"

	_ "github.com/metal-stack/metal-apiserver/pkg/db/generic/migrations"
)

func Test_MigrationChildPrefixLength(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	type tmpPartition struct {
		ID                         string `rethinkdb:"id"`
		PrivateNetworkPrefixLength uint8  `rethinkdb:"privatenetworkprefixlength"`
	}

	ds, c, rethinkCloser := test.StartRethink(t, log)
	defer func() {
		rethinkCloser()
	}()

	ctx := t.Context()

	var (
		p1 = &tmpPartition{
			ID:                         "p1",
			PrivateNetworkPrefixLength: 22,
		}
		p2 = &tmpPartition{
			ID:                         "p2",
			PrivateNetworkPrefixLength: 24,
		}
		p3 = &tmpPartition{
			ID: "p3",
		}
		n1 = &metal.Network{
			Base: metal.Base{
				ID: "n1",
			},
			PartitionID: "p1",
			Prefixes: metal.Prefixes{
				{IP: "10.0.0.0", Length: "8"},
			},
			PrivateSuper: true,
		}
		n2 = &metal.Network{
			Base: metal.Base{
				ID: "n2",
			},
			Prefixes: metal.Prefixes{
				{IP: "2001::", Length: "64"},
			},
			PartitionID:  "p2",
			PrivateSuper: true,
		}
		n3 = &metal.Network{
			Base: metal.Base{
				ID: "n3",
			},
			Prefixes: metal.Prefixes{
				{IP: "100.1.0.0", Length: "22"},
			},
			PartitionID:  "p2",
			PrivateSuper: false,
		}
		n4 = &metal.Network{
			Base: metal.Base{
				ID: "n4",
			},
			Prefixes: metal.Prefixes{
				{IP: "100.1.0.0", Length: "22"},
			},
			PartitionID:  "p3",
			PrivateSuper: true,
		}
		n5 = &metal.Network{
			Base: metal.Base{
				ID: "n5",
			},
			Prefixes: metal.Prefixes{
				{IP: "9.0.0.0", Length: "8"},
			},
			PrivateSuper: true,
		}
	)

	session, err := r.Connect(c)
	require.NoError(t, err)

	_, err = r.DB("metal").Table("partition").Insert(p1).RunWrite(session)
	require.NoError(t, err)
	_, err = r.DB("metal").Table("partition").Insert(p2).RunWrite(session)
	require.NoError(t, err)
	_, err = r.DB("metal").Table("partition").Insert(p3).RunWrite(session)
	require.NoError(t, err)

	_, err = ds.Network().Create(ctx, n1)
	require.NoError(t, err)
	_, err = ds.Network().Create(ctx, n2)
	require.NoError(t, err)
	_, err = ds.Network().Create(ctx, n3)
	require.NoError(t, err)
	_, err = ds.Network().Create(ctx, n4)
	require.NoError(t, err)
	_, err = ds.Network().Create(ctx, n5)
	require.NoError(t, err)

	err = generic.Migrate(ctx, c, log, nil, false)
	require.NoError(t, err)

	p, err := ds.Partition().Get(ctx, p1.ID)
	require.NoError(t, err)
	require.NotNil(t, p)
	p, err = ds.Partition().Get(ctx, p2.ID)
	require.NoError(t, err)
	require.NotNil(t, p)

	n1fetched, err := ds.Network().Get(ctx, n1.ID)
	require.NoError(t, err)
	require.NotNil(t, n1fetched)
	require.Equal(t, p1.PrivateNetworkPrefixLength, n1fetched.DefaultChildPrefixLength[metal.IPv4AddressFamily], "childprefixlength:%v", n1fetched.DefaultChildPrefixLength)

	n2fetched, err := ds.Network().Get(ctx, n2.ID)
	require.NoError(t, err)
	require.NotNil(t, n2fetched)
	require.Equal(t, p2.PrivateNetworkPrefixLength, n2fetched.DefaultChildPrefixLength[metal.IPv4AddressFamily], "childprefixlength:%v", n2fetched.DefaultChildPrefixLength)
	require.Equal(t, metal.ChildPrefixLength{metal.IPv4AddressFamily: 24, metal.IPv6AddressFamily: 64}, n2fetched.DefaultChildPrefixLength)

	n3fetched, err := ds.Network().Get(ctx, n3.ID)
	require.NoError(t, err)
	require.NotNil(t, n3fetched)
	require.Nil(t, n3fetched.DefaultChildPrefixLength)

	n4fetched, err := ds.Network().Get(ctx, n4.ID)
	require.NoError(t, err)
	require.NotNil(t, n4fetched)
	require.NotNil(t, n4fetched.DefaultChildPrefixLength)
	require.Equal(t, uint8(22), n4fetched.DefaultChildPrefixLength[metal.IPv4AddressFamily])
}

func Test_MigrationNetworkType(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	ds, c, rethinkCloser := test.StartRethink(t, log)
	defer func() {
		rethinkCloser()
	}()

	ctx := t.Context()

	nws := []*metal.Network{
		{Base: metal.Base{ID: "internet"}, Vrf: 10, Nat: true, Shared: false},
		{Base: metal.Base{ID: "underlay"}, Underlay: true},
		{Base: metal.Base{ID: "dc-interconnect"}, Vrf: 201},
		{Base: metal.Base{ID: "dc-interconnect-child-1"}, ParentNetworkID: "tenant-super", Vrf: 201},
		{Base: metal.Base{ID: "dc-interconnect-child-2"}, ParentNetworkID: "tenant-super", Vrf: 201},
		{Base: metal.Base{ID: "tenant-super"}, PrivateSuper: true},
		{Base: metal.Base{ID: "private-network-1"}, ParentNetworkID: "tenant-super", Vrf: 101},
		{Base: metal.Base{ID: "private-network-2"}, ParentNetworkID: "tenant-super", Vrf: 102},
		{Base: metal.Base{ID: "private-network-3"}, ParentNetworkID: "tenant-super", Vrf: 103},
		{Base: metal.Base{ID: "private-network-4"}, ParentNetworkID: "tenant-super", Vrf: 104},
		{Base: metal.Base{ID: "partition-storage"}, ParentNetworkID: "tenant-super", Vrf: 105, Shared: true},
	}

	for _, nw := range nws {
		_, err := ds.Network().Create(ctx, nw)
		require.NoError(t, err)
	}

	err := generic.Migrate(ctx, c, log, nil, false)
	require.NoError(t, err)

	internet, err := ds.Network().Get(ctx, "internet")
	require.NoError(t, err)
	require.NotNil(t, internet)
	require.Equal(t, metal.IPv4MasqueradeNATType, *internet.NATType)
	require.Equal(t, metal.SharedNetworkType, *internet.NetworkType)

	underlay, err := ds.Network().Get(ctx, "underlay")
	require.NoError(t, err)
	require.NotNil(t, underlay)
	require.Equal(t, metal.NoneNATType, *underlay.NATType)
	require.Equal(t, metal.UnderlayNetworkType, *underlay.NetworkType)

	dcInterconnect, err := ds.Network().Get(ctx, "dc-interconnect")
	require.NoError(t, err)
	require.NotNil(t, dcInterconnect)
	require.Equal(t, metal.NoneNATType, *dcInterconnect.NATType)
	require.Equal(t, metal.SuperVrfSharedNetworkType, *dcInterconnect.NetworkType)

	dcInterconnect1, err := ds.Network().Get(ctx, "dc-interconnect-child-1")
	require.NoError(t, err)
	require.NotNil(t, dcInterconnect1)
	require.Equal(t, metal.NoneNATType, *dcInterconnect1.NATType)
	require.Equal(t, metal.VrfSharedNetworkType, *dcInterconnect1.NetworkType)

	dcInterconnect2, err := ds.Network().Get(ctx, "dc-interconnect-child-2")
	require.NoError(t, err)
	require.NotNil(t, dcInterconnect2)
	require.Equal(t, metal.NoneNATType, *dcInterconnect2.NATType)
	require.Equal(t, metal.VrfSharedNetworkType, *dcInterconnect2.NetworkType)

	tenantSuper, err := ds.Network().Get(ctx, "tenant-super")
	require.NoError(t, err)
	require.NotNil(t, tenantSuper)
	require.Equal(t, metal.NoneNATType, *tenantSuper.NATType)
	require.Equal(t, metal.PrivateSuperNetworkType, *tenantSuper.NetworkType)

	private1, err := ds.Network().Get(ctx, "private-network-1")
	require.NoError(t, err)
	require.NotNil(t, private1)
	require.Equal(t, metal.NoneNATType, *private1.NATType)
	require.Equal(t, metal.PrivateNetworkType, *private1.NetworkType)

	private2, err := ds.Network().Get(ctx, "private-network-2")
	require.NoError(t, err)
	require.NotNil(t, private2)
	require.Equal(t, metal.NoneNATType, *private2.NATType)
	require.Equal(t, metal.PrivateNetworkType, *private2.NetworkType)

	private3, err := ds.Network().Get(ctx, "private-network-3")
	require.NoError(t, err)
	require.NotNil(t, private3)
	require.Equal(t, metal.NoneNATType, *private3.NATType)
	require.Equal(t, metal.PrivateNetworkType, *private3.NetworkType)

	private4, err := ds.Network().Get(ctx, "private-network-4")
	require.NoError(t, err)
	require.NotNil(t, private4)
	require.Equal(t, metal.NoneNATType, *private4.NATType)
	require.Equal(t, metal.PrivateNetworkType, *private4.NetworkType)

	partitionStorage, err := ds.Network().Get(ctx, "partition-storage")
	require.NoError(t, err)
	require.NotNil(t, partitionStorage)
	require.Equal(t, metal.NoneNATType, *partitionStorage.NATType)
	require.Equal(t, metal.PrivateSharedNetworkType, *partitionStorage.NetworkType)
}
