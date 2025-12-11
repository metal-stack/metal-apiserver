package admin

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
				Id:             "sw1",
				Description:    "switch 01",
				Meta:           &apiv2.Meta{Generation: generation},
				Rack:           pointer.Pointer("rack01"),
				Partition:      "partition-a",
				ReplaceMode:    apiv2.SwitchReplaceMode_SWITCH_REPLACE_MODE_OPERATIONAL,
				ManagementIp:   "1.1.1.1",
				ManagementUser: pointer.Pointer("admin"),
				ConsoleCommand: pointer.Pointer("tty"),
				MachineConnections: []*apiv2.MachineConnection{
					{
						MachineId: "m1",
						Nic: &apiv2.SwitchNic{
							Name:       "Ethernet0",
							Identifier: "Eth1/1",
							Mac:        "11:11:11:11:11:11",
							Vrf:        nil,
							State: &apiv2.NicState{
								Desired: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP.Enum(),
								Actual:  apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
							},
							BgpFilter: &apiv2.BGPFilter{},
							BgpPortState: &apiv2.SwitchBGPPortState{
								Neighbor:              "Ethernet1",
								PeerGroup:             "external",
								VrfName:               "Vrf200",
								BgpState:              apiv2.BGPState_BGP_STATE_CONNECT,
								BgpTimerUpEstablished: timestamppb.New(time.Unix(now.Unix(), 0)),
								SentPrefixCounter:     0,
								AcceptedPrefixCounter: 0,
							},
						},
					},
				},
				Nics: []*apiv2.SwitchNic{
					{
						Name:       "Ethernet0",
						Identifier: "Eth1/1",
						Mac:        "11:11:11:11:11:11",
						Vrf:        nil,
						State: &apiv2.NicState{
							Desired: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP.Enum(),
							Actual:  apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
						},
						BgpFilter: &apiv2.BGPFilter{},
						BgpPortState: &apiv2.SwitchBGPPortState{
							Neighbor:              "Ethernet1",
							PeerGroup:             "external",
							VrfName:               "Vrf200",
							BgpState:              apiv2.BGPState_BGP_STATE_CONNECT,
							BgpTimerUpEstablished: timestamppb.New(time.Unix(now.Unix(), 0)),
							SentPrefixCounter:     0,
							AcceptedPrefixCounter: 0,
						},
					},
					{
						Name:       "Ethernet1",
						Identifier: "Eth1/2",
						Mac:        "22:22:22:22:22:22",
						Vrf:        pointer.Pointer("Vrf200"),
						State: &apiv2.NicState{
							Desired: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP.Enum(),
							Actual:  apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_DOWN,
						},
						BgpFilter: &apiv2.BGPFilter{},
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
				Id:             "sw2",
				Description:    "switch 02",
				Meta:           &apiv2.Meta{Generation: generation},
				Rack:           pointer.Pointer("rack01"),
				Partition:      "partition-a",
				ReplaceMode:    apiv2.SwitchReplaceMode_SWITCH_REPLACE_MODE_OPERATIONAL,
				ManagementIp:   "1.1.1.2",
				ManagementUser: pointer.Pointer("root"),
				ConsoleCommand: pointer.Pointer("tty"),
				Nics: []*apiv2.SwitchNic{
					{
						Name:       "swp1s0",
						Identifier: "33:33:33:33:33:33",
						Mac:        "33:33:33:33:33:33",
						Vrf:        nil,
						BgpFilter:  &apiv2.BGPFilter{},
						State: &apiv2.NicState{
							Desired: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP.Enum(),
							Actual:  apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
						},
					},
					{
						Name:       "swp1s1",
						Identifier: "44:44:44:44:44:44",
						Mac:        "44:44:44:44:44:44",
						Vrf:        pointer.Pointer("vrf200"),
						State: &apiv2.NicState{
							Desired: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP.Enum(),
							Actual:  apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
						},
						BgpFilter: &apiv2.BGPFilter{},
						BgpPortState: &apiv2.SwitchBGPPortState{
							Neighbor:              "Ethernet3",
							PeerGroup:             "external",
							VrfName:               "Vrf200",
							BgpState:              apiv2.BGPState_BGP_STATE_ESTABLISHED,
							BgpTimerUpEstablished: timestamppb.New(time.Unix(now.Add(time.Minute).Unix(), 0)),
							SentPrefixCounter:     100,
							AcceptedPrefixCounter: 1,
						},
					},
				},
				Os: &apiv2.SwitchOS{
					Vendor:           apiv2.SwitchOSVendor_SWITCH_OS_VENDOR_CUMULUS,
					Version:          "v5.9",
					MetalCoreVersion: "v0.13.0",
				},
			},
		}
	}
	sw3 = func(generation uint64) *repository.SwitchServiceCreateRequest {
		return &repository.SwitchServiceCreateRequest{
			Switch: &apiv2.Switch{
				Id:             "sw3",
				Description:    "switch 03",
				Meta:           &apiv2.Meta{Generation: generation},
				Rack:           pointer.Pointer("rack02"),
				Partition:      "partition-b",
				ReplaceMode:    apiv2.SwitchReplaceMode_SWITCH_REPLACE_MODE_REPLACE,
				ManagementIp:   "1.1.1.3",
				ManagementUser: pointer.Pointer("admin"),
				ConsoleCommand: pointer.Pointer("tty"),
				Nics: []*apiv2.SwitchNic{
					{
						Name:       "Ethernet0",
						Identifier: "Eth1/1",
						Mac:        "55:55:55:55:55:55",
						Vrf:        nil,
						BgpFilter:  &apiv2.BGPFilter{},
						State: &apiv2.NicState{
							Desired: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP.Enum(),
							Actual:  apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
						},
					},
					{
						Name:       "Ethernet1",
						Identifier: "Eth1/2",
						Mac:        "66:66:66:66:66:66",
						Vrf:        pointer.Pointer("Vrf300"),
						State: &apiv2.NicState{
							Desired: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP.Enum(),
							Actual:  apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_DOWN,
						},
						BgpFilter: &apiv2.BGPFilter{},
						BgpPortState: &apiv2.SwitchBGPPortState{
							Neighbor:              "Ethernet2",
							PeerGroup:             "external",
							VrfName:               "Vrf300",
							BgpState:              apiv2.BGPState_BGP_STATE_IDLE,
							BgpTimerUpEstablished: timestamppb.New(time.Unix(now.Unix(), 0)),
							SentPrefixCounter:     0,
							AcceptedPrefixCounter: 0,
						},
					},
				},
				Os: &apiv2.SwitchOS{
					Vendor:           apiv2.SwitchOSVendor_SWITCH_OS_VENDOR_SONIC,
					Version:          "ec202211",
					MetalCoreVersion: "v0.13.0",
				},
			},
		}
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

	sw3Status = &repository.SwitchStatus{
		ID: sw3(0).Switch.Id,
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
		return []*repository.SwitchServiceCreateRequest{sw1(generation), sw2(generation), sw3(generation)}
	}
)

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
	test.CreateMachines(t, testStore, []*metal.Machine{
		{
			Base: metal.Base{ID: "m1"},
		},
	})
	test.CreateSwitches(t, repo, switches(0))

	tests := []struct {
		name    string
		rq      *adminv2.SwitchServiceGetRequest
		want    *adminv2.SwitchServiceGetResponse
		wantErr error
	}{
		{
			name: "get existing",
			rq: &adminv2.SwitchServiceGetRequest{
				Id: "sw1",
			},
			want: &adminv2.SwitchServiceGetResponse{
				Switch: sw1(0).Switch,
			},
			wantErr: nil,
		},
		{
			name: "get non-existing",
			rq: &adminv2.SwitchServiceGetRequest{
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

func Test_switchServiceServer_List(t *testing.T) {
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
	test.CreateMachines(t, testStore, []*metal.Machine{
		{
			Base: metal.Base{ID: "m1"},
		},
	})
	test.CreateSwitches(t, repo, switches(0))

	tests := []struct {
		name    string
		rq      *adminv2.SwitchServiceListRequest
		want    *adminv2.SwitchServiceListResponse
		wantErr error
	}{
		{
			name: "get all",
			rq:   &adminv2.SwitchServiceListRequest{},
			want: &adminv2.SwitchServiceListResponse{
				Switches: []*apiv2.Switch{sw1(0).Switch, sw2(0).Switch, sw3(0).Switch},
			},
			wantErr: nil,
		},
		{
			name: "list by rack",
			rq: &adminv2.SwitchServiceListRequest{
				Query: &apiv2.SwitchQuery{
					Rack: pointer.Pointer("rack01"),
				},
			},
			want: &adminv2.SwitchServiceListResponse{
				Switches: []*apiv2.Switch{sw1(0).Switch, sw2(0).Switch},
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

			got, err := s.List(ctx, tt.rq)
			if diff := cmp.Diff(tt.wantErr, err, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("switchServiceServer.List() error diff = %s", diff)
				return
			}
			if diff := cmp.Diff(tt.want, got,
				protocmp.Transform(),
				protocmp.IgnoreFields(
					&apiv2.Meta{}, "created_at", "updated_at",
				)); diff != "" {
				t.Errorf("switchServiceServer.List() diff = %s", diff)
			}
		})
	}
}

func Test_switchServiceServer_Update(t *testing.T) {
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
	test.CreateMachines(t, testStore, []*metal.Machine{
		{
			Base: metal.Base{ID: "m1"},
		},
	})
	switchMap := test.CreateSwitches(t, repo, switches(0))

	tests := []struct {
		name    string
		rq      *adminv2.SwitchServiceUpdateRequest
		want    *adminv2.SwitchServiceUpdateResponse
		wantErr error
	}{
		{
			name: "no updates made",
			rq: &adminv2.SwitchServiceUpdateRequest{
				Id: "sw3",
				UpdateMeta: &apiv2.UpdateMeta{
					UpdatedAt: switchMap["sw3"].Meta.UpdatedAt,
				},
			},
			want: &adminv2.SwitchServiceUpdateResponse{
				Switch: sw3(1).Switch,
			},
			wantErr: nil,
		},
		{
			name: "update all valid fields",
			rq: &adminv2.SwitchServiceUpdateRequest{
				Id: "sw1",
				UpdateMeta: &apiv2.UpdateMeta{
					UpdatedAt: switchMap["sw1"].Meta.UpdatedAt,
				},
				Description:    pointer.Pointer("new description"),
				ReplaceMode:    pointer.Pointer(apiv2.SwitchReplaceMode_SWITCH_REPLACE_MODE_REPLACE),
				ManagementIp:   pointer.Pointer("1.1.1.5"),
				ManagementUser: pointer.Pointer("metal"),
				ConsoleCommand: pointer.Pointer("ssh"),
				Nics: []*apiv2.SwitchNic{
					{
						Name:       "Ethernet0",
						Identifier: "Eth1/1",
						Mac:        "11:11:11:11:11:11",
						Vrf:        pointer.Pointer("Vrf100"),
						BgpFilter:  &apiv2.BGPFilter{},
						State: &apiv2.NicState{
							Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
						},
						BgpPortState: &apiv2.SwitchBGPPortState{
							Neighbor:              "Ethernet1",
							PeerGroup:             "external",
							VrfName:               "Vrf200",
							BgpState:              apiv2.BGPState_BGP_STATE_ESTABLISHED,
							BgpTimerUpEstablished: timestamppb.New(time.Unix(now.Unix(), 0)),
							SentPrefixCounter:     0,
							AcceptedPrefixCounter: 0,
						},
					},
					{
						Name:       "Ethernet2",
						Identifier: "Eth/1/3",
						Mac:        "aa:aa:aa:aa:aa:aa",
						Vrf:        nil,
						BgpFilter:  &apiv2.BGPFilter{},
						State: &apiv2.NicState{
							Desired: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP.Enum(),
							Actual:  apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
						},
					},
				},
				Os: &apiv2.SwitchOS{
					Vendor:           apiv2.SwitchOSVendor_SWITCH_OS_VENDOR_SONIC,
					Version:          "ec202211",
					MetalCoreVersion: "v0.14.0",
				},
			},
			want: &adminv2.SwitchServiceUpdateResponse{
				Switch: &apiv2.Switch{
					Id: "sw1",
					Meta: &apiv2.Meta{
						Generation: 1,
					},
					Description:    "new description",
					Rack:           pointer.Pointer("rack01"),
					Partition:      "partition-a",
					ReplaceMode:    apiv2.SwitchReplaceMode_SWITCH_REPLACE_MODE_REPLACE,
					ManagementIp:   "1.1.1.5",
					ManagementUser: pointer.Pointer("metal"),
					MachineConnections: []*apiv2.MachineConnection{
						{
							MachineId: "m1",
							Nic: &apiv2.SwitchNic{
								Name:       "Ethernet0",
								Identifier: "Eth1/1",
								Mac:        "11:11:11:11:11:11",
								Vrf:        pointer.Pointer("Vrf100"),
								BgpFilter:  &apiv2.BGPFilter{},
								State: &apiv2.NicState{
									Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
								},
								BgpPortState: &apiv2.SwitchBGPPortState{
									Neighbor:              "Ethernet1",
									PeerGroup:             "external",
									VrfName:               "Vrf200",
									BgpState:              apiv2.BGPState_BGP_STATE_ESTABLISHED,
									BgpTimerUpEstablished: timestamppb.New(time.Unix(now.Unix(), 0)),
									SentPrefixCounter:     0,
									AcceptedPrefixCounter: 0,
								},
							},
						},
					},
					ConsoleCommand: pointer.Pointer("ssh"),
					Nics: []*apiv2.SwitchNic{
						{
							Name:       "Ethernet0",
							Identifier: "Eth1/1",
							Mac:        "11:11:11:11:11:11",
							Vrf:        pointer.Pointer("Vrf100"),
							BgpFilter:  &apiv2.BGPFilter{},
							State: &apiv2.NicState{
								Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
							},
							BgpPortState: &apiv2.SwitchBGPPortState{
								Neighbor:              "Ethernet1",
								PeerGroup:             "external",
								VrfName:               "Vrf200",
								BgpState:              apiv2.BGPState_BGP_STATE_ESTABLISHED,
								BgpTimerUpEstablished: timestamppb.New(time.Unix(now.Unix(), 0)),
								SentPrefixCounter:     0,
								AcceptedPrefixCounter: 0,
							},
						},
						{
							Name:       "Ethernet2",
							Identifier: "Eth/1/3",
							Mac:        "aa:aa:aa:aa:aa:aa",
							Vrf:        nil,
							State: &apiv2.NicState{
								Desired: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP.Enum(),
								Actual:  apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
							},
							BgpFilter: &apiv2.BGPFilter{},
						},
					},
					Os: &apiv2.SwitchOS{
						Vendor:           apiv2.SwitchOSVendor_SWITCH_OS_VENDOR_SONIC,
						Version:          "ec202211",
						MetalCoreVersion: "v0.14.0",
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "cannot update os vendor",
			rq: &adminv2.SwitchServiceUpdateRequest{
				Id: "sw2",
				Os: &apiv2.SwitchOS{
					Vendor: apiv2.SwitchOSVendor_SWITCH_OS_VENDOR_SONIC,
				},
			},
			want:    nil,
			wantErr: errorutil.InvalidArgument("cannot update switch os vendor from Cumulus to SONiC, use replace instead"),
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

			got, err := s.Update(ctx, tt.rq)
			if diff := cmp.Diff(tt.wantErr, err, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("switchServiceServer.Update() error diff = %s", diff)
				return
			}
			if diff := cmp.Diff(tt.want, got,
				protocmp.Transform(),
				protocmp.IgnoreFields(
					&apiv2.Meta{}, "created_at", "updated_at",
				)); diff != "" {
				t.Errorf("switchServiceServer.Update() diff = %s", diff)
			}
		})
	}
}

func Test_switchServiceServer_Delete(t *testing.T) {
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
	test.CreateMachines(t, testStore, []*metal.Machine{
		{
			Base: metal.Base{ID: "m1"},
		},
	})
	test.CreateSwitches(t, repo, switches(0))
	test.CreateSwitchStatuses(t, testStore, []*repository.SwitchStatus{sw1Status, sw3Status})

	tests := []struct {
		name    string
		rq      *adminv2.SwitchServiceDeleteRequest
		want    *adminv2.SwitchServiceDeleteResponse
		wantErr error
	}{
		{
			name: "delete switch",
			rq: &adminv2.SwitchServiceDeleteRequest{
				Id: "sw3",
			},
			want: &adminv2.SwitchServiceDeleteResponse{
				Switch: sw3(0).Switch,
			},
			wantErr: nil,
		},
		{
			name: "cannot delete switch with machines connected",
			rq: &adminv2.SwitchServiceDeleteRequest{
				Id: "sw1",
			},
			want:    nil,
			wantErr: errorutil.FailedPrecondition("cannot delete switch sw1 while it still has machines connected to it"),
		},
		{
			name: "but with force you can",
			rq: &adminv2.SwitchServiceDeleteRequest{
				Id:    "sw1",
				Force: true,
			},
			want: &adminv2.SwitchServiceDeleteResponse{
				Switch: sw1(0).Switch,
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
				status, err := testStore.GetSwitchStatus(tt.rq.Id)
				require.NoError(t, err)
				require.NotNil(t, status)
			}

			got, err := s.Delete(ctx, tt.rq)
			if diff := cmp.Diff(tt.wantErr, err, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("switchServiceServer.Delete() error diff = %s", diff)
				return
			}
			if diff := cmp.Diff(tt.want, got,
				protocmp.Transform(),
				protocmp.IgnoreFields(
					&apiv2.Meta{}, "created_at", "updated_at",
				)); diff != "" {
				t.Errorf("switchServiceServer.Delete() diff = %s", diff)
			}

			if tt.wantErr == nil {
				sw, err := s.Get(ctx, &adminv2.SwitchServiceGetRequest{
					Id: tt.rq.Id,
				})
				require.True(t, errorutil.IsNotFound(err))
				require.Nil(t, sw)

				status, err := testStore.GetSwitchStatus(tt.rq.Id)
				require.True(t, errorutil.IsNotFound(err))
				require.Nil(t, status)
			}
		})
	}
}

func Test_switchServiceServer_Port(t *testing.T) {
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
	test.CreateMachines(t, testStore, []*metal.Machine{
		{
			Base: metal.Base{ID: "m1"},
		},
	})
	test.CreateSwitches(t, repo, switches(0))

	tests := []struct {
		name    string
		rq      *adminv2.SwitchServicePortRequest
		want    *adminv2.SwitchServicePortResponse
		wantErr error
	}{
		{
			name:    "port status must be specified",
			rq:      &adminv2.SwitchServicePortRequest{},
			want:    nil,
			wantErr: errorutil.InvalidArgument("failed to parse port status \"SWITCH_PORT_STATUS_UNSPECIFIED\": unable to fetch stringvalue from SWITCH_PORT_STATUS_UNSPECIFIED"),
		},
		{
			name: "port status UNKNOWN is invalid",
			rq: &adminv2.SwitchServicePortRequest{
				Status: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UNKNOWN,
			},
			want:    nil,
			wantErr: errorutil.InvalidArgument("port status \"UNKNOWN\" must be one of [\"UP\", \"DOWN\"]"),
		},
		{
			name: "switch does not exist",
			rq: &adminv2.SwitchServicePortRequest{
				Status:  apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
				Id:      "sw5",
				NicName: "Ethernet0",
			},
			want:    nil,
			wantErr: errorutil.NotFound("no switch with id \"sw5\" found"),
		},
		{
			name: "port does not exist on switch",
			rq: &adminv2.SwitchServicePortRequest{
				Status:  apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
				Id:      "sw1",
				NicName: "Ethernet100",
			},
			want:    nil,
			wantErr: errorutil.InvalidArgument("port Ethernet100 does not exist on switch sw1"),
		},
		{
			name: "nic is not connected to a machine",
			rq: &adminv2.SwitchServicePortRequest{
				Status:  apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
				Id:      "sw1",
				NicName: "Ethernet1",
			},
			want:    nil,
			wantErr: errorutil.FailedPrecondition("port Ethernet1 is not connected to any machine"),
		},
		{
			name: "nic update successful",
			rq: &adminv2.SwitchServicePortRequest{
				Status:  apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_DOWN,
				Id:      "sw1",
				NicName: "Ethernet0",
			},
			want: &adminv2.SwitchServicePortResponse{
				Switch: &apiv2.Switch{
					Id:             "sw1",
					Description:    "switch 01",
					Meta:           &apiv2.Meta{Generation: 1},
					Rack:           pointer.Pointer("rack01"),
					Partition:      "partition-a",
					ReplaceMode:    apiv2.SwitchReplaceMode_SWITCH_REPLACE_MODE_OPERATIONAL,
					ManagementIp:   "1.1.1.1",
					ManagementUser: pointer.Pointer("admin"),
					ConsoleCommand: pointer.Pointer("tty"),
					MachineConnections: []*apiv2.MachineConnection{
						{
							MachineId: "m1",
							Nic: &apiv2.SwitchNic{
								Name:       "Ethernet0",
								Identifier: "Eth1/1",
								Mac:        "11:11:11:11:11:11",
								Vrf:        nil,
								State: &apiv2.NicState{
									Desired: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_DOWN.Enum(),
									Actual:  apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
								},
								BgpFilter: &apiv2.BGPFilter{},
								BgpPortState: &apiv2.SwitchBGPPortState{
									Neighbor:              "Ethernet1",
									PeerGroup:             "external",
									VrfName:               "Vrf200",
									BgpState:              apiv2.BGPState_BGP_STATE_CONNECT,
									BgpTimerUpEstablished: timestamppb.New(time.Unix(now.Unix(), 0)),
									SentPrefixCounter:     0,
									AcceptedPrefixCounter: 0,
								},
							},
						},
					},
					Nics: []*apiv2.SwitchNic{
						{
							Name:       "Ethernet0",
							Identifier: "Eth1/1",
							Mac:        "11:11:11:11:11:11",
							Vrf:        nil,
							State: &apiv2.NicState{
								Desired: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_DOWN.Enum(),
								Actual:  apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
							},
							BgpFilter: &apiv2.BGPFilter{},
							BgpPortState: &apiv2.SwitchBGPPortState{
								Neighbor:              "Ethernet1",
								PeerGroup:             "external",
								VrfName:               "Vrf200",
								BgpState:              apiv2.BGPState_BGP_STATE_CONNECT,
								BgpTimerUpEstablished: timestamppb.New(time.Unix(now.Unix(), 0)),
								SentPrefixCounter:     0,
								AcceptedPrefixCounter: 0,
							},
						},
						{
							Name:       "Ethernet1",
							Identifier: "Eth1/2",
							Mac:        "22:22:22:22:22:22",
							Vrf:        pointer.Pointer("Vrf200"),
							State: &apiv2.NicState{
								Desired: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP.Enum(),
								Actual:  apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_DOWN,
							},
							BgpFilter: &apiv2.BGPFilter{},
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
			got, err := s.Port(ctx, tt.rq)
			if diff := cmp.Diff(tt.wantErr, err, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("switchServiceServer.Port() error diff = %s", diff)
				return
			}
			if diff := cmp.Diff(tt.want, got,
				protocmp.Transform(),
				protocmp.IgnoreFields(
					&apiv2.Meta{}, "created_at", "updated_at",
				)); diff != "" {
				t.Errorf("switchServiceServer.Port() diff = %s", diff)
			}
		})
	}
}
