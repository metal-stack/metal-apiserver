package infra

import (
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	infrav2 "github.com/metal-stack/api/go/metalstack/infra/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	"github.com/metal-stack/metal-apiserver/pkg/test"
	sc "github.com/metal-stack/metal-apiserver/pkg/test/scenarios"
	"github.com/samber/lo"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func Test_switchServiceServer_Register(t *testing.T) {
	t.Parallel()

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	ctx := t.Context()

	dc := test.NewDatacenter(t, log)
	defer dc.Close()

	tests := []struct {
		name    string
		rq      func() *infrav2.SwitchServiceRegisterRequest
		want    func() *infrav2.SwitchServiceRegisterResponse
		mods    func() *test.Asserters
		wantErr error
	}{
		{
			name: "register new switch",
			rq: func() *infrav2.SwitchServiceRegisterRequest {
				return &infrav2.SwitchServiceRegisterRequest{
					Switch: &apiv2.Switch{
						Id:           "p01-r01leaf01-1",
						Rack:         new(sc.P01Rack01),
						Partition:    sc.Partition1,
						ManagementIp: "1.1.1.1",
						ReplaceMode:  apiv2.SwitchReplaceMode_SWITCH_REPLACE_MODE_OPERATIONAL,
						Os: &apiv2.SwitchOS{
							Vendor:           apiv2.SwitchOSVendor_SWITCH_OS_VENDOR_CUMULUS,
							Version:          "v5.9",
							MetalCoreVersion: "v0.13.0",
						},
					},
				}
			},
			want: func() *infrav2.SwitchServiceRegisterResponse {
				return &infrav2.SwitchServiceRegisterResponse{
					Switch: &apiv2.Switch{
						Id:           "p01-r01leaf01-1",
						Meta:         &apiv2.Meta{},
						Rack:         new(sc.P01Rack01),
						Partition:    sc.Partition1,
						ManagementIp: "1.1.1.1",
						ReplaceMode:  apiv2.SwitchReplaceMode_SWITCH_REPLACE_MODE_OPERATIONAL,
						Os: &apiv2.SwitchOS{
							Vendor:           apiv2.SwitchOSVendor_SWITCH_OS_VENDOR_CUMULUS,
							Version:          "v5.9",
							MetalCoreVersion: "v0.13.0",
						},
					},
				}
			},
			mods: func() *test.Asserters {
				return &test.Asserters{
					Switches: func(switches map[string]*apiv2.Switch) {
						switches["p01-r01leaf01-1"] = &apiv2.Switch{
							Id:           "p01-r01leaf01-1",
							Meta:         &apiv2.Meta{},
							Rack:         new(sc.P01Rack01),
							Partition:    sc.Partition1,
							ManagementIp: "1.1.1.1",
							ReplaceMode:  apiv2.SwitchReplaceMode_SWITCH_REPLACE_MODE_OPERATIONAL,
							Os: &apiv2.SwitchOS{
								Vendor:           apiv2.SwitchOSVendor_SWITCH_OS_VENDOR_CUMULUS,
								Version:          "v5.9",
								MetalCoreVersion: "v0.13.0",
							},
						}
					},
					SwitchStatuses: func(switchStatuses map[string]*metal.SwitchStatus) {
						switchStatuses["p01-r01leaf01-1"] = &metal.SwitchStatus{
							Base: metal.Base{
								ID: "p01-r01leaf01-1",
							},
						}
					},
				}
			},
			wantErr: nil,
		},
		{
			name: "registering existing operational switch updates the switch",
			rq: func() *infrav2.SwitchServiceRegisterRequest {
				return &infrav2.SwitchServiceRegisterRequest{
					Switch: &apiv2.Switch{
						Id:             sc.P01Rack01Switch1,
						Description:    "new description",
						Partition:      sc.Partition1,
						ReplaceMode:    apiv2.SwitchReplaceMode_SWITCH_REPLACE_MODE_OPERATIONAL,
						ManagementIp:   "1.1.1.1",
						ManagementUser: new("admin"),
						ConsoleCommand: new("tty"),
						Os: &apiv2.SwitchOS{
							Vendor:           apiv2.SwitchOSVendor_SWITCH_OS_VENDOR_SONIC,
							Version:          sc.SwitchOSSonic2021.Version,
							MetalCoreVersion: "v0.13.0",
						},
						Nics: []*apiv2.SwitchNic{
							{
								Name:       "Ethernet0",
								Identifier: "Ethernet0",
								Mac:        "11:11:11:11:11:11", // MAC does not get updated but is necessary for the validation to pass
							},
							{
								Name:       "Ethernet2", // doesn't make sense; just testing whether port names are updated
								Identifier: "Ethernet1",
								Mac:        "22:22:22:22:22:22",
								State: &apiv2.NicState{
									Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
								},
								Vrf: new("must not be updated"),
							},
						},
					},
				}
			},
			want: func() *infrav2.SwitchServiceRegisterResponse {
				sw := dc.GetSwitches()[sc.P01Rack01Switch1]
				sw.Description = "new description"
				sw.ManagementIp = "1.1.1.1"
				sw.ManagementUser = new("admin")
				sw.ConsoleCommand = new("tty")
				sw.ReplaceMode = apiv2.SwitchReplaceMode_SWITCH_REPLACE_MODE_OPERATIONAL
				sw.Nics = []*apiv2.SwitchNic{
					{
						Name:       "Ethernet0",
						Identifier: "Ethernet0",
						BgpFilter:  &apiv2.BGPFilter{},
						State: &apiv2.NicState{
							Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
						},
					},
					{
						Name:       "Ethernet2",
						Identifier: "Ethernet1",
						BgpFilter:  &apiv2.BGPFilter{},
						State: &apiv2.NicState{
							Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
						},
					},
				}
				sw.Os.MetalCoreVersion = "v0.13.0"

				return &infrav2.SwitchServiceRegisterResponse{
					Switch: sw,
				}
			},
			mods: func() *test.Asserters {
				return &test.Asserters{
					Switches: func(switches map[string]*apiv2.Switch) {
						sw := switches[sc.P01Rack01Switch1]
						sw.Description = "new description"
						sw.ManagementIp = "1.1.1.1"
						sw.ManagementUser = new("admin")
						sw.ConsoleCommand = new("tty")
						sw.ReplaceMode = apiv2.SwitchReplaceMode_SWITCH_REPLACE_MODE_OPERATIONAL
						sw.Nics = []*apiv2.SwitchNic{
							{
								Name:       "Ethernet0",
								Identifier: "Ethernet0",
								BgpFilter:  &apiv2.BGPFilter{},
								State: &apiv2.NicState{
									Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
								},
							},
							{
								Name:       "Ethernet2",
								Identifier: "Ethernet1",
								BgpFilter:  &apiv2.BGPFilter{},
								State: &apiv2.NicState{
									Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
								},
							},
						}
						sw.Os.MetalCoreVersion = "v0.13.0"
					},
				}
			},
			wantErr: nil,
		},
		{
			name: "try replace but no switches found in the rack",
			rq: func() *infrav2.SwitchServiceRegisterRequest {
				return &infrav2.SwitchServiceRegisterRequest{
					Switch: &apiv2.Switch{
						Id:           sc.P01Rack04Switch1,
						Rack:         new("p01-rack05"),
						Partition:    sc.Partition1,
						ManagementIp: "1.1.1.1",
						ReplaceMode:  apiv2.SwitchReplaceMode_SWITCH_REPLACE_MODE_OPERATIONAL,
						Os: &apiv2.SwitchOS{
							Vendor:           apiv2.SwitchOSVendor_SWITCH_OS_VENDOR_SONIC,
							Version:          "ec202211",
							MetalCoreVersion: "v0.14.0",
						},
					},
				}
			},
			want:    nil,
			wantErr: errorutil.NotFound("failed to determine twin for switch %s: could not find any switch in rack p01-rack05", sc.P01Rack04Switch1),
		},
		{
			name: "try replace but no twin switch found",
			rq: func() *infrav2.SwitchServiceRegisterRequest {
				return &infrav2.SwitchServiceRegisterRequest{
					Switch: &apiv2.Switch{
						Id:           sc.P01Rack04Switch1,
						Rack:         new(sc.P01Rack04),
						Partition:    sc.Partition1,
						ManagementIp: "1.1.1.1",
						ReplaceMode:  apiv2.SwitchReplaceMode_SWITCH_REPLACE_MODE_OPERATIONAL,
						Os: &apiv2.SwitchOS{
							Vendor:           apiv2.SwitchOSVendor_SWITCH_OS_VENDOR_SONIC,
							Version:          "ec202211",
							MetalCoreVersion: "v0.14.0",
						},
					},
				}
			},
			want:    nil,
			wantErr: errorutil.NotFound("failed to determine twin for switch %s: no twin found for switch %s in partition %v and rack %v", sc.P01Rack04Switch1, sc.P01Rack04Switch1, sc.Partition1, sc.P01Rack04),
		},
		{
			name: "try replace but multiple potential twins exist",
			rq: func() *infrav2.SwitchServiceRegisterRequest {
				return &infrav2.SwitchServiceRegisterRequest{
					Switch: &apiv2.Switch{
						Id:           sc.P01Rack01Switch2_1,
						Rack:         new(sc.P01Rack01),
						Partition:    sc.Partition1,
						ManagementIp: "1.1.1.1",
						ReplaceMode:  apiv2.SwitchReplaceMode_SWITCH_REPLACE_MODE_OPERATIONAL,
						Os: &apiv2.SwitchOS{
							Vendor:           apiv2.SwitchOSVendor_SWITCH_OS_VENDOR_SONIC,
							Version:          "ec202211",
							MetalCoreVersion: "v0.14.0",
						},
					},
				}
			},
			want:    nil,
			wantErr: errorutil.FailedPrecondition("failed to determine twin for switch %s: found multiple twin switches for %s (%s and %s)", sc.P01Rack01Switch2_1, sc.P01Rack01Switch2_1, sc.P01Rack01Switch2, sc.P01Rack01Switch1),
		},
		{
			name: "successful replacement",
			rq: func() *infrav2.SwitchServiceRegisterRequest {
				return &infrav2.SwitchServiceRegisterRequest{
					Switch: &apiv2.Switch{
						Id:           sc.P02Rack03Switch2,
						Partition:    sc.Partition2,
						Rack:         new(sc.P02Rack03),
						ManagementIp: "1.1.1.1",
						MachineConnections: []*apiv2.MachineConnection{
							{
								MachineId: sc.Machine7,
								Nic: &apiv2.SwitchNic{
									Name:       "Ethernet0",
									Identifier: "Ethernet0",
									Mac:        "11:11:11:11:11:11",
									State: &apiv2.NicState{
										Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
									},
								},
							},
						},
						Nics: []*apiv2.SwitchNic{
							{
								Name:       "Ethernet0",
								Identifier: "Ethernet0",
								Mac:        "11:11:11:11:11:11",
								State: &apiv2.NicState{
									Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
								},
							},
							{
								Name:       "Ethernet1",
								Identifier: "Ethernet1",
								Mac:        "22:22:22:22:22:22",
								State: &apiv2.NicState{
									Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
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
			},
			want: func() *infrav2.SwitchServiceRegisterResponse {
				return &infrav2.SwitchServiceRegisterResponse{
					Switch: &apiv2.Switch{
						Id:           sc.P02Rack03Switch2,
						Meta:         &apiv2.Meta{},
						Partition:    sc.Partition2,
						Rack:         new(sc.P02Rack03),
						ManagementIp: "1.1.1.1",
						ReplaceMode:  apiv2.SwitchReplaceMode_SWITCH_REPLACE_MODE_OPERATIONAL,
						MachineConnections: []*apiv2.MachineConnection{
							{
								MachineId: sc.Machine7,
								Nic: &apiv2.SwitchNic{
									Name:       "Ethernet0",
									Identifier: "Ethernet0",
									Mac:        "11:11:11:11:11:11",
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
								Identifier: "Ethernet0",
								Mac:        "11:11:11:11:11:11",
								BgpFilter:  &apiv2.BGPFilter{},
								State: &apiv2.NicState{
									Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
								},
							},
							{
								Name:       "Ethernet1",
								Identifier: "Ethernet1",
								Mac:        "22:22:22:22:22:22",
								BgpFilter:  &apiv2.BGPFilter{},
								State: &apiv2.NicState{
									Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
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
			},
			mods: func() *test.Asserters {
				return &test.Asserters{
					Switches: func(switches map[string]*apiv2.Switch) {
						sw := switches[sc.P02Rack03Switch2]
						sw.ManagementIp = "1.1.1.1"
						sw.ReplaceMode = apiv2.SwitchReplaceMode_SWITCH_REPLACE_MODE_OPERATIONAL
						nic1 := &apiv2.SwitchNic{
							Name:       "Ethernet0",
							Identifier: "Ethernet0",
							Mac:        "11:11:11:11:11:11",
							BgpFilter:  &apiv2.BGPFilter{},
							State: &apiv2.NicState{
								Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
							},
						}
						sw.MachineConnections = []*apiv2.MachineConnection{
							{
								MachineId: sc.Machine7,
								Nic:       nic1,
							},
						}
						sw.Nics = []*apiv2.SwitchNic{
							{
								Name:       "Ethernet0",
								Identifier: "Ethernet0",
								Mac:        "11:11:11:11:11:11",
								BgpFilter:  &apiv2.BGPFilter{},
								State: &apiv2.NicState{
									Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
								},
							},
							{
								Name:       "Ethernet1",
								Identifier: "Ethernet1",
								Mac:        "22:22:22:22:22:22",
								BgpFilter:  &apiv2.BGPFilter{},
								State: &apiv2.NicState{
									Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
								},
							},
						}
						sw.Os = &apiv2.SwitchOS{
							Vendor:           apiv2.SwitchOSVendor_SWITCH_OS_VENDOR_SONIC,
							Version:          "ec202211",
							MetalCoreVersion: "v0.13.0",
						}
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

			var (
				rq       *infrav2.SwitchServiceRegisterRequest
				want     *infrav2.SwitchServiceRegisterResponse
				snapshot = dc.Snapshot()
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

			got, err := s.Register(ctx, rq)
			if diff := cmp.Diff(tt.wantErr, err, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("switchServiceServer.Register() error diff = %s", diff)
			}
			if diff := cmp.Diff(want, got,
				protocmp.Transform(),
				protocmp.IgnoreFields(
					&apiv2.SwitchSync{}, "time", "duration",
				),
				protocmp.IgnoreFields(
					&apiv2.Meta{}, "created_at", "updated_at", "generation",
				)); diff != "" {
				t.Errorf("switchServiceServer.Register() diff = %s", diff)
			}

			var mods *test.Asserters
			if tt.mods != nil {
				mods = tt.mods()
			}
			err = dc.Assert(snapshot, mods)
			require.NoError(t, err)
		})
	}
}

func Test_switchServiceServer_Get(t *testing.T) {
	t.Parallel()

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	ctx := t.Context()

	dc := test.NewDatacenter(t, log)
	defer dc.Close()
	dc.Create(&sc.SwitchesWithMachinesDatacenter)
	snapshot := dc.Snapshot()

	tests := []struct {
		name    string
		rq      *infrav2.SwitchServiceGetRequest
		want    *infrav2.SwitchServiceGetResponse
		wantErr error
	}{
		{
			name: "get existing",
			rq: &infrav2.SwitchServiceGetRequest{
				Id: sc.P01Rack01Switch1,
			},
			want: &infrav2.SwitchServiceGetResponse{
				Switch: dc.GetSwitches()[sc.P01Rack01Switch1],
			},
			wantErr: nil,
		},
		{
			name: "get non-existing",
			rq: &infrav2.SwitchServiceGetRequest{
				Id: "sw50",
			},
			want:    nil,
			wantErr: errorutil.NotFound("no switch with id \"sw50\" found"),
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
			err = dc.Assert(snapshot, nil)
			require.NoError(t, err)
		})
	}
}

func Test_switchServiceServer_Heartbeat(t *testing.T) {
	t.Parallel()

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	ctx := t.Context()

	dc := test.NewDatacenter(t, log)
	defer dc.Close()

	tests := []struct {
		name    string
		rq      func() *infrav2.SwitchServiceHeartbeatRequest
		want    func() *infrav2.SwitchServiceHeartbeatResponse
		mods    func() *test.Asserters
		wantErr error
	}{
		{
			name: "switch status empty, no error, no change",
			rq: func() *infrav2.SwitchServiceHeartbeatRequest {
				return &infrav2.SwitchServiceHeartbeatRequest{
					Id:       sc.P01Rack01Switch1,
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
							BgpTimerUpEstablished: &timestamppb.Timestamp{},
							SentPrefixCounter:     0,
							AcceptedPrefixCounter: 0,
						},
					},
				}
			},
			want: func() *infrav2.SwitchServiceHeartbeatResponse {
				return &infrav2.SwitchServiceHeartbeatResponse{
					Id: sc.P01Rack01Switch1,
					LastSync: &apiv2.SwitchSync{
						Duration: durationpb.New(time.Second),
						Error:    nil,
					},
				}
			},
			mods: func() *test.Asserters {
				return &test.Asserters{
					Switches: func(switches map[string]*apiv2.Switch) {
						sw := switches[sc.P01Rack01Switch1]
						sw.LastSync = &apiv2.SwitchSync{
							Duration: durationpb.New(time.Second),
						}
						sw.LastSyncError = &apiv2.SwitchSync{
							Duration: &durationpb.Duration{},
						}
						nic, found := lo.Find(sw.Nics, func(n *apiv2.SwitchNic) bool {
							return n.Name == "Ethernet0"
						})
						require.True(t, found)
						nic.State.Actual = apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_DOWN
						nic, found = lo.Find(sw.Nics, func(n *apiv2.SwitchNic) bool {
							return n.Name == "Ethernet1"
						})
						require.True(t, found)
						nic.State.Actual = apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_DOWN
						nic.BgpPortState = &apiv2.SwitchBGPPortState{
							Neighbor:              "Ethernet2",
							PeerGroup:             "external",
							VrfName:               "Vrf200",
							BgpState:              apiv2.BGPState_BGP_STATE_CONNECT,
							BgpTimerUpEstablished: &timestamppb.Timestamp{},
							SentPrefixCounter:     0,
							AcceptedPrefixCounter: 0,
						}
						sw.MachineConnections[0].Nic.State.Actual = apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_DOWN
					},
					SwitchStatuses: func(switchStatuses map[string]*metal.SwitchStatus) {
						status := switchStatuses[sc.P01Rack01Switch1]
						status.LastSync = &metal.SwitchSync{
							Duration: time.Second,
						}
					},
				}
			},
			wantErr: nil,
		},
		// {
		// 	name: "switch status exists, error occurred, no change",
		// 	rq: &infrav2.SwitchServiceHeartbeatRequest{
		// 		Id:       sw1.Switch.Id,
		// 		Duration: durationpb.New(time.Second),
		// 		Error:    new("sync failed"),
		// 		PortStates: map[string]apiv2.SwitchPortStatus{
		// 			"Ethernet0": apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_DOWN,
		// 			"Ethernet1": apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_DOWN,
		// 		},
		// 		BgpPortStates: map[string]*apiv2.SwitchBGPPortState{
		// 			"Ethernet1": {
		// 				Neighbor:              "Ethernet2",
		// 				PeerGroup:             "external",
		// 				VrfName:               "Vrf200",
		// 				BgpState:              apiv2.BGPState_BGP_STATE_CONNECT,
		// 				BgpTimerUpEstablished: timestamppb.New(time.Unix(now.Unix(), 0)),
		// 				SentPrefixCounter:     0,
		// 				AcceptedPrefixCounter: 0,
		// 			},
		// 		},
		// 	},
		// 	want: &infrav2.SwitchServiceHeartbeatResponse{
		// 		Id: sw1.Switch.Id,
		// 		LastSync: &apiv2.SwitchSync{
		// 			Time:     timestamppb.New(now),
		// 			Duration: durationpb.New(time.Second),
		// 			Error:    nil,
		// 		},
		// 		LastSyncError: &apiv2.SwitchSync{
		// 			Duration: durationpb.New(time.Second),
		// 			Error:    new("sync failed"),
		// 		},
		// 	},
		// 	wantSwitch: sw1.Switch,
		// 	wantErr:    nil,
		// },
		// {
		// 	name: "error occurred, update anyway",
		// 	rq: &infrav2.SwitchServiceHeartbeatRequest{
		// 		Id:       sw2.Switch.Id,
		// 		Duration: durationpb.New(time.Second),
		// 		Error:    new("failed to sync"),
		// 		PortStates: map[string]apiv2.SwitchPortStatus{
		// 			"Ethernet0": apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
		// 			"Ethernet1": apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_DOWN,
		// 		},
		// 		BgpPortStates: map[string]*apiv2.SwitchBGPPortState{
		// 			"Ethernet1": {
		// 				Neighbor:              "Ethernet2",
		// 				PeerGroup:             "external",
		// 				VrfName:               "Vrf200",
		// 				BgpState:              apiv2.BGPState_BGP_STATE_ESTABLISHED,
		// 				BgpTimerUpEstablished: timestamppb.New(time.Unix(now.Unix(), 0)),
		// 				SentPrefixCounter:     0,
		// 				AcceptedPrefixCounter: 0,
		// 			},
		// 		},
		// 	},
		// 	want: &infrav2.SwitchServiceHeartbeatResponse{
		// 		Id: sw2.Switch.Id,
		// 		LastSync: &apiv2.SwitchSync{
		// 			Duration: durationpb.New(time.Second),
		// 			Error:    nil,
		// 		},
		// 		LastSyncError: &apiv2.SwitchSync{
		// 			Duration: durationpb.New(time.Second),
		// 			Error:    new("failed to sync"),
		// 		},
		// 	},
		// 	wantSwitch: &apiv2.Switch{
		// 		Id:          sw2.Switch.Id,
		// 		Partition:   "partition-a",
		// 		ReplaceMode: apiv2.SwitchReplaceMode_SWITCH_REPLACE_MODE_OPERATIONAL,
		// 		Meta:        &apiv2.Meta{Generation: 1},
		// 		Rack:        new("r02"),
		// 		Nics: []*apiv2.SwitchNic{
		// 			{
		// 				Name:       "Ethernet0",
		// 				Identifier: "Eth1/1",
		// 				BgpFilter:  &apiv2.BGPFilter{},
		// 				State: &apiv2.NicState{
		// 					Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
		// 				},
		// 			},
		// 			{
		// 				Name:       "Ethernet1",
		// 				Identifier: "Eth1/2",
		// 				BgpFilter:  &apiv2.BGPFilter{},
		// 				State: &apiv2.NicState{
		// 					Desired: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP.Enum(),
		// 					Actual:  apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_DOWN,
		// 				},
		// 				BgpPortState: &apiv2.SwitchBGPPortState{
		// 					Neighbor:              "Ethernet2",
		// 					PeerGroup:             "external",
		// 					VrfName:               "Vrf200",
		// 					BgpState:              apiv2.BGPState_BGP_STATE_ESTABLISHED,
		// 					BgpTimerUpEstablished: timestamppb.New(time.Unix(now.Unix(), 0)),
		// 					SentPrefixCounter:     0,
		// 					AcceptedPrefixCounter: 0,
		// 				},
		// 			},
		// 		},
		// 		Os: &apiv2.SwitchOS{
		// 			Vendor:           apiv2.SwitchOSVendor_SWITCH_OS_VENDOR_SONIC,
		// 			Version:          "ec202111",
		// 			MetalCoreVersion: "v0.13.0",
		// 		},
		// 	},
		// 	wantErr: nil,
		// },
		// {
		// 	name: "no error occurred",
		// 	rq: &infrav2.SwitchServiceHeartbeatRequest{
		// 		Id:       sw1.Switch.Id,
		// 		Duration: durationpb.New(2 * time.Second),
		// 		PortStates: map[string]apiv2.SwitchPortStatus{
		// 			"Ethernet0": apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
		// 			"Ethernet1": apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UNKNOWN,
		// 		},
		// 		BgpPortStates: map[string]*apiv2.SwitchBGPPortState{},
		// 	},
		// 	want: &infrav2.SwitchServiceHeartbeatResponse{
		// 		Id: sw1.Switch.Id,
		// 		LastSync: &apiv2.SwitchSync{
		// 			Duration: durationpb.New(2 * time.Second),
		// 			Error:    nil,
		// 		},
		// 		LastSyncError: &apiv2.SwitchSync{
		// 			Duration: durationpb.New(time.Second),
		// 			Error:    new("sync failed"),
		// 		},
		// 	},
		// 	wantSwitch: &apiv2.Switch{
		// 		Id:          sw1.Switch.Id,
		// 		Partition:   "partition-a",
		// 		ReplaceMode: apiv2.SwitchReplaceMode_SWITCH_REPLACE_MODE_OPERATIONAL,
		// 		Meta:        &apiv2.Meta{Generation: 1},
		// 		Rack:        new("r01"),
		// 		Nics: []*apiv2.SwitchNic{
		// 			{
		// 				Name:       "Ethernet0",
		// 				Identifier: "Eth1/1",
		// 				BgpFilter:  &apiv2.BGPFilter{},
		// 				State: &apiv2.NicState{
		// 					Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
		// 				},
		// 			},
		// 			{
		// 				Name:       "Ethernet1",
		// 				Identifier: "Eth1/2",
		// 				BgpFilter:  &apiv2.BGPFilter{},
		// 				State: &apiv2.NicState{
		// 					Desired: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP.Enum(),
		// 					Actual:  apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UNKNOWN,
		// 				},
		// 			},
		// 		},
		// 		Os: &apiv2.SwitchOS{
		// 			Vendor:           apiv2.SwitchOSVendor_SWITCH_OS_VENDOR_SONIC,
		// 			Version:          "ec202111",
		// 			MetalCoreVersion: "v0.13.0",
		// 		},
		// 	},
		// 	wantErr: nil,
		// },
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dc.Create(&sc.SwitchesWithMachinesDatacenter)
			defer dc.Cleanup()

			var (
				rq       *infrav2.SwitchServiceHeartbeatRequest
				want     *infrav2.SwitchServiceHeartbeatResponse
				snapshot = dc.Snapshot()
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

			got, err := s.Heartbeat(ctx, rq)
			if diff := cmp.Diff(tt.wantErr, err, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("switchServiceServer.Heartbeat() error diff = %s", diff)
				return
			}
			if diff := cmp.Diff(
				want, got,
				protocmp.Transform(),
				protocmp.IgnoreFields(
					&apiv2.SwitchSync{}, "time",
				),
			); diff != "" {
				t.Errorf("switchServiceServer.Heartbeat() diff = %v", diff)
			}

			var mods *test.Asserters
			if tt.mods != nil {
				mods = tt.mods()
			}
			err = dc.Assert(snapshot, mods,
				cmpopts.IgnoreFields(
					metal.SwitchSync{}, "Time",
				),
				protocmp.IgnoreFields(
					&timestamppb.Timestamp{}, "seconds", "nanos",
				),
				protocmp.IgnoreFields(
					&apiv2.SwitchSync{}, "time",
				),
			)
			require.NoError(t, err)
		})
	}
}

// added this test here because using testStore inside the repository package creates an import cycle
// func Test_switchRepository_ConnectMachineWithSwitches(t *testing.T) {
// 	t.Parallel()

// 	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
// 	ctx := t.Context()

// 	dc := test.NewDatacenter(t, log)
// 	defer dc.Close()
// 	dc.Create(&sc.SwitchesWithMachinesDatacenter)

// 	tests := []struct {
// 		name         string
// 		m            *apiv2.Machine
// 		wantSwitches []*apiv2.Switch
// 		wantErr      error
// 	}{
// 		{
// 			name: "partition id not given",
// 			m: &apiv2.Machine{
// 				Uuid: "m1",
// 			},
// 			wantErr: errorutil.InvalidArgument("partition id of machine m1 is empty"),
// 		},
// 		{
// 			name: "no hardware given",
// 			m: &apiv2.Machine{
// 				Uuid: "m1",
// 				Partition: &apiv2.Partition{
// 					Id: "partition-a",
// 				},
// 			},
// 			wantErr: errorutil.InvalidArgument("no hardware information for machine m1 given"),
// 		},
// 		{
// 			name: "machine is not connected",
// 			m: &apiv2.Machine{
// 				Uuid: "m1",
// 				Partition: &apiv2.Partition{
// 					Id: "partition-a",
// 				},
// 				Hardware: &apiv2.MachineHardware{
// 					Nics: []*apiv2.MachineNic{
// 						{
// 							Neighbors: []*apiv2.MachineNic{},
// 						},
// 					},
// 				},
// 			},
// 			wantErr: errorutil.FailedPrecondition("machine m1 is not connected to exactly two switches, found connections to switches []"),
// 		},
// 		{
// 			name: "machine is connected to three switches",
// 			m: &apiv2.Machine{
// 				Uuid: "m1",
// 				Partition: &apiv2.Partition{
// 					Id: "partition-a",
// 				},
// 				Hardware: &apiv2.MachineHardware{
// 					Nics: []*apiv2.MachineNic{
// 						{
// 							Neighbors: []*apiv2.MachineNic{
// 								{Hostname: "leaf01"},
// 								{Hostname: "leaf02"},
// 								{Hostname: "leaf01-1"},
// 							},
// 						},
// 					},
// 				},
// 			},
// 			wantErr: errorutil.FailedPrecondition("machine m1 is not connected to exactly two switches, found connections to switches [leaf01 leaf02 leaf01-1]"),
// 		},
// 		{
// 			name: "switches are in different racks",
// 			m: &apiv2.Machine{
// 				Uuid: "m1",
// 				Partition: &apiv2.Partition{
// 					Id: "partition-a",
// 				},
// 				Hardware: &apiv2.MachineHardware{
// 					Nics: []*apiv2.MachineNic{
// 						{
// 							Name: "lan0",
// 							Neighbors: []*apiv2.MachineNic{
// 								{
// 									Name:       "Ethernet0",
// 									Identifier: "Eth1/1",
// 									Hostname:   "sw1",
// 								},
// 							},
// 						},
// 						{
// 							Name: "lan1",
// 							Neighbors: []*apiv2.MachineNic{
// 								{
// 									Name:       "Ethernet0",
// 									Identifier: "Eth1/1",
// 									Hostname:   "sw2",
// 								},
// 							},
// 						},
// 					},
// 				},
// 			},
// 			wantErr: errorutil.FailedPrecondition("connected switches of a machine must reside in the same rack, rack of switch sw1: r01, rack of switch sw2: r02, machine: m1"),
// 		},
// 		{
// 			name: "different number of connections per switch",
// 			m: &apiv2.Machine{
// 				Uuid: "m1",
// 				Partition: &apiv2.Partition{
// 					Id: "partition-a",
// 				},
// 				Hardware: &apiv2.MachineHardware{
// 					Nics: []*apiv2.MachineNic{
// 						{
// 							Name: "lan0",
// 							Neighbors: []*apiv2.MachineNic{
// 								{
// 									Name:       "Ethernet0",
// 									Identifier: "Eth1/1",
// 									Hostname:   "sw6",
// 								},
// 							},
// 						},
// 						{
// 							Name: "lan1",
// 							Neighbors: []*apiv2.MachineNic{
// 								{
// 									Name:       "Ethernet1",
// 									Identifier: "Eth1/2",
// 									Hostname:   "sw6",
// 								},
// 							},
// 						},
// 						{
// 							Name: "lan2",
// 							Neighbors: []*apiv2.MachineNic{
// 								{
// 									Name:       "swp1s0",
// 									Identifier: "bb:bb:bb:bb:bb:bb",
// 									Hostname:   "sw5",
// 								},
// 							},
// 						},
// 					},
// 				},
// 			},
// 			wantErr: errorutil.FailedPrecondition("machine connections must be identical on both switches but machine m1 has 2 connections to switch sw6 and 1 connections to switch sw5"),
// 		},
// 		{
// 			name: "switch ports the machine is connected to do not match",
// 			m: &apiv2.Machine{
// 				Uuid: "m1",
// 				Partition: &apiv2.Partition{
// 					Id: "partition-a",
// 				},
// 				Hardware: &apiv2.MachineHardware{
// 					Nics: []*apiv2.MachineNic{
// 						{
// 							Name: "lan1",
// 							Neighbors: []*apiv2.MachineNic{
// 								{
// 									Name:       "swp1s1",
// 									Identifier: "bb:bb:bb:bb:bb:11",
// 									Hostname:   "sw5",
// 								},
// 							},
// 						},
// 						{
// 							Name: "lan0",
// 							Neighbors: []*apiv2.MachineNic{
// 								{
// 									Name:       "Ethernet0",
// 									Identifier: "Eth1/1",
// 									Hostname:   "sw6",
// 								},
// 							},
// 						},
// 					},
// 				},
// 			},
// 			wantErr: errorutil.FailedPrecondition("machine m1 is connected to port swp1s1 on switch sw5 but not to the corresponding port Ethernet1 of switch sw6"),
// 		},
// 		{
// 			name: "machine is connected to different switches than before",
// 			m: &apiv2.Machine{
// 				Uuid: "m1",
// 				Partition: &apiv2.Partition{
// 					Id: "partition-a",
// 				},
// 				Hardware: &apiv2.MachineHardware{
// 					Nics: []*apiv2.MachineNic{
// 						{
// 							Name: "lan0",
// 							Neighbors: []*apiv2.MachineNic{
// 								{
// 									Name:       "Ethernet0",
// 									Identifier: "Eth1/1",
// 									Hostname:   "sw6",
// 								},
// 							},
// 						},
// 						{
// 							Name: "lan1",
// 							Neighbors: []*apiv2.MachineNic{
// 								{
// 									Name:       "swp1s0",
// 									Identifier: "bb:bb:bb:bb:bb:bb",
// 									Hostname:   "sw5",
// 								},
// 							},
// 						},
// 					},
// 				},
// 			},
// 			wantSwitches: []*apiv2.Switch{
// 				{
// 					Id:                 sw3.Switch.Id,
// 					MachineConnections: []*apiv2.MachineConnection{},
// 				},
// 				{
// 					Id:                 sw4.Switch.Id,
// 					MachineConnections: []*apiv2.MachineConnection{},
// 				},
// 				{
// 					Id: sw5.Switch.Id,
// 					MachineConnections: []*apiv2.MachineConnection{
// 						{
// 							MachineId: m1.ID,
// 							Nic: &apiv2.SwitchNic{
// 								Name:       "swp1s0",
// 								BgpFilter:  &apiv2.BGPFilter{},
// 								Identifier: "bb:bb:bb:bb:bb:bb",
// 								State: &apiv2.NicState{
// 									Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
// 								},
// 							},
// 						},
// 					},
// 				},
// 				{
// 					Id: sw6.Switch.Id,
// 					MachineConnections: []*apiv2.MachineConnection{
// 						{
// 							MachineId: m1.ID,
// 							Nic: &apiv2.SwitchNic{
// 								Name:       "Ethernet0",
// 								BgpFilter:  &apiv2.BGPFilter{},
// 								Identifier: "Eth1/1",
// 								State: &apiv2.NicState{
// 									Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
// 								},
// 							},
// 						},
// 					},
// 				},
// 			},
// 			wantErr: nil,
// 		},
// 	}
// 	for _, tt := range tests {
// 		s := &switchServiceServer{
// 			log:  log,
// 			repo: testStore.Store,
// 		}

// 		t.Run(tt.name, func(t *testing.T) {
// 			err := s.repo.Switch().AdditionalMethods().ConnectMachineWithSwitches(ctx, tt.m)
// 			if diff := cmp.Diff(tt.wantErr, err, errorutil.ErrorStringComparer()); diff != "" {
// 				t.Errorf("switchRepository.ConnectMachineWithSwitches() error diff = %s", diff)
// 				return
// 			}

// 			for _, wantSwitch := range tt.wantSwitches {
// 				gotSwitch, err := s.repo.Switch().Get(ctx, wantSwitch.Id)
// 				require.NoError(t, err)

// 				if diff := cmp.Diff(wantSwitch.MachineConnections, gotSwitch.MachineConnections, protocmp.Transform()); diff != "" {
// 					t.Errorf("switchRepository.ConnectMachineWithSwitches() diff = %s", diff)
// 				}
// 			}
// 		})
// 	}
// }
