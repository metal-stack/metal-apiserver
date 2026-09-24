package boot

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"buf.build/go/protovalidate"
	"github.com/google/go-cmp/cmp"
	"github.com/metal-stack/api/go/enum"
	"github.com/metal-stack/api/go/errorutil"
	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	infrav2 "github.com/metal-stack/api/go/metalstack/infra/v2"
	"github.com/metal-stack/api/go/tag"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-apiserver/pkg/repository/api"
	"github.com/metal-stack/metal-apiserver/pkg/test"
	"github.com/metal-stack/metal-apiserver/pkg/token"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/testing/protocmp"
)

var (
	m0  = "00000000-0000-0000-0000-000000000000"
	m1  = "00000000-0000-0000-0000-000000000001"
	m99 = "00000000-0000-0000-0000-000000000099"

	p1 = "00000000-0000-0000-0000-000000000001"
	p2 = "00000000-0000-0000-0000-000000000002"

	partition1 = "partition-1"
	partition2 = "partition-2"

	sw1 = &api.SwitchServiceCreateRequest{
		Switch: &apiv2.Switch{
			Id:          "sw1",
			Meta:        &apiv2.Meta{},
			Partition:   partition1,
			Rack:        new("r01"),
			ReplaceMode: apiv2.SwitchReplaceMode_SWITCH_REPLACE_MODE_OPERATIONAL,
			Nics: []*apiv2.SwitchNic{
				{
					Name:       "Ethernet0",
					Identifier: "Eth1/1",
					State: &apiv2.NicState{
						Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
					},
				},
				{
					Name:       "Ethernet1",
					Identifier: "Eth1/2",
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

	sw2 = &api.SwitchServiceCreateRequest{
		Switch: &apiv2.Switch{
			Id:          "sw2",
			Meta:        &apiv2.Meta{},
			Partition:   partition1,
			Rack:        new("r01"),
			ReplaceMode: apiv2.SwitchReplaceMode_SWITCH_REPLACE_MODE_OPERATIONAL,
			Nics: []*apiv2.SwitchNic{
				{
					Name:       "Ethernet0",
					Identifier: "Eth1/1",
					State: &apiv2.NicState{
						Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
					},
				},
				{
					Name:       "Ethernet1",
					Identifier: "Eth1/2",
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
)

func Test_bootServiceServer_Boot(t *testing.T) {
	t.Parallel()

	testStore, closer := test.StartRepositoryWithCleanup(t)
	log := testStore.GetLogger()
	defer closer()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintln(w, "a image")
	}))

	validURL := ts.URL
	defer ts.Close()

	ctx := t.Context()

	test.CreatePartitions(t, testStore, []*adminv2.PartitionServiceCreateRequest{
		{
			Partition: &apiv2.Partition{Id: partition1, BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL, Commandline: "console=ttyS1"}},
		},
	})

	tests := []struct {
		name    string
		req     *infrav2.BootServiceBootRequest
		want    *infrav2.BootServiceBootResponse
		wantErr error
	}{
		{
			name: "partition is present",
			req:  &infrav2.BootServiceBootRequest{Mac: "00:00:00:00:00:01", Partition: partition1},
			want: &infrav2.BootServiceBootResponse{
				Kernel:       validURL,
				InitRamDisks: []string{validURL},
				Cmdline:      new("console=ttyS1"),
			},
			wantErr: nil,
		},
		{
			name:    "partition is not present",
			req:     &infrav2.BootServiceBootRequest{Mac: "00:00:00:00:00:01", Partition: partition2},
			want:    nil,
			wantErr: errorutil.NotFound(`no partition with id "partition-2" found`),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &bootServiceServer{
				log:                  log,
				repo:                 testStore.Store,
				bmcSuperuserPassword: "",
			}
			if tt.wantErr == nil {
				// Execute proto based validation
				test.Validate(t, tt.req)
			}
			got, err := b.Boot(ctx, tt.req)
			if diff := cmp.Diff(tt.wantErr, err, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("bootServiceServer.Boot() error diff = %s", diff)
			}
			if diff := cmp.Diff(
				tt.want, got,
				protocmp.Transform(),
				protocmp.IgnoreFields(
					&apiv2.Meta{}, "created_at", "updated_at",
				),
			); diff != "" {
				t.Errorf("bootServiceServer.Boot()  = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_bootServiceServer_Dhcp(t *testing.T) {
	t.Parallel()

	testStore, closer := test.StartRepositoryWithCleanup(t)
	log := testStore.GetLogger()
	defer closer()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintln(w, "a image")
	}))

	validURL := ts.URL
	defer ts.Close()

	ctx := t.Context()

	test.CreateTenants(t, testStore, []*apiv2.TenantServiceCreateRequest{{Name: "t1"}})
	test.CreateProjects(t, testStore, []*apiv2.ProjectServiceCreateRequest{{Name: p1, Login: "t1"}, {Name: p2, Login: "t1"}})
	test.CreatePartitions(t, testStore, []*adminv2.PartitionServiceCreateRequest{
		{
			Partition: &apiv2.Partition{Id: partition1, BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}},
		},
	})
	test.CreateSizes(t, testStore, []*adminv2.SizeServiceCreateRequest{
		{
			Size: &apiv2.Size{Id: "c1-large-x86"},
		},
	})
	test.CreateImages(t, testStore, []*adminv2.ImageServiceCreateRequest{
		{Image: &apiv2.Image{Id: "debian-12", Url: validURL, Features: []apiv2.ImageFeature{apiv2.ImageFeature_IMAGE_FEATURE_MACHINE}}},
	})

	// We need to create machines directly on the database because there is no MachineCreateRequest available and never will.
	test.CreateMachines(t, testStore, []*metal.Machine{
		{ID: m1, PartitionID: partition1, SizeID: "c1-large-x86"},
	})

	tests := []struct {
		name        string
		req         *infrav2.BootServiceDhcpRequest
		want        *infrav2.BootServiceDhcpResponse
		wantMachine *apiv2.Machine
		wantErr     error
	}{
		{
			name: "unknown machine pxe boots",
			req: &infrav2.BootServiceDhcpRequest{
				Uuid:      m0,
				Partition: partition1,
			},
			want: &infrav2.BootServiceDhcpResponse{},
			wantMachine: &apiv2.Machine{
				Uuid: m0,
				Meta: &apiv2.Meta{Generation: 1},
				Size: &apiv2.Size{Id: "unknown", Name: new("unknown")},
				Partition: &apiv2.Partition{
					Id:                partition1,
					Meta:              &apiv2.Meta{},
					BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL},
				},
				Hardware: &apiv2.MachineHardware{},
				Status: &apiv2.MachineStatus{
					Condition:  &apiv2.MachineCondition{State: apiv2.MachineState_MACHINE_STATE_AVAILABLE},
					LedState:   &apiv2.MachineChassisIdentifyLEDState{},
					Liveliness: apiv2.MachineLiveliness_MACHINE_LIVELINESS_ALIVE,
				},
				RecentProvisioningEvents: &apiv2.MachineRecentProvisioningEvents{
					Events: []*apiv2.MachineProvisioningEvent{
						{Event: apiv2.MachineProvisioningEventType_MACHINE_PROVISIONING_EVENT_TYPE_PXE_BOOTING, Message: "machine sent extended dhcp request"},
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "existing machine pxe boots",
			req: &infrav2.BootServiceDhcpRequest{
				Uuid:      m1,
				Partition: partition1,
			},
			want: &infrav2.BootServiceDhcpResponse{},
			wantMachine: &apiv2.Machine{
				Uuid: m1,
				Meta: &apiv2.Meta{}, // FIXME why no generation value here ?
				Size: &apiv2.Size{Id: "c1-large-x86", Meta: &apiv2.Meta{}},
				Partition: &apiv2.Partition{
					Id:                partition1,
					Meta:              &apiv2.Meta{},
					BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL},
				},
				Hardware: &apiv2.MachineHardware{},
				Status: &apiv2.MachineStatus{
					Condition:  &apiv2.MachineCondition{State: apiv2.MachineState_MACHINE_STATE_AVAILABLE},
					LedState:   &apiv2.MachineChassisIdentifyLEDState{},
					Liveliness: apiv2.MachineLiveliness_MACHINE_LIVELINESS_ALIVE,
				},
				RecentProvisioningEvents: &apiv2.MachineRecentProvisioningEvents{
					Events: []*apiv2.MachineProvisioningEvent{
						{Event: apiv2.MachineProvisioningEventType_MACHINE_PROVISIONING_EVENT_TYPE_PXE_BOOTING, Message: "machine sent extended dhcp request"},
					},
				},
			},
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &bootServiceServer{
				log:                  log,
				repo:                 testStore.Store,
				bmcSuperuserPassword: "",
			}
			if tt.wantErr == nil {
				// Execute proto based validation
				test.Validate(t, tt.req)
			}
			got, err := b.Dhcp(ctx, tt.req)
			if diff := cmp.Diff(tt.wantErr, err, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("bootServiceServer.Dhcp() error diff = %s", diff)
			}
			if diff := cmp.Diff(
				tt.want, got,
				protocmp.Transform(),
				protocmp.IgnoreFields(
					&apiv2.Meta{}, "created_at", "updated_at",
				),
			); diff != "" {
				t.Errorf("bootServiceServer.Dhcp() diff = %s", diff)
			}
			gotMachine, err := testStore.UnscopedMachine().Get(ctx, tt.req.Uuid)
			require.NoError(t, err)
			if diff := cmp.Diff(
				tt.wantMachine, gotMachine,
				protocmp.Transform(),
				protocmp.IgnoreFields(
					&apiv2.Meta{}, "created_at", "updated_at",
				),
				protocmp.IgnoreFields(
					&apiv2.MachineProvisioningEvent{}, "time",
				),
				protocmp.IgnoreFields(
					&apiv2.MachineRecentProvisioningEvents{}, "last_event_time",
				),
			); diff != "" {
				t.Errorf("bootServiceServer.Dhcp() diff = %s", diff)
			}
		})
	}
}

func Test_bootServiceServer_SuperUserPassword(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		password string
		req      *infrav2.BootServiceSuperUserPasswordRequest
		want     *infrav2.BootServiceSuperUserPasswordResponse
		wantErr  error
	}{
		{
			name:     "password is set",
			password: "geheim",
			req:      &infrav2.BootServiceSuperUserPasswordRequest{Uuid: m0},
			want:     &infrav2.BootServiceSuperUserPasswordResponse{FeatureDisabled: false, SuperUserPassword: "geheim"},
		},
		{
			name: "password is not set",
			req:  &infrav2.BootServiceSuperUserPasswordRequest{Uuid: m0},
			want: &infrav2.BootServiceSuperUserPasswordResponse{FeatureDisabled: true, SuperUserPassword: ""},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &bootServiceServer{
				log:                  slog.Default(),
				bmcSuperuserPassword: tt.password,
			}
			if tt.wantErr == nil {
				// Execute proto based validation
				test.Validate(t, tt.req)
			}
			got, err := b.SuperUserPassword(t.Context(), tt.req)
			if diff := cmp.Diff(tt.wantErr, err, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("bootServiceServer.SuperUserPassword() error diff = %s", diff)
			}
			if diff := cmp.Diff(
				tt.want, got,
				protocmp.Transform(),
				protocmp.IgnoreFields(
					&apiv2.Meta{}, "created_at", "updated_at",
				),
			); diff != "" {
				t.Errorf("bootServiceServer.SuperUserPassword()  = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_bootServiceServer_Register(t *testing.T) {
	t.Parallel()

	testStore, closer := test.StartRepositoryWithCleanup(t)
	defer closer()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintln(w, "a image")
	}))

	validURL := ts.URL
	defer ts.Close()

	ctx := t.Context()

	test.CreateTenants(t, testStore, []*apiv2.TenantServiceCreateRequest{{Name: "t1"}})
	test.CreateProjects(t, testStore, []*apiv2.ProjectServiceCreateRequest{{Name: p1, Login: "t1"}, {Name: p2, Login: "t1"}})
	test.CreatePartitions(t, testStore, []*adminv2.PartitionServiceCreateRequest{
		{
			Partition: &apiv2.Partition{Id: partition1, BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}},
		},
	})
	test.CreateSizes(t, testStore, []*adminv2.SizeServiceCreateRequest{
		{
			Size: &apiv2.Size{Id: "c1-large-x86", Constraints: []*apiv2.SizeConstraint{
				{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_CORES, Min: 4, Max: 4},
				{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_MEMORY, Min: 1024, Max: 1024},
				{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_STORAGE, Min: 1024, Max: 1024},
			}},
		},
	})
	test.CreateImages(t, testStore, []*adminv2.ImageServiceCreateRequest{
		{Image: &apiv2.Image{Id: "debian-12", Url: validURL, Features: []apiv2.ImageFeature{apiv2.ImageFeature_IMAGE_FEATURE_MACHINE}}},
	})

	// We need to create machines directly on the database because there is no MachineCreateRequest available and never will.
	test.CreateMachines(t, testStore, []*metal.Machine{
		{ID: m1, PartitionID: partition1, SizeID: "c1-large-x86"},
	})

	test.CreateSwitches(t, testStore, []*api.SwitchServiceCreateRequest{sw1, sw2})

	tests := []struct {
		name         string
		req          *infrav2.BootServiceRegisterRequest
		want         *infrav2.BootServiceRegisterResponse
		wantMachine  *apiv2.Machine
		wantSwitches []*apiv2.Switch
		wantErr      error
	}{
		{
			name:    "machine id is empty",
			req:     &infrav2.BootServiceRegisterRequest{Uuid: ""},
			want:    nil,
			wantErr: errorutil.InvalidArgument("uuid is empty"),
		},
		{
			name:    "machine hardware is nil",
			req:     &infrav2.BootServiceRegisterRequest{Uuid: m99},
			want:    nil,
			wantErr: errorutil.InvalidArgument("hardware is nil"),
		},
		{
			name:    "machine bios is nil",
			req:     &infrav2.BootServiceRegisterRequest{Uuid: m99, Hardware: &apiv2.MachineHardware{}},
			want:    nil,
			wantErr: errorutil.InvalidArgument("bios is nil"),
		},
		{
			name: "machine uuid is not known",
			req: &infrav2.BootServiceRegisterRequest{
				Uuid: m99,
				Hardware: &apiv2.MachineHardware{Nics: []*apiv2.MachineNic{
					{Name: "lan0", Mac: "00:00:00:00:00:01", Neighbors: []*apiv2.MachineNic{{Name: "Ethernet0", Mac: "00:00:00:00:00:03", Identifier: "Eth1/1", Hostname: "sw1"}}},
					{Name: "lan1", Mac: "00:00:00:00:00:02", Neighbors: []*apiv2.MachineNic{{Name: "Ethernet0", Mac: "00:00:00:00:00:04", Identifier: "Eth1/1", Hostname: "sw2"}}},
				}},
				Bios:      &apiv2.MachineBios{},
				Partition: partition1,
			},
			want: &infrav2.BootServiceRegisterResponse{Uuid: m99, Size: "unknown", Partition: partition1},
			wantMachine: &apiv2.Machine{
				Meta: &apiv2.Meta{Generation: 2},
				Uuid: m99,
				Rack: "r01",
				Size: &apiv2.Size{Id: "unknown", Name: new("unknown")},
				Hardware: &apiv2.MachineHardware{
					Nics: []*apiv2.MachineNic{
						{Name: "lan0", Mac: "00:00:00:00:00:01", Neighbors: []*apiv2.MachineNic{{Name: "Ethernet0", Mac: "00:00:00:00:00:03", Identifier: "Eth1/1", Hostname: "sw1"}}},
						{Name: "lan1", Mac: "00:00:00:00:00:02", Neighbors: []*apiv2.MachineNic{{Name: "Ethernet0", Mac: "00:00:00:00:00:04", Identifier: "Eth1/1", Hostname: "sw2"}}},
					},
				},
				Partition: &apiv2.Partition{
					Id:                partition1,
					Meta:              &apiv2.Meta{},
					BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL},
				},
				Status: &apiv2.MachineStatus{
					LedState:  &apiv2.MachineChassisIdentifyLEDState{},
					Condition: &apiv2.MachineCondition{State: apiv2.MachineState_MACHINE_STATE_AVAILABLE},
				},
				RecentProvisioningEvents: &apiv2.MachineRecentProvisioningEvents{
					Events: []*apiv2.MachineProvisioningEvent{
						{Event: apiv2.MachineProvisioningEventType_MACHINE_PROVISIONING_EVENT_TYPE_ALIVE, Message: "machine registered"},
					},
				},
			},
			wantSwitches: []*apiv2.Switch{
				{
					Id:          "sw1",
					Meta:        &apiv2.Meta{Generation: uint64(1)},
					Partition:   partition1,
					Rack:        new("r01"),
					ReplaceMode: apiv2.SwitchReplaceMode_SWITCH_REPLACE_MODE_OPERATIONAL,
					MachineConnections: []*apiv2.MachineConnection{
						{MachineId: m99, Nic: &apiv2.SwitchNic{
							Name: "Ethernet0", Identifier: "Eth1/1", BgpFilter: &apiv2.BGPFilter{}, State: &apiv2.NicState{Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP},
						}},
					},
					Nics: []*apiv2.SwitchNic{
						{
							Name:       "Ethernet0",
							BgpFilter:  &apiv2.BGPFilter{},
							Identifier: "Eth1/1",
							State: &apiv2.NicState{
								Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
							},
						},
						{
							Name:       "Ethernet1",
							BgpFilter:  &apiv2.BGPFilter{},
							Identifier: "Eth1/2",
							State: &apiv2.NicState{
								Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
							},
						},
					},
					Os: &apiv2.SwitchOS{
						Vendor: apiv2.SwitchOSVendor_SWITCH_OS_VENDOR_SONIC,
					},
				},
				{
					Id:          "sw2",
					Meta:        &apiv2.Meta{Generation: uint64(1)},
					Partition:   partition1,
					Rack:        new("r01"),
					ReplaceMode: apiv2.SwitchReplaceMode_SWITCH_REPLACE_MODE_OPERATIONAL,
					MachineConnections: []*apiv2.MachineConnection{
						{MachineId: m99, Nic: &apiv2.SwitchNic{
							Name: "Ethernet0", Identifier: "Eth1/1", BgpFilter: &apiv2.BGPFilter{}, State: &apiv2.NicState{Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP},
						}},
					},
					Nics: []*apiv2.SwitchNic{
						{
							Name:       "Ethernet0",
							BgpFilter:  &apiv2.BGPFilter{},
							Identifier: "Eth1/1",
							State: &apiv2.NicState{
								Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
							},
						},
						{
							Name:       "Ethernet1",
							BgpFilter:  &apiv2.BGPFilter{},
							Identifier: "Eth1/2",
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
		{
			name: "machine is known, size is detected",
			req: &infrav2.BootServiceRegisterRequest{
				Uuid: m1,
				Hardware: &apiv2.MachineHardware{
					Memory: 1024,
					Cpus:   []*apiv2.MetalCPU{{Cores: 4}},
					Disks:  []*apiv2.MachineBlockDevice{{Name: "/dev/sda", Size: 1024}},
					Nics: []*apiv2.MachineNic{
						{Name: "lan0", Mac: "00:00:00:00:00:01", Neighbors: []*apiv2.MachineNic{{Name: "Ethernet1", Mac: "00:00:00:00:00:03", Identifier: "Eth1/2", Hostname: "sw1"}}},
						{Name: "lan1", Mac: "00:00:00:00:00:02", Neighbors: []*apiv2.MachineNic{{Name: "Ethernet1", Mac: "00:00:00:00:00:04", Identifier: "Eth1/2", Hostname: "sw2"}}},
					},
				},
				Bios: &apiv2.MachineBios{Version: "v1.0.1", Vendor: "SMC"},
				Bmc: &apiv2.MachineBMC{
					Address: "192.168.0.1:623", Mac: "00:00:00:00:00:01", User: "metal", Password: "secret", Interface: "eth0",
					Version:    "bmc123",
					PowerState: "ON",
				},
				Fru: &apiv2.MachineFRU{
					ChassisPartNumber: new("123"), ChassisPartSerial: new("234"),
					BoardMfg: new("bmfg"), BoardMfgSerial: new("b123"), BoardPartNumber: new("bpn"),
					ProductManufacturer: new("pmfg"), ProductPartNumber: new("bpn"), ProductSerial: new("p123"),
				},
				MetalHammerVersion: "v1.0.1",
				Partition:          partition1,
			},
			want: &infrav2.BootServiceRegisterResponse{Uuid: m1, Size: "c1-large-x86", Partition: partition1},
			wantMachine: &apiv2.Machine{
				Meta: &apiv2.Meta{Generation: 2},
				Uuid: m1,
				Rack: "r01",
				Size: &apiv2.Size{
					Meta: &apiv2.Meta{},
					Id:   "c1-large-x86",
					Constraints: []*apiv2.SizeConstraint{
						{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_CORES, Min: 4, Max: 4},
						{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_MEMORY, Min: 1024, Max: 1024},
						{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_STORAGE, Min: 1024, Max: 1024},
					},
				},
				Hardware: &apiv2.MachineHardware{
					Memory: 1024,
					Cpus:   []*apiv2.MetalCPU{{Cores: 4}},
					Disks:  []*apiv2.MachineBlockDevice{{Name: "/dev/sda", Size: 1024}},
					Nics: []*apiv2.MachineNic{
						{Name: "lan0", Mac: "00:00:00:00:00:01", Neighbors: []*apiv2.MachineNic{{Name: "Ethernet1", Mac: "00:00:00:00:00:03", Identifier: "Eth1/2", Hostname: "sw1"}}},
						{Name: "lan1", Mac: "00:00:00:00:00:02", Neighbors: []*apiv2.MachineNic{{Name: "Ethernet1", Mac: "00:00:00:00:00:04", Identifier: "Eth1/2", Hostname: "sw2"}}},
					},
				},
				Partition: &apiv2.Partition{
					Id:                partition1,
					Meta:              &apiv2.Meta{},
					BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL},
				},
				Status: &apiv2.MachineStatus{
					LedState:           &apiv2.MachineChassisIdentifyLEDState{},
					Condition:          &apiv2.MachineCondition{State: apiv2.MachineState_MACHINE_STATE_AVAILABLE},
					Liveliness:         apiv2.MachineLiveliness_MACHINE_LIVELINESS_ALIVE,
					MetalHammerVersion: "v1.0.1",
				},
				RecentProvisioningEvents: &apiv2.MachineRecentProvisioningEvents{
					Events: []*apiv2.MachineProvisioningEvent{},
				},
			},
			wantSwitches: []*apiv2.Switch{
				{
					Id:          "sw1",
					Meta:        &apiv2.Meta{Generation: 2},
					Partition:   partition1,
					Rack:        new("r01"),
					ReplaceMode: apiv2.SwitchReplaceMode_SWITCH_REPLACE_MODE_OPERATIONAL,
					MachineConnections: []*apiv2.MachineConnection{
						{MachineId: m1, Nic: &apiv2.SwitchNic{Name: "Ethernet1", Identifier: "Eth1/2", BgpFilter: &apiv2.BGPFilter{}, State: &apiv2.NicState{Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP}}},
						{MachineId: m99, Nic: &apiv2.SwitchNic{Name: "Ethernet0", Identifier: "Eth1/1", BgpFilter: &apiv2.BGPFilter{}, State: &apiv2.NicState{Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP}}},
					},
					Nics: []*apiv2.SwitchNic{
						{
							Name:       "Ethernet0",
							BgpFilter:  &apiv2.BGPFilter{},
							Identifier: "Eth1/1",
							State: &apiv2.NicState{
								Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
							},
						},
						{
							Name:       "Ethernet1",
							BgpFilter:  &apiv2.BGPFilter{},
							Identifier: "Eth1/2",
							State: &apiv2.NicState{
								Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
							},
						},
					},
					Os: &apiv2.SwitchOS{
						Vendor: apiv2.SwitchOSVendor_SWITCH_OS_VENDOR_SONIC,
					},
				},
				{
					Id:          "sw2",
					Meta:        &apiv2.Meta{Generation: 2},
					Partition:   partition1,
					Rack:        new("r01"),
					ReplaceMode: apiv2.SwitchReplaceMode_SWITCH_REPLACE_MODE_OPERATIONAL,
					MachineConnections: []*apiv2.MachineConnection{
						{MachineId: m1, Nic: &apiv2.SwitchNic{Name: "Ethernet1", Identifier: "Eth1/2", BgpFilter: &apiv2.BGPFilter{}, State: &apiv2.NicState{Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP}}},
						{MachineId: m99, Nic: &apiv2.SwitchNic{Name: "Ethernet0", Identifier: "Eth1/1", BgpFilter: &apiv2.BGPFilter{}, State: &apiv2.NicState{Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP}}},
					},
					Nics: []*apiv2.SwitchNic{
						{
							Name:       "Ethernet0",
							BgpFilter:  &apiv2.BGPFilter{},
							Identifier: "Eth1/1",
							State: &apiv2.NicState{
								Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
							},
						},
						{
							Name:       "Ethernet1",
							BgpFilter:  &apiv2.BGPFilter{},
							Identifier: "Eth1/2",
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
			b := &bootServiceServer{
				log:  slog.Default(),
				repo: testStore.Store,
			}
			if tt.wantErr == nil {
				// Execute proto based validation
				test.Validate(t, tt.req)
			}
			got, err := b.Register(ctx, tt.req)
			if diff := cmp.Diff(tt.wantErr, err, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("bootServiceServer.Register() error diff = %s", diff)
			}
			if diff := cmp.Diff(
				tt.want, got,
				protocmp.Transform(),
				protocmp.IgnoreFields(
					&apiv2.Meta{}, "created_at", "updated_at",
				),
			); diff != "" {
				t.Errorf("bootServiceServer.Register()  = %v, want %v", got, tt.want)
			}
			if tt.wantMachine != nil {
				m, err := testStore.UnscopedMachine().Get(ctx, tt.req.Uuid)
				require.NoError(t, err)
				if diff := cmp.Diff(
					tt.wantMachine, m,
					protocmp.Transform(),
					protocmp.IgnoreFields(
						&apiv2.Meta{}, "created_at", "updated_at",
					),
					protocmp.IgnoreFields(
						&apiv2.MachineProvisioningEvent{}, "time",
					),
				); diff != "" {
					t.Errorf("bootServiceServer.Register() diff =%s", diff)
				}
			}
			if len(tt.wantSwitches) > 0 {
				sws, err := testStore.Switch().List(ctx, &apiv2.SwitchQuery{Partition: &partition1})
				require.NoError(t, err)
				if diff := cmp.Diff(
					tt.wantSwitches, sws,
					protocmp.Transform(),
					protocmp.IgnoreFields(
						&apiv2.Meta{}, "created_at", "updated_at",
					),
				); diff != "" {
					t.Errorf("bootServiceServer.Register() switches diff =%s", diff)
				}
			}
		})
	}
}

func Test_bootServiceServer_InstallationSucceeded(t *testing.T) {
	// TODO this test is only completely useful if the register call is also done before to have the machine connections in the switch.
	t.Parallel()

	testStore, closer := test.StartRepositoryWithCleanup(t)
	defer closer()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintln(w, "a image")
	}))

	validURL := ts.URL
	defer ts.Close()

	ctx := t.Context()
	log := slog.Default()

	test.CreateTenants(t, testStore, []*apiv2.TenantServiceCreateRequest{{Name: "t1"}})
	test.CreateProjects(t, testStore, []*apiv2.ProjectServiceCreateRequest{{Name: p1, Login: "t1"}, {Name: p2, Login: "t1"}})
	test.CreatePartitions(t, testStore, []*adminv2.PartitionServiceCreateRequest{
		{
			Partition: &apiv2.Partition{Id: partition1, BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}},
		},
	})
	test.CreateSizes(t, testStore, []*adminv2.SizeServiceCreateRequest{
		{
			Size: &apiv2.Size{Id: "c1-large-x86", Constraints: []*apiv2.SizeConstraint{
				{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_CORES, Min: 4, Max: 4},
				{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_MEMORY, Min: 1024, Max: 1024},
				{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_STORAGE, Min: 1024, Max: 1024},
			}},
		},
	})
	test.CreateImages(t, testStore, []*adminv2.ImageServiceCreateRequest{
		{Image: &apiv2.Image{Id: "debian-12", Url: validURL, Features: []apiv2.ImageFeature{apiv2.ImageFeature_IMAGE_FEATURE_MACHINE}}},
		{Image: &apiv2.Image{Id: "firewall-debian-13", Url: validURL, Features: []apiv2.ImageFeature{apiv2.ImageFeature_IMAGE_FEATURE_FIREWALL}}},
	})

	netmap := test.CreateNetworks(t, testStore, []*adminv2.NetworkServiceCreateRequest{
		{
			Id: new("internet"), Name: new("internet"),
			Partition: &partition1, Type: apiv2.NetworkType_NETWORK_TYPE_EXTERNAL,
			Prefixes: []string{"142.0.0.0/16"},
			NatType:  apiv2.NATType_NAT_TYPE_IPV4_MASQUERADE.Enum(),
			Vrf:      new(uint32(49)),
		},
		{
			Id: new("super"), Name: new("super"),
			Partition: &partition1, Type: apiv2.NetworkType_NETWORK_TYPE_SUPER,
			Prefixes: []string{"10.0.0.0/16"}, DefaultChildPrefixLength: &apiv2.ChildPrefixLength{Ipv4: new(uint32(24))},
			NatType: apiv2.NATType_NAT_TYPE_IPV4_MASQUERADE.Enum(),
		},
		{Name: new("private"), Project: new(p1), Partition: &partition1, Type: apiv2.NetworkType_NETWORK_TYPE_CHILD, NatType: apiv2.NATType_NAT_TYPE_IPV4_MASQUERADE.Enum()},
	})

	var privateNetwork *apiv2.Network
	for _, nw := range netmap {
		if nw.Type == apiv2.NetworkType_NETWORK_TYPE_CHILD {
			privateNetwork = nw
		}
	}

	// We need to create machines directly on the database because there is no MachineCreateRequest available and never will.
	test.CreateMachines(t, testStore, []*metal.Machine{
		{
			ID:          m1,
			PartitionID: partition1,
			RackID:      "r01",
			SizeID:      "c1-large-x86",
			Allocation: &metal.MachineAllocation{
				Role:    metal.RoleMachine,
				Project: p1,
				ImageID: "debian-12",
				MachineNetworks: []*metal.MachineNetwork{
					{NetworkID: privateNetwork.Id, Private: true, Vrf: 42},
				}},
		},
		{
			ID:          m0,
			PartitionID: partition1,
			RackID:      "r01",
			SizeID:      "c1-large-x86",
			Allocation: &metal.MachineAllocation{
				Role:    metal.RoleFirewall,
				Project: p1,
				ImageID: "firewall-debian-13",
				MachineNetworks: []*metal.MachineNetwork{
					{NetworkID: "internet", Vrf: 49},
					{NetworkID: privateNetwork.Id, Private: true, Vrf: 42},
				}},
		},
	})

	test.CreateSwitches(t, testStore, []*api.SwitchServiceCreateRequest{sw1, sw2})

	tests := []struct {
		name         string
		req          *infrav2.BootServiceInstallationSucceededRequest
		want         *infrav2.BootServiceInstallationSucceededResponse
		wantMachine  *apiv2.Machine
		wantSwitches []*apiv2.Switch
		wantErr      error
	}{
		{
			name: "machine installation succeeded",
			req:  &infrav2.BootServiceInstallationSucceededRequest{Uuid: m1, ConsolePassword: "password"},
			want: &infrav2.BootServiceInstallationSucceededResponse{},
			wantMachine: &apiv2.Machine{
				Uuid:      m1,
				Meta:      &apiv2.Meta{Generation: 1},
				Partition: &apiv2.Partition{Id: partition1, Meta: &apiv2.Meta{}, BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}},
				Rack:      "r01",
				Size: &apiv2.Size{Id: "c1-large-x86", Meta: &apiv2.Meta{}, Constraints: []*apiv2.SizeConstraint{
					{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_CORES, Min: 4, Max: 4},
					{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_MEMORY, Min: 1024, Max: 1024},
					{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_STORAGE, Min: 1024, Max: 1024},
				}},
				Allocation: &apiv2.MachineAllocation{
					Meta:           &apiv2.Meta{},
					AllocationType: apiv2.MachineAllocationType_MACHINE_ALLOCATION_TYPE_MACHINE,
					Image: &apiv2.Image{
						Id:             "debian-12",
						Meta:           &apiv2.Meta{},
						Url:            validURL,
						Features:       []apiv2.ImageFeature{apiv2.ImageFeature_IMAGE_FEATURE_MACHINE},
						Name:           new(""),
						Description:    new(""),
						Classification: apiv2.ImageClassification_IMAGE_CLASSIFICATION_PREVIEW,
					},
					Networks: []*apiv2.MachineNetwork{
						{Network: privateNetwork.Id, NetworkType: apiv2.NetworkType_NETWORK_TYPE_CHILD, NatType: apiv2.NATType_NAT_TYPE_IPV4_MASQUERADE, Vrf: uint64(42)},
					},
					Project: p1,
				},
				Hardware:                 &apiv2.MachineHardware{},
				RecentProvisioningEvents: &apiv2.MachineRecentProvisioningEvents{},
				Status: &apiv2.MachineStatus{
					Liveliness: apiv2.MachineLiveliness_MACHINE_LIVELINESS_ALIVE,
					Condition:  &apiv2.MachineCondition{State: apiv2.MachineState_MACHINE_STATE_AVAILABLE},
					LedState:   &apiv2.MachineChassisIdentifyLEDState{},
				},
			},
			wantSwitches: []*apiv2.Switch{
				{
					Id:          "sw1",
					Meta:        &apiv2.Meta{},
					Partition:   partition1,
					Rack:        new("r01"),
					ReplaceMode: apiv2.SwitchReplaceMode_SWITCH_REPLACE_MODE_OPERATIONAL,
					Nics: []*apiv2.SwitchNic{
						{
							Name:       "Ethernet0",
							BgpFilter:  &apiv2.BGPFilter{},
							Identifier: "Eth1/1",
							State: &apiv2.NicState{
								Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
							},
						},
						{
							Name:       "Ethernet1",
							BgpFilter:  &apiv2.BGPFilter{},
							Identifier: "Eth1/2",
							State: &apiv2.NicState{
								Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
							},
						},
					},
					Os: &apiv2.SwitchOS{
						Vendor: apiv2.SwitchOSVendor_SWITCH_OS_VENDOR_SONIC,
					},
				},
				{
					Id:          "sw2",
					Meta:        &apiv2.Meta{},
					Partition:   partition1,
					Rack:        new("r01"),
					ReplaceMode: apiv2.SwitchReplaceMode_SWITCH_REPLACE_MODE_OPERATIONAL,
					Nics: []*apiv2.SwitchNic{
						{
							Name:       "Ethernet0",
							BgpFilter:  &apiv2.BGPFilter{},
							Identifier: "Eth1/1",
							State: &apiv2.NicState{
								Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
							},
						},
						{
							Name:       "Ethernet1",
							BgpFilter:  &apiv2.BGPFilter{},
							Identifier: "Eth1/2",
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
		},
		{
			name: "firewall installation succeeded",
			req:  &infrav2.BootServiceInstallationSucceededRequest{Uuid: m0, ConsolePassword: "fw-password"},
			want: &infrav2.BootServiceInstallationSucceededResponse{},
			wantMachine: &apiv2.Machine{
				Uuid:      m0,
				Meta:      &apiv2.Meta{Generation: 1},
				Partition: &apiv2.Partition{Id: partition1, Meta: &apiv2.Meta{}, BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}},
				Rack:      "r01",
				Size: &apiv2.Size{Id: "c1-large-x86", Meta: &apiv2.Meta{}, Constraints: []*apiv2.SizeConstraint{
					{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_CORES, Min: 4, Max: 4},
					{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_MEMORY, Min: 1024, Max: 1024},
					{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_STORAGE, Min: 1024, Max: 1024},
				}},
				Allocation: &apiv2.MachineAllocation{
					Meta:           &apiv2.Meta{},
					AllocationType: apiv2.MachineAllocationType_MACHINE_ALLOCATION_TYPE_FIREWALL,
					Image: &apiv2.Image{
						Id:             "firewall-debian-13",
						Meta:           &apiv2.Meta{},
						Url:            validURL,
						Features:       []apiv2.ImageFeature{apiv2.ImageFeature_IMAGE_FEATURE_FIREWALL},
						Name:           new(""),
						Description:    new(""),
						Classification: apiv2.ImageClassification_IMAGE_CLASSIFICATION_PREVIEW,
					},
					Networks: []*apiv2.MachineNetwork{
						{Network: "internet", NetworkType: apiv2.NetworkType_NETWORK_TYPE_EXTERNAL, NatType: apiv2.NATType_NAT_TYPE_IPV4_MASQUERADE, Vrf: uint64(49)},
						{Network: privateNetwork.Id, NetworkType: apiv2.NetworkType_NETWORK_TYPE_CHILD, NatType: apiv2.NATType_NAT_TYPE_IPV4_MASQUERADE, Vrf: uint64(42)},
					},
					Project: p1,
				},
				Hardware:                 &apiv2.MachineHardware{},
				RecentProvisioningEvents: &apiv2.MachineRecentProvisioningEvents{},
				Status: &apiv2.MachineStatus{
					Liveliness: apiv2.MachineLiveliness_MACHINE_LIVELINESS_ALIVE,
					Condition:  &apiv2.MachineCondition{State: apiv2.MachineState_MACHINE_STATE_AVAILABLE},
					LedState:   &apiv2.MachineChassisIdentifyLEDState{},
				},
			},
			wantSwitches: []*apiv2.Switch{
				{
					Id:          "sw1",
					Meta:        &apiv2.Meta{},
					Partition:   partition1,
					Rack:        new("r01"),
					ReplaceMode: apiv2.SwitchReplaceMode_SWITCH_REPLACE_MODE_OPERATIONAL,
					Nics: []*apiv2.SwitchNic{
						{
							Name:       "Ethernet0",
							BgpFilter:  &apiv2.BGPFilter{},
							Identifier: "Eth1/1",
							State: &apiv2.NicState{
								Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
							},
						},
						{
							Name:       "Ethernet1",
							BgpFilter:  &apiv2.BGPFilter{},
							Identifier: "Eth1/2",
							State: &apiv2.NicState{
								Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
							},
						},
					},
					Os: &apiv2.SwitchOS{
						Vendor: apiv2.SwitchOSVendor_SWITCH_OS_VENDOR_SONIC,
					},
				},
				{
					Id:          "sw2",
					Meta:        &apiv2.Meta{},
					Partition:   partition1,
					Rack:        new("r01"),
					ReplaceMode: apiv2.SwitchReplaceMode_SWITCH_REPLACE_MODE_OPERATIONAL,
					Nics: []*apiv2.SwitchNic{
						{
							Name:       "Ethernet0",
							BgpFilter:  &apiv2.BGPFilter{},
							Identifier: "Eth1/1",
							State: &apiv2.NicState{
								Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
							},
						},
						{
							Name:       "Ethernet1",
							BgpFilter:  &apiv2.BGPFilter{},
							Identifier: "Eth1/2",
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
		},
		// {
		// 	name: "machine installation failed",
		// },
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &bootServiceServer{
				log:  log,
				repo: testStore.Store,
			}
			if tt.wantErr == nil {
				// Execute proto based validation
				test.Validate(t, tt.req)
			}

			bmcCtx, bmcCancelWatch := context.WithCancel(t.Context())
			defer bmcCancelWatch()

			strVal, err := enum.GetStringValue(apiv2.MachineBMCCommand_MACHINE_BMC_COMMAND_MACHINE_CREATED)
			require.NoError(t, err)

			go func() {
				msgs := testStore.GetQueue().WaitMachineCommand(bmcCtx, partition1)

				select {
				case msg := <-msgs:
					require.Equal(t, *strVal, msg.Command)
					_, err := testStore.UnscopedMachine().AdditionalMethods().BMCCommandDone(bmcCtx, &infrav2.BMCCommandDoneRequest{
						CommandId: msg.CommandID,
						Error:     nil,
					})
					require.NoError(t, err)
				case <-ctx.Done():
					return
				}
			}()

			got, err := b.InstallationSucceeded(ctx, tt.req)
			if diff := cmp.Diff(tt.wantErr, err, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("bootServiceServer.InstallationSucceeded() error diff = %s", diff)
			}
			if diff := cmp.Diff(
				tt.want, got,
				protocmp.Transform(),
				protocmp.IgnoreFields(
					&apiv2.Meta{}, "created_at", "updated_at",
				),
			); diff != "" {
				t.Errorf("bootServiceServer.InstallationSucceeded()  = %v, want %v", got, tt.want)
			}
			if tt.wantMachine != nil {
				m, err := testStore.UnscopedMachine().Get(ctx, tt.req.Uuid)
				require.NoError(t, err)
				if diff := cmp.Diff(
					tt.wantMachine, m,
					protocmp.Transform(),
					protocmp.IgnoreFields(
						&apiv2.Meta{}, "created_at", "updated_at",
					),
					protocmp.IgnoreFields(
						&apiv2.MachineProvisioningEvent{}, "time",
					),
					protocmp.IgnoreFields(
						&apiv2.Image{}, "expires_at",
					),
				); diff != "" {
					t.Errorf("bootServiceServer.InstallationSucceeded() diff =%s", diff)
				}

				consolePassword, err := testStore.UnscopedMachine().AdditionalMethods().GetConsolePassword(ctx, tt.req.Uuid)
				require.NoError(t, err)
				require.Equal(t, tt.req.ConsolePassword, consolePassword)
			}
			if len(tt.wantSwitches) > 0 {
				sws, err := testStore.Switch().List(ctx, &apiv2.SwitchQuery{Partition: &partition1})
				require.NoError(t, err)
				if diff := cmp.Diff(
					tt.wantSwitches, sws,
					protocmp.Transform(),
					protocmp.IgnoreFields(
						&apiv2.Meta{}, "created_at", "updated_at",
					),
				); diff != "" {
					t.Errorf("bootServiceServer.InstallationSucceeded() switches diff =%s", diff)
				}
			}
		})
	}
}

func Test_bootServiceServer_MachineToken(t *testing.T) {
	t.Parallel()

	testStore, closer := test.StartRepositoryWithCleanup(t, test.WithValkey(true), test.WithPostgres(true))
	log := testStore.GetLogger()
	defer closer()

	type state struct {
		providerTenant     string
		projectRoles       map[string]apiv2.ProjectRole
		tenantRoles        map[string]apiv2.TenantRole
		independentTenants map[string]*apiv2.Labels
	}
	tests := []struct {
		name           string
		sessionToken   *apiv2.Token
		req            *infrav2.BootServiceMachineTokenRequest
		state          state
		wantErr        bool
		wantErrMessage string
		wantToken      *apiv2.Token
	}{
		{
			name: "pixie-core can create token for metal-hammer with machine roles",
			sessionToken: &apiv2.Token{
				User:      "pixie-core",
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
				InfraRole: apiv2.InfraRole_INFRA_ROLE_EDITOR.Enum(),
				MachineRoles: map[string]apiv2.MachineRole{
					"*": apiv2.MachineRole_MACHINE_ROLE_EDITOR,
				},
			},
			req: &infrav2.BootServiceMachineTokenRequest{
				Uuid: "de240964-ff9f-4e3d-95b2-8a96e43788f1",
				User: "metal-hammer",
			},
			state: state{
				providerTenant: test.DefaultProviderTenant,
				tenantRoles:    map[string]apiv2.TenantRole{},
				independentTenants: map[string]*apiv2.Labels{
					"metal-hammer": {
						Labels: map[string]string{
							tag.MachineBootstrapperTenant: "",
						},
					},
				},
			},
			wantToken: &apiv2.Token{
				User:        "metal-hammer",
				Description: "machine token for de240964-ff9f-4e3d-95b2-8a96e43788f1",
				TokenType:   apiv2.TokenType_TOKEN_TYPE_API,
				MachineRoles: map[string]apiv2.MachineRole{
					"de240964-ff9f-4e3d-95b2-8a96e43788f1": apiv2.MachineRole_MACHINE_ROLE_EDITOR,
				},
				Meta: &apiv2.Meta{},
			},
		},
		{
			name: "pixie-core misses infra role editor",
			sessionToken: &apiv2.Token{
				User:      "pixie-core",
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
			},
			req: &infrav2.BootServiceMachineTokenRequest{
				Uuid: "de240964-ff9f-4e3d-95b2-8a96e43788f1",
				User: "metal-hammer",
			},
			state: state{
				providerTenant: test.DefaultProviderTenant,
				tenantRoles:    map[string]apiv2.TenantRole{},
				independentTenants: map[string]*apiv2.Labels{
					"metal-hammer": {
						Labels: map[string]string{
							tag.MachineBootstrapperTenant: "",
						},
					},
				},
			},
			wantErr:        true,
			wantErrMessage: "permission_denied: only admins or infra editors can specify token user",
		},
		{
			name: "pixie-core misses permissions for machine roles",
			sessionToken: &apiv2.Token{
				User:      "pixie-core",
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
				InfraRole: apiv2.InfraRole_INFRA_ROLE_EDITOR.Enum(),
			},
			req: &infrav2.BootServiceMachineTokenRequest{
				Uuid: "de240964-ff9f-4e3d-95b2-8a96e43788f1",
				User: "metal-hammer",
			},
			state: state{
				providerTenant: test.DefaultProviderTenant,
				tenantRoles:    map[string]apiv2.TenantRole{},
				independentTenants: map[string]*apiv2.Labels{
					"metal-hammer": {
						Labels: map[string]string{
							tag.MachineBootstrapperTenant: "",
						},
					},
				},
			},
			wantErr:        true,
			wantErrMessage: "permission_denied: requested machine roles are not allowed: [de240964-ff9f-4e3d-95b2-8a96e43788f1]",
		},
		{
			name: "cannot create token for tenant that has no special bootstrapper label",
			sessionToken: &apiv2.Token{
				User:      "pixie-core",
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
				InfraRole: apiv2.InfraRole_INFRA_ROLE_EDITOR.Enum(),
				MachineRoles: map[string]apiv2.MachineRole{
					"*": apiv2.MachineRole_MACHINE_ROLE_EDITOR,
				},
			},
			req: &infrav2.BootServiceMachineTokenRequest{
				Uuid: "de240964-ff9f-4e3d-95b2-8a96e43788f1",
				User: "metal-hammer",
			},
			state: state{
				providerTenant: test.DefaultProviderTenant,
				tenantRoles:    map[string]apiv2.TenantRole{},
				independentTenants: map[string]*apiv2.Labels{
					"metal-hammer": {
						Labels: map[string]string{
							"a": "b",
						},
					},
				},
			},
			wantErr:        true,
			wantErrMessage: `invalid_argument: tenant "metal-hammer" must have a label "tenant.metal-stack.io/machine-bootstrapper" to be used for machine token creation`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(innerT *testing.T) {
			defer testStore.Cleanup(t)

			ctx, cancel := context.WithCancel(token.ContextWithToken(innerT.Context(), tt.sessionToken))
			defer cancel()

			test.CreateTenants(innerT, testStore, []*apiv2.TenantServiceCreateRequest{
				{
					Name: tt.sessionToken.User,
				},
			})
			test.CreateTenantMemberships(innerT, testStore, tt.sessionToken.User, []*api.TenantMemberCreateRequest{
				{
					MemberID: tt.sessionToken.User,
					Role:     apiv2.TenantRole_TENANT_ROLE_OWNER,
				},
			})

			for id, perm := range tt.state.tenantRoles {
				if id != tt.sessionToken.User {
					test.CreateTenants(innerT, testStore, []*apiv2.TenantServiceCreateRequest{
						{
							Name: id,
						},
					})
				}
				test.CreateTenantMemberships(innerT, testStore, id, []*api.TenantMemberCreateRequest{
					{
						MemberID: tt.sessionToken.User,
						Role:     perm,
					},
				})
			}

			for id, labels := range tt.state.independentTenants {
				test.CreateTenants(innerT, testStore, []*apiv2.TenantServiceCreateRequest{
					{
						Name:   id,
						Labels: labels,
					},
				})
				test.CreateTenantMemberships(innerT, testStore, id, []*api.TenantMemberCreateRequest{
					{
						MemberID: id,
						Role:     apiv2.TenantRole_TENANT_ROLE_OWNER,
					},
				})
			}

			for id, perm := range tt.state.projectRoles {
				test.CreateProjects(innerT, testStore, []*apiv2.ProjectServiceCreateRequest{
					{
						Login: tt.sessionToken.User,
						Name:  id,
					},
				})
				test.CreateProjectMemberships(innerT, testStore, id, []*api.ProjectMemberCreateRequest{
					{
						TenantId: tt.sessionToken.User,
						Role:     perm,
					},
				})
			}
			service := New(Config{
				Log:  log,
				Repo: testStore.Store,
			})

			if tt.wantErr == false {
				// Execute proto based validation
				err := protovalidate.Validate(tt.req)
				require.NoError(t, err)
			}

			response, err := service.MachineToken(ctx, tt.req)
			switch {
			case tt.wantErr && err != nil:
				if dff := cmp.Diff(tt.wantErrMessage, err.Error()); dff != "" {
					t.Fatal(dff)
				}
			case tt.wantErr && err == nil:
				t.Fatalf("want error %q, got response %q", tt.wantErrMessage, response)
			case err != nil:
				t.Fatalf("want response, got error %q", err)

			default:
				if response.Secret == "" {
					t.Error("response secret for token may not be empty")
				}
				require.NotNil(t, tt.wantToken, "token returned, nil expected")

				if diff := cmp.Diff(
					tt.wantToken, response.Token,
					protocmp.Transform(),
					protocmp.IgnoreFields(
						&apiv2.Token{}, "issued_at", "expires", "uuid",
					),
					protocmp.IgnoreFields(
						&apiv2.Meta{}, "created_at", "updated_at", "generation",
					),
				); diff != "" {
					innerT.Errorf("diff: %s", diff)
				}
			}
		})
	}
}
