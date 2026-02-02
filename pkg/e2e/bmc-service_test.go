package e2e

import (
	"context"
	"fmt"
	"io"
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
			Bmc: &apiv2.MachineBMC{Address: "192.168.0.1:623", User: "metal", Password: "secret", Mac: "00:00:00:00:00:01"},
		},
	}})
	require.NoError(t, err)
	// Now we have a machine with bmc details

	var (
		waitResponses []*infrav2.WaitForBMCCommandResponse
		mu            sync.RWMutex
	)

	metalBmcSimulation := func() {
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
			resp := stream.Msg()
			time.Sleep(100 * time.Millisecond) // Ensure task is in pending for a while
			_, err := apiClient.Infrav2().BMC().BMCCommandDone(ctx, &infrav2.BMCCommandDoneRequest{CommandId: resp.CommandId})
			if err != nil {
				log.Error("error sending done response", "error", err)
			}
			waitResponses = append(waitResponses, resp)
			mu.Unlock()
		}
	}
	// Start two instances of metal-bmc waiting for machine bmc commands
	go metalBmcSimulation()
	go metalBmcSimulation()

	// Give subscription time to establish
	time.Sleep(100 * time.Millisecond)

	// Publish a message
	_, err = apiClient.Adminv2().Machine().BMCCommand(ctx,
		&adminv2.MachineServiceBMCCommandRequest{
			Uuid:    m0,
			Command: apiv2.MachineBMCCommand_MACHINE_BMC_COMMAND_BOOT_FROM_PXE},
	)
	require.NoError(t, err)

	tasks, err := apiClient.Adminv2().Task().List(ctx, &adminv2.TaskServiceListRequest{})
	require.NoError(t, err)
	require.Len(t, tasks.Tasks, 1)
	require.Equal(t, adminv2.TaskState_TASK_STATE_PENDING, tasks.Tasks[0].State)

	require.EventuallyWithT(t, func(c *assert.CollectT) {
		mu.RLock()
		defer mu.RUnlock()
		require.NotNil(c, waitResponses)
		require.Len(c, waitResponses, 1)
		require.Equal(c, apiv2.MachineBMCCommand_MACHINE_BMC_COMMAND_BOOT_FROM_PXE, waitResponses[0].BmcCommand)
	}, 5*time.Second, 100*time.Millisecond)

	tasks, err = apiClient.Adminv2().Task().List(ctx, &adminv2.TaskServiceListRequest{})
	require.NoError(t, err)
	require.Len(t, tasks.Tasks, 1)
	require.Equal(t, adminv2.TaskState_TASK_STATE_COMPLETED, tasks.Tasks[0].State)

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
			Bmc: &apiv2.MachineBMC{Address: "192.168.0.1:623", User: "metal", Password: "secret", Mac: "00:00:00:00:00:01"},
		},
	}})
	require.NoError(t, err)
	// Now we have a machine

	asyncClient := bmcServiceClient{log: log, client: apiClient}

	// Subscribe asynchronously
	msgChan, _ := asyncClient.subscribeAsync(ctx, p.Partition.Id)

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

type bmcServiceClient struct {
	client client.Client
	log    *slog.Logger
}

// MessageHandler is called when a message is received
type MessageHandler func(*infrav2.WaitForBMCCommandResponse) error

// Subscribe subscribes to a topic and calls the handler for each message
func (c *bmcServiceClient) subscribe(ctx context.Context, topic string, handler MessageHandler) error {
	stream, err := c.client.Infrav2().BMC().WaitForBMCCommand(ctx, &infrav2.WaitForBMCCommandRequest{Partition: topic})
	if err != nil {
		return fmt.Errorf("failed to subscribe: %w", err)
	}
	defer func() {
		_ = stream.Close()
	}()

	c.log.Info("subscribed to machine bmc command", "topic", topic)

	// Receive messages
	for stream.Receive() {
		msg := stream.Msg()
		if err := handler(msg); err != nil {
			c.log.Error("handler error", "error", err)
		}
	}

	if err := stream.Err(); err != nil {
		if err == io.EOF || err == context.Canceled {
			return nil
		}
		return fmt.Errorf("stream error: %w", err)
	}

	return nil
}

// SubscribeAsync subscribes asynchronously and returns a channel of messages
func (c *bmcServiceClient) subscribeAsync(ctx context.Context, topic string) (<-chan *infrav2.WaitForBMCCommandResponse, <-chan error) {
	var (
		msgChan = make(chan *infrav2.WaitForBMCCommandResponse, 100)
		errChan = make(chan error, 1)
	)

	go func() {
		defer close(msgChan)
		defer close(errChan)

		err := c.subscribe(ctx, topic, func(msg *infrav2.WaitForBMCCommandResponse) error {
			select {
			case msgChan <- msg:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})

		if err != nil && err != context.Canceled {
			errChan <- err
		}
	}()

	return msgChan, errChan
}
