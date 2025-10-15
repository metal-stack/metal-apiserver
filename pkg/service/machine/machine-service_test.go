package machine

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"connectrpc.com/connect"
	"github.com/google/go-cmp/cmp"
	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	"github.com/metal-stack/metal-apiserver/pkg/test"
	"github.com/metal-stack/metal-lib/pkg/pointer"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var (
	m1 = "00000000-0000-0000-0000-000000000001"
	m2 = "00000000-0000-0000-0000-000000000002"
	m3 = "00000000-0000-0000-0000-000000000003"
	m4 = "00000000-0000-0000-0000-000000000004"
	m5 = "00000000-0000-0000-0000-000000000005"

	p1 = "00000000-0000-0000-0000-000000000001"
	p2 = "00000000-0000-0000-0000-000000000002"
)

func Test_machineServiceServer_Get(t *testing.T) {
	t.Parallel()

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
	test.CreateImages(t, repo, []*adminv2.ImageServiceCreateRequest{
		{Image: &apiv2.Image{Id: "debian-12", Url: validURL, Features: []apiv2.ImageFeature{apiv2.ImageFeature_IMAGE_FEATURE_MACHINE}}},
	})

	// We need to create machines directly on the database because there is no MachineCreateRequest available and never will.
	// Once the boot-service is available we can simulate a pxe booting machine the actually create a machine from the api level.
	test.CreateMachines(t, testStore, []*metal.Machine{
		{Base: metal.Base{ID: m1}, PartitionID: "partition-1", SizeID: "c1-large-x86", Allocation: &metal.MachineAllocation{Project: p1, ImageID: "debian-12"}},
	})

	tests := []struct {
		name    string
		rq      *apiv2.MachineServiceGetRequest
		want    *apiv2.MachineServiceGetResponse
		wantErr error
	}{
		{
			name: "get existing",
			rq:   &apiv2.MachineServiceGetRequest{Uuid: m1, Project: p1},
			want: &apiv2.MachineServiceGetResponse{
				Machine: &apiv2.Machine{
					Uuid:      m1,
					Meta:      &apiv2.Meta{},
					Partition: &apiv2.Partition{Id: "partition-1", BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}, Meta: &apiv2.Meta{}},
					Bios:      &apiv2.MachineBios{},
					Hardware:  &apiv2.MachineHardware{},
					Size:      &apiv2.Size{Id: "c1-large-x86", Meta: &apiv2.Meta{}},
					RecentProvisioningEvents: &apiv2.MachineRecentProvisioningEvents{
						Events: []*apiv2.MachineProvisioningEvent{{Event: apiv2.MachineProvisioningEventType_MACHINE_PROVISIONING_EVENT_TYPE_ALIVE, Message: "machine created for test"}},
					},
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
			wantErr: nil,
		},
		{
			name:    "get existing with wrong project",
			rq:      &apiv2.MachineServiceGetRequest{Uuid: m1, Project: p2},
			want:    nil,
			wantErr: errorutil.NotFound(`*metal.Machine with id %q not found`, m1),
		},
		{
			name:    "get non existing",
			rq:      &apiv2.MachineServiceGetRequest{Uuid: "m99"},
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
			got, err := m.Get(ctx, connect.NewRequest(tt.rq))
			if diff := cmp.Diff(err, tt.wantErr, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("diff = %s", diff)
			}

			if diff := cmp.Diff(
				tt.want, pointer.SafeDeref(got).Msg,
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
				t.Errorf("machineServiceServer.Get() = %v, want %v diff: %s", got.Msg, tt.want, diff)
			}
		})
	}
}

func Test_machineServiceServer_List(t *testing.T) {
	t.Parallel()

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
		{Base: metal.Base{ID: m2}, PartitionID: "partition-1", SizeID: "c1-medium-x86"},
		{Base: metal.Base{ID: m3}, PartitionID: "partition-1", SizeID: "c1-large-x86", Allocation: &metal.MachineAllocation{Project: p1, ImageID: "debian-12"}},
		{Base: metal.Base{ID: m4}, PartitionID: "partition-1", SizeID: "c1-large-x86", Allocation: &metal.MachineAllocation{Project: p2, ImageID: "debian-12"}},
		{Base: metal.Base{ID: m5}, PartitionID: "partition-1", SizeID: "c1-large-x86", Allocation: &metal.MachineAllocation{Project: p2, ImageID: "debian-12"}},
	})

	tests := []struct {
		name    string
		rq      *apiv2.MachineServiceListRequest
		want    *apiv2.MachineServiceListResponse
		wantErr error
	}{
		{
			name: "List from p1",
			rq:   &apiv2.MachineServiceListRequest{Project: p1},
			want: &apiv2.MachineServiceListResponse{
				Machines: []*apiv2.Machine{
					{
						Uuid:      m3,
						Meta:      &apiv2.Meta{},
						Partition: &apiv2.Partition{Id: "partition-1", BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}, Meta: &apiv2.Meta{}},
						Bios:      &apiv2.MachineBios{},
						Hardware:  &apiv2.MachineHardware{},
						Size:      &apiv2.Size{Id: "c1-large-x86", Meta: &apiv2.Meta{}, Constraints: []*apiv2.SizeConstraint{{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_CORES, Min: 8, Max: 8}}},
						RecentProvisioningEvents: &apiv2.MachineRecentProvisioningEvents{
							Events: []*apiv2.MachineProvisioningEvent{{Event: apiv2.MachineProvisioningEventType_MACHINE_PROVISIONING_EVENT_TYPE_ALIVE, Message: "machine created for test"}},
						},
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
			rq:   &apiv2.MachineServiceListRequest{Project: p2, Query: &apiv2.MachineQuery{Uuid: pointer.Pointer(m4)}},
			want: &apiv2.MachineServiceListResponse{
				Machines: []*apiv2.Machine{
					{
						Uuid:      m4,
						Meta:      &apiv2.Meta{},
						Partition: &apiv2.Partition{Id: "partition-1", BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}, Meta: &apiv2.Meta{}},
						Bios:      &apiv2.MachineBios{},
						Hardware:  &apiv2.MachineHardware{},
						Size:      &apiv2.Size{Id: "c1-large-x86", Meta: &apiv2.Meta{}, Constraints: []*apiv2.SizeConstraint{{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_CORES, Min: 8, Max: 8}}},
						RecentProvisioningEvents: &apiv2.MachineRecentProvisioningEvents{
							Events: []*apiv2.MachineProvisioningEvent{{Event: apiv2.MachineProvisioningEventType_MACHINE_PROVISIONING_EVENT_TYPE_ALIVE, Message: "machine created for test"}},
						},
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
			got, err := m.List(ctx, connect.NewRequest(tt.rq))
			if diff := cmp.Diff(err, tt.wantErr, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("diff = %s", diff)
			}

			if diff := cmp.Diff(
				tt.want, pointer.SafeDeref(got).Msg,
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
				t.Errorf("machineServiceServer.List() = %v, want %v diff: %s", got.Msg, tt.want, diff)
			}
		})
	}
}

func Test_machineServiceServer_Update(t *testing.T) {
	t.Parallel()

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
	machineMap := test.CreateMachines(t, testStore, []*metal.Machine{
		{
			Base: metal.Base{ID: m1}, PartitionID: "partition-1", SizeID: "c1-medium-x86",
		},
		{
			Base: metal.Base{ID: m2}, PartitionID: "partition-1", SizeID: "c1-medium-x86",
		},
		{
			Base: metal.Base{ID: m3}, PartitionID: "partition-1", SizeID: "c1-medium-x86",
			Allocation: &metal.MachineAllocation{Project: p1, ImageID: "debian-12"},
		},
		{
			Base: metal.Base{ID: m4}, PartitionID: "partition-1", SizeID: "c1-medium-x86",
			Allocation: &metal.MachineAllocation{
				Description: "my-machine",
				Project:     p2,
				ImageID:     "debian-12",
				SSHPubKeys:  []string{"key-1", "key-2"},
			},
		},
		{
			Base: metal.Base{ID: m5}, PartitionID: "partition-1", SizeID: "c1-medium-x86",
			Allocation: &metal.MachineAllocation{Project: p2, ImageID: "debian-12"},
		},
	})

	tests := []struct {
		name    string
		rq      *apiv2.MachineServiceUpdateRequest
		want    *apiv2.MachineServiceUpdateResponse
		wantErr error
	}{
		{
			name:    "update without allocation",
			rq:      &apiv2.MachineServiceUpdateRequest{Uuid: m1, UpdateMeta: &apiv2.UpdateMeta{}},
			want:    nil,
			wantErr: errorutil.InvalidArgument("only allocated machines can be updated"),
		},
		{
			name: "Update tags",
			rq: &apiv2.MachineServiceUpdateRequest{
				Uuid: m3,
				UpdateMeta: &apiv2.UpdateMeta{
					UpdatedAt: timestamppb.New(machineMap[m3].Changed),
				},
				Project: p1,
				Labels: &apiv2.UpdateLabels{
					Update: &apiv2.Labels{Labels: map[string]string{"color": "red"}},
				}},
			want: &apiv2.MachineServiceUpdateResponse{
				Machine: &apiv2.Machine{
					Uuid:      m3,
					Meta:      &apiv2.Meta{Generation: 1},
					Partition: &apiv2.Partition{Id: "partition-1", BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}, Meta: &apiv2.Meta{}},
					Bios:      &apiv2.MachineBios{},
					Hardware:  &apiv2.MachineHardware{},
					Size:      &apiv2.Size{Id: "c1-medium-x86", Meta: &apiv2.Meta{}, Constraints: []*apiv2.SizeConstraint{{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_CORES, Min: 4, Max: 4}}},
					RecentProvisioningEvents: &apiv2.MachineRecentProvisioningEvents{
						Events: []*apiv2.MachineProvisioningEvent{{Event: apiv2.MachineProvisioningEventType_MACHINE_PROVISIONING_EVENT_TYPE_ALIVE, Message: "machine created for test"}},
					},
					Status: &apiv2.MachineStatus{
						Condition:  &apiv2.MachineCondition{},
						LedState:   &apiv2.MachineChassisIdentifyLEDState{},
						Liveliness: apiv2.MachineLiveliness_MACHINE_LIVELINESS_ALIVE,
					},
					Allocation: &apiv2.MachineAllocation{
						Project: p1,
						Meta: &apiv2.Meta{
							Labels: &apiv2.Labels{
								Labels: map[string]string{"color": "red"},
							},
						}, Image: &apiv2.Image{
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
			wantErr: nil,
		},
		{
			name: "Update Description and ssh public key",
			rq: &apiv2.MachineServiceUpdateRequest{
				Uuid: m4, Project: p2,
				UpdateMeta: &apiv2.UpdateMeta{
					UpdatedAt: timestamppb.New(machineMap[m4].Changed),
				},
				Description:   pointer.Pointer("my-beloved-machine"),
				SshPublicKeys: []string{"key-2", "key-3"},
			},
			want: &apiv2.MachineServiceUpdateResponse{
				Machine: &apiv2.Machine{
					Uuid:      m4,
					Meta:      &apiv2.Meta{Generation: 1},
					Partition: &apiv2.Partition{Id: "partition-1", BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}, Meta: &apiv2.Meta{}},
					Bios:      &apiv2.MachineBios{},
					Hardware:  &apiv2.MachineHardware{},
					Size:      &apiv2.Size{Id: "c1-medium-x86", Meta: &apiv2.Meta{}, Constraints: []*apiv2.SizeConstraint{{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_CORES, Min: 4, Max: 4}}},
					RecentProvisioningEvents: &apiv2.MachineRecentProvisioningEvents{
						Events: []*apiv2.MachineProvisioningEvent{{Event: apiv2.MachineProvisioningEventType_MACHINE_PROVISIONING_EVENT_TYPE_ALIVE, Message: "machine created for test"}},
					},
					Status: &apiv2.MachineStatus{
						Condition:  &apiv2.MachineCondition{},
						LedState:   &apiv2.MachineChassisIdentifyLEDState{},
						Liveliness: apiv2.MachineLiveliness_MACHINE_LIVELINESS_ALIVE,
					},
					Allocation: &apiv2.MachineAllocation{
						Project:       p2,
						Meta:          &apiv2.Meta{},
						Description:   "my-beloved-machine",
						SshPublicKeys: []string{"key-2", "key-3"},
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
			got, err := m.Update(ctx, connect.NewRequest(tt.rq))
			if diff := cmp.Diff(err, tt.wantErr, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("diff = %s", diff)
			}

			if diff := cmp.Diff(
				tt.want, pointer.SafeDeref(got).Msg,
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
				t.Errorf("machineServiceServer.Update() = %v, want %v diff: %s", got.Msg, tt.want, diff)
			}
		})
	}
}
