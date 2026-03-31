package admin

import (
	"log/slog"
	"os"
	"slices"
	"strings"
	"testing"

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
)

func Test_switchServiceServer_Get(t *testing.T) {
	t.Parallel()

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	ctx := t.Context()

	dc := test.NewDatacenter(t, log)
	defer dc.Close()
	dc.Create(&sc.SwitchesWithMachinesDatacenter)

	tests := []struct {
		name    string
		rq      *adminv2.SwitchServiceGetRequest
		want    *adminv2.SwitchServiceGetResponse
		wantErr error
	}{
		{
			name: "get existing",
			rq: &adminv2.SwitchServiceGetRequest{
				Id: dc.GetSwitches()[sc.P01Rack01Switch1].Id,
			},
			want: &adminv2.SwitchServiceGetResponse{
				Switch: dc.GetSwitches()[sc.P01Rack01Switch1],
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
				repo: dc.GetTestStore().Store,
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
	t.Parallel()

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	ctx := t.Context()

	dc := test.NewDatacenter(t, log)
	defer dc.Close()
	dc.Create(&sc.SwitchesWithMachinesDatacenter)

	tests := []struct {
		name     string
		rq       *adminv2.SwitchServiceListRequest
		wantFunc func() *adminv2.SwitchServiceListResponse
	}{
		{
			name: "get all",
			rq:   &adminv2.SwitchServiceListRequest{},
			wantFunc: func() *adminv2.SwitchServiceListResponse {
				switches := lo.Values(dc.GetSwitches())
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
			wantFunc: func() *adminv2.SwitchServiceListResponse {
				return &adminv2.SwitchServiceListResponse{
					Switches: []*apiv2.Switch{dc.GetSwitches()[sc.P01Rack01Switch1], dc.GetSwitches()[sc.P01Rack01Switch2]},
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
			wantFunc: func() *adminv2.SwitchServiceListResponse {
				return &adminv2.SwitchServiceListResponse{
					Switches: []*apiv2.Switch{dc.GetSwitches()[sc.P01Rack01Switch1]},
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
			wantFunc: func() *adminv2.SwitchServiceListResponse {
				return &adminv2.SwitchServiceListResponse{
					Switches: []*apiv2.Switch{
						dc.GetSwitches()[sc.P02Rack01Switch1],
						dc.GetSwitches()[sc.P02Rack01Switch2],
						dc.GetSwitches()[sc.P02Rack02Switch1],
						dc.GetSwitches()[sc.P02Rack02Switch2],
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
			wantFunc: func() *adminv2.SwitchServiceListResponse {
				return &adminv2.SwitchServiceListResponse{
					Switches: []*apiv2.Switch{
						dc.GetSwitches()[sc.P01Rack03Switch2],
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
			wantFunc: func() *adminv2.SwitchServiceListResponse {
				return &adminv2.SwitchServiceListResponse{
					Switches: []*apiv2.Switch{
						dc.GetSwitches()[sc.P01Rack03Switch1],
					},
				}
			},
		},
	}
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
			if tt.wantFunc != nil {
				want = tt.wantFunc()
			}

			if diff := cmp.Diff(want, got,
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
	t.Parallel()

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	ctx := t.Context()

	dc := test.NewDatacenter(t, log)
	defer dc.Close()

	tests := []struct {
		name    string
		rq      func() *adminv2.SwitchServiceUpdateRequest
		want    func() *adminv2.SwitchServiceUpdateResponse
		wantErr error
	}{
		{
			name: "no updates made",
			rq: func() *adminv2.SwitchServiceUpdateRequest {
				return &adminv2.SwitchServiceUpdateRequest{
					Id: dc.GetSwitches()[sc.P01Rack01Switch1].Id,
					UpdateMeta: &apiv2.UpdateMeta{
						UpdatedAt: dc.GetSwitches()[sc.P01Rack01Switch1].Meta.UpdatedAt,
					},
				}
			},
			want: func() *adminv2.SwitchServiceUpdateResponse {
				return &adminv2.SwitchServiceUpdateResponse{
					Switch: dc.GetSwitches()[sc.P01Rack01Switch1],
				}
			},
			wantErr: nil,
		},
		// {
		// 	name: "update all valid fields",
		// 	rq: &adminv2.SwitchServiceUpdateRequest{
		// 		// Id: sw1.Switch.Id,
		// 		// UpdateMeta: &apiv2.UpdateMeta{
		// 		// 	UpdatedAt: switchMap["sw1"].Meta.UpdatedAt,
		// 		// },
		// 		Description:    new("new description"),
		// 		ReplaceMode:    new(apiv2.SwitchReplaceMode_SWITCH_REPLACE_MODE_REPLACE),
		// 		ManagementIp:   new("1.1.1.5"),
		// 		ManagementUser: new("metal"),
		// 		ConsoleCommand: new("ssh"),
		// 		Nics: []*apiv2.SwitchNic{
		// 			{
		// 				Name:       "Ethernet0",
		// 				Identifier: "Eth1/1",
		// 				Mac:        "11:11:11:11:11:11",
		// 				Vrf:        new("Vrf100"),
		// 				BgpFilter:  &apiv2.BGPFilter{},
		// 				State: &apiv2.NicState{
		// 					Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
		// 				},
		// 				BgpPortState: &apiv2.SwitchBGPPortState{
		// 					Neighbor:  "Ethernet1",
		// 					PeerGroup: "external",
		// 					VrfName:   "Vrf200",
		// 					BgpState:  apiv2.BGPState_BGP_STATE_ESTABLISHED,
		// 					// BgpTimerUpEstablished: timestamppb.New(time.Unix(now.Unix(), 0)),
		// 					SentPrefixCounter:     0,
		// 					AcceptedPrefixCounter: 0,
		// 				},
		// 			},
		// 			{
		// 				Name:       "Ethernet2",
		// 				Identifier: "Eth/1/3",
		// 				Mac:        "aa:aa:aa:aa:aa:aa",
		// 				Vrf:        nil,
		// 				BgpFilter:  &apiv2.BGPFilter{},
		// 				State: &apiv2.NicState{
		// 					Desired: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP.Enum(),
		// 					Actual:  apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
		// 				},
		// 			},
		// 		},
		// 		Os: &apiv2.SwitchOS{
		// 			Vendor:           apiv2.SwitchOSVendor_SWITCH_OS_VENDOR_SONIC,
		// 			Version:          "ec202211",
		// 			MetalCoreVersion: "v0.14.0",
		// 		},
		// 	},
		// 	want: &adminv2.SwitchServiceUpdateResponse{
		// 		Switch: &apiv2.Switch{
		// 			// Id: sw1.Switch.Id,
		// 			Meta: &apiv2.Meta{
		// 				Generation: 1,
		// 			},
		// 			Description:    "new description",
		// 			Rack:           new("rack01"),
		// 			Partition:      "partition-a",
		// 			ReplaceMode:    apiv2.SwitchReplaceMode_SWITCH_REPLACE_MODE_REPLACE,
		// 			ManagementIp:   "1.1.1.5",
		// 			ManagementUser: new("metal"),
		// 			MachineConnections: []*apiv2.MachineConnection{
		// 				{
		// 					// MachineId: m1.ID,
		// 					Nic: &apiv2.SwitchNic{
		// 						Name:       "Ethernet0",
		// 						Identifier: "Eth1/1",
		// 						Mac:        "11:11:11:11:11:11",
		// 						Vrf:        new("Vrf100"),
		// 						BgpFilter:  &apiv2.BGPFilter{},
		// 						State: &apiv2.NicState{
		// 							Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
		// 						},
		// 						BgpPortState: &apiv2.SwitchBGPPortState{
		// 							Neighbor:  "Ethernet1",
		// 							PeerGroup: "external",
		// 							VrfName:   "Vrf200",
		// 							BgpState:  apiv2.BGPState_BGP_STATE_ESTABLISHED,
		// 							// BgpTimerUpEstablished: timestamppb.New(time.Unix(now.Unix(), 0)),
		// 							SentPrefixCounter:     0,
		// 							AcceptedPrefixCounter: 0,
		// 						},
		// 					},
		// 				},
		// 			},
		// 			ConsoleCommand: new("ssh"),
		// 			Nics: []*apiv2.SwitchNic{
		// 				{
		// 					Name:       "Ethernet0",
		// 					Identifier: "Eth1/1",
		// 					Mac:        "11:11:11:11:11:11",
		// 					Vrf:        new("Vrf100"),
		// 					BgpFilter:  &apiv2.BGPFilter{},
		// 					State: &apiv2.NicState{
		// 						Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
		// 					},
		// 					BgpPortState: &apiv2.SwitchBGPPortState{
		// 						Neighbor:  "Ethernet1",
		// 						PeerGroup: "external",
		// 						VrfName:   "Vrf200",
		// 						BgpState:  apiv2.BGPState_BGP_STATE_ESTABLISHED,
		// 						// BgpTimerUpEstablished: timestamppb.New(time.Unix(now.Unix(), 0)),
		// 						SentPrefixCounter:     0,
		// 						AcceptedPrefixCounter: 0,
		// 					},
		// 				},
		// 				{
		// 					Name:       "Ethernet2",
		// 					Identifier: "Eth/1/3",
		// 					Mac:        "aa:aa:aa:aa:aa:aa",
		// 					Vrf:        nil,
		// 					State: &apiv2.NicState{
		// 						Desired: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP.Enum(),
		// 						Actual:  apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
		// 					},
		// 					BgpFilter: &apiv2.BGPFilter{},
		// 				},
		// 			},
		// 			Os: &apiv2.SwitchOS{
		// 				Vendor:           apiv2.SwitchOSVendor_SWITCH_OS_VENDOR_SONIC,
		// 				Version:          "ec202211",
		// 				MetalCoreVersion: "v0.14.0",
		// 			},
		// 		},
		// 	},
		// 	wantErr: nil,
		// },
		// {
		// 	name: "cannot update os vendor",
		// 	rq: &adminv2.SwitchServiceUpdateRequest{
		// 		// Id: sw2.Switch.Id,
		// 		Os: &apiv2.SwitchOS{
		// 			Vendor: apiv2.SwitchOSVendor_SWITCH_OS_VENDOR_SONIC,
		// 		},
		// 	},
		// 	want:    nil,
		// 	wantErr: errorutil.InvalidArgument("cannot update switch os vendor from Cumulus to SONiC, use replace instead"),
		// },
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dc.Create(&sc.SwitchesWithMachinesDatacenter)
			defer dc.Cleanup()

			var (
				rq   *adminv2.SwitchServiceUpdateRequest
				want *adminv2.SwitchServiceUpdateResponse
			)

			if tt.rq != nil {
				rq = tt.rq()
			}
			if tt.want != nil {
				want = tt.want()
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

			// TODO: add dc.Assert
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
				Id: sc.P01Rack03Switch1,
			},
			want: func() *adminv2.SwitchServiceDeleteResponse {
				return &adminv2.SwitchServiceDeleteResponse{
					Switch: dc.GetSwitches()[sc.P01Rack03Switch1],
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
			want: func() *adminv2.SwitchServiceDeleteResponse {
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
			want: func() *adminv2.SwitchServiceDeleteResponse {
				return &adminv2.SwitchServiceDeleteResponse{
					Switch: dc.GetSwitches()[sc.P01Rack02Switch1],
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
	t.Parallel()

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	ctx := t.Context()

	dc := test.NewDatacenter(t, log)
	defer dc.Close()

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
				Status: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
				// Id:      sw1.Switch.Id,
				NicName: "Ethernet100",
			},
			want:    nil,
			wantErr: errorutil.InvalidArgument("port Ethernet100 does not exist on switch sw1"),
		},
		{
			name: "nic is not connected to a machine",
			rq: &adminv2.SwitchServicePortRequest{
				Status: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
				// Id:      sw1.Switch.Id,
				NicName: "Ethernet1",
			},
			want:    nil,
			wantErr: errorutil.FailedPrecondition("port Ethernet1 is not connected to any machine"),
		},
		{
			name: "nic update successful",
			rq: &adminv2.SwitchServicePortRequest{
				Status: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_DOWN,
				// Id:      sw1.Switch.Id,
				NicName: "Ethernet0",
			},
			want: &adminv2.SwitchServicePortResponse{
				Switch: &apiv2.Switch{
					// Id:             sw1.Switch.Id,
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
							// MachineId: m1.ID,
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
									Neighbor:  "Ethernet1",
									PeerGroup: "external",
									VrfName:   "Vrf200",
									BgpState:  apiv2.BGPState_BGP_STATE_CONNECT,
									// BgpTimerUpEstablished: timestamppb.New(time.Unix(now.Unix(), 0)),
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
								Neighbor:  "Ethernet1",
								PeerGroup: "external",
								VrfName:   "Vrf200",
								BgpState:  apiv2.BGPState_BGP_STATE_CONNECT,
								// BgpTimerUpEstablished: timestamppb.New(time.Unix(now.Unix(), 0)),
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
								Neighbor:  "Ethernet2",
								PeerGroup: "external",
								VrfName:   "Vrf200",
								BgpState:  apiv2.BGPState_BGP_STATE_CONNECT,
								// BgpTimerUpEstablished: timestamppb.New(time.Unix(now.Unix(), 0)),
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
			dc.Create(&sc.SwitchesWithMachinesDatacenter)
			defer dc.Cleanup()

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
	t.Parallel()

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	ctx := t.Context()

	dc := test.NewDatacenter(t, log)
	defer dc.Close()

	tests := []struct {
		name    string
		rq      *adminv2.SwitchServiceMigrateRequest
		want    *adminv2.SwitchServiceMigrateResponse
		wantErr error
	}{
		{
			name: "cannot migrate from one rack to another",
			rq:   &adminv2.SwitchServiceMigrateRequest{
				// OldSwitch: sw4.Switch.Id,
				// NewSwitch: sw401.Switch.Id,
			},
			want: nil,
			// wantErr: errorutil.FailedPrecondition("cannot migrate from switch %s in rack %s to switch %s in rack %s, switches must be in the same rack", sw4.Switch.Id, *sw4.Switch.Rack, sw401.Switch.Id, *sw401.Switch.Rack),
		},
		{
			name: "cannot migrate to switch that already has connections",
			rq:   &adminv2.SwitchServiceMigrateRequest{
				// OldSwitch: sw4.Switch.Id,
				// NewSwitch: sw402.Switch.Id,
			},
			want: nil,
			// wantErr: errorutil.FailedPrecondition("cannot migrate from switch %s to switch %s because the new switch already has machine connections", sw4.Switch.Id, sw402.Switch.Id),
		},
		{
			name: "migrate successfully",
			rq:   &adminv2.SwitchServiceMigrateRequest{
				// OldSwitch: sw5.Switch.Id,
				// NewSwitch: sw501.Switch.Id,
			},
			want: &adminv2.SwitchServiceMigrateResponse{
				Switch: &apiv2.Switch{
					// Id:          sw501.Switch.Id,
					Partition:   "partition-a",
					Rack:        new("r03"),
					ReplaceMode: apiv2.SwitchReplaceMode_SWITCH_REPLACE_MODE_OPERATIONAL,
					Meta:        &apiv2.Meta{Generation: 1},
					MachineConnections: []*apiv2.MachineConnection{
						{
							// MachineId: m1.ID,
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
			dc.Create(&sc.SwitchesWithMachinesDatacenter)
			defer dc.Cleanup()

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
						Id:        sc.P01Rack01Switch1,
						Partition: sc.Partition1,
						Rack:      sc.P01Rack01,
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
									Identifier: "Eth1/1",
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
						Id:        sc.P01Rack02Switch1,
						Partition: sc.Partition1,
						Rack:      sc.P01Rack02,
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
						Id:        sc.P01Rack02Switch2,
						Partition: sc.Partition1,
						Rack:      sc.P01Rack02,
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
						Id:        sc.P02Rack01Switch1,
						Partition: sc.Partition2,
						Rack:      sc.P01Rack01,
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
						Rack:      sc.P01Rack01,
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
						Id:        sc.P02Rack02Switch1,
						Partition: sc.Partition2,
						Rack:      sc.P01Rack02,
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
									Identifier: "Eth2/2",
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
						Rack:      sc.P01Rack02,
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
									Identifier: "Eth2/2",
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
			},
		},
		{
			name: "query by switch id",
			rq: &adminv2.SwitchServiceConnectedMachinesRequest{
				Query: &apiv2.SwitchQuery{
					Id: new(sc.P01Rack01Switch2),
				},
			},
			want: &adminv2.SwitchServiceConnectedMachinesResponse{
				SwitchesWithMachines: []*apiv2.SwitchWithMachines{
					{
						Id:        sc.P01Rack01Switch2,
						Partition: sc.Partition1,
						Rack:      sc.P01Rack01,
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
						Id:        sc.P02Rack02Switch1,
						Partition: sc.Partition2,
						Rack:      sc.P01Rack02,
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
						Rack:      sc.P01Rack02,
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
			want: &adminv2.SwitchServiceConnectedMachinesResponse{
				SwitchesWithMachines: []*apiv2.SwitchWithMachines{
					{
						Id:        sc.P01Rack01Switch1,
						Partition: sc.Partition1,
						Rack:      sc.P01Rack01,
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
									Identifier: "Eth1/1",
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
						Id:        sc.P01Rack02Switch1,
						Partition: sc.Partition1,
						Rack:      sc.P01Rack02,
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
						Id:        sc.P01Rack02Switch2,
						Partition: sc.Partition1,
						Rack:      sc.P01Rack02,
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
