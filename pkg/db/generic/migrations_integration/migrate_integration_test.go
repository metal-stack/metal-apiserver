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
	require.Equal(t, p1.PrivateNetworkPrefixLength, n1fetched.DefaultChildPrefixLength[metal.AddressFamilyIPv4], "childprefixlength:%v", n1fetched.DefaultChildPrefixLength)

	n2fetched, err := ds.Network().Get(ctx, n2.ID)
	require.NoError(t, err)
	require.NotNil(t, n2fetched)
	require.Equal(t, p2.PrivateNetworkPrefixLength, n2fetched.DefaultChildPrefixLength[metal.AddressFamilyIPv4], "childprefixlength:%v", n2fetched.DefaultChildPrefixLength)
	require.Equal(t, metal.ChildPrefixLength{metal.AddressFamilyIPv4: 24, metal.AddressFamilyIPv6: 64}, n2fetched.DefaultChildPrefixLength)

	n3fetched, err := ds.Network().Get(ctx, n3.ID)
	require.NoError(t, err)
	require.NotNil(t, n3fetched)
	require.Nil(t, n3fetched.DefaultChildPrefixLength)

	n4fetched, err := ds.Network().Get(ctx, n4.ID)
	require.NoError(t, err)
	require.NotNil(t, n4fetched)
	require.NotNil(t, n4fetched.DefaultChildPrefixLength)
	require.Equal(t, uint8(22), n4fetched.DefaultChildPrefixLength[metal.AddressFamilyIPv4])
}
