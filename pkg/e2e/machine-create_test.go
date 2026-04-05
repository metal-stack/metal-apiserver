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
	"github.com/metal-stack/metal-apiserver/pkg/repository/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	partition1 = "partition-1"
	sw1        = &api.SwitchServiceCreateRequest{
		Switch: &apiv2.Switch{
			Id:           "sw1",
			Meta:         &apiv2.Meta{},
			Partition:    partition1,
			Rack:         new("r01"),
			ManagementIp: "1.2.3.3",
			ReplaceMode:  apiv2.SwitchReplaceMode_SWITCH_REPLACE_MODE_OPERATIONAL,
			Nics: []*apiv2.SwitchNic{
				{
					Name:       "Ethernet0",
					Identifier: "Eth1/1",
					State: &apiv2.NicState{
						Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
					},
					Mac: "00:00:00:00:00:01",
				},
				{
					Name:       "Ethernet1",
					Identifier: "Eth1/2",
					State: &apiv2.NicState{
						Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
					},
					Mac: "00:00:00:00:00:02",
				},
			},
			Os: &apiv2.SwitchOS{
				Version:          "1.0",
				MetalCoreVersion: "1.0",
				Vendor:           apiv2.SwitchOSVendor_SWITCH_OS_VENDOR_SONIC,
			},
		},
	}

	sw2 = &api.SwitchServiceCreateRequest{
		Switch: &apiv2.Switch{
			Id:           "sw2",
			Meta:         &apiv2.Meta{},
			Partition:    partition1,
			Rack:         new("r01"),
			ManagementIp: "1.2.3.4",
			ReplaceMode:  apiv2.SwitchReplaceMode_SWITCH_REPLACE_MODE_OPERATIONAL,
			Nics: []*apiv2.SwitchNic{
				{
					Name:       "Ethernet0",
					Identifier: "Eth1/1",
					State: &apiv2.NicState{
						Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
					},
					Mac: "01:00:00:00:00:01",
				},
				{
					Name:       "Ethernet1",
					Identifier: "Eth1/2",
					State: &apiv2.NicState{
						Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
					},
					Mac: "01:00:00:00:00:02",
				},
			},
			Os: &apiv2.SwitchOS{
				Version:          "1.0",
				MetalCoreVersion: "1.0",
				Vendor:           apiv2.SwitchOSVendor_SWITCH_OS_VENDOR_SONIC,
			},
		},
	}
)

func TestMachineCreate(t *testing.T) {
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

	t1, err := apiClient.Apiv2().Tenant().Create(ctx, &apiv2.TenantServiceCreateRequest{Name: "t1"})
	require.NoError(t, err)
	project1, err := apiClient.Apiv2().Project().Create(ctx, &apiv2.ProjectServiceCreateRequest{Name: "p1", Login: t1.Tenant.Login})
	require.NoError(t, err)
	require.NotEmpty(t, project1)

	_, err = apiClient.Adminv2().Size().Create(ctx, &adminv2.SizeServiceCreateRequest{
		Size: &apiv2.Size{Id: "c1-large-x86", Constraints: []*apiv2.SizeConstraint{
			{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_CORES, Min: 4, Max: 4},
			{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_MEMORY, Min: 1024, Max: 1024},
			{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_STORAGE, Min: 1024, Max: 1024 * 1024 * 1024 * 1024},
		}},
	})
	require.NoError(t, err)
	_, err = apiClient.Adminv2().Filesystem().Create(ctx, &adminv2.FilesystemServiceCreateRequest{
		FilesystemLayout: &apiv2.FilesystemLayout{
			Id: "debian",
			Constraints: &apiv2.FilesystemLayoutConstraints{
				Sizes: []string{"c1-large-x86"},
				Images: map[string]string{
					"debian": ">= 12.0",
				},
			},
			Disks: []*apiv2.Disk{
				{
					Device: "/dev/sda",
					Partitions: []*apiv2.DiskPartition{
						{
							Number:  0,
							Size:    1024,
							GptType: apiv2.GPTType_GPT_TYPE_LINUX.Enum(),
						},
					},
				},
			},
		},
	})
	require.NoError(t, err)

	_, err = apiClient.Adminv2().Image().Create(ctx, &adminv2.ImageServiceCreateRequest{
		Image: &apiv2.Image{
			Id:             "debian-12.0.0",
			Url:            validURL,
			Features:       []apiv2.ImageFeature{apiv2.ImageFeature_IMAGE_FEATURE_MACHINE},
			Classification: apiv2.ImageClassification_IMAGE_CLASSIFICATION_SUPPORTED,
		},
	})
	require.NoError(t, err)

	p, err := apiClient.Adminv2().Partition().Create(ctx, &adminv2.PartitionServiceCreateRequest{
		Partition: &apiv2.Partition{Id: partition1, BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}},
	})
	require.NoError(t, err)

	_, err = apiClient.Adminv2().Network().Create(ctx, &adminv2.NetworkServiceCreateRequest{
		Id:       new("internet"),
		Prefixes: []string{"1.2.3.0/24"},
		Type:     apiv2.NetworkType_NETWORK_TYPE_EXTERNAL,
		NatType:  apiv2.NATType_NAT_TYPE_IPV4_MASQUERADE.Enum(),
		Vrf:      new(uint32(11)),
	})
	require.NoError(t, err)
	_, err = apiClient.Adminv2().Network().Create(ctx, &adminv2.NetworkServiceCreateRequest{
		Id:                       new("partition-super"),
		Partition:                new(partition1),
		Prefixes:                 []string{"12.110.0.0/16"},
		DestinationPrefixes:      []string{"1.2.3.0/24"},
		DefaultChildPrefixLength: &apiv2.ChildPrefixLength{Ipv4: new(uint32(22))},
		Type:                     apiv2.NetworkType_NETWORK_TYPE_SUPER,
	})
	require.NoError(t, err)

	_, err = apiClient.Infrav2().Switch().Register(ctx, &infrav2.SwitchServiceRegisterRequest{
		Switch: sw1.Switch,
	})
	require.NoError(t, err)
	_, err = apiClient.Infrav2().Switch().Register(ctx, &infrav2.SwitchServiceRegisterRequest{
		Switch: sw2.Switch,
	})
	require.NoError(t, err)

	_, err = apiClient.Infrav2().Boot().Dhcp(ctx, &infrav2.BootServiceDhcpRequest{Uuid: m0, Partition: p.Partition.Id})
	require.NoError(t, err)
	_, err = apiClient.Infrav2().Boot().Register(ctx, &infrav2.BootServiceRegisterRequest{
		Uuid: m0,
		Hardware: &apiv2.MachineHardware{
			Memory: 1024,
			Cpus: []*apiv2.MetalCPU{
				{Cores: 4},
			},
			Disks: []*apiv2.MachineBlockDevice{
				{
					Name: "/dev/sda",
					Size: 10 * 1024 * 1024 * 1024,
				},
			},
			Nics: []*apiv2.MachineNic{
				{Name: "lan0", Mac: "00:00:00:00:00:01", Neighbors: []*apiv2.MachineNic{{Name: "Ethernet1", Mac: "00:00:00:00:00:03", Identifier: "Eth1/2", Hostname: "sw1"}}},
				{Name: "lan1", Mac: "00:00:00:00:00:02", Neighbors: []*apiv2.MachineNic{{Name: "Ethernet1", Mac: "00:00:00:00:00:04", Identifier: "Eth1/2", Hostname: "sw2"}}},
			},
		},
		Bios:      &apiv2.MachineBios{Version: "1.0"},
		Partition: p.Partition.Id,
	})
	require.NoError(t, err)

	var (
		machineWaitResponse *infrav2.BootServiceWaitResponse
		waiterror           error
		mu                  sync.Mutex
	)
	machineWaitSimulation := func() {
		stream, err := apiClient.Infrav2().Boot().Wait(ctx, &infrav2.BootServiceWaitRequest{
			Uuid: m0,
		})
		if err != nil {
			waiterror = err
			return
		}
		defer func() {
			_ = stream.Close()
		}()
		for stream.Receive() {
			mu.Lock()
			defer mu.Unlock()
			machineWaitResponse = stream.Msg()
			t.Logf("machine wait stopped %v", machineWaitResponse)
			return
		}
	}

	go machineWaitSimulation()
	time.Sleep(1 * time.Second)

	// Create a child network
	projectNetwork, err := apiClient.Apiv2().Network().Create(ctx, &apiv2.NetworkServiceCreateRequest{
		Project:   project1.Project.Uuid,
		Name:      new("project-network"),
		Partition: &partition1,
	})
	require.NoError(t, err)

	// Create a machine by uuid
	_, err = apiClient.Apiv2().Machine().Create(ctx, &apiv2.MachineServiceCreateRequest{
		Project:        project1.Project.Uuid,
		Uuid:           new(m0),
		Name:           "e2e-test",
		Hostname:       new("e2e-test"),
		Image:          "debian-12.0.0",
		AllocationType: apiv2.MachineAllocationType_MACHINE_ALLOCATION_TYPE_MACHINE,
		Networks: []*apiv2.MachineAllocationNetwork{
			{Network: projectNetwork.Network.Id},
		},
	})
	require.NoError(t, err)

	require.EventuallyWithT(t, func(c *assert.CollectT) {
		require.NoError(c, waiterror)
		require.NotNil(c, machineWaitResponse)
		require.NotNil(c, machineWaitResponse.Allocation)
		require.Equal(c, "e2e-test", machineWaitResponse.Allocation.Hostname)
		require.Len(c, machineWaitResponse.Allocation.Networks, 1)
	}, 5*time.Second, 100*time.Millisecond)

	// We need to cancel the context because otherwise the Wait blocks the grpc http server from stopping
	cancel()
}
