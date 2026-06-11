package e2e

import (
	"context"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/metal-stack/api/go/client"
	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	infrav2 "github.com/metal-stack/api/go/metalstack/infra/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

	m := createMachine(t, apiClient, apiv2.MachineAllocationType_MACHINE_ALLOCATION_TYPE_MACHINE)

	deleteMachine(t, apiClient, m)
}

func TestFirewallDelete(t *testing.T) {
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

	m := createMachine(t, apiClient, apiv2.MachineAllocationType_MACHINE_ALLOCATION_TYPE_FIREWALL)

	deleteMachine(t, apiClient, m)
}

func deleteMachine(t *testing.T, apiClient client.Client, m *apiv2.Machine) {
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

	_, err := apiClient.Apiv2().Machine().Delete(ctx, &apiv2.MachineServiceDeleteRequest{
		Uuid:    m.Uuid,
		Project: m.Allocation.Project,
	})
	require.NoError(t, err)
	require.NoError(t, bmcerror)

	nodes, err := apiClient.Adminv2().VPN().ListNodes(ctx, &adminv2.VPNServiceListNodesRequest{})
	require.NoError(t, err)

	assert.Empty(t, nodes.Nodes)
}
