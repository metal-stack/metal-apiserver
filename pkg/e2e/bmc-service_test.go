package e2e

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
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

	machinebmccommand "github.com/metal-stack/metal-apiserver/pkg/async/machine-bmc-command"
)

var (
	m0 = "00000000-0000-0000-0000-000000000000"
)

func TestWaitForBMCCommandSync(t *testing.T) {
	// TODO test more scenarios with more receivers
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	baseURL, adminToken, _, closer := StartApiserver(t, log)
	defer closer()
	require.NotNil(t, baseURL, adminToken)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintln(w, "a image")
	}))

	validURL := ts.URL
	defer ts.Close()

	apiClient, err := client.New(&client.DialConfig{
		BaseURL:   baseURL,
		Token:     adminToken,
		UserAgent: "integration test",
		Log:       log,
	})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(t.Context())

	p, err := apiClient.Adminv2().Partition().Create(ctx, &adminv2.PartitionServiceCreateRequest{
		Partition: &apiv2.Partition{Id: "partition-1", BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}},
	})
	require.NoError(t, err)

	_, err = apiClient.Infrav2().Boot().Dhcp(ctx, &infrav2.BootServiceDhcpRequest{Uuid: m0, Partition: p.Partition.Id})
	require.NoError(t, err)
	_, err = apiClient.Infrav2().BMC().UpdateBMCInfo(ctx, &infrav2.UpdateBMCInfoRequest{Partition: p.Partition.Id, BmcReports: map[string]*apiv2.MachineBMCReport{
		m0: {
			Bmc: &apiv2.MachineBMC{Address: "192.168.0.1", User: "metal", Password: "secret", Mac: "00:00:00:00:00:01"},
		},
	}})
	require.NoError(t, err)
	// Now we have a machine with bmc details

	var (
		waitResponses []*infrav2.WaitForBMCCommandResponse
		mu            sync.RWMutex
	)
	go func() {
		// We simulate metal-bmc watching for machine events
		stream, err := apiClient.Infrav2().BMC().WaitForBMCCommand(ctx, &infrav2.WaitForBMCCommandRequest{Partition: p.Partition.Id})
		if err != nil {
			return
		}
		defer func() {
			_ = stream.Close()
		}()

		for stream.Receive() {
			mu.Lock()
			waitResponses = append(waitResponses, stream.Msg())
			mu.Unlock()
		}
	}()

	go func() {
		// We simulate a second metal-bmc watching for machine events
		stream, err := apiClient.Infrav2().BMC().WaitForBMCCommand(ctx, &infrav2.WaitForBMCCommandRequest{Partition: p.Partition.Id})
		if err != nil {
			return
		}
		defer func() {
			_ = stream.Close()
		}()

		for stream.Receive() {
			mu.Lock()
			waitResponses = append(waitResponses, stream.Msg())
			mu.Unlock()
		}
	}()

	// Give subscription time to establish
	time.Sleep(1 * time.Second)

	// Publish a message
	_, err = apiClient.Adminv2().Machine().BMCCommand(ctx,
		&adminv2.MachineServiceBMCCommandRequest{
			Uuid:    m0,
			Command: apiv2.MachineBMCCommand_MACHINE_BMC_COMMAND_BOOT_FROM_DISK},
	)
	require.NoError(t, err)

	require.EventuallyWithT(t, func(c *assert.CollectT) {
		mu.RLock()
		defer mu.RUnlock()
		require.NotNil(c, waitResponses)
		require.Len(c, waitResponses, 1)
		require.Equal(c, apiv2.MachineBMCCommand_MACHINE_BMC_COMMAND_BOOT_FROM_DISK, waitResponses[0].BmcCommand)
	}, 5*time.Second, 100*time.Millisecond)

	// We need to cancel the context because otherwise the WaitForMachineEvent blocks the grpc http server from stopping
	cancel()
}

func TestWaitForBMCCommandAsync(t *testing.T) {
	// TODO test more scenarios with more receivers
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	baseURL, adminToken, _, closer := StartApiserver(t, log)
	defer closer()
	require.NotNil(t, baseURL, adminToken)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintln(w, "a image")
	}))

	validURL := ts.URL
	defer ts.Close()

	apiClient, err := client.New(&client.DialConfig{
		BaseURL:   baseURL,
		Token:     adminToken,
		UserAgent: "integration test",
		Log:       log,
	})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(t.Context())

	p, err := apiClient.Adminv2().Partition().Create(ctx, &adminv2.PartitionServiceCreateRequest{
		Partition: &apiv2.Partition{Id: "partition-1", BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}},
	})
	require.NoError(t, err)

	_, err = apiClient.Infrav2().Boot().Dhcp(ctx, &infrav2.BootServiceDhcpRequest{Uuid: m0, Partition: p.Partition.Id})
	require.NoError(t, err)
	_, err = apiClient.Infrav2().BMC().UpdateBMCInfo(ctx, &infrav2.UpdateBMCInfoRequest{Partition: p.Partition.Id, BmcReports: map[string]*apiv2.MachineBMCReport{
		m0: {
			Bmc: &apiv2.MachineBMC{Address: "192.168.0.1", User: "metal", Password: "secret", Mac: "00:00:00:00:00:01"},
		},
	}})
	require.NoError(t, err)
	// Now we have a machine

	asyncClient := machinebmccommand.NewClient(log, apiClient)

	// Subscribe asynchronously
	msgChan, _ := asyncClient.SubscribeAsync(ctx, p.Partition.Id)

	// Give subscription time to establish
	time.Sleep(1 * time.Second)

	// Publish a message
	_, err = apiClient.Adminv2().Machine().BMCCommand(ctx,
		&adminv2.MachineServiceBMCCommandRequest{
			Uuid:    m0,
			Command: apiv2.MachineBMCCommand_MACHINE_BMC_COMMAND_BOOT_FROM_DISK},
	)
	require.NoError(t, err)

	msg := <-msgChan
	assert.Equal(t, apiv2.MachineBMCCommand_MACHINE_BMC_COMMAND_BOOT_FROM_DISK, msg.BmcCommand)
	assert.NotNil(t, msg.MachineBmc)

	cancel()
}
