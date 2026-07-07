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
	"github.com/hibiken/asynq"
	"github.com/metal-stack/api/go/errorutil"
	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-apiserver/pkg/test"
	sc "github.com/metal-stack/metal-apiserver/pkg/test/scenarios"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/testing/protocmp"
)

var (
	m1 = "00000000-0000-0000-0000-000000000001"
	m3 = "00000000-0000-0000-0000-000000000003"
	m4 = "00000000-0000-0000-0000-000000000004"

	p1 = "00000000-0000-0000-0000-000000000001"
	p2 = "00000000-0000-0000-0000-000000000002"
)

func Test_machineServiceServer_Get(t *testing.T) {
	t.Parallel()
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	testStore, closer := test.StartRepositoryWithCleanup(t, log)
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
			Partition: &apiv2.Partition{Id: "partition-1", BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}},
		},
	})
	test.CreateSizes(t, testStore, []*adminv2.SizeServiceCreateRequest{
		{
			Size: &apiv2.Size{Id: "c1-large-x86"},
		},
	})

	// We need to create machines directly on the database because there is no MachineCreateRequest available and never will.
	// Once the boot-service is available we can simulate a pxe booting machine the actually create a machine from the api level.
	test.CreateMachines(t, testStore, []*metal.Machine{
		{Base: metal.Base{ID: m1}, PartitionID: "partition-1", SizeID: "c1-large-x86"},
	})

	tests := []struct {
		name    string
		rq      *adminv2.MachineServiceGetRequest
		want    *adminv2.MachineServiceGetResponse
		wantErr error
	}{
		{
			name: "get existing",
			rq:   &adminv2.MachineServiceGetRequest{Uuid: m1},
			want: &adminv2.MachineServiceGetResponse{
				Machine: &apiv2.Machine{
					Uuid:                     m1,
					Meta:                     &apiv2.Meta{Generation: 0},
					Partition:                &apiv2.Partition{Id: "partition-1", BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}, Meta: &apiv2.Meta{}},
					Hardware:                 &apiv2.MachineHardware{},
					Size:                     &apiv2.Size{Id: "c1-large-x86", Meta: &apiv2.Meta{}},
					RecentProvisioningEvents: &apiv2.MachineRecentProvisioningEvents{},
					Status: &apiv2.MachineStatus{
						Condition:  &apiv2.MachineCondition{State: apiv2.MachineState_MACHINE_STATE_AVAILABLE},
						LedState:   &apiv2.MachineChassisIdentifyLEDState{},
						Liveliness: apiv2.MachineLiveliness_MACHINE_LIVELINESS_ALIVE,
					},
				},
			},
			wantErr: nil,
		},
		{
			name:    "get non existing",
			rq:      &adminv2.MachineServiceGetRequest{Uuid: "m99"},
			want:    nil,
			wantErr: errorutil.NotFound(`no machine with id "m99" found`),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &machineServiceServer{
				log:  log,
				repo: testStore.Store,
			}
			if tt.wantErr == nil {
				// Execute proto based validation
				test.Validate(t, tt.rq)
			}
			got, err := m.Get(ctx, tt.rq)
			if diff := cmp.Diff(err, tt.wantErr, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("diff = %s", diff)
			}

			if diff := cmp.Diff(
				tt.want, got,
				protocmp.Transform(),
				protocmp.IgnoreFields(
					&apiv2.Meta{}, "created_at", "updated_at",
				),
				protocmp.IgnoreFields(
					&apiv2.MachineProvisioningEvent{}, "time",
				),
			); diff != "" {
				t.Errorf("machineServiceServer.Get() = %v, want %v diff: %s", got, tt.want, diff)
			}
		})
	}
}

func Test_machineServiceServer_List(t *testing.T) {
	t.Parallel()
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	testStore, closer := test.StartRepositoryWithCleanup(t, log)
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
			Partition: &apiv2.Partition{Id: "partition-1", BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}},
		},
	})
	test.CreateImages(t, testStore, []*adminv2.ImageServiceCreateRequest{
		{Image: &apiv2.Image{Id: "debian-12", Url: validURL, Features: []apiv2.ImageFeature{apiv2.ImageFeature_IMAGE_FEATURE_MACHINE}}},
	})
	test.CreateSizes(t, testStore, []*adminv2.SizeServiceCreateRequest{
		{Size: &apiv2.Size{Id: "c1-large-x86", Constraints: []*apiv2.SizeConstraint{{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_CORES, Min: 8, Max: 8}}}},
		{Size: &apiv2.Size{Id: "c1-medium-x86", Constraints: []*apiv2.SizeConstraint{{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_CORES, Min: 4, Max: 4}}}},
	})

	// We need to create machines directly on the database because there is no MachineCreateRequest available and never will.
	// Once the boot-service is available we can simulate a pxe booting machine the actually create a machine from the api level.
	test.CreateMachines(t, testStore, []*metal.Machine{
		{Base: metal.Base{ID: m1}, PartitionID: "partition-1", SizeID: "c1-medium-x86"},
		{Base: metal.Base{ID: "m2"}, PartitionID: "partition-1", SizeID: "c1-medium-x86"},
		{Base: metal.Base{ID: m3}, PartitionID: "partition-1", SizeID: "c1-large-x86", Allocation: &metal.MachineAllocation{Project: p1, ImageID: "debian-12"}},
		{Base: metal.Base{ID: m4}, PartitionID: "partition-1", SizeID: "c1-large-x86", Allocation: &metal.MachineAllocation{Project: p2, ImageID: "debian-12"}},
		{Base: metal.Base{ID: "m5"}, PartitionID: "partition-1", SizeID: "c1-large-x86", Allocation: &metal.MachineAllocation{Project: p2, ImageID: "debian-12"}},
	})

	tests := []struct {
		name    string
		rq      *adminv2.MachineServiceListRequest
		want    *adminv2.MachineServiceListResponse
		wantErr error
	}{
		{
			name: "List from p1",
			rq:   &adminv2.MachineServiceListRequest{Query: &apiv2.MachineQuery{Allocation: &apiv2.MachineAllocationQuery{Project: new(p1)}}},
			want: &adminv2.MachineServiceListResponse{
				Machines: []*apiv2.Machine{
					{
						Uuid:                     m3,
						Meta:                     &apiv2.Meta{},
						Partition:                &apiv2.Partition{Id: "partition-1", BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}, Meta: &apiv2.Meta{}},
						Hardware:                 &apiv2.MachineHardware{},
						Size:                     &apiv2.Size{Id: "c1-large-x86", Meta: &apiv2.Meta{}, Constraints: []*apiv2.SizeConstraint{{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_CORES, Min: 8, Max: 8}}},
						RecentProvisioningEvents: &apiv2.MachineRecentProvisioningEvents{},
						Status: &apiv2.MachineStatus{
							Condition:  &apiv2.MachineCondition{State: apiv2.MachineState_MACHINE_STATE_AVAILABLE},
							LedState:   &apiv2.MachineChassisIdentifyLEDState{},
							Liveliness: apiv2.MachineLiveliness_MACHINE_LIVELINESS_ALIVE,
						},
						Allocation: &apiv2.MachineAllocation{
							Project: p1,
							Meta:    &apiv2.Meta{},
							Image: &apiv2.Image{
								Id:             "debian-12",
								Meta:           &apiv2.Meta{},
								Url:            validURL,
								Features:       []apiv2.ImageFeature{apiv2.ImageFeature_IMAGE_FEATURE_MACHINE},
								Classification: apiv2.ImageClassification_IMAGE_CLASSIFICATION_PREVIEW,
								Description:    new(""),
								Name:           new(""),
							},
						},
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "list from p2",
			rq:   &adminv2.MachineServiceListRequest{Query: &apiv2.MachineQuery{Uuid: new(m4), Allocation: &apiv2.MachineAllocationQuery{Project: new(p2)}}},
			want: &adminv2.MachineServiceListResponse{
				Machines: []*apiv2.Machine{
					{
						Uuid:                     m4,
						Meta:                     &apiv2.Meta{},
						Partition:                &apiv2.Partition{Id: "partition-1", BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}, Meta: &apiv2.Meta{}},
						Hardware:                 &apiv2.MachineHardware{},
						Size:                     &apiv2.Size{Id: "c1-large-x86", Meta: &apiv2.Meta{}, Constraints: []*apiv2.SizeConstraint{{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_CORES, Min: 8, Max: 8}}},
						RecentProvisioningEvents: &apiv2.MachineRecentProvisioningEvents{},
						Status: &apiv2.MachineStatus{
							Condition:  &apiv2.MachineCondition{State: apiv2.MachineState_MACHINE_STATE_AVAILABLE},
							LedState:   &apiv2.MachineChassisIdentifyLEDState{},
							Liveliness: apiv2.MachineLiveliness_MACHINE_LIVELINESS_ALIVE,
						},
						Allocation: &apiv2.MachineAllocation{
							Project: p2,
							Meta:    &apiv2.Meta{},
							Image: &apiv2.Image{
								Id:             "debian-12",
								Meta:           &apiv2.Meta{},
								Url:            validURL,
								Features:       []apiv2.ImageFeature{apiv2.ImageFeature_IMAGE_FEATURE_MACHINE},
								Classification: apiv2.ImageClassification_IMAGE_CLASSIFICATION_PREVIEW,
								Description:    new(""),
								Name:           new(""),
							},
						},
					},
				},
			},
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &machineServiceServer{
				log:  log,
				repo: testStore.Store,
			}
			if tt.wantErr == nil {
				// Execute proto based validation
				test.Validate(t, tt.rq)
			}
			got, err := m.List(ctx, tt.rq)
			if diff := cmp.Diff(err, tt.wantErr, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("diff = %s", diff)
			}

			if diff := cmp.Diff(
				tt.want, got,
				protocmp.Transform(),
				protocmp.IgnoreFields(
					&apiv2.Image{}, "expires_at",
				),
				protocmp.IgnoreFields(
					&apiv2.Meta{}, "created_at", "updated_at",
				),
				protocmp.IgnoreFields(
					&apiv2.MachineProvisioningEvent{}, "time",
				),
			); diff != "" {
				t.Errorf("machineServiceServer.List() = %v, want %v diff: %s", got, tt.want, diff)
			}
		})
	}
}

func Test_machineServiceServer_BMCCommand(t *testing.T) {
	t.Parallel()
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	testStore, closer := test.StartRepositoryWithCleanup(t, log)
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
			Partition: &apiv2.Partition{Id: "partition-1", BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}},
		},
	})
	test.CreateImages(t, testStore, []*adminv2.ImageServiceCreateRequest{
		{Image: &apiv2.Image{Id: "debian-12", Url: validURL, Features: []apiv2.ImageFeature{apiv2.ImageFeature_IMAGE_FEATURE_MACHINE}}},
	})
	test.CreateSizes(t, testStore, []*adminv2.SizeServiceCreateRequest{
		{Size: &apiv2.Size{Id: "c1-large-x86", Constraints: []*apiv2.SizeConstraint{{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_CORES, Min: 8, Max: 8}}}},
		{Size: &apiv2.Size{Id: "c1-medium-x86", Constraints: []*apiv2.SizeConstraint{{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_CORES, Min: 4, Max: 4}}}},
	})

	// We need to create machines directly on the database because there is no MachineCreateRequest available and never will.
	// Once the boot-service is available we can simulate a pxe booting machine the actually create a machine from the api level.
	test.CreateMachines(t, testStore, []*metal.Machine{
		{Base: metal.Base{ID: m1}, PartitionID: "partition-1", SizeID: "c1-medium-x86"},
		{Base: metal.Base{ID: m3}, PartitionID: "partition-1", SizeID: "c1-large-x86", IPMI: metal.IPMI{Address: "10.0.0.1", User: "metal", Password: "secret"}},
	})

	tests := []struct {
		name    string
		rq      *adminv2.MachineServiceBMCCommandRequest
		want    *adminv2.MachineServiceBMCCommandResponse
		wantErr error
	}{
		{
			name: "boot from disk command, machine without bmc details",
			rq: &adminv2.MachineServiceBMCCommandRequest{
				Uuid:    m1,
				Command: apiv2.MachineBMCCommand_MACHINE_BMC_COMMAND_BOOT_FROM_DISK,
			},
			want:    nil,
			wantErr: errorutil.FailedPrecondition(`machine "00000000-0000-0000-0000-000000000001" does not have bmc connections details yet`),
		},
		{
			name: "boot from disk command, machine with bmc connection details",
			rq: &adminv2.MachineServiceBMCCommandRequest{
				Uuid:    m3,
				Command: apiv2.MachineBMCCommand_MACHINE_BMC_COMMAND_BOOT_FROM_DISK,
			},
			want:    &adminv2.MachineServiceBMCCommandResponse{},
			wantErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &machineServiceServer{
				log:  log,
				repo: testStore.Store,
			}
			if tt.wantErr == nil {
				// Execute proto based validation
				test.Validate(t, tt.rq)
			}
			got, err := m.BMCCommand(ctx, tt.rq)
			if diff := cmp.Diff(err, tt.wantErr, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("diff = %s", diff)
			}

			if diff := cmp.Diff(
				tt.want, got,
				protocmp.Transform(),
				protocmp.IgnoreFields(
					&apiv2.Image{}, "expires_at",
				),
				protocmp.IgnoreFields(
					&apiv2.Meta{}, "created_at", "updated_at",
				),
				protocmp.IgnoreFields(
					&apiv2.MachineProvisioningEvent{}, "time",
				),
			); diff != "" {
				t.Errorf("machineServiceServer.BMCCommand() = %v, want %v diff: %s", got, tt.want, diff)
			}

			if tt.want != nil {
				tasks, err := m.repo.Task().List(nil)
				require.NoError(t, err)
				require.Len(t, tasks, 1)
				require.Equal(t, asynq.TaskStatePending, tasks[0].State)
				require.Contains(t, string(tasks[0].Payload), "boot-from-disk")
			}
		})
	}
}

func Test_machineServiceServer_Issues(t *testing.T) {
	t.Parallel()
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	testStore, closer := test.StartRepositoryWithCleanup(t, log)
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
			Partition: &apiv2.Partition{Id: "partition-1", BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}},
		},
	})
	test.CreateSizes(t, testStore, []*adminv2.SizeServiceCreateRequest{
		{
			Size: &apiv2.Size{Id: "c1-large-x86"},
		},
	})

	// machine with no event container -> will have an issue
	noEcMachine := "00000000-0000-0000-0000-0000000000ec"
	// machine with liveliness dead -> will have a liveliness-dead issue
	deadMachine := "00000000-0000-0000-0000-0000000000de"
	// machine with liveliness unknown -> will have a liveliness-unknown issue
	unknownMachine := "00000000-0000-0000-0000-0000000000uk"
	// machine with crash loop -> will have a crashloop issue
	crashLoopMachine := "00000000-0000-0000-0000-0000000000cl"
	// machine with failed machine reclaim -> will have a failed-machine-reclaim issue
	failedReclaimMachine := "00000000-0000-0000-0000-0000000000fr"
	// machine with no liveliness -> will have a liveliness-not-available issue
	naLivelinessMachine := "00000000-0000-0000-0000-0000000000na"
	// machine with no partition -> will have a no-partition issue
	noPartitionMachine := "00000000-0000-0000-0000-0000000000np"
	// machine with bmc without mac -> will have bmc-without-mac issue
	noBmcMacMachine := "00000000-0000-0000-0000-0000000000bb"
	// machine with bmc without ip -> will have bmc-without-ip issue
	noBmcIpMachine := "00000000-0000-0000-0000-0000000000bi"

	// create a healthy machine with IPMI -> will have no issues
	test.CreateMachines(t, testStore, []*metal.Machine{
		{
			Base:        metal.Base{ID: m1},
			PartitionID: "partition-1",
			SizeID:      "c1-large-x86",
			IPMI: metal.IPMI{
				Address:     "10.0.0.1",
				MacAddress:  "aa:bb:cc:dd:ee:ff",
				LastUpdated: time.Now(),
			},
		},
	})

	// create a machine directly on the database to avoid event container creation
	_, err := testStore.GetDatastore().Machine().Create(ctx, &metal.Machine{
		Base:        metal.Base{ID: noEcMachine},
		PartitionID: "partition-1",
		SizeID:      "c1-large-x86",
		IPMI: metal.IPMI{
			Address:     "10.0.0.2",
			MacAddress:  "aa:bb:cc:dd:ee:00",
			LastUpdated: time.Now(),
		},
	})
	require.NoError(t, err)

	// create machines with event containers that have specific issue-triggering states
	machineEventContainers := []*metal.ProvisioningEventContainer{
		{
			Base:       metal.Base{ID: deadMachine},
			Liveliness: metal.MachineLivelinessDead,
			Events:     metal.ProvisioningEvents{{Time: time.Now(), Event: metal.ProvisioningEventAlive}},
		},
		{
			Base:       metal.Base{ID: unknownMachine},
			Liveliness: metal.MachineLivelinessUnknown,
			Events:     metal.ProvisioningEvents{{Time: time.Now(), Event: metal.ProvisioningEventAlive}},
		},
		{
			Base:       metal.Base{ID: crashLoopMachine},
			CrashLoop:  true,
			Liveliness: metal.MachineLivelinessAlive,
			Events:     metal.ProvisioningEvents{{Time: time.Now(), Event: metal.ProvisioningEventCrashed}},
		},
		{
			Base:                 metal.Base{ID: failedReclaimMachine},
			FailedMachineReclaim: true,
			Liveliness:           metal.MachineLivelinessAlive,
			Events:               metal.ProvisioningEvents{{Time: time.Now(), Event: metal.ProvisioningEventPhonedHome}},
		},
		{
			Base:       metal.Base{ID: naLivelinessMachine},
			Liveliness: metal.MachineLiveliness(""),
			Events:     metal.ProvisioningEvents{{Time: time.Now(), Event: metal.ProvisioningEventAlive}},
		},
	}
	for _, ec := range machineEventContainers {
		_, err := testStore.GetDatastore().Event().Create(ctx, ec)
		require.NoError(t, err)
	}
	// create machines that correspond to the event containers - each with unique BMC IP
	for i, ec := range machineEventContainers {
		ip := fmt.Sprintf("10.0.0.%d", i+100)
		mac := fmt.Sprintf("aa:bb:cc:dd:ee:%02x", i+10)
		_, err = testStore.GetDatastore().Machine().Create(ctx, &metal.Machine{
			Base:        metal.Base{ID: ec.ID},
			PartitionID: "partition-1",
			SizeID:      "c1-large-x86",
			IPMI: metal.IPMI{
				Address:     ip,
				MacAddress:  mac,
				LastUpdated: time.Now(),
			},
		})
		require.NoError(t, err)
	}

	// create machines for issues based on machine properties rather than event containers
	// noPartitionMachine has no PartitionID
	// noBmcMacMachine and noBmcIpMachine have PartitionID but their BMC will also trigger no_partition if empty
	machinePropertyMachines := []*metal.Machine{
		{
			Base: metal.Base{ID: noPartitionMachine},
			// intentionally empty PartitionID to trigger no-partition issue
			IPMI: metal.IPMI{
				Address:     "10.0.0.110",
				MacAddress:  "aa:bb:cc:dd:ee:04",
				LastUpdated: time.Now(),
			},
		},
		{
			Base:        metal.Base{ID: noBmcMacMachine},
			PartitionID: "partition-1",
			IPMI: metal.IPMI{
				Address:     "10.0.0.111",
				LastUpdated: time.Now(),
			},
		},
		{
			Base:        metal.Base{ID: noBmcIpMachine},
			PartitionID: "partition-1",
			IPMI: metal.IPMI{
				MacAddress:  "aa:bb:cc:dd:ee:06",
				LastUpdated: time.Now(),
			},
		},
	}
	for _, m := range machinePropertyMachines {
		_, err := testStore.GetDatastore().Machine().Create(ctx, m)
		require.NoError(t, err)
		// event containers created without specific issue-triggering states
		_, err = testStore.GetDatastore().Event().Create(ctx, &metal.ProvisioningEventContainer{
			Base:       metal.Base{ID: m.ID},
			Liveliness: metal.MachineLivelinessAlive,
			Events:     metal.ProvisioningEvents{{Time: time.Now(), Event: metal.ProvisioningEventAlive}},
		})
		require.NoError(t, err)
	}

	testCases := []struct {
		name    string
		rq      *adminv2.MachineServiceIssuesRequest
		want    *adminv2.MachineServiceIssuesResponse
		wantErr error
	}{
		{
			name: "no event container",
			rq: &adminv2.MachineServiceIssuesRequest{
				Query: &apiv2.MachineIssuesQuery{
					MachineQuery: &apiv2.MachineQuery{
						Uuid: &noEcMachine,
					},
				},
			},
			want: &adminv2.MachineServiceIssuesResponse{
				Issues: []*apiv2.MachineIssues{
					{
						Uuid: noEcMachine,
						Issues: []*apiv2.MachineIssue{
							{
								Type:         apiv2.MachineIssueType_MACHINE_ISSUE_TYPE_NO_EVENT_CONTAINER,
								Severity:     apiv2.MachineIssueSeverity_MACHINE_ISSUE_SEVERITY_MAJOR,
								Description:  "machine has no event container",
								ReferenceUrl: "https://metal-stack.io/docs/troubleshooting/#no-event-container",
							},
						},
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "liveliness dead",
			rq: &adminv2.MachineServiceIssuesRequest{
				Query: &apiv2.MachineIssuesQuery{
					MachineQuery: &apiv2.MachineQuery{
						Uuid: &deadMachine,
					},
				},
			},
			want: &adminv2.MachineServiceIssuesResponse{
				Issues: []*apiv2.MachineIssues{
					{
						Uuid: deadMachine,
						Issues: []*apiv2.MachineIssue{
							{
								Type:         apiv2.MachineIssueType_MACHINE_ISSUE_TYPE_LIVELINESS_DEAD,
								Severity:     apiv2.MachineIssueSeverity_MACHINE_ISSUE_SEVERITY_MAJOR,
								Description:  "the machine is not sending events anymore",
								ReferenceUrl: "https://metal-stack.io/docs/troubleshooting/#liveliness-dead",
							},
						},
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "liveliness unknown",
			rq: &adminv2.MachineServiceIssuesRequest{
				Query: &apiv2.MachineIssuesQuery{
					MachineQuery: &apiv2.MachineQuery{
						Uuid: &unknownMachine,
					},
				},
			},
			want: &adminv2.MachineServiceIssuesResponse{
				Issues: []*apiv2.MachineIssues{
					{
						Uuid: unknownMachine,
						Issues: []*apiv2.MachineIssue{
							{
								Type:         apiv2.MachineIssueType_MACHINE_ISSUE_TYPE_LIVELINESS_UNKNOWN,
								Severity:     apiv2.MachineIssueSeverity_MACHINE_ISSUE_SEVERITY_MAJOR,
								Description:  "the machine is not sending LLDP alive messages anymore",
								ReferenceUrl: "https://metal-stack.io/docs/troubleshooting/#liveliness-unknown",
							},
						},
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "liveliness not available",
			rq: &adminv2.MachineServiceIssuesRequest{
				Query: &apiv2.MachineIssuesQuery{
					MachineQuery: &apiv2.MachineQuery{
						Uuid: &naLivelinessMachine,
					},
				},
			},
			want: &adminv2.MachineServiceIssuesResponse{
				Issues: []*apiv2.MachineIssues{
					{
						Uuid: naLivelinessMachine,
						Issues: []*apiv2.MachineIssue{
							{
								Type:         apiv2.MachineIssueType_MACHINE_ISSUE_TYPE_LIVELINESS_NOT_AVAILABLE,
								Severity:     apiv2.MachineIssueSeverity_MACHINE_ISSUE_SEVERITY_MINOR,
								Description:  "the machine liveliness is not available",
								ReferenceUrl: "https://metal-stack.io/docs/troubleshooting/#liveliness-not-available",
							},
						},
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "crash loop",
			rq: &adminv2.MachineServiceIssuesRequest{
				Query: &apiv2.MachineIssuesQuery{
					MachineQuery: &apiv2.MachineQuery{
						Uuid: &crashLoopMachine,
					},
				},
			},
			want: &adminv2.MachineServiceIssuesResponse{
				Issues: []*apiv2.MachineIssues{
					{
						Uuid: crashLoopMachine,
						Issues: []*apiv2.MachineIssue{
							{
								Type:         apiv2.MachineIssueType_MACHINE_ISSUE_TYPE_CRASH_LOOP,
								Severity:     apiv2.MachineIssueSeverity_MACHINE_ISSUE_SEVERITY_MAJOR,
								Description:  "machine is in a provisioning crash loop (⭕)",
								ReferenceUrl: "https://metal-stack.io/docs/troubleshooting/#crashloop",
							},
						},
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "failed machine reclaim",
			rq: &adminv2.MachineServiceIssuesRequest{
				Query: &apiv2.MachineIssuesQuery{
					MachineQuery: &apiv2.MachineQuery{
						Uuid: &failedReclaimMachine,
					},
				},
			},
			want: &adminv2.MachineServiceIssuesResponse{
				Issues: []*apiv2.MachineIssues{
					{
						Uuid: failedReclaimMachine,
						Issues: []*apiv2.MachineIssue{
							{
								Type:         apiv2.MachineIssueType_MACHINE_ISSUE_TYPE_FAILED_MACHINE_RECLAIM,
								Severity:     apiv2.MachineIssueSeverity_MACHINE_ISSUE_SEVERITY_CRITICAL,
								Description:  "machine phones home but not allocated",
								ReferenceUrl: "https://metal-stack.io/docs/troubleshooting/#failed-machine-reclaim",
							},
						},
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "no partition",
			rq: &adminv2.MachineServiceIssuesRequest{
				Query: &apiv2.MachineIssuesQuery{
					MachineQuery: &apiv2.MachineQuery{
						Uuid: &noPartitionMachine,
					},
				},
			},
			want: &adminv2.MachineServiceIssuesResponse{
				Issues: []*apiv2.MachineIssues{
					{
						Uuid: noPartitionMachine,
						Issues: []*apiv2.MachineIssue{
							{
								Type:         apiv2.MachineIssueType_MACHINE_ISSUE_TYPE_NO_PARTITION,
								Severity:     apiv2.MachineIssueSeverity_MACHINE_ISSUE_SEVERITY_MAJOR,
								Description:  "machine with no partition",
								ReferenceUrl: "https://metal-stack.io/docs/troubleshooting/#no-partition",
							},
						},
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "bmc without mac",
			rq: &adminv2.MachineServiceIssuesRequest{
				Query: &apiv2.MachineIssuesQuery{
					MachineQuery: &apiv2.MachineQuery{
						Uuid: &noBmcMacMachine,
					},
				},
			},
			want: &adminv2.MachineServiceIssuesResponse{
				Issues: []*apiv2.MachineIssues{
					{
						Uuid: noBmcMacMachine,
						Issues: []*apiv2.MachineIssue{
							{
								Type:         apiv2.MachineIssueType_MACHINE_ISSUE_TYPE_BMC_WITHOUT_MAC,
								Severity:     apiv2.MachineIssueSeverity_MACHINE_ISSUE_SEVERITY_MAJOR,
								Description:  "BMC has no mac address",
								ReferenceUrl: "https://metal-stack.io/docs/troubleshooting/#bmc-without-mac",
							},
						},
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "bmc without ip",
			rq: &adminv2.MachineServiceIssuesRequest{
				Query: &apiv2.MachineIssuesQuery{
					MachineQuery: &apiv2.MachineQuery{
						Uuid: &noBmcIpMachine,
					},
				},
			},
			want: &adminv2.MachineServiceIssuesResponse{
				Issues: []*apiv2.MachineIssues{
					{
						Uuid: noBmcIpMachine,
						Issues: []*apiv2.MachineIssue{
							{
								Type:         apiv2.MachineIssueType_MACHINE_ISSUE_TYPE_BMC_WITHOUT_IP,
								Severity:     apiv2.MachineIssueSeverity_MACHINE_ISSUE_SEVERITY_MAJOR,
								Description:  "BMC has no ip address",
								ReferenceUrl: "https://metal-stack.io/docs/troubleshooting/#bmc-without-ip",
							},
						},
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "filter by only liveliness dead",
			rq: &adminv2.MachineServiceIssuesRequest{
				Query: &apiv2.MachineIssuesQuery{
					Only: []apiv2.MachineIssueType{apiv2.MachineIssueType_MACHINE_ISSUE_TYPE_LIVELINESS_DEAD},
				},
			},
			want: &adminv2.MachineServiceIssuesResponse{
				Issues: []*apiv2.MachineIssues{
					{
						Uuid: deadMachine,
						Issues: []*apiv2.MachineIssue{
							{
								Type:         apiv2.MachineIssueType_MACHINE_ISSUE_TYPE_LIVELINESS_DEAD,
								Severity:     apiv2.MachineIssueSeverity_MACHINE_ISSUE_SEVERITY_MAJOR,
								Description:  "the machine is not sending events anymore",
								ReferenceUrl: "https://metal-stack.io/docs/troubleshooting/#liveliness-dead",
							},
						},
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "omit liveliness issues",
			rq: &adminv2.MachineServiceIssuesRequest{
				Query: &apiv2.MachineIssuesQuery{
					Omit: []apiv2.MachineIssueType{
						apiv2.MachineIssueType_MACHINE_ISSUE_TYPE_LIVELINESS_DEAD,
						apiv2.MachineIssueType_MACHINE_ISSUE_TYPE_LIVELINESS_UNKNOWN,
						apiv2.MachineIssueType_MACHINE_ISSUE_TYPE_LIVELINESS_NOT_AVAILABLE,
					},
				},
			},
			want: &adminv2.MachineServiceIssuesResponse{
				Issues: []*apiv2.MachineIssues{
					{
						Uuid: noBmcMacMachine,
						Issues: []*apiv2.MachineIssue{
							{
								Type:         apiv2.MachineIssueType_MACHINE_ISSUE_TYPE_BMC_WITHOUT_MAC,
								Severity:     apiv2.MachineIssueSeverity_MACHINE_ISSUE_SEVERITY_MAJOR,
								Description:  "BMC has no mac address",
								ReferenceUrl: "https://metal-stack.io/docs/troubleshooting/#bmc-without-mac",
							},
						},
					},
					{
						Uuid: noBmcIpMachine,
						Issues: []*apiv2.MachineIssue{
							{
								Type:         apiv2.MachineIssueType_MACHINE_ISSUE_TYPE_BMC_WITHOUT_IP,
								Severity:     apiv2.MachineIssueSeverity_MACHINE_ISSUE_SEVERITY_MAJOR,
								Description:  "BMC has no ip address",
								ReferenceUrl: "https://metal-stack.io/docs/troubleshooting/#bmc-without-ip",
							},
						},
					},
					{
						Uuid: crashLoopMachine,
						Issues: []*apiv2.MachineIssue{
							{
								Type:         apiv2.MachineIssueType_MACHINE_ISSUE_TYPE_CRASH_LOOP,
								Severity:     apiv2.MachineIssueSeverity_MACHINE_ISSUE_SEVERITY_MAJOR,
								Description:  "machine is in a provisioning crash loop (⭕)",
								ReferenceUrl: "https://metal-stack.io/docs/troubleshooting/#crashloop",
							},
						},
					},
					{
						Uuid: noEcMachine,
						Issues: []*apiv2.MachineIssue{
							{
								Type:         apiv2.MachineIssueType_MACHINE_ISSUE_TYPE_NO_EVENT_CONTAINER,
								Severity:     apiv2.MachineIssueSeverity_MACHINE_ISSUE_SEVERITY_MAJOR,
								Description:  "machine has no event container",
								ReferenceUrl: "https://metal-stack.io/docs/troubleshooting/#no-event-container",
							},
						},
					},
					{
						Uuid: failedReclaimMachine,
						Issues: []*apiv2.MachineIssue{
							{
								Type:         apiv2.MachineIssueType_MACHINE_ISSUE_TYPE_FAILED_MACHINE_RECLAIM,
								Severity:     apiv2.MachineIssueSeverity_MACHINE_ISSUE_SEVERITY_CRITICAL,
								Description:  "machine phones home but not allocated",
								ReferenceUrl: "https://metal-stack.io/docs/troubleshooting/#failed-machine-reclaim",
							},
						},
					},
					{
						Uuid: noPartitionMachine,
						Issues: []*apiv2.MachineIssue{
							{
								Type:         apiv2.MachineIssueType_MACHINE_ISSUE_TYPE_NO_PARTITION,
								Severity:     apiv2.MachineIssueSeverity_MACHINE_ISSUE_SEVERITY_MAJOR,
								Description:  "machine with no partition",
								ReferenceUrl: "https://metal-stack.io/docs/troubleshooting/#no-partition",
							},
						},
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "filter by severity critical",
			rq: &adminv2.MachineServiceIssuesRequest{
				Query: &apiv2.MachineIssuesQuery{
					Severity: apiv2.MachineIssueSeverity_MACHINE_ISSUE_SEVERITY_CRITICAL.Enum(),
				},
			},
			want: &adminv2.MachineServiceIssuesResponse{
				Issues: []*apiv2.MachineIssues{
					{
						Uuid: failedReclaimMachine,
						Issues: []*apiv2.MachineIssue{
							{
								Type:         apiv2.MachineIssueType_MACHINE_ISSUE_TYPE_FAILED_MACHINE_RECLAIM,
								Severity:     apiv2.MachineIssueSeverity_MACHINE_ISSUE_SEVERITY_CRITICAL,
								Description:  "machine phones home but not allocated",
								ReferenceUrl: "https://metal-stack.io/docs/troubleshooting/#failed-machine-reclaim",
							},
						},
					},
				},
			},
			wantErr: nil,
		},
	}
	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			m := &machineServiceServer{
				log:  log,
				repo: testStore.Store,
			}
			got, err := m.Issues(ctx, tt.rq)
			if diff := cmp.Diff(err, tt.wantErr, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("diff = %s", diff)
			}

			if diff := cmp.Diff(
				tt.want, got,
				protocmp.Transform(),
			); diff != "" {
				t.Errorf("machineServiceServer.Issues() = %v, want %v diff: %s", got, tt.want, diff)
			}
		})
	}
}

func Test_machineServiceServer_SetState(t *testing.T) {
	t.Parallel()
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	dc := test.NewDatacenter(t, log)
	defer dc.Close()

	testDC := sc.DefaultDatacenter
	testDC.Machines = append(testDC.Machines, &sc.MachineWithLiveliness{
		Machine: &metal.Machine{
			Base:        metal.Base{ID: sc.Machine5},
			PartitionID: sc.Partition1,
			SizeID:      sc.SizeC1Large,
			State: metal.MachineState{
				Value:       metal.LockedState,
				Description: "locked for testing",
			},
		},
		Liveliness: metal.MachineLivelinessAlive,
	})
	dc.Create(&testDC)

	tests := []struct {
		name    string
		req     *adminv2.MachineServiceSetStateRequest
		want    *adminv2.MachineServiceSetStateResponse
		wantErr error
	}{
		{
			name: "taint a machine",
			req: &adminv2.MachineServiceSetStateRequest{
				Uuid:        sc.Machine1,
				State:       apiv2.MachineState_MACHINE_STATE_TAINTED,
				Description: "tainted during test",
			},
			want: &adminv2.MachineServiceSetStateResponse{
				Machine: &apiv2.Machine{
					Uuid:                     sc.Machine1,
					Allocation:               &apiv2.MachineAllocation{},
					Hardware:                 &apiv2.MachineHardware{},
					Meta:                     &apiv2.Meta{Generation: 1},
					Partition:                &apiv2.Partition{},
					Size:                     &apiv2.Size{},
					RecentProvisioningEvents: &apiv2.MachineRecentProvisioningEvents{},
					Status: &apiv2.MachineStatus{
						Condition: &apiv2.MachineCondition{
							State:       apiv2.MachineState_MACHINE_STATE_TAINTED,
							Description: "tainted during test",
							Issuer:      "unknown issuer",
						},
						LedState:   &apiv2.MachineChassisIdentifyLEDState{},
						Liveliness: apiv2.MachineLiveliness_MACHINE_LIVELINESS_ALIVE,
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "untaint a tainted machine",
			req: &adminv2.MachineServiceSetStateRequest{
				Uuid:  sc.Machine1,
				State: apiv2.MachineState_MACHINE_STATE_AVAILABLE,
			},
			want: &adminv2.MachineServiceSetStateResponse{
				Machine: &apiv2.Machine{
					Uuid:                     sc.Machine1,
					Allocation:               &apiv2.MachineAllocation{},
					Hardware:                 &apiv2.MachineHardware{},
					Meta:                     &apiv2.Meta{Generation: 2},
					Partition:                &apiv2.Partition{},
					Size:                     &apiv2.Size{},
					RecentProvisioningEvents: &apiv2.MachineRecentProvisioningEvents{},
					Status: &apiv2.MachineStatus{
						Condition: &apiv2.MachineCondition{
							State:       apiv2.MachineState_MACHINE_STATE_AVAILABLE,
							Description: "",
							Issuer:      "unknown issuer",
						},
						LedState:   &apiv2.MachineChassisIdentifyLEDState{},
						Liveliness: apiv2.MachineLiveliness_MACHINE_LIVELINESS_ALIVE,
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "lock a machine",
			req: &adminv2.MachineServiceSetStateRequest{
				Uuid:        sc.Machine1,
				State:       apiv2.MachineState_MACHINE_STATE_LOCKED,
				Description: "Locked during test",
			},
			want: &adminv2.MachineServiceSetStateResponse{
				Machine: &apiv2.Machine{
					Uuid:                     sc.Machine1,
					Allocation:               &apiv2.MachineAllocation{},
					Hardware:                 &apiv2.MachineHardware{},
					Meta:                     &apiv2.Meta{Generation: 3},
					Partition:                &apiv2.Partition{},
					Size:                     &apiv2.Size{},
					RecentProvisioningEvents: &apiv2.MachineRecentProvisioningEvents{},
					Status: &apiv2.MachineStatus{
						Condition: &apiv2.MachineCondition{
							State:       apiv2.MachineState_MACHINE_STATE_LOCKED,
							Description: "Locked during test",
							Issuer:      "unknown issuer",
						},
						LedState:   &apiv2.MachineChassisIdentifyLEDState{},
						Liveliness: apiv2.MachineLiveliness_MACHINE_LIVELINESS_ALIVE,
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "taint a locked a machine",
			req: &adminv2.MachineServiceSetStateRequest{
				Uuid:        sc.Machine5,
				State:       apiv2.MachineState_MACHINE_STATE_TAINTED,
				Description: "Locked during test",
			},
			want:    nil,
			wantErr: errorutil.FailedPrecondition(`machine is currently "locked", must made available first`),
		},
		{
			name: "unlock a locked a machine",
			req: &adminv2.MachineServiceSetStateRequest{
				Uuid:  sc.Machine5,
				State: apiv2.MachineState_MACHINE_STATE_AVAILABLE,
			},
			want: &adminv2.MachineServiceSetStateResponse{
				Machine: &apiv2.Machine{
					Uuid:                     sc.Machine5,
					Allocation:               &apiv2.MachineAllocation{},
					Hardware:                 &apiv2.MachineHardware{},
					Meta:                     &apiv2.Meta{Generation: 1},
					Partition:                &apiv2.Partition{},
					Size:                     &apiv2.Size{},
					RecentProvisioningEvents: &apiv2.MachineRecentProvisioningEvents{},
					Status: &apiv2.MachineStatus{
						Condition: &apiv2.MachineCondition{
							State:       apiv2.MachineState_MACHINE_STATE_AVAILABLE,
							Description: "",
							Issuer:      "unknown issuer",
						},
						LedState:   &apiv2.MachineChassisIdentifyLEDState{},
						Liveliness: apiv2.MachineLiveliness_MACHINE_LIVELINESS_ALIVE,
					},
				},
			},
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &machineServiceServer{
				log:  log,
				repo: dc.GetTestStore().Store,
			}
			got, err := m.SetState(t.Context(), tt.req)
			if diff := cmp.Diff(err, tt.wantErr, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("diff = %s", diff)
			}
			if diff := cmp.Diff(
				tt.want, got,
				protocmp.Transform(),
				protocmp.IgnoreFields(
					&apiv2.Machine{}, "size", "partition", "allocation", "recent_provisioning_events",
				),
				protocmp.IgnoreFields(
					&apiv2.Meta{}, "created_at", "updated_at",
				),
			); diff != "" {
				t.Errorf("machineServiceServer.SetState() = %v, want %vņdiff: %s", got, tt.want, diff)
			}
		})
	}
}
