package tx_test

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/metal-stack/api-server/pkg/db/generic"
	"github.com/metal-stack/api-server/pkg/db/metal"
	"github.com/metal-stack/api-server/pkg/db/queries"
	"github.com/metal-stack/api-server/pkg/db/tx"
	"github.com/metal-stack/api-server/pkg/test"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	ipamv1 "github.com/metal-stack/go-ipam/api/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTasks(t *testing.T) {
	ctx := context.Background()
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	container, rethinkSession, err := test.StartRethink(t)
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

	ds, err := generic.New(log, "metal", rethinkSession)
	require.NoError(t, err)

	actionFn := func(ctx context.Context, job tx.Step) error {
		switch job.Action {
		case tx.ActionIpDelete:

			metalIP, err := ds.IP().Find(ctx, queries.IpFilter(&apiv2.IPQuery{Uuid: &job.ID}))
			if err != nil && !generic.IsNotFound(err) {
				return err
			}
			log.Info("ds find", "metalip", metalIP)

			_, err = ipam.ReleaseIP(ctx, connect.NewRequest(&ipamv1.ReleaseIPRequest{PrefixCidr: metalIP.ParentPrefixCidr, Ip: metalIP.IPAddress}))
			if err != nil {
				log.Error("ipam release", "error", err)
				var connectErr *connect.Error
				errors.As(err, &connectErr)
				if connectErr.Code() != connect.CodeNotFound {
					return err
				}
			}

			err = ds.IP().Delete(ctx, metalIP)
			if err != nil && !generic.IsNotFound(err) {
				log.Error("ds delete", "error", err)
				return err
			}

			return nil
		case tx.ActionNetworkDelete:
			return fmt.Errorf("action:%s is not implemented yet", job.Action)
		default:
			return fmt.Errorf("action:%s is not implemented yet", job.Action)
		}
	}

	q, err := tx.New(log, client, actionFn)
	require.NoError(t, err)

	pfx, err := ipam.CreatePrefix(ctx, connect.NewRequest(&ipamv1.CreatePrefixRequest{Cidr: "1.2.3.0/24"}))
	require.NoError(t, err)
	ipamIP, err := ipam.AcquireIP(ctx, connect.NewRequest(&ipamv1.AcquireIPRequest{PrefixCidr: pfx.Msg.Prefix.Cidr}))
	require.NoError(t, err)

	allocationUUID := uuid.NewString()
	metalIP, err := ds.IP().Create(ctx, &metal.IP{
		IPAddress:        ipamIP.Msg.Ip.Ip,
		AllocationUUID:   allocationUUID,
		ParentPrefixCidr: pfx.Msg.Prefix.Cidr,
	})
	require.NoError(t, err)

	ipdeleteTx := &tx.Task{
		Steps: []tx.Step{
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
		// metal entity must be gone
		_, err = ds.IP().Get(ctx, metalIP.IPAddress)
		require.EqualError(t, err, generic.NotFound("no ip with id %q found", metalIP.IPAddress).Error())

		// ipam entity as well, check by trying to acquire the same again
		_, err = ipam.AcquireIP(ctx, connect.NewRequest(&ipamv1.AcquireIPRequest{PrefixCidr: pfx.Msg.Prefix.Cidr, Ip: &ipamIP.Msg.Ip.Ip}))
		require.NoError(t, err)
	}, time.Second, 100*time.Millisecond)

}
