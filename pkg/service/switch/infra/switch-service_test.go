package infra

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	infrav2 "github.com/metal-stack/api/go/metalstack/infra/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	"github.com/metal-stack/metal-apiserver/pkg/repository"
	"github.com/metal-stack/metal-apiserver/pkg/test"
	"github.com/metal-stack/metal-lib/pkg/pointer"
	"github.com/metal-stack/metal-lib/pkg/testcommon"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var (
	now = time.Now()
	sw1 = func(generation uint64) *repository.SwitchServiceCreateRequest {
		return &repository.SwitchServiceCreateRequest{
			Switch: &apiv2.Switch{
				Id:          "sw1",
				Partition:   "partition-a",
				ReplaceMode: apiv2.SwitchReplaceMode_SWITCH_REPLACE_MODE_OPERATIONAL,
				Meta:        &apiv2.Meta{Generation: generation},
				MachineConnections: []*apiv2.MachineConnection{
					{
						MachineId: "m1",
						Nic: &apiv2.SwitchNic{
							Name:       "Ethernet0",
							Identifier: "Eth1/1",
							BgpFilter:  &apiv2.BGPFilter{},
							State: &apiv2.NicState{
								Desired: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP.Enum(),
								Actual:  apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_DOWN,
							},
						},
					},
				},
				Nics: []*apiv2.SwitchNic{
					{
						Name:       "Ethernet0",
						Identifier: "Eth1/1",
						BgpFilter:  &apiv2.BGPFilter{},
						State: &apiv2.NicState{
							Desired: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP.Enum(),
							Actual:  apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_DOWN,
						},
					},
					{
						Name:       "Ethernet1",
						Identifier: "Eth1/2",
						BgpFilter:  &apiv2.BGPFilter{},
						State: &apiv2.NicState{
							Desired: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP.Enum(),
							Actual:  apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_DOWN,
						},
						BgpPortState: &apiv2.SwitchBGPPortState{
							Neighbor:              "Ethernet2",
							PeerGroup:             "external",
							VrfName:               "Vrf200",
							BgpState:              apiv2.BGPState_BGP_STATE_CONNECT,
							BgpTimerUpEstablished: timestamppb.New(time.Unix(now.Unix(), 0)),
							SentPrefixCounter:     0,
							AcceptedPrefixCounter: 0,
						},
					},
				},
				Os: &apiv2.SwitchOS{
					Vendor:           apiv2.SwitchOSVendor_SWITCH_OS_VENDOR_SONIC,
					Version:          "ec202111",
					MetalCoreVersion: "v0.13.0",
				},
			},
		}
	}

	sw2 = func(generation uint64) *repository.SwitchServiceCreateRequest {
		return &repository.SwitchServiceCreateRequest{
			Switch: &apiv2.Switch{
				Id:          "sw2",
				Partition:   "partition-a",
				ReplaceMode: apiv2.SwitchReplaceMode_SWITCH_REPLACE_MODE_OPERATIONAL,
				Meta:        &apiv2.Meta{Generation: generation},
				Nics: []*apiv2.SwitchNic{
					{
						Name:       "Ethernet0",
						Identifier: "Eth1/1",
						BgpFilter:  &apiv2.BGPFilter{},
						State: &apiv2.NicState{
							Desired: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP.Enum(),
							Actual:  apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_DOWN,
						},
					},
					{
						Name:       "Ethernet1",
						Identifier: "Eth1/2",
						BgpFilter:  &apiv2.BGPFilter{},
						State: &apiv2.NicState{
							Desired: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP.Enum(),
							Actual:  apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_DOWN,
						},
						BgpPortState: &apiv2.SwitchBGPPortState{
							Neighbor:              "Ethernet2",
							PeerGroup:             "external",
							VrfName:               "Vrf200",
							BgpState:              apiv2.BGPState_BGP_STATE_CONNECT,
							BgpTimerUpEstablished: timestamppb.New(time.Unix(now.Unix(), 0)),
							SentPrefixCounter:     0,
							AcceptedPrefixCounter: 0,
						},
					},
				},
				Os: &apiv2.SwitchOS{
					Vendor:           apiv2.SwitchOSVendor_SWITCH_OS_VENDOR_SONIC,
					Version:          "ec202111",
					MetalCoreVersion: "v0.13.0",
				},
			},
		}
	}

	sw3 = &repository.SwitchServiceCreateRequest{
		Switch: &apiv2.Switch{
			Id:          "sw3",
			Partition:   "partition-a",
			ReplaceMode: apiv2.SwitchReplaceMode_SWITCH_REPLACE_MODE_OPERATIONAL,
			MachineConnections: []*apiv2.MachineConnection{
				{
					MachineId: "m1",
					Nic: &apiv2.SwitchNic{
						Name:       "swp1s0",
						Identifier: "aa:aa:aa:aa:aa:aa",
						State: &apiv2.NicState{
							Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
						},
					},
				},
			},
			Nics: []*apiv2.SwitchNic{
				{
					Name:       "swp1s0",
					Identifier: "aa:aa:aa:aa:aa:aa",
					State: &apiv2.NicState{
						Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
					},
				},
			},
			Os: &apiv2.SwitchOS{
				Vendor: apiv2.SwitchOSVendor_SWITCH_OS_VENDOR_CUMULUS,
			},
		},
	}

	sw4 = &repository.SwitchServiceCreateRequest{
		Switch: &apiv2.Switch{
			Id:          "sw4",
			Partition:   "partition-a",
			ReplaceMode: apiv2.SwitchReplaceMode_SWITCH_REPLACE_MODE_OPERATIONAL,
			MachineConnections: []*apiv2.MachineConnection{
				{
					MachineId: "m1",
					Nic: &apiv2.SwitchNic{
						Name:       "Ethernet0",
						Identifier: "Eth1/1",
						State: &apiv2.NicState{
							Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
						},
					},
				},
			},
			Nics: []*apiv2.SwitchNic{
				{
					Name:       "Ethernet0",
					Identifier: "Eth1/1",
					State: &apiv2.NicState{
						Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
					},
				},
			},
			Os: &apiv2.SwitchOS{
				Vendor: apiv2.SwitchOSVendor_SWITCH_OS_VENDOR_SONIC,
			},
		},
	}

	sw5 = &repository.SwitchServiceCreateRequest{
		Switch: &apiv2.Switch{
			Id:          "sw5",
			Partition:   "partition-a",
			ReplaceMode: apiv2.SwitchReplaceMode_SWITCH_REPLACE_MODE_OPERATIONAL,
			Nics: []*apiv2.SwitchNic{
				{
					Name:       "swp1s0",
					Identifier: "bb:bb:bb:bb:bb:bb",
					State: &apiv2.NicState{
						Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
					},
				},
			},
			Os: &apiv2.SwitchOS{
				Vendor: apiv2.SwitchOSVendor_SWITCH_OS_VENDOR_CUMULUS,
			},
		},
	}

	sw6 = &repository.SwitchServiceCreateRequest{
		Switch: &apiv2.Switch{
			Id:          "sw6",
			Partition:   "partition-a",
			ReplaceMode: apiv2.SwitchReplaceMode_SWITCH_REPLACE_MODE_OPERATIONAL,
			Nics: []*apiv2.SwitchNic{
				{
					Name:       "Ethernet0",
					Identifier: "Eth1/1",
					State: &apiv2.NicState{
						Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
					},
				},
			},
			Os: &apiv2.SwitchOS{
				Vendor: apiv2.SwitchOSVendor_SWITCH_OS_VENDOR_SONIC,
			},
		},
	}

	sw1Status = &repository.SwitchStatus{
		ID: sw1(0).Switch.Id,
		LastSync: &infrav2.SwitchSync{
			Time:     timestamppb.New(now),
			Duration: durationpb.New(time.Second),
			Error:    nil,
		},
		LastSyncError: &infrav2.SwitchSync{
			Time:     timestamppb.New(now.Add(-time.Minute)),
			Duration: durationpb.New(time.Second * 2),
			Error:    pointer.Pointer("fail"),
		},
	}

	switches = func(generation uint64) []*repository.SwitchServiceCreateRequest {
		return []*repository.SwitchServiceCreateRequest{sw1(generation), sw2(generation), sw3, sw4, sw5, sw6}
	}

	m1 = &metal.Machine{
		Base: metal.Base{ID: "m1"},
		Hardware: metal.MachineHardware{
			Nics: metal.Nics{
				{
					Name: "lan0",
					Neighbors: metal.Nics{
						{
							Name:       "swp1s0",
							Identifier: "aa:aa:aa:aa:aa:aa",
							Hostname:   "sw3",
						},
					},
				},
				{
					Name: "lan1",
					Neighbors: metal.Nics{
						{
							Name:       "Ethernet0",
							Identifier: "Eth1/1",
							Hostname:   "sw4",
						},
					},
				},
			},
		},
	}
)

func Test_switchServiceServer_Register(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	ctx := t.Context()

	testStore, closer := test.StartRepositoryWithCleanup(t, log)
	defer closer()
	repo := testStore.Store

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintln(w, "a image")
	}))
	defer ts.Close()

	validURL := ts.URL

	var (
		partitions = []*adminv2.PartitionServiceCreateRequest{
			{Partition: &apiv2.Partition{Id: "partition-a", BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}}},
			{Partition: &apiv2.Partition{Id: "partition-b", BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}}},
		}
	)

	test.CreatePartitions(t, repo, partitions)
	test.CreateSwitches(t, repo, switches(0))

	tests := []struct {
		name    string
		rq      *infrav2.SwitchServiceRegisterRequest
		want    *infrav2.SwitchServiceRegisterResponse
		wantErr error
	}{
		{
			name: "register new switch",
			rq: &infrav2.SwitchServiceRegisterRequest{
				Switch: &apiv2.Switch{
					Id:           "sw3",
					Rack:         nil,
					Partition:    "partition-b",
					ManagementIp: "1.1.1.1",
					ReplaceMode:  apiv2.SwitchReplaceMode_SWITCH_REPLACE_MODE_OPERATIONAL,
					Nics:         []*apiv2.SwitchNic{},
					Os: &apiv2.SwitchOS{
						Vendor:           apiv2.SwitchOSVendor_SWITCH_OS_VENDOR_CUMULUS,
						Version:          "v5.9",
						MetalCoreVersion: "v0.13.0",
					},
				},
			},
			want: &infrav2.SwitchServiceRegisterResponse{
				Switch: &apiv2.Switch{
					Id:           "sw3",
					Meta:         &apiv2.Meta{Generation: 0},
					Rack:         nil,
					Partition:    "partition-b",
					ManagementIp: "1.1.1.1",
					ReplaceMode:  apiv2.SwitchReplaceMode_SWITCH_REPLACE_MODE_OPERATIONAL,
					Nics:         []*apiv2.SwitchNic{},
					Os: &apiv2.SwitchOS{
						Vendor:           apiv2.SwitchOSVendor_SWITCH_OS_VENDOR_CUMULUS,
						Version:          "v5.9",
						MetalCoreVersion: "v0.13.0",
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "register existing operational switch",
			rq: &infrav2.SwitchServiceRegisterRequest{
				Switch: &apiv2.Switch{
					Id:             "sw1",
					Description:    "new description",
					Partition:      "partition-a",
					ReplaceMode:    apiv2.SwitchReplaceMode_SWITCH_REPLACE_MODE_OPERATIONAL,
					ManagementIp:   "1.1.1.1",
					ManagementUser: pointer.Pointer("admin"),
					ConsoleCommand: pointer.Pointer("tty"),
					Nics: []*apiv2.SwitchNic{
						{
							Name:       "Ethernet0",
							Identifier: "Eth1/1",
							Mac:        "11:11:11:11:11:11", // MAC does not get updated but is necessary for the validation to pass
						},
						{
							Name:       "Ethernet2",
							Identifier: "Eth1/2",
							Mac:        "22:22:22:22:22:22",
							Vrf:        pointer.Pointer("must not be updated"),
						},
					},
				},
			},
			want: &infrav2.SwitchServiceRegisterResponse{
				Switch: &apiv2.Switch{
					Id:             "sw1",
					Description:    "new description",
					Meta:           &apiv2.Meta{Generation: 1},
					Partition:      "partition-a",
					ReplaceMode:    apiv2.SwitchReplaceMode_SWITCH_REPLACE_MODE_OPERATIONAL,
					ManagementIp:   "1.1.1.1",
					ManagementUser: pointer.Pointer("admin"),
					ConsoleCommand: pointer.Pointer("tty"),
					Nics: []*apiv2.SwitchNic{
						{
							Name:       "Ethernet0",
							Identifier: "Eth1/1",
							BgpFilter:  &apiv2.BGPFilter{},
							State: &apiv2.NicState{
								Desired: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP.Enum(),
								Actual:  apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_DOWN,
							},
						},
						{
							Name:       "Ethernet2",
							Identifier: "Eth1/2",
							BgpFilter:  &apiv2.BGPFilter{},
							State: &apiv2.NicState{
								Desired: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP.Enum(),
								Actual:  apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_DOWN,
							},
							BgpPortState: &apiv2.SwitchBGPPortState{
								Neighbor:              "Ethernet2",
								PeerGroup:             "external",
								VrfName:               "Vrf200",
								BgpState:              apiv2.BGPState_BGP_STATE_CONNECT,
								BgpTimerUpEstablished: timestamppb.New(time.Unix(now.Unix(), 0)),
								SentPrefixCounter:     0,
								AcceptedPrefixCounter: 0,
							},
						},
					},
					Os: &apiv2.SwitchOS{
						Vendor:           apiv2.SwitchOSVendor_SWITCH_OS_VENDOR_SONIC,
						Version:          "ec202111",
						MetalCoreVersion: "v0.13.0",
					},
				},
			},
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &switchServiceServer{
				log:  log,
				repo: repo,
			}

			if tt.wantErr == nil {
				test.Validate(t, tt.rq)
			}

			got, err := s.Register(ctx, tt.rq)
			if diff := cmp.Diff(tt.wantErr, err, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("switchServiceServer.Register() error diff = %s", diff)
				return
			}
			if diff := cmp.Diff(tt.want, got,
				protocmp.Transform(),
				protocmp.IgnoreFields(
					&apiv2.Meta{}, "created_at", "updated_at",
				),
			); diff != "" {
				t.Errorf("switchServiceServer.Register() diff = %s", diff)
			}
		})
	}
}

func Test_switchServiceServer_Get(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	ctx := t.Context()

	testStore, closer := test.StartRepositoryWithCleanup(t, log)
	defer closer()
	repo := testStore.Store

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintln(w, "a image")
	}))
	defer ts.Close()

	validURL := ts.URL

	var (
		partitions = []*adminv2.PartitionServiceCreateRequest{
			{Partition: &apiv2.Partition{Id: "partition-a", BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}}},
			{Partition: &apiv2.Partition{Id: "partition-b", BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}}},
		}
	)

	test.CreatePartitions(t, repo, partitions)
	test.CreateSwitches(t, repo, switches(0))

	tests := []struct {
		name    string
		rq      *infrav2.SwitchServiceGetRequest
		want    *infrav2.SwitchServiceGetResponse
		wantErr error
	}{
		{
			name: "get existing",
			rq: &infrav2.SwitchServiceGetRequest{
				Id: "sw1",
			},
			want: &infrav2.SwitchServiceGetResponse{
				Switch: sw1(0).Switch,
			},
			wantErr: nil,
		},
		{
			name: "get non-existing",
			rq: &infrav2.SwitchServiceGetRequest{
				Id: "sw4",
			},
			want:    nil,
			wantErr: errorutil.NotFound("no switch with id \"sw4\" found"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &switchServiceServer{
				log:  log,
				repo: repo,
			}

			if tt.wantErr == nil {
				test.Validate(t, tt.rq)
			}

			got, err := s.Get(ctx, tt.rq)
			if diff := cmp.Diff(tt.wantErr, err, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("switchServiceServer.Get() error diff = %s", diff)
				return
			}
			if diff := cmp.Diff(tt.want, got,
				protocmp.Transform(),
				protocmp.IgnoreFields(
					&apiv2.Meta{}, "created_at", "updated_at",
				)); diff != "" {
				t.Errorf("switchServiceServer.Get() diff = %s", diff)
			}
		})
	}
}

func Test_switchServiceServer_Heartbeat(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	ctx := t.Context()

	testStore, closer := test.StartRepositoryWithCleanup(t, log)
	defer closer()
	repo := testStore.Store

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintln(w, "a image")
	}))
	defer ts.Close()

	validURL := ts.URL

	var (
		partitions = []*adminv2.PartitionServiceCreateRequest{
			{Partition: &apiv2.Partition{Id: "partition-a", BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}}},
			{Partition: &apiv2.Partition{Id: "partition-b", BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}}},
		}
	)

	test.CreatePartitions(t, repo, partitions)
	test.CreateMachines(t, testStore, []*metal.Machine{m1})
	test.CreateSwitches(t, repo, switches(0))
	test.CreateSwitchStatuses(t, testStore, []*repository.SwitchStatus{sw1Status})

	tests := []struct {
		name       string
		rq         *infrav2.SwitchServiceHeartbeatRequest
		want       *infrav2.SwitchServiceHeartbeatResponse
		wantSwitch *apiv2.Switch
		wantErr    error
	}{
		{
			name: "switch status empty, no error, no change",
			rq: &infrav2.SwitchServiceHeartbeatRequest{
				Id:       sw2(0).Switch.Id,
				Duration: durationpb.New(time.Second),
				Error:    nil,
				PortStates: map[string]apiv2.SwitchPortStatus{
					"Ethernet0": apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_DOWN,
					"Ethernet1": apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_DOWN,
				},
				BgpPortStates: map[string]*apiv2.SwitchBGPPortState{
					"Ethernet1": {
						Neighbor:              "Ethernet2",
						PeerGroup:             "external",
						VrfName:               "Vrf200",
						BgpState:              apiv2.BGPState_BGP_STATE_CONNECT,
						BgpTimerUpEstablished: timestamppb.New(time.Unix(now.Unix(), 0)),
						SentPrefixCounter:     0,
						AcceptedPrefixCounter: 0,
					},
				},
			},
			want: &infrav2.SwitchServiceHeartbeatResponse{
				Id: sw2(0).Switch.Id,
				LastSync: &infrav2.SwitchSync{
					Duration: durationpb.New(time.Second),
					Error:    nil,
				},
				LastSyncError: nil,
			},
			wantSwitch: sw2(0).Switch,
			wantErr:    nil,
		},
		{
			name: "switch status exists, error occurred, no change",
			rq: &infrav2.SwitchServiceHeartbeatRequest{
				Id:       sw1(0).Switch.Id,
				Duration: durationpb.New(time.Second),
				Error:    pointer.Pointer("sync failed"),
				PortStates: map[string]apiv2.SwitchPortStatus{
					"Ethernet0": apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_DOWN,
					"Ethernet1": apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_DOWN,
				},
				BgpPortStates: map[string]*apiv2.SwitchBGPPortState{
					"Ethernet1": {
						Neighbor:              "Ethernet2",
						PeerGroup:             "external",
						VrfName:               "Vrf200",
						BgpState:              apiv2.BGPState_BGP_STATE_CONNECT,
						BgpTimerUpEstablished: timestamppb.New(time.Unix(now.Unix(), 0)),
						SentPrefixCounter:     0,
						AcceptedPrefixCounter: 0,
					},
				},
			},
			want: &infrav2.SwitchServiceHeartbeatResponse{
				Id: sw1(0).Switch.Id,
				LastSync: &infrav2.SwitchSync{
					Time:     timestamppb.New(now),
					Duration: durationpb.New(time.Second),
					Error:    nil,
				},
				LastSyncError: &infrav2.SwitchSync{
					Duration: durationpb.New(time.Second),
					Error:    pointer.Pointer("sync failed"),
				},
			},
			wantSwitch: sw1(0).Switch,
			wantErr:    nil,
		},
		{
			name: "error occurred, update anyway",
			rq: &infrav2.SwitchServiceHeartbeatRequest{
				Id:       sw2(0).Switch.Id,
				Duration: durationpb.New(time.Second),
				Error:    pointer.Pointer("failed to sync"),
				PortStates: map[string]apiv2.SwitchPortStatus{
					"Ethernet0": apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
					"Ethernet1": apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_DOWN,
				},
				BgpPortStates: map[string]*apiv2.SwitchBGPPortState{
					"Ethernet1": {
						Neighbor:              "Ethernet2",
						PeerGroup:             "external",
						VrfName:               "Vrf200",
						BgpState:              apiv2.BGPState_BGP_STATE_ESTABLISHED,
						BgpTimerUpEstablished: timestamppb.New(time.Unix(now.Unix(), 0)),
						SentPrefixCounter:     0,
						AcceptedPrefixCounter: 0,
					},
				},
			},
			want: &infrav2.SwitchServiceHeartbeatResponse{
				Id: sw2(0).Switch.Id,
				LastSync: &infrav2.SwitchSync{
					Duration: durationpb.New(time.Second),
					Error:    nil,
				},
				LastSyncError: &infrav2.SwitchSync{
					Duration: durationpb.New(time.Second),
					Error:    pointer.Pointer("failed to sync"),
				},
			},
			wantSwitch: &apiv2.Switch{
				Id:          "sw2",
				Partition:   "partition-a",
				ReplaceMode: apiv2.SwitchReplaceMode_SWITCH_REPLACE_MODE_OPERATIONAL,
				Meta:        &apiv2.Meta{Generation: 1},
				Nics: []*apiv2.SwitchNic{
					{
						Name:       "Ethernet0",
						Identifier: "Eth1/1",
						BgpFilter:  &apiv2.BGPFilter{},
						State: &apiv2.NicState{
							Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
						},
					},
					{
						Name:       "Ethernet1",
						Identifier: "Eth1/2",
						BgpFilter:  &apiv2.BGPFilter{},
						State: &apiv2.NicState{
							Desired: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP.Enum(),
							Actual:  apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_DOWN,
						},
						BgpPortState: &apiv2.SwitchBGPPortState{
							Neighbor:              "Ethernet2",
							PeerGroup:             "external",
							VrfName:               "Vrf200",
							BgpState:              apiv2.BGPState_BGP_STATE_ESTABLISHED,
							BgpTimerUpEstablished: timestamppb.New(time.Unix(now.Unix(), 0)),
							SentPrefixCounter:     0,
							AcceptedPrefixCounter: 0,
						},
					},
				},
				Os: &apiv2.SwitchOS{
					Vendor:           apiv2.SwitchOSVendor_SWITCH_OS_VENDOR_SONIC,
					Version:          "ec202111",
					MetalCoreVersion: "v0.13.0",
				},
			},
			wantErr: nil,
		},
		{
			name: "no error occurred",
			rq: &infrav2.SwitchServiceHeartbeatRequest{
				Id:       sw1(0).Switch.Id,
				Duration: durationpb.New(2 * time.Second),
				PortStates: map[string]apiv2.SwitchPortStatus{
					"Ethernet0": apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
					"Ethernet1": apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UNKNOWN,
				},
				BgpPortStates: map[string]*apiv2.SwitchBGPPortState{},
			},
			want: &infrav2.SwitchServiceHeartbeatResponse{
				Id: sw1(0).Switch.Id,
				LastSync: &infrav2.SwitchSync{
					Duration: durationpb.New(2 * time.Second),
					Error:    nil,
				},
				LastSyncError: &infrav2.SwitchSync{
					Duration: durationpb.New(time.Second),
					Error:    pointer.Pointer("sync failed"),
				},
			},
			wantSwitch: &apiv2.Switch{
				Id:          "sw1",
				Partition:   "partition-a",
				ReplaceMode: apiv2.SwitchReplaceMode_SWITCH_REPLACE_MODE_OPERATIONAL,
				Meta:        &apiv2.Meta{Generation: 1},
				MachineConnections: []*apiv2.MachineConnection{
					{
						MachineId: "m1",
						Nic: &apiv2.SwitchNic{
							Name:       "Ethernet0",
							Identifier: "Eth1/1",
							BgpFilter:  &apiv2.BGPFilter{},
							State: &apiv2.NicState{
								Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
							},
						},
					},
				},
				Nics: []*apiv2.SwitchNic{
					{
						Name:       "Ethernet0",
						Identifier: "Eth1/1",
						BgpFilter:  &apiv2.BGPFilter{},
						State: &apiv2.NicState{
							Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
						},
					},
					{
						Name:       "Ethernet1",
						Identifier: "Eth1/2",
						BgpFilter:  &apiv2.BGPFilter{},
						State: &apiv2.NicState{
							Desired: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP.Enum(),
							Actual:  apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UNKNOWN,
						},
					},
				},
				Os: &apiv2.SwitchOS{
					Vendor:           apiv2.SwitchOSVendor_SWITCH_OS_VENDOR_SONIC,
					Version:          "ec202111",
					MetalCoreVersion: "v0.13.0",
				},
			},
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &switchServiceServer{
				log:  log,
				repo: repo,
			}

			if tt.wantErr == nil {
				test.Validate(t, tt.rq)
			}

			got, err := s.Heartbeat(ctx, tt.rq)
			if diff := cmp.Diff(tt.wantErr, err, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("switchServiceServer.Heartbeat() error diff = %s", diff)
				return
			}
			if diff := cmp.Diff(
				tt.want, got,
				protocmp.Transform(),
				protocmp.IgnoreFields(
					&infrav2.SwitchSync{}, "time",
				),
			); diff != "" {
				t.Errorf("switchServiceServer.Heartbeat() diff = %v", diff)
			}

			sw, err := repo.Switch().Get(ctx, got.Id)
			require.NoError(t, err)

			if diff := cmp.Diff(
				tt.wantSwitch, sw,
				protocmp.Transform(),
				protocmp.IgnoreFields(
					&apiv2.Meta{}, "created_at", "updated_at",
				),
			); diff != "" {
				t.Errorf("switchServiceServer.Heartbeat() switch diff = %v", diff)
			}
		})
	}
}

// added this test here because using testStore inside the repository package creates an import cycle
func Test_switchRepository_ConnectMachineWithSwitches(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	ctx := t.Context()

	testStore, closer := test.StartRepositoryWithCleanup(t, log)
	defer closer()
	repo := testStore.Store

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintln(w, "a image")
	}))
	defer ts.Close()

	validURL := ts.URL

	var (
		partitions = []*adminv2.PartitionServiceCreateRequest{
			{Partition: &apiv2.Partition{Id: "partition-a", BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}}},
			{Partition: &apiv2.Partition{Id: "partition-b", BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}}},
		}
	)

	test.CreatePartitions(t, repo, partitions)
	test.CreateMachines(t, testStore, []*metal.Machine{m1})
	test.CreateSwitches(t, repo, switches(0))

	tests := []struct {
		name         string
		m            *apiv2.Machine
		wantSwitches []*apiv2.Switch
		wantErr      error
	}{
		{
			name: "partition id not given",
			m: &apiv2.Machine{
				Uuid: "m1",
			},
			wantErr: errorutil.InvalidArgument("partition id of machine m1 is empty"),
		},
		{
			name: "no hardware given",
			m: &apiv2.Machine{
				Uuid: "m1",
				Partition: &apiv2.Partition{
					Id: "partition-a",
				},
			},
			wantErr: errorutil.InvalidArgument("no hardware information for machine m1 given"),
		},
		{
			name: "machine is not connected",
			m: &apiv2.Machine{
				Uuid: "m1",
				Partition: &apiv2.Partition{
					Id: "partition-a",
				},
				Hardware: &apiv2.MachineHardware{
					Nics: []*apiv2.MachineNic{
						{
							Neighbors: []*apiv2.MachineNic{},
						},
					},
				},
			},
			wantErr: errorutil.FailedPrecondition("machine m1 is not connected to exactly two switches, found connections to switches []"),
		},
		{
			name: "machine is connected to three switches",
			m: &apiv2.Machine{
				Uuid: "m1",
				Partition: &apiv2.Partition{
					Id: "partition-a",
				},
				Hardware: &apiv2.MachineHardware{
					Nics: []*apiv2.MachineNic{
						{
							Neighbors: []*apiv2.MachineNic{
								{Hostname: "leaf01"},
								{Hostname: "leaf02"},
								{Hostname: "leaf01-1"},
							},
						},
					},
				},
			},
			wantErr: errorutil.FailedPrecondition("machine m1 is not connected to exactly two switches, found connections to switches [leaf01 leaf02 leaf01-1]"),
		},
		{
			name: "machine is connected to different switches than before",
			m: &apiv2.Machine{
				Uuid: "m1",
				Partition: &apiv2.Partition{
					Id: "partition-a",
				},
				Hardware: &apiv2.MachineHardware{
					Nics: []*apiv2.MachineNic{
						{
							Name: "lan0",
							Neighbors: []*apiv2.MachineNic{
								{
									Name:       "Ethernet0",
									Identifier: "Eth1/1",
									Hostname:   "sw6",
								},
							},
						},
						{
							Name: "lan1",
							Neighbors: []*apiv2.MachineNic{
								{
									Name:       "swp1s0",
									Identifier: "bb:bb:bb:bb:bb:bb",
									Hostname:   "sw5",
								},
							},
						},
					},
				},
			},
			wantSwitches: []*apiv2.Switch{
				{
					Id:                 "sw3",
					MachineConnections: []*apiv2.MachineConnection{},
				},
				{
					Id:                 "sw4",
					MachineConnections: []*apiv2.MachineConnection{},
				},
				{
					Id: "sw5",
					MachineConnections: []*apiv2.MachineConnection{
						{
							MachineId: "m1",
							Nic: &apiv2.SwitchNic{
								Name:       "swp1s0",
								BgpFilter:  &apiv2.BGPFilter{},
								Identifier: "bb:bb:bb:bb:bb:bb",
								State: &apiv2.NicState{
									Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
								},
							},
						},
					},
				},
				{
					Id: "sw6",
					MachineConnections: []*apiv2.MachineConnection{
						{
							MachineId: "m1",
							Nic: &apiv2.SwitchNic{
								Name:       "Ethernet0",
								BgpFilter:  &apiv2.BGPFilter{},
								Identifier: "Eth1/1",
								State: &apiv2.NicState{
									Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
								},
							},
						},
					},
				},
			},
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		s := &switchServiceServer{
			log:  log,
			repo: repo,
		}

		t.Run(tt.name, func(t *testing.T) {
			err := s.repo.Switch().AdditionalMethods().ConnectMachineWithSwitches(ctx, tt.m)
			if diff := cmp.Diff(tt.wantErr, err, testcommon.ErrorStringComparer()); diff != "" {
				t.Errorf("switchRepository.ConnectMachineWithSwitches() error diff = %s", diff)
				return
			}

			for _, wantSwitch := range tt.wantSwitches {
				gotSwitch, err := s.repo.Switch().Get(ctx, wantSwitch.Id)
				require.NoError(t, err)

				if diff := cmp.Diff(wantSwitch.MachineConnections, gotSwitch.MachineConnections, protocmp.Transform()); diff != "" {
					t.Errorf("switchRepository.ConnectMachineWithSwitches() diff = %s", diff)
				}
			}
		})
	}
}
