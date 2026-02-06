package admin

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/hibiken/asynq"
	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	"github.com/metal-stack/metal-apiserver/pkg/test"
	"github.com/metal-stack/metal-lib/pkg/pointer"
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
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	testStore, closer := test.StartRepositoryWithCleanup(t, log)
	defer closer()
	repo := testStore.Store

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintln(w, "a image")
	}))

	validURL := ts.URL
	defer ts.Close()

	ctx := t.Context()

	test.CreateTenants(t, testStore, []*apiv2.TenantServiceCreateRequest{{Name: "t1"}})
	test.CreateProjects(t, repo, []*apiv2.ProjectServiceCreateRequest{{Name: p1, Login: "t1"}, {Name: p2, Login: "t1"}})
	test.CreatePartitions(t, repo, []*adminv2.PartitionServiceCreateRequest{
		{
			Partition: &apiv2.Partition{Id: "partition-1", BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}},
		},
	})
	test.CreateSizes(t, repo, []*adminv2.SizeServiceCreateRequest{
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
						Condition:  &apiv2.MachineCondition{},
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
				repo: repo,
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
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	testStore, closer := test.StartRepositoryWithCleanup(t, log)
	defer closer()
	repo := testStore.Store

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintln(w, "a image")
	}))

	validURL := ts.URL
	defer ts.Close()

	ctx := t.Context()

	test.CreateTenants(t, testStore, []*apiv2.TenantServiceCreateRequest{{Name: "t1"}})
	test.CreateProjects(t, repo, []*apiv2.ProjectServiceCreateRequest{{Name: p1, Login: "t1"}, {Name: p2, Login: "t1"}})
	test.CreatePartitions(t, repo, []*adminv2.PartitionServiceCreateRequest{
		{
			Partition: &apiv2.Partition{Id: "partition-1", BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}},
		},
	})
	test.CreateImages(t, repo, []*adminv2.ImageServiceCreateRequest{
		{Image: &apiv2.Image{Id: "debian-12", Url: validURL, Features: []apiv2.ImageFeature{apiv2.ImageFeature_IMAGE_FEATURE_MACHINE}}},
	})
	test.CreateSizes(t, repo, []*adminv2.SizeServiceCreateRequest{
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
			rq:   &adminv2.MachineServiceListRequest{Query: &apiv2.MachineQuery{Allocation: &apiv2.MachineAllocationQuery{Project: pointer.Pointer(p1)}}},
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
							Condition:  &apiv2.MachineCondition{},
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
								Description:    pointer.Pointer(""),
								Name:           pointer.Pointer(""),
							},
						},
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "list from p2",
			rq:   &adminv2.MachineServiceListRequest{Query: &apiv2.MachineQuery{Uuid: pointer.Pointer(m4), Allocation: &apiv2.MachineAllocationQuery{Project: pointer.Pointer(p2)}}},
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
							Condition:  &apiv2.MachineCondition{},
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
								Description:    pointer.Pointer(""),
								Name:           pointer.Pointer(""),
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
				repo: repo,
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
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	testStore, closer := test.StartRepositoryWithCleanup(t, log)
	defer closer()
	repo := testStore.Store

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintln(w, "a image")
	}))

	validURL := ts.URL
	defer ts.Close()

	ctx := t.Context()

	test.CreateTenants(t, testStore, []*apiv2.TenantServiceCreateRequest{{Name: "t1"}})
	test.CreateProjects(t, repo, []*apiv2.ProjectServiceCreateRequest{{Name: p1, Login: "t1"}, {Name: p2, Login: "t1"}})
	test.CreatePartitions(t, repo, []*adminv2.PartitionServiceCreateRequest{
		{
			Partition: &apiv2.Partition{Id: "partition-1", BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}},
		},
	})
	test.CreateImages(t, repo, []*adminv2.ImageServiceCreateRequest{
		{Image: &apiv2.Image{Id: "debian-12", Url: validURL, Features: []apiv2.ImageFeature{apiv2.ImageFeature_IMAGE_FEATURE_MACHINE}}},
	})
	test.CreateSizes(t, repo, []*adminv2.SizeServiceCreateRequest{
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
			wantErr: errorutil.FailedPrecondition("machine 00000000-0000-0000-0000-000000000001 does not have bmc connections details yet"),
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
				repo: repo,
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
