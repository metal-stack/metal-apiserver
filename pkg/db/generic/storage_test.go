package generic_test

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-apiserver/pkg/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_storage_Watch(t *testing.T) {
	t.Parallel()

	var (
		log         = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
		ctx         = t.Context()
		testMachine = &metal.Machine{
			ID: "123",
			Allocation: &metal.MachineAllocation{
				UUID: "456",
				MachineNetworks: []*metal.MachineNetwork{
					{
						NetworkID: "a",
						IPs: []string{
							"1.2.3.4",
						},
						NetworkType: metal.NetworkTypeChildShared,
					},
				},
			},
		}
	)

	ds, _, rethinkCloser := test.StartRethink(t, log)
	defer func() {
		rethinkCloser()
	}()

	watchCtx, watchCancel := context.WithTimeout(ctx, 5*time.Second)
	defer watchCancel()

	channel, err := ds.Machine().Watch(watchCtx, testMachine.ID)
	require.NoError(t, err)

	m, err := ds.Machine().Create(ctx, testMachine)
	require.NoError(t, err)

	pair := <-channel

	assert.Nil(t, pair.Old)
	require.NotNil(t, pair.New)
	assert.Equal(t, "123", pair.New.ID)
	require.NotNil(t, pair.New.Allocation)
	require.NotNil(t, pair.New.Allocation.MachineNetworks)
	require.Len(t, pair.New.Allocation.MachineNetworks, 1)
	assert.Equal(t, "456", pair.New.Allocation.UUID)
	assert.Equal(t, "a", pair.New.Allocation.MachineNetworks[0].NetworkID)

	err = ds.Machine().Update(ctx, &metal.Machine{
		ID:      testMachine.ID,
		Changed: m.Changed,
	})
	require.NoError(t, err)

	pair = <-channel

	require.NotNil(t, pair.Old)
	assert.Equal(t, "123", pair.Old.ID)
	require.NotNil(t, pair.Old.Allocation)
	assert.Equal(t, "456", pair.Old.Allocation.UUID)
	require.NotNil(t, pair.New)
	assert.Equal(t, "123", pair.New.ID)
	require.Nil(t, pair.New.Allocation)
}
