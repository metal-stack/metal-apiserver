package admin

import (
	"log/slog"
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
	"github.com/metal-stack/metal-apiserver/pkg/test"
	sc "github.com/metal-stack/metal-apiserver/pkg/test/scenarios"
	"github.com/samber/lo"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func Test_switchServiceServer_Get(t *testing.T) {
	var (
		log = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
		ctx = t.Context()
	)

	tests := []struct {
		name    string
		rq      *adminv2.SwitchServiceGetRequest
		want    func(*test.Entities) *adminv2.SwitchServiceGetResponse
		wantErr error
	}{
		{
			name: "get existing",
			rq: &adminv2.SwitchServiceGetRequest{
				Id: sc.P01Rack01Switch1,
			},
			want: func(e *test.Entities) *adminv2.SwitchServiceGetResponse {
				return &adminv2.SwitchServiceGetResponse{
					Switch: e.Switches[sc.P01Rack01Switch1],
				}
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

	dc := test.NewDatacenter(t, log)
	defer dc.Close()
	dc.Create(&sc.SwitchesWithMachinesDatacenter)
	snapshot := dc.Snapshot()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &switchServiceServer{
				log:  log,
				repo: dc.GetTestStore().Store,
			}

			if tt.wantErr == nil {
				test.Validate(t, tt.rq)
			}

			var want *adminv2.SwitchServiceGetResponse
			if tt.want != nil {
				want = tt.want(dc.Snapshot())
			}

			got, err := s.Get(ctx, tt.rq)
			if diff := cmp.Diff(tt.wantErr, err, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("switchServiceServer.Get() error diff = %s", diff)
				return
			}
			if diff := cmp.Diff(want, got,
				protocmp.Transform(),
				protocmp.IgnoreFields(
					&apiv2.Meta{}, "created_at", "updated_at",
				)); diff != "" {
				t.Errorf("switchServiceServer.Get() diff = %s", diff)
			}
			err = dc.AssertSnapshot(snapshot, nil)
			require.NoError(t, err)
		})
	}
}

func Test_switchServiceServer_List(t *testing.T) {
	var (
		log = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
		ctx = t.Context()
	)

	tests := []struct {
		name string
		rq   *adminv2.SwitchServiceListRequest
		want func(*test.Entities) *adminv2.SwitchServiceListResponse
	}{
		{
			name: "get all",
			rq:   &adminv2.SwitchServiceListRequest{},
			want: func(e *test.Entities) *adminv2.SwitchServiceListResponse {
				switches := lo.Values(e.Switches)
				slices.SortFunc(switches, func(sw1, sw2 *apiv2.Switch) int {
					return strings.Compare(sw1.Id, sw2.Id)
				})

				return &adminv2.SwitchServiceListResponse{Switches: switches}
			},
		},
		{
			name: "list by rack",
			rq: &adminv2.SwitchServiceListRequest{
				Query: &apiv2.SwitchQuery{
					Rack: new(sc.P01Rack01),
				},
			},
			want: func(e *test.Entities) *adminv2.SwitchServiceListResponse {
				return &adminv2.SwitchServiceListResponse{
					Switches: []*apiv2.Switch{
						e.Switches[sc.P01Rack01Switch1],
						e.Switches[sc.P01Rack01Switch2],
						e.Switches[sc.P01Rack01Switch2_1],
					},
				}
			},
		},
		{
			name: "list by id",
			rq: &adminv2.SwitchServiceListRequest{
				Query: &apiv2.SwitchQuery{
					Id: new(sc.P01Rack01Switch1),
				},
			},
			want: func(e *test.Entities) *adminv2.SwitchServiceListResponse {
				return &adminv2.SwitchServiceListResponse{
					Switches: []*apiv2.Switch{e.Switches[sc.P01Rack01Switch1]},
				}
			},
		},
		{
			name: "list by partition",
			rq: &adminv2.SwitchServiceListRequest{
				Query: &apiv2.SwitchQuery{
					Partition: new(sc.Partition2),
				},
			},
			want: func(e *test.Entities) *adminv2.SwitchServiceListResponse {
				return &adminv2.SwitchServiceListResponse{
					Switches: []*apiv2.Switch{
						e.Switches[sc.P02Rack01Switch1],
						e.Switches[sc.P02Rack01Switch2],
						e.Switches[sc.P02Rack01Switch2_1],
						e.Switches[sc.P02Rack02Switch1],
						e.Switches[sc.P02Rack02Switch2],
						e.Switches[sc.P02Rack02Switch2_1],
						e.Switches[sc.P02Rack03Switch1],
						e.Switches[sc.P02Rack03Switch2],
					},
				}
			},
		},
		{
			name: "list by os vendor",
			rq: &adminv2.SwitchServiceListRequest{
				Query: &apiv2.SwitchQuery{
					Os: &apiv2.SwitchOSQuery{
						Vendor: apiv2.SwitchOSVendor_SWITCH_OS_VENDOR_CUMULUS.Enum(),
					},
				},
			},
			want: func(e *test.Entities) *adminv2.SwitchServiceListResponse {
				return &adminv2.SwitchServiceListResponse{
					Switches: []*apiv2.Switch{
						e.Switches[sc.P01Rack02Switch1],
						e.Switches[sc.P01Rack03Switch2],
						e.Switches[sc.P02Rack01Switch2_1],
						e.Switches[sc.P02Rack03Switch2],
					},
				}
			},
		},
		{
			name: "list by os version",
			rq: &adminv2.SwitchServiceListRequest{
				Query: &apiv2.SwitchQuery{
					Os: &apiv2.SwitchOSQuery{
						Version: new("2022"),
					},
				},
			},
			want: func(e *test.Entities) *adminv2.SwitchServiceListResponse {
				return &adminv2.SwitchServiceListResponse{
					Switches: []*apiv2.Switch{
						e.Switches[sc.P01Rack03Switch1],
					},
				}
			},
		},
	}

	dc := test.NewDatacenter(t, log)
	defer dc.Close()
	dc.Create(&sc.SwitchesWithMachinesDatacenter)
	snapshot := dc.Snapshot()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &switchServiceServer{
				log:  log,
				repo: dc.GetTestStore().Store,
			}

			test.Validate(t, tt.rq)
			got, err := s.List(ctx, tt.rq)
			require.NoError(t, err)

			var want *adminv2.SwitchServiceListResponse
			if tt.want != nil {
				want = tt.want(dc.Snapshot())
			}

			if diff := cmp.Diff(want, got,
				protocmp.Transform(),
				protocmp.IgnoreFields(
					&apiv2.Meta{}, "created_at", "updated_at",
				)); diff != "" {
				t.Errorf("switchServiceServer.List() diff = %s", diff)
			}
			err = dc.AssertSnapshot(snapshot, nil)
			require.NoError(t, err)
		})
	}
}

func Test_switchServiceServer_Update(t *testing.T) {
	var (
		log = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
		ctx = t.Context()
		now = time.Now()
	)

	tests := []struct {
		name    string
		rq      func(*test.Entities) *adminv2.SwitchServiceUpdateRequest
		want    func(*test.Entities) *adminv2.SwitchServiceUpdateResponse
		mods    func() *test.Asserters
		wantErr error
	}{
		{
			name: "no updates made",
			rq: func(e *test.Entities) *adminv2.SwitchServiceUpdateRequest {
				return &adminv2.SwitchServiceUpdateRequest{
					Id: sc.P01Rack01Switch1,
					UpdateMeta: &apiv2.UpdateMeta{
						UpdatedAt: e.Switches[sc.P01Rack01Switch1].Meta.UpdatedAt,
					},
				}
			},
			want: func(e *test.Entities) *adminv2.SwitchServiceUpdateResponse {
				return &adminv2.SwitchServiceUpdateResponse{
					Switch: e.Switches[sc.P01Rack01Switch1],
				}
			},
			wantErr: nil,
		},
		{
			name: "update all valid fields",
			rq: func(e *test.Entities) *adminv2.SwitchServiceUpdateRequest {
				return &adminv2.SwitchServiceUpdateRequest{
					Id: sc.P01Rack01Switch1,
					UpdateMeta: &apiv2.UpdateMeta{
						UpdatedAt: e.Switches[sc.P01Rack01Switch1].Meta.UpdatedAt,
					},
					Description:    new("new description"),
					ReplaceMode:    new(apiv2.SwitchReplaceMode_SWITCH_REPLACE_MODE_REPLACE),
					ManagementIp:   new("1.1.1.5"),
					ManagementUser: new("metal"),
					ConsoleCommand: new("ssh"),
					Nics: []*apiv2.SwitchNic{
						{
							Name:       "Ethernet0",
							Identifier: "Ethernet0",
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
							Identifier: "Ethernet2",
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
				}
			},
			want: func(e *test.Entities) *adminv2.SwitchServiceUpdateResponse {
				sw := e.Switches[sc.P01Rack01Switch1]
				nic1 := &apiv2.SwitchNic{
					Name:       "Ethernet0",
					Identifier: "Ethernet0",
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
				}
				nic2 := &apiv2.SwitchNic{
					Name:       "Ethernet2",
					Identifier: "Ethernet2",
					Mac:        "aa:aa:aa:aa:aa:aa",
					Vrf:        nil,
					BgpFilter:  &apiv2.BGPFilter{},
					State: &apiv2.NicState{
						Desired: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP.Enum(),
						Actual:  apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
					},
				}
				sw.Description = "new description"
				sw.ReplaceMode = apiv2.SwitchReplaceMode_SWITCH_REPLACE_MODE_REPLACE
				sw.ManagementIp = "1.1.1.5"
				sw.ManagementUser = new("metal")
				sw.ConsoleCommand = new("ssh")
				sw.Nics = []*apiv2.SwitchNic{nic1, nic2}
				sw.MachineConnections = []*apiv2.MachineConnection{
					{
						MachineId: sc.Machine1,
						Nic:       nic1,
					},
				}
				sw.Os.Version = "ec202211"
				sw.Os.MetalCoreVersion = "v0.14.0"

				return &adminv2.SwitchServiceUpdateResponse{
					Switch: sw,
				}
			},
			mods: func() *test.Asserters {
				return &test.Asserters{
					Switches: func(switches map[string]*apiv2.Switch) {
						sw := switches[sc.P01Rack01Switch1]
						nic1 := &apiv2.SwitchNic{
							Name:       "Ethernet0",
							Identifier: "Ethernet0",
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
						}
						nic2 := &apiv2.SwitchNic{
							Name:       "Ethernet2",
							Identifier: "Ethernet2",
							Mac:        "aa:aa:aa:aa:aa:aa",
							Vrf:        nil,
							BgpFilter:  &apiv2.BGPFilter{},
							State: &apiv2.NicState{
								Desired: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP.Enum(),
								Actual:  apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
							},
						}
						sw.Description = "new description"
						sw.ReplaceMode = apiv2.SwitchReplaceMode_SWITCH_REPLACE_MODE_REPLACE
						sw.ManagementIp = "1.1.1.5"
						sw.ManagementUser = new("metal")
						sw.ConsoleCommand = new("ssh")
						sw.Nics = []*apiv2.SwitchNic{nic1, nic2}
						sw.MachineConnections = []*apiv2.MachineConnection{
							{
								MachineId: sc.Machine1,
								Nic:       nic1,
							},
						}
						sw.Os.Version = "ec202211"
						sw.Os.MetalCoreVersion = "v0.14.0"
						switches[sc.P01Rack01Switch1] = sw
					},
				}
			},
			wantErr: nil,
		},
		{
			name: "cannot update os vendor",
			rq: func(e *test.Entities) *adminv2.SwitchServiceUpdateRequest {
				return &adminv2.SwitchServiceUpdateRequest{
					Id: e.Switches[sc.P01Rack03Switch2].Id,
					UpdateMeta: &apiv2.UpdateMeta{
						UpdatedAt: e.Switches[sc.P01Rack03Switch2].Meta.UpdatedAt,
					},
					Os: &apiv2.SwitchOS{
						Vendor: apiv2.SwitchOSVendor_SWITCH_OS_VENDOR_SONIC,
					},
				}
			},
			want:    nil,
			wantErr: errorutil.InvalidArgument("cannot update switch os vendor from Cumulus to SONiC, use replace instead"),
		},
	}

	dc := test.NewDatacenter(t, log)
	defer dc.Close()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dc.Create(&sc.SwitchesWithMachinesDatacenter)
			defer dc.Cleanup()

			var (
				rq       *adminv2.SwitchServiceUpdateRequest
				want     *adminv2.SwitchServiceUpdateResponse
				snapshot = dc.Snapshot()
			)

			if tt.rq != nil {
				rq = tt.rq(dc.Snapshot())
			}
			if tt.want != nil {
				want = tt.want(dc.Snapshot())
			}

			s := &switchServiceServer{
				log:  log,
				repo: dc.GetTestStore().Store,
			}
			if tt.wantErr == nil {
				test.Validate(t, rq)
			}

			got, err := s.Update(ctx, rq)
			if diff := cmp.Diff(tt.wantErr, err, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("switchServiceServer.Update() error diff = %s", diff)
				return
			}
			if diff := cmp.Diff(want, got,
				protocmp.Transform(),
				protocmp.IgnoreFields(
					&apiv2.Meta{}, "created_at", "updated_at", "generation",
				)); diff != "" {
				t.Errorf("switchServiceServer.Update() diff = %s", diff)
			}

			var mods *test.Asserters
			if tt.mods != nil {
				mods = tt.mods()
			}
			err = dc.AssertSnapshot(snapshot, mods)
			require.NoError(t, err)
		})
	}
}

func Test_switchServiceServer_Delete(t *testing.T) {
	var (
		log = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
		ctx = t.Context()
	)

	tests := []struct {
		name    string
		rq      *adminv2.SwitchServiceDeleteRequest
		want    func(*test.Datacenter) *adminv2.SwitchServiceDeleteResponse
		mods    func() *test.Asserters
		wantErr error
	}{
		{
			name: "delete switch",
			rq: &adminv2.SwitchServiceDeleteRequest{
				Id: sc.P01Rack03Switch1,
			},
			want: func(dc *test.Datacenter) *adminv2.SwitchServiceDeleteResponse {
				sw := dc.GetSwitches()[sc.P01Rack03Switch1]
				sw.LastSync = nil
				sw.LastSyncError = nil
				return &adminv2.SwitchServiceDeleteResponse{
					Switch: sw,
				}
			},
			mods: func() *test.Asserters {
				return &test.Asserters{
					Switches: func(switches map[string]*apiv2.Switch) {
						delete(switches, sc.P01Rack03Switch1)
					},
					SwitchStatuses: func(switchStatuses map[string]*metal.SwitchStatus) {
						delete(switchStatuses, sc.P01Rack03Switch1)
					},
				}
			},
			wantErr: nil,
		},
		{
			name: "cannot delete switch with machines connected",
			rq: &adminv2.SwitchServiceDeleteRequest{
				Id: sc.P01Rack02Switch1,
			},
			want: func(dc *test.Datacenter) *adminv2.SwitchServiceDeleteResponse {
				return nil
			},
			wantErr: errorutil.FailedPrecondition("cannot delete switch %s while it still has machines connected to it", sc.P01Rack02Switch1),
		},
		{
			name: "but with force you can",
			rq: &adminv2.SwitchServiceDeleteRequest{
				Id:    sc.P01Rack02Switch1,
				Force: true,
			},
			want: func(dc *test.Datacenter) *adminv2.SwitchServiceDeleteResponse {
				sw := dc.GetSwitches()[sc.P01Rack02Switch1]
				sw.LastSync = nil
				sw.LastSyncError = nil
				return &adminv2.SwitchServiceDeleteResponse{
					Switch: sw,
				}
			},
			mods: func() *test.Asserters {
				return &test.Asserters{
					Switches: func(switches map[string]*apiv2.Switch) {
						delete(switches, sc.P01Rack02Switch1)
					},
					SwitchStatuses: func(switchStatuses map[string]*metal.SwitchStatus) {
						delete(switchStatuses, sc.P01Rack02Switch1)
					},
				}
			},
			wantErr: nil,
		},
	}

	dc := test.NewDatacenter(t, log)
	defer dc.Close()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dc.Create(&sc.SwitchesWithMachinesDatacenter)
			defer dc.Cleanup()

			snapshot := dc.Snapshot()

			s := &switchServiceServer{
				log:  log,
				repo: dc.GetTestStore().Store,
			}
			if tt.wantErr == nil {
				test.Validate(t, tt.rq)
			}

			var want *adminv2.SwitchServiceDeleteResponse
			if tt.want != nil {
				want = tt.want(dc)
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
			err = dc.AssertSnapshot(snapshot, mods)
			require.NoError(t, err)
		})
	}
}

func Test_switchServiceServer_Port(t *testing.T) {
	var (
		log = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
		ctx = t.Context()
	)

	tests := []struct {
		name    string
		rq      *adminv2.SwitchServicePortRequest
		want    func(*test.Datacenter) *adminv2.SwitchServicePortResponse
		mods    func() *test.Asserters
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
				NicName: "Ethernet0"},
			want:    nil,
			wantErr: errorutil.NotFound("no switch with id \"sw10\" found"),
		},
		{
			name: "port does not exist on switch",
			rq: &adminv2.SwitchServicePortRequest{
				Status:  apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
				Id:      sc.P01Rack01Switch1,
				NicName: "swp1",
			},
			want:    nil,
			wantErr: errorutil.InvalidArgument("port swp1 does not exist on switch %s", sc.P01Rack01Switch1),
		},
		{
			name: "nic is not connected to a machine",
			rq: &adminv2.SwitchServicePortRequest{
				Status:  apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
				Id:      sc.P01Rack01Switch1,
				NicName: "Ethernet1",
			},
			want:    nil,
			wantErr: errorutil.FailedPrecondition("port Ethernet1 is not connected to any machine"),
		},
		{
			name: "nic update successful",
			rq: &adminv2.SwitchServicePortRequest{
				Status:  apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_DOWN,
				Id:      sc.P01Rack01Switch1,
				NicName: "Ethernet0",
			},
			want: func(dc *test.Datacenter) *adminv2.SwitchServicePortResponse {
				sw := dc.GetSwitches()[sc.P01Rack01Switch1]
				sw.Nics[0].State.Desired = apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_DOWN.Enum()
				return &adminv2.SwitchServicePortResponse{
					Switch: sw,
				}
			},
			mods: func() *test.Asserters {
				return &test.Asserters{
					Switches: func(switches map[string]*apiv2.Switch) {
						sw := switches[sc.P01Rack01Switch1]
						sw.Nics[0].State.Desired = apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_DOWN.Enum()
						con, found := lo.Find(sw.MachineConnections, func(c *apiv2.MachineConnection) bool {
							return c.Nic.Name == "Ethernet0"
						})
						require.True(t, found)
						con.Nic.State.Desired = apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_DOWN.Enum()
					},
				}
			},
			wantErr: nil,
		},
	}

	dc := test.NewDatacenter(t, log)
	defer dc.Close()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dc.Create(&sc.SwitchesWithMachinesDatacenter)
			defer dc.Cleanup()

			var (
				snapshot = dc.Snapshot()
				want     *adminv2.SwitchServicePortResponse
			)

			if tt.want != nil {
				want = tt.want(dc)
			}

			s := &switchServiceServer{
				log:  log,
				repo: dc.GetTestStore().Store,
			}
			if tt.wantErr == nil {
				test.Validate(t, tt.rq)
			}

			got, err := s.Port(ctx, tt.rq)
			if diff := cmp.Diff(tt.wantErr, err, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("switchServiceServer.Port() error diff = %s", diff)
				return
			}
			if diff := cmp.Diff(want, got,
				protocmp.Transform(),
				protocmp.IgnoreFields(
					&apiv2.Meta{}, "created_at", "updated_at", "generation",
				)); diff != "" {
				t.Errorf("switchServiceServer.Port() diff = %s", diff)
			}

			var mods *test.Asserters
			if tt.mods != nil {
				mods = tt.mods()
			}
			err = dc.AssertSnapshot(snapshot, mods)
			require.NoError(t, err)
		})
	}
}

func Test_switchServiceServer_Migrate(t *testing.T) {
	var (
		log = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
		ctx = t.Context()
	)

	tests := []struct {
		name    string
		rq      *adminv2.SwitchServiceMigrateRequest
		want    func(*test.Datacenter) *adminv2.SwitchServiceMigrateResponse
		mods    func() *test.Asserters
		wantErr error
	}{
		{
			name: "cannot migrate from one rack to another",
			rq: &adminv2.SwitchServiceMigrateRequest{
				OldSwitch: sc.P01Rack01Switch1,
				NewSwitch: sc.P01Rack02Switch1,
			},
			want:    nil,
			wantErr: errorutil.FailedPrecondition("cannot migrate from switch %s in rack %s to switch %s in rack %s, switches must be in the same rack", sc.P01Rack01Switch1, sc.P01Rack01, sc.P01Rack02Switch1, sc.P01Rack02),
		},
		{
			name: "cannot migrate to switch that already has connections",
			rq: &adminv2.SwitchServiceMigrateRequest{
				OldSwitch: sc.P02Rack02Switch2,
				NewSwitch: sc.P02Rack02Switch2_1,
			},
			want:    nil,
			wantErr: errorutil.FailedPrecondition("cannot migrate from switch %s to switch %s because the new switch already has machine connections", sc.P02Rack02Switch2, sc.P02Rack02Switch2_1),
		},
		{
			name: "migrate from cumulus to sonic",
			rq: &adminv2.SwitchServiceMigrateRequest{
				OldSwitch: sc.P01Rack02Switch1,
				NewSwitch: sc.P01Rack02Switch1_1,
			},
			want: func(dc *test.Datacenter) *adminv2.SwitchServiceMigrateResponse {
				sw := dc.GetSwitches()[sc.P01Rack02Switch1_1]
				nic, found := lo.Find(sw.Nics, func(n *apiv2.SwitchNic) bool {
					return n.Name == "Ethernet0"
				})
				require.True(t, found)
				sw.MachineConnections = []*apiv2.MachineConnection{
					{
						MachineId: sc.Machine2,
						Nic:       nic,
					},
				}
				sw.ReplaceMode = apiv2.SwitchReplaceMode_SWITCH_REPLACE_MODE_OPERATIONAL

				return &adminv2.SwitchServiceMigrateResponse{
					Switch: sw,
				}
			},
			mods: func() *test.Asserters {
				return &test.Asserters{
					Switches: func(switches map[string]*apiv2.Switch) {
						sw1 := switches[sc.P01Rack02Switch1]
						sw2 := switches[sc.P01Rack02Switch1_1]
						sw1.MachineConnections = []*apiv2.MachineConnection{}
						nic, found := lo.Find(sw2.Nics, func(n *apiv2.SwitchNic) bool {
							return n.Name == "Ethernet0"
						})
						require.True(t, found)
						sw2.MachineConnections = []*apiv2.MachineConnection{
							{
								MachineId: sc.Machine2,
								Nic:       nic,
							},
						}
						sw2.ReplaceMode = apiv2.SwitchReplaceMode_SWITCH_REPLACE_MODE_OPERATIONAL
					},
				}
			},
			wantErr: nil,
		},
		{
			name: "migrate from sonic to cumulus",
			rq: &adminv2.SwitchServiceMigrateRequest{
				OldSwitch: sc.P02Rack01Switch2,
				NewSwitch: sc.P02Rack01Switch2_1,
			},
			want: func(dc *test.Datacenter) *adminv2.SwitchServiceMigrateResponse {

				sw := dc.GetSwitches()[sc.P02Rack01Switch2_1]
				nic, found := lo.Find(sw.Nics, func(n *apiv2.SwitchNic) bool {
					return n.Name == "swp1s0"
				})
				require.True(t, found)
				sw.MachineConnections = []*apiv2.MachineConnection{
					{
						MachineId: sc.Machine3,
						Nic:       nic,
					},
				}
				sw.ReplaceMode = apiv2.SwitchReplaceMode_SWITCH_REPLACE_MODE_OPERATIONAL

				return &adminv2.SwitchServiceMigrateResponse{
					Switch: sw,
				}
			},
			mods: func() *test.Asserters {
				return &test.Asserters{
					Switches: func(switches map[string]*apiv2.Switch) {
						sw1 := switches[sc.P02Rack01Switch2]
						sw2 := switches[sc.P02Rack01Switch2_1]
						nic, found := lo.Find(sw2.Nics, func(n *apiv2.SwitchNic) bool {
							return n.Name == "swp1s0"
						})
						require.True(t, found)
						sw1.MachineConnections = []*apiv2.MachineConnection{}
						sw2.MachineConnections = []*apiv2.MachineConnection{
							{
								MachineId: sc.Machine3,
								Nic:       nic,
							},
						}
						sw2.ReplaceMode = apiv2.SwitchReplaceMode_SWITCH_REPLACE_MODE_OPERATIONAL
					},
				}
			},
			wantErr: nil,
		},
	}

	dc := test.NewDatacenter(t, log)
	defer dc.Close()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dc.Create(&sc.SwitchesWithMachinesDatacenter)
			defer dc.Cleanup()

			var (
				snapshot = dc.Snapshot()
				want     *adminv2.SwitchServiceMigrateResponse
			)

			if tt.want != nil {
				want = tt.want(dc)
			}

			s := &switchServiceServer{
				log:  log,
				repo: dc.GetTestStore().Store,
			}
			if tt.wantErr == nil {
				test.Validate(t, tt.rq)
			}

			got, err := s.Migrate(ctx, tt.rq)
			if diff := cmp.Diff(tt.wantErr, err, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("switchServiceServer.Migrate() error diff = %s", diff)
				return
			}
			if diff := cmp.Diff(want, got,
				protocmp.Transform(),
				protocmp.IgnoreFields(
					&apiv2.Meta{}, "created_at", "updated_at", "generation",
				)); diff != "" {
				t.Errorf("switchServiceServer.Migrate() diff = %s", diff)
			}

			var mods *test.Asserters
			if tt.mods != nil {
				mods = tt.mods()
			}
			err = dc.AssertSnapshot(snapshot, mods)
			require.NoError(t, err)
		})
	}
}

func Test_switchServiceServer_ConnectedMachines(t *testing.T) {
	var (
		log = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
		ctx = t.Context()
	)

	tests := []struct {
		name string
		rq   *adminv2.SwitchServiceConnectedMachinesRequest
		want func(*test.Datacenter) *adminv2.SwitchServiceConnectedMachinesResponse
	}{
		{
			name: "get all",
			rq:   &adminv2.SwitchServiceConnectedMachinesRequest{},
			want: func(dc *test.Datacenter) *adminv2.SwitchServiceConnectedMachinesResponse {
				return &adminv2.SwitchServiceConnectedMachinesResponse{
					SwitchesWithMachines: []*apiv2.SwitchWithMachines{
						{
							Id:        sc.P01Rack01Switch1,
							Partition: sc.Partition1,
							Rack:      sc.P01Rack01,
							Connections: []*apiv2.SwitchNicWithMachine{
								{
									Nic: &apiv2.SwitchNic{
										Name:       "Ethernet0",
										Identifier: "Ethernet0",
										BgpFilter:  &apiv2.BGPFilter{},
										State: &apiv2.NicState{
											Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
										},
									},
									Machine: dc.GetMachines()[sc.Machine1],
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
							Id:        sc.P01Rack01Switch2,
							Partition: sc.Partition1,
							Rack:      sc.P01Rack01,
							Connections: []*apiv2.SwitchNicWithMachine{
								{
									Nic: &apiv2.SwitchNic{
										Name:       "Ethernet0",
										Identifier: "Ethernet0",
										BgpFilter:  &apiv2.BGPFilter{},
										State: &apiv2.NicState{
											Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
										},
									},
									Machine: dc.GetMachines()[sc.Machine1],
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
							Id:        sc.P01Rack01Switch2_1,
							Partition: sc.Partition1,
							Rack:      sc.P01Rack01,
						},
						{
							Id:        sc.P01Rack02Switch1,
							Partition: sc.Partition1,
							Rack:      sc.P01Rack02,
							Connections: []*apiv2.SwitchNicWithMachine{
								{
									Nic: &apiv2.SwitchNic{
										Name:       "swp1s0",
										Identifier: "swp1s0",
										BgpFilter:  &apiv2.BGPFilter{},
										State: &apiv2.NicState{
											Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
										},
									},
									Machine: dc.GetMachines()[sc.Machine2],
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
							Id:        sc.P01Rack02Switch1_1,
							Partition: sc.Partition1,
							Rack:      sc.P01Rack02,
						},
						{
							Id:        sc.P01Rack02Switch2,
							Partition: sc.Partition1,
							Rack:      sc.P01Rack02,
							Connections: []*apiv2.SwitchNicWithMachine{
								{
									Nic: &apiv2.SwitchNic{
										Name:       "Ethernet0",
										Identifier: "Ethernet0",
										BgpFilter:  &apiv2.BGPFilter{},
										State: &apiv2.NicState{
											Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
										},
									},
									Machine: dc.GetMachines()[sc.Machine2],
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
							Id:        sc.P01Rack03Switch1,
							Partition: sc.Partition1,
							Rack:      sc.P01Rack03,
						},
						{
							Id:        sc.P01Rack03Switch2,
							Partition: sc.Partition1,
							Rack:      sc.P01Rack03,
						},
						{
							Id:        sc.P01Rack04Switch1,
							Partition: sc.Partition1,
							Rack:      sc.P01Rack04,
						},
						{
							Id:        sc.P02Rack01Switch1,
							Partition: sc.Partition2,
							Rack:      sc.P02Rack01,
							Connections: []*apiv2.SwitchNicWithMachine{
								{
									Nic: &apiv2.SwitchNic{
										Name:       "Ethernet0",
										Identifier: "Ethernet0",
										BgpFilter:  &apiv2.BGPFilter{},
										State: &apiv2.NicState{
											Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
										},
									},
									Machine: dc.GetMachines()[sc.Machine3],
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
							Id:        sc.P02Rack01Switch2,
							Partition: sc.Partition2,
							Rack:      sc.P02Rack01,
							Connections: []*apiv2.SwitchNicWithMachine{
								{
									Nic: &apiv2.SwitchNic{
										Name:       "Ethernet0",
										Identifier: "Ethernet0",
										BgpFilter:  &apiv2.BGPFilter{},
										State: &apiv2.NicState{
											Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
										},
									},
									Machine: dc.GetMachines()[sc.Machine3],
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
							Id:        sc.P02Rack01Switch2_1,
							Partition: sc.Partition2,
							Rack:      sc.P02Rack01,
						},
						{
							Id:        sc.P02Rack02Switch1,
							Partition: sc.Partition2,
							Rack:      sc.P02Rack02,
							Connections: []*apiv2.SwitchNicWithMachine{
								{
									Nic: &apiv2.SwitchNic{
										Name:       "Ethernet0",
										Identifier: "Ethernet0",
										BgpFilter:  &apiv2.BGPFilter{},
										State: &apiv2.NicState{
											Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
										},
									},
									Machine: dc.GetMachines()[sc.Machine4],
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
										Identifier: "Ethernet1",
										BgpFilter:  &apiv2.BGPFilter{},
										State: &apiv2.NicState{
											Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
										},
									},
									Machine: dc.GetMachines()[sc.Machine5],
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
							Id:        sc.P02Rack02Switch2,
							Partition: sc.Partition2,
							Rack:      sc.P02Rack02,
							Connections: []*apiv2.SwitchNicWithMachine{
								{
									Nic: &apiv2.SwitchNic{
										Name:       "Ethernet0",
										Identifier: "Ethernet0",
										BgpFilter:  &apiv2.BGPFilter{},
										State: &apiv2.NicState{
											Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
										},
									},
									Machine: dc.GetMachines()[sc.Machine4],
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
										Identifier: "Ethernet1",
										BgpFilter:  &apiv2.BGPFilter{},
										State: &apiv2.NicState{
											Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
										},
									},
									Machine: dc.GetMachines()[sc.Machine5],
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
							Id:        sc.P02Rack02Switch2_1,
							Partition: sc.Partition2,
							Rack:      sc.P02Rack02,
							Connections: []*apiv2.SwitchNicWithMachine{
								{
									Nic: &apiv2.SwitchNic{
										Name:       "Ethernet0",
										Identifier: "Ethernet0",
										BgpFilter:  &apiv2.BGPFilter{},
										State: &apiv2.NicState{
											Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
										},
									},
									Machine: dc.GetMachines()[sc.Machine6],
									Fru: &apiv2.MachineFRU{
										ChassisPartNumber:   new(string),
										ChassisPartSerial:   new(string),
										BoardMfg:            new(string),
										BoardMfgSerial:      new(string),
										BoardPartNumber:     new(string),
										ProductManufacturer: new(string),
										ProductPartNumber:   new(string),
										ProductSerial:       new("PS6"),
									},
								},
							},
						},
						{
							Id:        sc.P02Rack03Switch1,
							Partition: sc.Partition2,
							Rack:      sc.P02Rack03,
							Connections: []*apiv2.SwitchNicWithMachine{
								{
									Nic: &apiv2.SwitchNic{
										Name:       "Ethernet0",
										Identifier: "Ethernet0",
										BgpFilter:  &apiv2.BGPFilter{},
										State: &apiv2.NicState{
											Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
										},
									},
									Machine: dc.GetMachines()[sc.Machine7],
									Fru: &apiv2.MachineFRU{
										ChassisPartNumber:   new(string),
										ChassisPartSerial:   new(string),
										BoardMfg:            new(string),
										BoardMfgSerial:      new(string),
										BoardPartNumber:     new(string),
										ProductManufacturer: new(string),
										ProductPartNumber:   new(string),
										ProductSerial:       new("PS7"),
									},
								},
							},
						},
						{
							Id:        sc.P02Rack03Switch2,
							Partition: sc.Partition2,
							Rack:      sc.P02Rack03,
							Connections: []*apiv2.SwitchNicWithMachine{
								{
									Nic: &apiv2.SwitchNic{
										Name:       "swp1s0",
										Identifier: "swp1s0",
										BgpFilter:  &apiv2.BGPFilter{},
										State: &apiv2.NicState{
											Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
										},
									},
									Machine: dc.GetMachines()[sc.Machine7],
									Fru: &apiv2.MachineFRU{
										ChassisPartNumber:   new(string),
										ChassisPartSerial:   new(string),
										BoardMfg:            new(string),
										BoardMfgSerial:      new(string),
										BoardPartNumber:     new(string),
										ProductManufacturer: new(string),
										ProductPartNumber:   new(string),
										ProductSerial:       new("PS7"),
									},
								},
							},
						},
					},
				}
			},
		},
		{
			name: "query by switch id",
			rq: &adminv2.SwitchServiceConnectedMachinesRequest{
				Query: &apiv2.SwitchQuery{
					Id: new(sc.P01Rack01Switch2),
				},
			},
			want: func(dc *test.Datacenter) *adminv2.SwitchServiceConnectedMachinesResponse {
				return &adminv2.SwitchServiceConnectedMachinesResponse{
					SwitchesWithMachines: []*apiv2.SwitchWithMachines{
						{
							Id:        sc.P01Rack01Switch2,
							Partition: sc.Partition1,
							Rack:      sc.P01Rack01,
							Connections: []*apiv2.SwitchNicWithMachine{
								{
									Nic: &apiv2.SwitchNic{
										Name:       "Ethernet0",
										Identifier: "Ethernet0",
										BgpFilter:  &apiv2.BGPFilter{},
										State: &apiv2.NicState{
											Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
										},
									},
									Machine: dc.GetMachines()[sc.Machine1],
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
				}
			},
		},
		{
			name: "query by machine id",
			rq: &adminv2.SwitchServiceConnectedMachinesRequest{
				MachineQuery: &apiv2.MachineQuery{
					Uuid: new(sc.Machine5),
				},
			},
			want: func(dc *test.Datacenter) *adminv2.SwitchServiceConnectedMachinesResponse {
				return &adminv2.SwitchServiceConnectedMachinesResponse{
					SwitchesWithMachines: []*apiv2.SwitchWithMachines{
						{
							Id:        sc.P02Rack02Switch1,
							Partition: sc.Partition2,
							Rack:      sc.P02Rack02,
							Connections: []*apiv2.SwitchNicWithMachine{
								{
									Nic: &apiv2.SwitchNic{
										Name:       "Ethernet1",
										Identifier: "Ethernet1",
										BgpFilter:  &apiv2.BGPFilter{},
										State: &apiv2.NicState{
											Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
										},
									},
									Machine: dc.GetMachines()[sc.Machine5],
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
							Id:        sc.P02Rack02Switch2,
							Partition: sc.Partition2,
							Rack:      sc.P02Rack02,
							Connections: []*apiv2.SwitchNicWithMachine{
								{
									Nic: &apiv2.SwitchNic{
										Name:       "Ethernet1",
										Identifier: "Ethernet1",
										BgpFilter:  &apiv2.BGPFilter{},
										State: &apiv2.NicState{
											Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
										},
									},
									Machine: dc.GetMachines()[sc.Machine5],
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
				}
			},
		},
		{
			name: "query by partition",
			rq: &adminv2.SwitchServiceConnectedMachinesRequest{
				Query: &apiv2.SwitchQuery{
					Partition: new(sc.Partition1),
				},
				MachineQuery: &apiv2.MachineQuery{},
			},
			want: func(dc *test.Datacenter) *adminv2.SwitchServiceConnectedMachinesResponse {
				return &adminv2.SwitchServiceConnectedMachinesResponse{
					SwitchesWithMachines: []*apiv2.SwitchWithMachines{
						{
							Id:        sc.P01Rack01Switch1,
							Partition: sc.Partition1,
							Rack:      sc.P01Rack01,
							Connections: []*apiv2.SwitchNicWithMachine{
								{
									Nic: &apiv2.SwitchNic{
										Name:       "Ethernet0",
										Identifier: "Ethernet0",
										BgpFilter:  &apiv2.BGPFilter{},
										State: &apiv2.NicState{
											Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
										},
									},
									Machine: dc.GetMachines()[sc.Machine1],
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
							Id:        sc.P01Rack01Switch2,
							Partition: sc.Partition1,
							Rack:      sc.P01Rack01,
							Connections: []*apiv2.SwitchNicWithMachine{
								{
									Nic: &apiv2.SwitchNic{
										Name:       "Ethernet0",
										Identifier: "Ethernet0",
										BgpFilter:  &apiv2.BGPFilter{},
										State: &apiv2.NicState{
											Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
										},
									},
									Machine: dc.GetMachines()[sc.Machine1],
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
							Id:        sc.P01Rack01Switch2_1,
							Partition: sc.Partition1,
							Rack:      sc.P01Rack01,
						},
						{
							Id:        sc.P01Rack02Switch1,
							Partition: sc.Partition1,
							Rack:      sc.P01Rack02,
							Connections: []*apiv2.SwitchNicWithMachine{
								{
									Nic: &apiv2.SwitchNic{
										Name:       "swp1s0",
										Identifier: "swp1s0",
										BgpFilter:  &apiv2.BGPFilter{},
										State: &apiv2.NicState{
											Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
										},
									},
									Machine: dc.GetMachines()[sc.Machine2],
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
							Id:        sc.P01Rack02Switch1_1,
							Partition: sc.Partition1,
							Rack:      sc.P01Rack02,
						},
						{
							Id:        sc.P01Rack02Switch2,
							Partition: sc.Partition1,
							Rack:      sc.P01Rack02,
							Connections: []*apiv2.SwitchNicWithMachine{
								{
									Nic: &apiv2.SwitchNic{
										Name:       "Ethernet0",
										Identifier: "Ethernet0",
										BgpFilter:  &apiv2.BGPFilter{},
										State: &apiv2.NicState{
											Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
										},
									},
									Machine: dc.GetMachines()[sc.Machine2],
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
							Id:        sc.P01Rack03Switch1,
							Partition: sc.Partition1,
							Rack:      sc.P01Rack03,
						},
						{
							Id:        sc.P01Rack03Switch2,
							Partition: sc.Partition1,
							Rack:      sc.P01Rack03,
						},
						{
							Id:        sc.P01Rack04Switch1,
							Partition: sc.Partition1,
							Rack:      sc.P01Rack04,
						},
					},
				}
			},
		},
	}

	dc := test.NewDatacenter(t, log)
	defer dc.Close()
	dc.Create(&sc.SwitchesWithMachinesDatacenter)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &switchServiceServer{
				log:  log,
				repo: dc.GetTestStore().Store,
			}

			test.Validate(t, tt.rq)
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
