package e2e

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"connectrpc.com/connect"
	"github.com/google/go-cmp/cmp"
	"github.com/metal-stack/api/go/client"
	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	infrav2 "github.com/metal-stack/api/go/metalstack/infra/v2"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
)

var (
	m0 = "00000000-0000-0000-0000-000000000000"
)

func TestWaitForBMCCommand(t *testing.T) {
	t.Parallel()
	// TODO test more scenarios with more receivers
	log, baseURL, adminToken, _, closer := StartApiserver(t)
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
	_, err = apiClient.Infrav2().BMC().UpdateBMCInfo(ctx, &infrav2.UpdateBMCInfoRequest{Partition: p.Partition.Id, BmcReports: []*apiv2.MachineBMCReport{
		{
			Uuid: m0, Bmc: &apiv2.MachineBMC{Address: "192.168.0.1:623", User: "metal", Password: "secret", Mac: "00:00:00:00:00:01"},
		},
	}})
	require.NoError(t, err)
	// Now we have a machine

	g, _ := errgroup.WithContext(t.Context())

	g.Go(func() error {
		messages, errs := client.ReconnectingStreamRead(ctx, func(ctx context.Context) (*connect.ServerStreamForClient[infrav2.WaitForBMCCommandResponse], error) {
			return apiClient.Infrav2().BMC().WaitForBMCCommand(ctx, &infrav2.WaitForBMCCommandRequest{Partition: p.Partition.Id})
		}, client.WithStreamBackoff(0), client.WithStreamLogger(log))

		for {
			select {
			case msg := <-messages:
				if diff := cmp.Diff(apiv2.MachineBMCCommand_MACHINE_BMC_COMMAND_BOOT_FROM_DISK, msg.BmcCommand); diff != "" {
					return fmt.Errorf("bmc cmd diff: %s", diff)
				}
				_, err := apiClient.Infrav2().BMC().BMCCommandDone(ctx, &infrav2.BMCCommandDoneRequest{CommandId: msg.CommandId})
				if err != nil {
					return err
				}
			case err := <-errs:
				return fmt.Errorf("unexpected error: %w", err)
			case <-ctx.Done():
				return nil
			}
		}
	})

	// Publish a message
	_, err = apiClient.Adminv2().Machine().BMCCommand(ctx,
		&adminv2.MachineServiceBMCCommandRequest{
			Uuid:    m0,
			Command: apiv2.MachineBMCCommand_MACHINE_BMC_COMMAND_BOOT_FROM_DISK},
	)
	require.NoError(t, err)

	tasks, err := apiClient.Adminv2().Task().List(ctx, &adminv2.TaskServiceListRequest{})
	require.NoError(t, err)
	require.Len(t, tasks.Tasks, 1)
	require.Equal(t, adminv2.TaskState_TASK_STATE_COMPLETED, tasks.Tasks[0].State)

	cancel()
	require.NoError(t, g.Wait())
}
