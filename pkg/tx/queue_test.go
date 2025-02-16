package tx_test

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/metal-stack/api-server/pkg/db/generic"
	"github.com/metal-stack/api-server/pkg/db/metal"
	"github.com/metal-stack/api-server/pkg/db/queries"
	"github.com/metal-stack/api-server/pkg/test"
	"github.com/metal-stack/api-server/pkg/tx"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	ipamv1 "github.com/metal-stack/go-ipam/api/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQueue(t *testing.T) {
	ctx := context.Background()

	container, c, err := test.StartRethink(t)
	require.NoError(t, err)
	defer func() {
		_ = container.Terminate(context.Background())
	}()

	valkeyContainer, client, err := test.StartValkey(t, ctx)
	require.NoError(t, err)
	defer func() {
		_ = valkeyContainer.Terminate(context.Background())
	}()

	ipam := test.StartIpam(t)

	log := slog.Default()

	ds, err := generic.New(log, "metal", c)
	require.NoError(t, err)

	actionFns := tx.ActionFns{
		tx.ActionIpDelete: func(id string) error {
			metalIP, err := ds.IP().Find(ctx, queries.IpFilter(&apiv2.IPServiceListRequest{Uuid: &id}))
			if err != nil && !generic.IsNotFound(err) {
				return err
			}

			_, err = ipam.ReleaseIP(ctx, connect.NewRequest(&ipamv1.ReleaseIPRequest{PrefixCidr: metalIP.ParentPrefixCidr, Ip: metalIP.IPAddress}))
			var connectErr *connect.Error
			if errors.As(err, &connectErr) {
				if connectErr.Code() != connect.CodeNotFound {
					return err
				}
			}

			err = ds.IP().Delete(ctx, metalIP)
			if err != nil && !generic.IsNotFound(err) {
				return err
			}

			return nil
		},
	}

	q, err := tx.New(log, client, actionFns)
	require.NoError(t, err)

	pfx, err := ipam.CreatePrefix(ctx, connect.NewRequest(&ipamv1.CreatePrefixRequest{Cidr: "1.2.3.0/24"}))
	require.NoError(t, err)
	ipamIP, err := ipam.AcquireIP(ctx, connect.NewRequest(&ipamv1.AcquireIPRequest{PrefixCidr: pfx.Msg.Prefix.Cidr}))
	require.NoError(t, err)

	allocationUUID := uuid.NewString()
	metalIP, err := ds.IP().Create(ctx, &metal.IP{
		IPAddress:      ipamIP.Msg.Ip.Ip,
		AllocationUUID: allocationUUID,
	})
	require.NoError(t, err)

	ipdeleteTx := &tx.Tx{
		Jobs: []tx.Job{
			{
				ID:     metalIP.AllocationUUID,
				Action: tx.ActionIpDelete,
			},
		},
	}

	err = q.Insert(ctx, ipdeleteTx)
	require.NoError(t, err)

	// Now check that the IPs are really released

	assert.EventuallyWithT(t, func(collect *assert.CollectT) {
		_, err = ds.IP().Get(ctx, metalIP.IPAddress)
		if err != nil && !generic.IsNotFound(err) {
			t.Fail()
		}
		pfx, err := ipam.GetPrefix(ctx, connect.NewRequest(&ipamv1.GetPrefixRequest{Cidr: pfx.Msg.Prefix.Cidr}))
		require.NoError(t, err)
		_, err = ipam.AcquireIP(ctx, connect.NewRequest(&ipamv1.AcquireIPRequest{PrefixCidr: pfx.Msg.Prefix.Cidr, Ip: &ipamIP.Msg.Ip.Ip}))
		require.Error(t, err)
	}, time.Second, 100*time.Millisecond)

}
