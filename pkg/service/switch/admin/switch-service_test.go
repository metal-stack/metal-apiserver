package admin

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	"github.com/metal-stack/metal-apiserver/pkg/repository"
	"github.com/metal-stack/metal-apiserver/pkg/test"
	sc "github.com/metal-stack/metal-apiserver/pkg/test/scenarios"
	"github.com/samber/lo"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var (
	now = time.Now()
	sw1 = &repository.SwitchServiceCreateRequest{
		Switch: &apiv2.Switch{
			Id:             "sw1",
			Meta:           &apiv2.Meta{},
			Description:    "switch 01",
			Rack:           new("rack01"),
			Partition:      "partition-a",
			ReplaceMode:    apiv2.SwitchReplaceMode_SWITCH_REPLACE_MODE_OPERATIONAL,
			ManagementIp:   "1.1.1.1",
			ManagementUser: new("admin"),
			ConsoleCommand: new("tty"),
			MachineConnections: []*apiv2.MachineConnection{
				{
					MachineId: m1.ID,
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
					Vrf:        new("Vrf200"),
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
	sw2 = &repository.SwitchServiceCreateRequest{
		Switch: &apiv2.Switch{
			Id:             "sw2",
			Meta:           &apiv2.Meta{},
			Description:    "switch 02",
			Rack:           new("rack01"),
			Partition:      "partition-a",
			ReplaceMode:    apiv2.SwitchReplaceMode_SWITCH_REPLACE_MODE_OPERATIONAL,
			ManagementIp:   "1.1.1.2",
			ManagementUser: new("root"),
			ConsoleCommand: new("tty"),
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
					Vrf:        new("vrf200"),
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
	sw3 = &repository.SwitchServiceCreateRequest{
		Switch: &apiv2.Switch{
			Id:             "sw3",
			Meta:           &apiv2.Meta{},
			Description:    "switch 03",
			Rack:           new("rack02"),
			Partition:      "partition-b",
			ReplaceMode:    apiv2.SwitchReplaceMode_SWITCH_REPLACE_MODE_REPLACE,
			ManagementIp:   "1.1.1.3",
			ManagementUser: new("admin"),
			ConsoleCommand: new("tty"),
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
					Vrf:        new("Vrf300"),
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
	sw4 = &repository.SwitchServiceCreateRequest{
		Switch: &apiv2.Switch{
			Id:          "sw4",
			Meta:        &apiv2.Meta{},
			Partition:   "partition-a",
			Rack:        new("r03"),
			ReplaceMode: apiv2.SwitchReplaceMode_SWITCH_REPLACE_MODE_OPERATIONAL,
			MachineConnections: []*apiv2.MachineConnection{
				{
					MachineId: m1.ID,
					Nic: &apiv2.SwitchNic{
						Name:       "swp1s0",
						Identifier: "aa:aa:aa:aa:aa:aa",
						BgpFilter:  &apiv2.BGPFilter{},
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
					BgpFilter:  &apiv2.BGPFilter{},
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
	sw401 = &repository.SwitchServiceCreateRequest{
		Switch: &apiv2.Switch{
			Id:          "sw4-1",
			Meta:        &apiv2.Meta{},
			Partition:   "partition-a",
			Rack:        new("r04"),
			ReplaceMode: apiv2.SwitchReplaceMode_SWITCH_REPLACE_MODE_OPERATIONAL,
			Nics: []*apiv2.SwitchNic{
				{
					Name:       "swp1s0",
					Identifier: "aa:aa:aa:aa:aa:aa",
					BgpFilter:  &apiv2.BGPFilter{},
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
	sw402 = &repository.SwitchServiceCreateRequest{
		Switch: &apiv2.Switch{
			Id:          "sw4-2",
			Meta:        &apiv2.Meta{},
			Partition:   "partition-a",
			Rack:        new("r03"),
			ReplaceMode: apiv2.SwitchReplaceMode_SWITCH_REPLACE_MODE_OPERATIONAL,
			MachineConnections: []*apiv2.MachineConnection{
				{
					MachineId: m1.ID,
					Nic: &apiv2.SwitchNic{
						Name:       "swp1s0",
						Identifier: "aa:aa:aa:aa:aa:aa",
						BgpFilter:  &apiv2.BGPFilter{},
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
					BgpFilter:  &apiv2.BGPFilter{},
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
	sw5 = &repository.SwitchServiceCreateRequest{
		Switch: &apiv2.Switch{
			Id:          "sw5",
			Meta:        &apiv2.Meta{},
			Partition:   "partition-a",
			Rack:        new("r03"),
			ReplaceMode: apiv2.SwitchReplaceMode_SWITCH_REPLACE_MODE_OPERATIONAL,
			MachineConnections: []*apiv2.MachineConnection{
				{
					MachineId: m1.ID,
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
			},
			Os: &apiv2.SwitchOS{
				Vendor: apiv2.SwitchOSVendor_SWITCH_OS_VENDOR_SONIC,
			},
		},
	}
	sw501 = &repository.SwitchServiceCreateRequest{
		Switch: &apiv2.Switch{
			Id:          "sw5-1",
			Meta:        &apiv2.Meta{},
			Partition:   "partition-a",
			Rack:        new("r03"),
			ReplaceMode: apiv2.SwitchReplaceMode_SWITCH_REPLACE_MODE_OPERATIONAL,
			Nics: []*apiv2.SwitchNic{
				{
					Name:       "Ethernet0",
					Identifier: "Eth1/1",
					BgpFilter:  &apiv2.BGPFilter{},
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

	switches = []*repository.SwitchServiceCreateRequest{sw1, sw2, sw3, sw4, sw401, sw402, sw5, sw501}

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

func Test_switchServiceServer_Get(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	ctx := t.Context()

	testStore, closer := test.StartRepositoryWithCleanup(t, log)
	defer closer()

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

	test.CreatePartitions(t, testStore, partitions)
	test.CreateMachines(t, testStore, []*metal.Machine{m1})
	test.CreateSwitches(t, testStore, switches)

	tests := []struct {
		name    string
		rq      *adminv2.SwitchServiceGetRequest
		want    *adminv2.SwitchServiceGetResponse
		wantErr error
	}{
		{
			name: "get existing",
			rq: &adminv2.SwitchServiceGetRequest{
				Id: sw1.Switch.Id,
			},
			want: &adminv2.SwitchServiceGetResponse{
				Switch: sw1.Switch,
			},
			wantErr: nil,
		},
		{
			name: "get non-existing",
			rq: &adminv2.SwitchServiceGetRequest{
				Id: "sw10",
			},
			want:    nil,
			wantErr: errorutil.NotFound("no switch with id \"sw10\" found"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &switchServiceServer{
				log:  log,
				repo: testStore.Store,
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

	test.CreatePartitions(t, testStore, partitions)
	test.CreateMachines(t, testStore, []*metal.Machine{m1})
	test.CreateSwitches(t, testStore, switches)

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
				Switches: lo.Map(switches, func(rq *repository.SwitchServiceCreateRequest, _ int) *apiv2.Switch { return rq.Switch }),
			},
			wantErr: nil,
		},
		{
			name: "list by rack",
			rq: &adminv2.SwitchServiceListRequest{
				Query: &apiv2.SwitchQuery{
					Rack: new("rack01"),
				},
			},
			want: &adminv2.SwitchServiceListResponse{
				Switches: []*apiv2.Switch{sw1.Switch, sw2.Switch},
			},
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &switchServiceServer{
				log:  log,
				repo: testStore.Store,
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

	test.CreatePartitions(t, testStore, partitions)
	test.CreateMachines(t, testStore, []*metal.Machine{m1})
	switchMap := test.CreateSwitches(t, testStore, switches)

	tests := []struct {
		name    string
		rq      *adminv2.SwitchServiceUpdateRequest
		want    *adminv2.SwitchServiceUpdateResponse
		wantErr error
	}{
		{
			name: "no updates made",
			rq: &adminv2.SwitchServiceUpdateRequest{
				Id: sw3.Switch.Id,
				UpdateMeta: &apiv2.UpdateMeta{
					UpdatedAt: switchMap["sw3"].Meta.UpdatedAt,
				},
			},
			want: &adminv2.SwitchServiceUpdateResponse{
				Switch: &apiv2.Switch{
					Id:             sw3.Switch.Id,
					Meta:           &apiv2.Meta{Generation: 1},
					Description:    "switch 03",
					Rack:           new("rack02"),
					Partition:      "partition-b",
					ReplaceMode:    apiv2.SwitchReplaceMode_SWITCH_REPLACE_MODE_REPLACE,
					ManagementIp:   "1.1.1.3",
					ManagementUser: new("admin"),
					ConsoleCommand: new("tty"),
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
							Vrf:        new("Vrf300"),
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
			},
			wantErr: nil,
		},
		{
			name: "update all valid fields",
			rq: &adminv2.SwitchServiceUpdateRequest{
				Id: sw1.Switch.Id,
				UpdateMeta: &apiv2.UpdateMeta{
					UpdatedAt: switchMap["sw1"].Meta.UpdatedAt,
				},
				Description:    new("new description"),
				ReplaceMode:    new(apiv2.SwitchReplaceMode_SWITCH_REPLACE_MODE_REPLACE),
				ManagementIp:   new("1.1.1.5"),
				ManagementUser: new("metal"),
				ConsoleCommand: new("ssh"),
				Nics: []*apiv2.SwitchNic{
					{
						Name:       "Ethernet0",
						Identifier: "Eth1/1",
						Mac:        "11:11:11:11:11:11",
						Vrf:        new("Vrf100"),
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
					Id: sw1.Switch.Id,
					Meta: &apiv2.Meta{
						Generation: 1,
					},
					Description:    "new description",
					Rack:           new("rack01"),
					Partition:      "partition-a",
					ReplaceMode:    apiv2.SwitchReplaceMode_SWITCH_REPLACE_MODE_REPLACE,
					ManagementIp:   "1.1.1.5",
					ManagementUser: new("metal"),
					MachineConnections: []*apiv2.MachineConnection{
						{
							MachineId: m1.ID,
							Nic: &apiv2.SwitchNic{
								Name:       "Ethernet0",
								Identifier: "Eth1/1",
								Mac:        "11:11:11:11:11:11",
								Vrf:        new("Vrf100"),
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
					ConsoleCommand: new("ssh"),
					Nics: []*apiv2.SwitchNic{
						{
							Name:       "Ethernet0",
							Identifier: "Eth1/1",
							Mac:        "11:11:11:11:11:11",
							Vrf:        new("Vrf100"),
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
				Id: sw2.Switch.Id,
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
				repo: testStore.Store,
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
	t.Parallel()

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	ctx := t.Context()

	dc := test.NewDatacenter(t, log)
	defer dc.Close()

	tests := []struct {
		name    string
		rq      *adminv2.SwitchServiceDeleteRequest
		want    func() *adminv2.SwitchServiceDeleteResponse
		mods    func() *test.Asserters
		wantErr error
	}{
		{
			name: "delete switch",
			rq: &adminv2.SwitchServiceDeleteRequest{
				Id: sc.SwmP01Rack03Switch1,
			},
			want: func() *adminv2.SwitchServiceDeleteResponse {
				return &adminv2.SwitchServiceDeleteResponse{
					Switch: dc.GetSwitches()[sc.SwmP01Rack03Switch1],
				}
			},
			mods: func() *test.Asserters {
				return &test.Asserters{
					Switches: func(switches map[string]*apiv2.Switch) {
						delete(switches, sc.SwmP01Rack03Switch1)
					},
					SwitchStatuses: func(switchStatuses map[string]*metal.SwitchStatus) {
						delete(switchStatuses, sc.SwmP01Rack03Switch1)
					},
				}
			},
			wantErr: nil,
		},
		{
			name: "cannot delete switch with machines connected",
			rq: &adminv2.SwitchServiceDeleteRequest{
				Id: sc.SwmP01Rack02Switch1,
			},
			want: func() *adminv2.SwitchServiceDeleteResponse {
				return nil
			},
			wantErr: errorutil.FailedPrecondition("cannot delete switch %s while it still has machines connected to it", sc.SwmP01Rack02Switch1),
		},
		{
			name: "but with force you can",
			rq: &adminv2.SwitchServiceDeleteRequest{
				Id:    sc.SwmP01Rack02Switch1,
				Force: true,
			},
			want: func() *adminv2.SwitchServiceDeleteResponse {
				return &adminv2.SwitchServiceDeleteResponse{
					Switch: dc.GetSwitches()[sc.SwmP01Rack02Switch1],
				}
			},
			mods: func() *test.Asserters {
				return &test.Asserters{
					Switches: func(switches map[string]*apiv2.Switch) {
						delete(switches, sc.SwmP01Rack02Switch1)
					},
					SwitchStatuses: func(switchStatuses map[string]*metal.SwitchStatus) {
						delete(switchStatuses, sc.SwmP01Rack02Switch1)
					},
				}
			},
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dc.Create(&sc.SwitchesWithMachinesDatacenter)
			defer dc.Cleanup()

			s := &switchServiceServer{
				log:  log,
				repo: dc.GetTestStore().Store,
			}
			if tt.wantErr == nil {
				test.Validate(t, tt.rq)
			}

			var want *adminv2.SwitchServiceDeleteResponse
			if tt.want != nil {
				want = tt.want()
			}

			got, err := s.Delete(ctx, tt.rq)
			if diff := cmp.Diff(tt.wantErr, err, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("switchServiceServer.Delete() error diff = %s", diff)
				return
			}
			if diff := cmp.Diff(want, got,
				protocmp.Transform(),
				protocmp.IgnoreFields(
					&apiv2.Meta{}, "created_at", "updated_at",
				)); diff != "" {
				t.Errorf("switchServiceServer.Delete() diff = %s", diff)
			}

			var mods *test.Asserters
			if tt.mods != nil {
				mods = tt.mods()
			}
			err = dc.Assert(mods)
			require.NoError(t, err)
		})
	}
}

func Test_switchServiceServer_Port(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	ctx := t.Context()

	testStore, closer := test.StartRepositoryWithCleanup(t, log)
	defer closer()

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

	test.CreatePartitions(t, testStore, partitions)
	test.CreateMachines(t, testStore, []*metal.Machine{m1})
	test.CreateSwitches(t, testStore, switches)

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
				Id:      "sw10",
				NicName: "Ethernet0",
			},
			want:    nil,
			wantErr: errorutil.NotFound("no switch with id \"sw10\" found"),
		},
		{
			name: "port does not exist on switch",
			rq: &adminv2.SwitchServicePortRequest{
				Status:  apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
				Id:      sw1.Switch.Id,
				NicName: "Ethernet100",
			},
			want:    nil,
			wantErr: errorutil.InvalidArgument("port Ethernet100 does not exist on switch sw1"),
		},
		{
			name: "nic is not connected to a machine",
			rq: &adminv2.SwitchServicePortRequest{
				Status:  apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
				Id:      sw1.Switch.Id,
				NicName: "Ethernet1",
			},
			want:    nil,
			wantErr: errorutil.FailedPrecondition("port Ethernet1 is not connected to any machine"),
		},
		{
			name: "nic update successful",
			rq: &adminv2.SwitchServicePortRequest{
				Status:  apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_DOWN,
				Id:      sw1.Switch.Id,
				NicName: "Ethernet0",
			},
			want: &adminv2.SwitchServicePortResponse{
				Switch: &apiv2.Switch{
					Id:             sw1.Switch.Id,
					Description:    "switch 01",
					Meta:           &apiv2.Meta{Generation: 1},
					Rack:           new("rack01"),
					Partition:      "partition-a",
					ReplaceMode:    apiv2.SwitchReplaceMode_SWITCH_REPLACE_MODE_OPERATIONAL,
					ManagementIp:   "1.1.1.1",
					ManagementUser: new("admin"),
					ConsoleCommand: new("tty"),
					MachineConnections: []*apiv2.MachineConnection{
						{
							MachineId: m1.ID,
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
							Vrf:        new("Vrf200"),
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
				repo: testStore.Store,
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

func Test_switchServiceServer_Migrate(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	ctx := t.Context()

	testStore, closer := test.StartRepositoryWithCleanup(t, log)
	defer closer()

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

	test.CreatePartitions(t, testStore, partitions)
	test.CreateMachines(t, testStore, []*metal.Machine{m1})
	test.CreateSwitches(t, testStore, switches)

	tests := []struct {
		name    string
		rq      *adminv2.SwitchServiceMigrateRequest
		want    *adminv2.SwitchServiceMigrateResponse
		wantErr error
	}{
		{
			name: "cannot migrate from one rack to another",
			rq: &adminv2.SwitchServiceMigrateRequest{
				OldSwitch: sw4.Switch.Id,
				NewSwitch: sw401.Switch.Id,
			},
			want:    nil,
			wantErr: errorutil.FailedPrecondition("cannot migrate from switch %s in rack %s to switch %s in rack %s, switches must be in the same rack", sw4.Switch.Id, *sw4.Switch.Rack, sw401.Switch.Id, *sw401.Switch.Rack),
		},
		{
			name: "cannot migrate to switch that already has connections",
			rq: &adminv2.SwitchServiceMigrateRequest{
				OldSwitch: sw4.Switch.Id,
				NewSwitch: sw402.Switch.Id,
			},
			want:    nil,
			wantErr: errorutil.FailedPrecondition("cannot migrate from switch %s to switch %s because the new switch already has machine connections", sw4.Switch.Id, sw402.Switch.Id),
		},
		{
			name: "migrate successfully",
			rq: &adminv2.SwitchServiceMigrateRequest{
				OldSwitch: sw5.Switch.Id,
				NewSwitch: sw501.Switch.Id,
			},
			want: &adminv2.SwitchServiceMigrateResponse{
				Switch: &apiv2.Switch{
					Id:          sw501.Switch.Id,
					Partition:   "partition-a",
					Rack:        new("r03"),
					ReplaceMode: apiv2.SwitchReplaceMode_SWITCH_REPLACE_MODE_OPERATIONAL,
					Meta:        &apiv2.Meta{Generation: 1},
					MachineConnections: []*apiv2.MachineConnection{
						{
							MachineId: m1.ID,
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
					},
					Os: &apiv2.SwitchOS{
						Vendor: apiv2.SwitchOSVendor_SWITCH_OS_VENDOR_SONIC,
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
				repo: testStore.Store,
			}
			if tt.wantErr == nil {
				test.Validate(t, tt.rq)
			}

			got, err := s.Migrate(ctx, tt.rq)
			if diff := cmp.Diff(tt.wantErr, err, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("switchServiceServer.Migrate() error diff = %s", diff)
				return
			}
			if diff := cmp.Diff(tt.want, got,
				protocmp.Transform(),
				protocmp.IgnoreFields(
					&apiv2.Meta{}, "created_at", "updated_at",
				)); diff != "" {
				t.Errorf("switchServiceServer.Migrate() diff = %s", diff)
			}
		})
	}
}

func Test_switchServiceServer_ConnectedMachines(t *testing.T) {
	t.Parallel()

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	ctx := t.Context()

	dc := test.NewDatacenter(t, log)
	defer dc.Close()
	dc.Create(&sc.SwitchesWithMachinesDatacenter)

	tests := []struct {
		name string
		rq   *adminv2.SwitchServiceConnectedMachinesRequest
		want *adminv2.SwitchServiceConnectedMachinesResponse
	}{
		// TODO: order of lists is weird. should we sort here or in the client?
		{
			name: "get all",
			rq:   &adminv2.SwitchServiceConnectedMachinesRequest{},
			want: &adminv2.SwitchServiceConnectedMachinesResponse{
				SwitchesWithMachines: []*apiv2.SwitchWithMachines{
					{
						Id:        sc.SwmP01Rack01Switch1,
						Partition: sc.SwmPartition1,
						Rack:      sc.SwmRack1,
						Connections: []*apiv2.SwitchNicWithMachine{
							{
								Nic: &apiv2.SwitchNic{
									Name:       "Ethernet0",
									Identifier: "Eth1/1",
									BgpFilter:  &apiv2.BGPFilter{},
									State: &apiv2.NicState{
										Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
									},
								},
								Machine: dc.GetMachines()[sc.SwmMachine1],
								Fru: &apiv2.MachineFRU{
									ChassisPartNumber:   new(string),
									ChassisPartSerial:   new(string),
									BoardMfg:            new(string),
									BoardMfgSerial:      new(string),
									BoardPartNumber:     new(string),
									ProductManufacturer: new(string),
									ProductPartNumber:   new(string),
									ProductSerial:       new("PS1"),
								},
							},
						},
					},
					{
						Id:        sc.SwmP01Rack01Switch2,
						Partition: sc.SwmPartition1,
						Rack:      sc.SwmRack1,
						Connections: []*apiv2.SwitchNicWithMachine{
							{
								Nic: &apiv2.SwitchNic{
									Name:       "Ethernet0",
									Identifier: "Eth1/1",
									BgpFilter:  &apiv2.BGPFilter{},
									State: &apiv2.NicState{
										Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
									},
								},
								Machine: dc.GetMachines()[sc.SwmMachine1],
								Fru: &apiv2.MachineFRU{
									ChassisPartNumber:   new(string),
									ChassisPartSerial:   new(string),
									BoardMfg:            new(string),
									BoardMfgSerial:      new(string),
									BoardPartNumber:     new(string),
									ProductManufacturer: new(string),
									ProductPartNumber:   new(string),
									ProductSerial:       new("PS1"),
								},
							},
						},
					},
					{
						Id:        sc.SwmP01Rack02Switch1,
						Partition: sc.SwmPartition1,
						Rack:      sc.SwmRack2,
						Connections: []*apiv2.SwitchNicWithMachine{
							{
								Nic: &apiv2.SwitchNic{
									Name:       "Ethernet0",
									Identifier: "Eth1/1",
									BgpFilter:  &apiv2.BGPFilter{},
									State: &apiv2.NicState{
										Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
									},
								},
								Machine: dc.GetMachines()[sc.SwmMachine2],
								Fru: &apiv2.MachineFRU{
									ChassisPartNumber:   new(string),
									ChassisPartSerial:   new(string),
									BoardMfg:            new(string),
									BoardMfgSerial:      new(string),
									BoardPartNumber:     new(string),
									ProductManufacturer: new(string),
									ProductPartNumber:   new(string),
									ProductSerial:       new("PS2"),
								},
							},
						},
					},
					{
						Id:        sc.SwmP01Rack02Switch2,
						Partition: sc.SwmPartition1,
						Rack:      sc.SwmRack2,
						Connections: []*apiv2.SwitchNicWithMachine{
							{
								Nic: &apiv2.SwitchNic{
									Name:       "Ethernet0",
									Identifier: "Eth1/1",
									BgpFilter:  &apiv2.BGPFilter{},
									State: &apiv2.NicState{
										Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
									},
								},
								Machine: dc.GetMachines()[sc.SwmMachine2],
								Fru: &apiv2.MachineFRU{
									ChassisPartNumber:   new(string),
									ChassisPartSerial:   new(string),
									BoardMfg:            new(string),
									BoardMfgSerial:      new(string),
									BoardPartNumber:     new(string),
									ProductManufacturer: new(string),
									ProductPartNumber:   new(string),
									ProductSerial:       new("PS2"),
								},
							},
						},
					},
					{
						Id:        sc.SwmP01Rack03Switch1,
						Partition: sc.SwmPartition1,
						Rack:      sc.SwmRack3,
					},
					{
						Id:        sc.SwmP01Rack03Switch2,
						Partition: sc.SwmPartition1,
						Rack:      sc.SwmRack3,
					},
					{
						Id:        sc.SwmP02Rack01Switch1,
						Partition: sc.SwmPartition2,
						Rack:      sc.SwmRack1,
						Connections: []*apiv2.SwitchNicWithMachine{
							{
								Nic: &apiv2.SwitchNic{
									Name:       "Ethernet0",
									Identifier: "Eth1/1",
									BgpFilter:  &apiv2.BGPFilter{},
									State: &apiv2.NicState{
										Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
									},
								},
								Machine: dc.GetMachines()[sc.SwmMachine3],
								Fru: &apiv2.MachineFRU{
									ChassisPartNumber:   new(string),
									ChassisPartSerial:   new(string),
									BoardMfg:            new(string),
									BoardMfgSerial:      new(string),
									BoardPartNumber:     new(string),
									ProductManufacturer: new(string),
									ProductPartNumber:   new(string),
									ProductSerial:       new("PS3"),
								},
							},
						},
					},
					{
						Id:        sc.SwmP02Rack01Switch2,
						Partition: sc.SwmPartition2,
						Rack:      sc.SwmRack1,
						Connections: []*apiv2.SwitchNicWithMachine{
							{
								Nic: &apiv2.SwitchNic{
									Name:       "Ethernet0",
									Identifier: "Eth1/1",
									BgpFilter:  &apiv2.BGPFilter{},
									State: &apiv2.NicState{
										Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
									},
								},
								Machine: dc.GetMachines()[sc.SwmMachine3],
								Fru: &apiv2.MachineFRU{
									ChassisPartNumber:   new(string),
									ChassisPartSerial:   new(string),
									BoardMfg:            new(string),
									BoardMfgSerial:      new(string),
									BoardPartNumber:     new(string),
									ProductManufacturer: new(string),
									ProductPartNumber:   new(string),
									ProductSerial:       new("PS3"),
								},
							},
						},
					},
					{
						Id:        sc.SwmP02Rack02Switch1,
						Partition: sc.SwmPartition2,
						Rack:      sc.SwmRack2,
						Connections: []*apiv2.SwitchNicWithMachine{
							{
								Nic: &apiv2.SwitchNic{
									Name:       "Ethernet0",
									Identifier: "Eth1/1",
									BgpFilter:  &apiv2.BGPFilter{},
									State: &apiv2.NicState{
										Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
									},
								},
								Machine: dc.GetMachines()[sc.SwmMachine4],
								Fru: &apiv2.MachineFRU{
									ChassisPartNumber:   new(string),
									ChassisPartSerial:   new(string),
									BoardMfg:            new(string),
									BoardMfgSerial:      new(string),
									BoardPartNumber:     new(string),
									ProductManufacturer: new(string),
									ProductPartNumber:   new(string),
									ProductSerial:       new("PS4"),
								},
							},
							{
								Nic: &apiv2.SwitchNic{
									Name:       "Ethernet1",
									Identifier: "Eth2/2",
									BgpFilter:  &apiv2.BGPFilter{},
									State: &apiv2.NicState{
										Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
									},
								},
								Machine: dc.GetMachines()[sc.SwmMachine5],
								Fru: &apiv2.MachineFRU{
									ChassisPartNumber:   new(string),
									ChassisPartSerial:   new(string),
									BoardMfg:            new(string),
									BoardMfgSerial:      new(string),
									BoardPartNumber:     new(string),
									ProductManufacturer: new(string),
									ProductPartNumber:   new(string),
									ProductSerial:       new("PS5"),
								},
							},
						},
					},
					{
						Id:        sc.SwmP02Rack02Switch2,
						Partition: sc.SwmPartition2,
						Rack:      sc.SwmRack2,
						Connections: []*apiv2.SwitchNicWithMachine{
							{
								Nic: &apiv2.SwitchNic{
									Name:       "Ethernet0",
									Identifier: "Eth1/1",
									BgpFilter:  &apiv2.BGPFilter{},
									State: &apiv2.NicState{
										Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
									},
								},
								Machine: dc.GetMachines()[sc.SwmMachine4],
								Fru: &apiv2.MachineFRU{
									ChassisPartNumber:   new(string),
									ChassisPartSerial:   new(string),
									BoardMfg:            new(string),
									BoardMfgSerial:      new(string),
									BoardPartNumber:     new(string),
									ProductManufacturer: new(string),
									ProductPartNumber:   new(string),
									ProductSerial:       new("PS4"),
								},
							},
							{
								Nic: &apiv2.SwitchNic{
									Name:       "Ethernet1",
									Identifier: "Eth2/2",
									BgpFilter:  &apiv2.BGPFilter{},
									State: &apiv2.NicState{
										Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
									},
								},
								Machine: dc.GetMachines()[sc.SwmMachine5],
								Fru: &apiv2.MachineFRU{
									ChassisPartNumber:   new(string),
									ChassisPartSerial:   new(string),
									BoardMfg:            new(string),
									BoardMfgSerial:      new(string),
									BoardPartNumber:     new(string),
									ProductManufacturer: new(string),
									ProductPartNumber:   new(string),
									ProductSerial:       new("PS5"),
								},
							},
						},
					},
				},
			},
		},
		{
			name: "query by switch id",
			rq: &adminv2.SwitchServiceConnectedMachinesRequest{
				Query: &apiv2.SwitchQuery{
					Id: new(sc.SwmP01Rack01Switch2),
				},
			},
			want: &adminv2.SwitchServiceConnectedMachinesResponse{
				SwitchesWithMachines: []*apiv2.SwitchWithMachines{
					{
						Id:        sc.SwmP01Rack01Switch2,
						Partition: sc.SwmPartition1,
						Rack:      sc.SwmRack1,
						Connections: []*apiv2.SwitchNicWithMachine{
							{
								Nic: &apiv2.SwitchNic{
									Name:       "Ethernet0",
									Identifier: "Eth1/1",
									BgpFilter:  &apiv2.BGPFilter{},
									State: &apiv2.NicState{
										Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
									},
								},
								Machine: dc.GetMachines()[sc.SwmMachine1],
								Fru: &apiv2.MachineFRU{
									ChassisPartNumber:   new(string),
									ChassisPartSerial:   new(string),
									BoardMfg:            new(string),
									BoardMfgSerial:      new(string),
									BoardPartNumber:     new(string),
									ProductManufacturer: new(string),
									ProductPartNumber:   new(string),
									ProductSerial:       new("PS1"),
								},
							},
						},
					},
				},
			},
		},
		{
			name: "query by machine id",
			rq: &adminv2.SwitchServiceConnectedMachinesRequest{
				MachineQuery: &apiv2.MachineQuery{
					Uuid: new(sc.Machine5),
				},
			},
			want: &adminv2.SwitchServiceConnectedMachinesResponse{
				SwitchesWithMachines: []*apiv2.SwitchWithMachines{
					{
						Id:        sc.SwmP02Rack02Switch1,
						Partition: sc.SwmPartition2,
						Rack:      sc.SwmRack2,
						Connections: []*apiv2.SwitchNicWithMachine{
							{
								Nic: &apiv2.SwitchNic{
									Name:       "Ethernet1",
									Identifier: "Eth2/2",
									BgpFilter:  &apiv2.BGPFilter{},
									State: &apiv2.NicState{
										Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
									},
								},
								Machine: dc.GetMachines()[sc.SwmMachine5],
								Fru: &apiv2.MachineFRU{
									ChassisPartNumber:   new(string),
									ChassisPartSerial:   new(string),
									BoardMfg:            new(string),
									BoardMfgSerial:      new(string),
									BoardPartNumber:     new(string),
									ProductManufacturer: new(string),
									ProductPartNumber:   new(string),
									ProductSerial:       new("PS5"),
								},
							},
						},
					},
					{
						Id:        sc.SwmP02Rack02Switch2,
						Partition: sc.SwmPartition2,
						Rack:      sc.SwmRack2,
						Connections: []*apiv2.SwitchNicWithMachine{
							{
								Nic: &apiv2.SwitchNic{
									Name:       "Ethernet1",
									Identifier: "Eth2/2",
									BgpFilter:  &apiv2.BGPFilter{},
									State: &apiv2.NicState{
										Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
									},
								},
								Machine: dc.GetMachines()[sc.SwmMachine5],
								Fru: &apiv2.MachineFRU{
									ChassisPartNumber:   new(string),
									ChassisPartSerial:   new(string),
									BoardMfg:            new(string),
									BoardMfgSerial:      new(string),
									BoardPartNumber:     new(string),
									ProductManufacturer: new(string),
									ProductPartNumber:   new(string),
									ProductSerial:       new("PS5"),
								},
							},
						},
					},
				},
			},
		},
		{
			name: "query by partition",
			rq: &adminv2.SwitchServiceConnectedMachinesRequest{
				Query: &apiv2.SwitchQuery{
					Partition: new(sc.SwmPartition1),
				},
				MachineQuery: &apiv2.MachineQuery{},
			},
			want: &adminv2.SwitchServiceConnectedMachinesResponse{
				SwitchesWithMachines: []*apiv2.SwitchWithMachines{
					{
						Id:        sc.SwmP01Rack01Switch1,
						Partition: sc.SwmPartition1,
						Rack:      sc.SwmRack1,
						Connections: []*apiv2.SwitchNicWithMachine{
							{
								Nic: &apiv2.SwitchNic{
									Name:       "Ethernet0",
									Identifier: "Eth1/1",
									BgpFilter:  &apiv2.BGPFilter{},
									State: &apiv2.NicState{
										Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
									},
								},
								Machine: dc.GetMachines()[sc.SwmMachine1],
								Fru: &apiv2.MachineFRU{
									ChassisPartNumber:   new(string),
									ChassisPartSerial:   new(string),
									BoardMfg:            new(string),
									BoardMfgSerial:      new(string),
									BoardPartNumber:     new(string),
									ProductManufacturer: new(string),
									ProductPartNumber:   new(string),
									ProductSerial:       new("PS1"),
								},
							},
						},
					},
					{
						Id:        sc.SwmP01Rack01Switch2,
						Partition: sc.SwmPartition1,
						Rack:      sc.SwmRack1,
						Connections: []*apiv2.SwitchNicWithMachine{
							{
								Nic: &apiv2.SwitchNic{
									Name:       "Ethernet0",
									Identifier: "Eth1/1",
									BgpFilter:  &apiv2.BGPFilter{},
									State: &apiv2.NicState{
										Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
									},
								},
								Machine: dc.GetMachines()[sc.SwmMachine1],
								Fru: &apiv2.MachineFRU{
									ChassisPartNumber:   new(string),
									ChassisPartSerial:   new(string),
									BoardMfg:            new(string),
									BoardMfgSerial:      new(string),
									BoardPartNumber:     new(string),
									ProductManufacturer: new(string),
									ProductPartNumber:   new(string),
									ProductSerial:       new("PS1"),
								},
							},
						},
					},
					{
						Id:        sc.SwmP01Rack02Switch1,
						Partition: sc.SwmPartition1,
						Rack:      sc.SwmRack2,
						Connections: []*apiv2.SwitchNicWithMachine{
							{
								Nic: &apiv2.SwitchNic{
									Name:       "Ethernet0",
									Identifier: "Eth1/1",
									BgpFilter:  &apiv2.BGPFilter{},
									State: &apiv2.NicState{
										Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
									},
								},
								Machine: dc.GetMachines()[sc.SwmMachine2],
								Fru: &apiv2.MachineFRU{
									ChassisPartNumber:   new(string),
									ChassisPartSerial:   new(string),
									BoardMfg:            new(string),
									BoardMfgSerial:      new(string),
									BoardPartNumber:     new(string),
									ProductManufacturer: new(string),
									ProductPartNumber:   new(string),
									ProductSerial:       new("PS2"),
								},
							},
						},
					},
					{
						Id:        sc.SwmP01Rack02Switch2,
						Partition: sc.SwmPartition1,
						Rack:      sc.SwmRack2,
						Connections: []*apiv2.SwitchNicWithMachine{
							{
								Nic: &apiv2.SwitchNic{
									Name:       "Ethernet0",
									Identifier: "Eth1/1",
									BgpFilter:  &apiv2.BGPFilter{},
									State: &apiv2.NicState{
										Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
									},
								},
								Machine: dc.GetMachines()[sc.SwmMachine2],
								Fru: &apiv2.MachineFRU{
									ChassisPartNumber:   new(string),
									ChassisPartSerial:   new(string),
									BoardMfg:            new(string),
									BoardMfgSerial:      new(string),
									BoardPartNumber:     new(string),
									ProductManufacturer: new(string),
									ProductPartNumber:   new(string),
									ProductSerial:       new("PS2"),
								},
							},
						},
					},
					{
						Id:        sc.SwmP01Rack03Switch1,
						Partition: sc.SwmPartition1,
						Rack:      sc.SwmRack3,
					},
					{
						Id:        sc.SwmP01Rack03Switch2,
						Partition: sc.SwmPartition1,
						Rack:      sc.SwmRack3,
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &switchServiceServer{
				log:  log,
				repo: dc.GetTestStore().Store,
			}
			got, err := s.ConnectedMachines(ctx, tt.rq)
			require.NoError(t, err)

			slices.SortStableFunc(got.SwitchesWithMachines, func(swm1, swm2 *apiv2.SwitchWithMachines) int {
				return strings.Compare(swm1.Id, swm2.Id)
			})

			if diff := cmp.Diff(tt.want, got, protocmp.Transform()); diff != "" {
				t.Errorf("switchServiceServer.ConnectedMachines() diff = %s", diff)
			}
		})
	}
}
