package e2e

import (
	"context"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/metal-stack/api/go/client"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	infrav2 "github.com/metal-stack/api/go/metalstack/infra/v2"
	"github.com/stretchr/testify/require"
)

// var (
// 	partition1 = "partition-1"
// 	sw1        = &api.SwitchServiceCreateRequest{
// 		Switch: &apiv2.Switch{
// 			Id:           "sw1",
// 			Meta:         &apiv2.Meta{},
// 			Partition:    partition1,
// 			Rack:         new("r01"),
// 			ManagementIp: "1.2.3.3",
// 			ReplaceMode:  apiv2.SwitchReplaceMode_SWITCH_REPLACE_MODE_OPERATIONAL,
// 			Nics: []*apiv2.SwitchNic{
// 				{
// 					Name:       "Ethernet0",
// 					Identifier: "Eth1/1",
// 					State: &apiv2.NicState{
// 						Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
// 					},
// 					Mac: "00:00:00:00:00:01",
// 				},
// 				{
// 					Name:       "Ethernet1",
// 					Identifier: "Eth1/2",
// 					State: &apiv2.NicState{
// 						Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
// 					},
// 					Mac: "00:00:00:00:00:02",
// 				},
// 			},
// 			Os: &apiv2.SwitchOS{
// 				Version:          "1.0",
// 				MetalCoreVersion: "1.0",
// 				Vendor:           apiv2.SwitchOSVendor_SWITCH_OS_VENDOR_SONIC,
// 			},
// 		},
// 	}

// 	sw2 = &api.SwitchServiceCreateRequest{
// 		Switch: &apiv2.Switch{
// 			Id:           "sw2",
// 			Meta:         &apiv2.Meta{},
// 			Partition:    partition1,
// 			Rack:         new("r01"),
// 			ManagementIp: "1.2.3.4",
// 			ReplaceMode:  apiv2.SwitchReplaceMode_SWITCH_REPLACE_MODE_OPERATIONAL,
// 			Nics: []*apiv2.SwitchNic{
// 				{
// 					Name:       "Ethernet0",
// 					Identifier: "Eth1/1",
// 					State: &apiv2.NicState{
// 						Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
// 					},
// 					Mac: "01:00:00:00:00:01",
// 				},
// 				{
// 					Name:       "Ethernet1",
// 					Identifier: "Eth1/2",
// 					State: &apiv2.NicState{
// 						Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
// 					},
// 					Mac: "01:00:00:00:00:02",
// 				},
// 			},
// 			Os: &apiv2.SwitchOS{
// 				Version:          "1.0",
// 				MetalCoreVersion: "1.0",
// 				Vendor:           apiv2.SwitchOSVendor_SWITCH_OS_VENDOR_SONIC,
// 			},
// 		},
// 	}
// )

func TestMachineDelete(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	baseURL, adminToken, _, closer := StartApiserver(t, log)
	defer closer()
	require.NotNil(t, baseURL, adminToken)

	apiClient, err := client.New(&client.DialConfig{
		BaseURL:   baseURL,
		Token:     adminToken,
		UserAgent: "integration test",
		Log:       log,
	})
	require.NoError(t, err)

	m := createMachine(t, apiClient)

	var (
		bmcDoneResponse *infrav2.WaitForBMCCommandResponse
		bmcerror        error
		mu              sync.Mutex
	)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	machineBmcDeleteSimulation := func() {
		stream, err := apiClient.Infrav2().BMC().WaitForBMCCommand(ctx, &infrav2.WaitForBMCCommandRequest{
			Partition: partition1,
		})
		if err != nil {
			bmcerror = err
			return
		}
		defer func() {
			_ = stream.Close()
		}()
		for stream.Receive() {
			mu.Lock()
			defer mu.Unlock()
			bmcDoneResponse = stream.Msg()

			_, err = apiClient.Infrav2().BMC().BMCCommandDone(ctx, &infrav2.BMCCommandDoneRequest{
				CommandId: bmcDoneResponse.CommandId,
				Error:     nil,
			})

			bmcerror = err

			t.Logf("bmc command wait stopped %v", bmcDoneResponse)
			return
		}
	}
	go machineBmcDeleteSimulation()
	time.Sleep(1 * time.Second)

	_, err = apiClient.Apiv2().Machine().Delete(ctx, &apiv2.MachineServiceDeleteRequest{
		Uuid:    m.Uuid,
		Project: m.Allocation.Project,
	})
	require.NoError(t, err)
	require.NoError(t, bmcerror)
}
