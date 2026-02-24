package machine

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/google/go-cmp/cmp"
	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	"github.com/metal-stack/metal-apiserver/pkg/test"
	sc "github.com/metal-stack/metal-apiserver/pkg/test/scenarios"
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
	test.CreateImages(t, testStore, []*adminv2.ImageServiceCreateRequest{
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
					Uuid:                     m1,
					Meta:                     &apiv2.Meta{},
					Partition:                &apiv2.Partition{Id: "partition-1", BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}, Meta: &apiv2.Meta{}},
					Hardware:                 &apiv2.MachineHardware{},
					Size:                     &apiv2.Size{Id: "c1-large-x86", Meta: &apiv2.Meta{}},
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
							Description:    new(""),
							Name:           new(""),
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
					&apiv2.Image{}, "expires_at",
				),
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
			rq:   &apiv2.MachineServiceListRequest{Project: p2, Query: &apiv2.MachineQuery{Uuid: new(m4)}},
			want: &apiv2.MachineServiceListResponse{
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

func Test_machineServiceServer_Update(t *testing.T) {
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
					Uuid:                     m3,
					Meta:                     &apiv2.Meta{Generation: 1},
					Partition:                &apiv2.Partition{Id: "partition-1", BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}, Meta: &apiv2.Meta{}},
					Hardware:                 &apiv2.MachineHardware{},
					Size:                     &apiv2.Size{Id: "c1-medium-x86", Meta: &apiv2.Meta{}, Constraints: []*apiv2.SizeConstraint{{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_CORES, Min: 4, Max: 4}}},
					RecentProvisioningEvents: &apiv2.MachineRecentProvisioningEvents{},
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
							Description:    new(""),
							Name:           new(""),
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
				Description:   new("my-beloved-machine"),
				SshPublicKeys: []string{"key-2", "key-3"},
			},
			want: &apiv2.MachineServiceUpdateResponse{
				Machine: &apiv2.Machine{
					Uuid:                     m4,
					Meta:                     &apiv2.Meta{Generation: 1},
					Partition:                &apiv2.Partition{Id: "partition-1", BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}, Meta: &apiv2.Meta{}},
					Hardware:                 &apiv2.MachineHardware{},
					Size:                     &apiv2.Size{Id: "c1-medium-x86", Meta: &apiv2.Meta{}, Constraints: []*apiv2.SizeConstraint{{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_CORES, Min: 4, Max: 4}}},
					RecentProvisioningEvents: &apiv2.MachineRecentProvisioningEvents{},
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
							Description:    new(""),
							Name:           new(""),
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
			got, err := m.Update(ctx, tt.rq)
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
				t.Errorf("machineServiceServer.Update() = %v, want %v diff: %s", got, tt.want, diff)
			}
		})
	}
}

func Test_machineServiceServer_ValidateCreate(t *testing.T) {
	t.Parallel()

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	ctx := t.Context()

	dc := test.NewDatacenter(t, log)
	dc.Create(&sc.DefaultDatacenter)
	defer dc.Close()

	tests := []struct {
		name                         string
		req                          *apiv2.MachineServiceCreateRequest
		createDatacenterFn           func() *sc.DatacenterSpec
		createDatacenterAndRequestFn func() *apiv2.MachineServiceCreateRequest
		want                         *apiv2.MachineServiceCreateResponse
		wantErr                      error
	}{
		{
			name:    "no project given",
			req:     &apiv2.MachineServiceCreateRequest{},
			want:    nil,
			wantErr: errorutil.NotFound("get of project with id "),
		},
		{
			name:    "project does not exist",
			req:     &apiv2.MachineServiceCreateRequest{Project: "abc"},
			want:    nil,
			wantErr: errorutil.NotFound("get of project with id abc"),
		},
		{
			name:    "partition does not exist",
			req:     &apiv2.MachineServiceCreateRequest{Project: sc.Tenant1Project1, Partition: "non-existing-partition"},
			want:    nil,
			wantErr: errorutil.NotFound(`no partition with id "non-existing-partition" found`),
		},
		{
			name:    "size does not exist",
			req:     &apiv2.MachineServiceCreateRequest{Project: sc.Tenant1Project1, Partition: sc.Partition1, Size: "unknown-size"},
			want:    nil,
			wantErr: errorutil.NotFound(`no size with id "unknown-size" found`),
		},
		// UUID is specified
		{
			name: "uuid is specified, but partition is given",
			req: &apiv2.MachineServiceCreateRequest{
				Uuid:      new(sc.Machine1),
				Project:   sc.Tenant1Project1,
				Partition: sc.Partition1,
			},
			want:    nil,
			wantErr: errorutil.InvalidArgument("when machine id is given, a partition must not be specified"),
		},
		{
			name: "uuid is specified, but size is given",
			req: &apiv2.MachineServiceCreateRequest{
				Uuid:    new(sc.Machine1),
				Project: sc.Tenant1Project1,
				Size:    sc.SizeC1Large,
			},
			want:    nil,
			wantErr: errorutil.InvalidArgument("when machine id is given, a size must not be specified"),
		},
		{
			name: "uuid is specified, but machine is allocated",
			req: &apiv2.MachineServiceCreateRequest{
				Uuid:    new(sc.Machine1),
				Project: sc.Tenant1Project1,
			},
			want:    nil,
			wantErr: errorutil.InvalidArgument("machine 00000000-0000-0000-0000-000000000001 is already allocated"),
		},
		{
			name: "uuid is specified, but machine is locked",
			req: &apiv2.MachineServiceCreateRequest{
				Uuid:    new(sc.Machine2),
				Project: sc.Tenant1Project1,
			},
			createDatacenterFn: func() *sc.DatacenterSpec {
				testDC := sc.DefaultDatacenter
				testDC.Machines = []*sc.MachineWithLiveliness{
					sc.MachineFunc(sc.Machine2, sc.Partition1, sc.SizeN1Medium, "", metal.MachineLivelinessAlive),
				}
				testDC.Machines[0].Machine.State.Value = metal.LockedState
				return &testDC
			},
			want:    nil,
			wantErr: errorutil.InvalidArgument("machine 00000000-0000-0000-0000-000000000002 is LOCKED"),
		},
		{
			name: "uuid is specified, but machine is reserved",
			req: &apiv2.MachineServiceCreateRequest{
				Uuid:    new(sc.Machine2),
				Project: sc.Tenant1Project1,
			},
			createDatacenterFn: func() *sc.DatacenterSpec {
				testDC := sc.DefaultDatacenter
				testDC.Machines = []*sc.MachineWithLiveliness{
					sc.MachineFunc(sc.Machine2, sc.Partition1, sc.SizeN1Medium, "", metal.MachineLivelinessAlive),
				}
				testDC.Machines[0].Machine.State.Value = metal.ReservedState
				return &testDC
			},
			want:    nil,
			wantErr: errorutil.InvalidArgument("machine 00000000-0000-0000-0000-000000000002 is RESERVED"),
		},
		{
			name: "uuid is specified, but machine is not waiting",
			req: &apiv2.MachineServiceCreateRequest{
				Uuid:    new(sc.Machine2),
				Project: sc.Tenant1Project1,
			},
			createDatacenterFn: func() *sc.DatacenterSpec {
				testDC := sc.DefaultDatacenter
				testDC.Machines = []*sc.MachineWithLiveliness{
					sc.MachineFunc(sc.Machine2, sc.Partition1, sc.SizeN1Medium, "", metal.MachineLivelinessAlive),
				}
				testDC.Machines[0].Machine.Waiting = false
				return &testDC
			},
			want:    nil,
			wantErr: errorutil.InvalidArgument("machine 00000000-0000-0000-0000-000000000002 is not waiting"),
		},
		// UUID is not specified
		{
			name: "partition is not present",
			req: &apiv2.MachineServiceCreateRequest{
				Project:   sc.Tenant1Project1,
				Partition: sc.Partition2,
			},
			want:    nil,
			wantErr: errorutil.NotFound(`no partition with id "partition-2" found`),
		},
		{
			name: "size is not present",
			req: &apiv2.MachineServiceCreateRequest{
				Project:   sc.Tenant1Project1,
				Partition: sc.Partition1,
				Size:      "unknown-size",
			},
			want:    nil,
			wantErr: errorutil.NotFound(`no size with id "unknown-size" found`),
		},
		{
			name: "image is not present",
			req: &apiv2.MachineServiceCreateRequest{
				Project:   sc.Tenant1Project1,
				Partition: sc.Partition1,
				Size:      sc.SizeC1Large,
				Image:     "unknown-11",
			},
			want:    nil,
			wantErr: errorutil.NotFound(`no image for os:unknown version:11.0.0 found`),
		},
		{
			name: "fsl is given but does not exists",
			req: &apiv2.MachineServiceCreateRequest{
				Project:          sc.Tenant1Project1,
				Partition:        sc.Partition1,
				Size:             sc.SizeC1Large,
				FilesystemLayout: new("debian-fsl"),
				Image:            "debian-13",
			},
			want:    nil,
			wantErr: errorutil.NotFound(`no filesystemlayout with id "debian-fsl" found`),
		},
		{
			name: "uuid and fsl is given but does not match hardware",
			req: &apiv2.MachineServiceCreateRequest{
				Uuid:             new(sc.Machine1),
				Project:          sc.Tenant1Project1,
				FilesystemLayout: new("debian-13"),
				Image:            "debian-13",
			},
			createDatacenterFn: func() *sc.DatacenterSpec {
				testDC := sc.DefaultDatacenter
				testDC.Machines = []*sc.MachineWithLiveliness{
					sc.MachineFunc(sc.Machine1, sc.Partition1, sc.SizeN1Medium, "", metal.MachineLivelinessAlive),
				}
				testDC.Machines[0].Machine.Waiting = true
				testDC.Machines[0].Machine.Hardware = metal.MachineHardware{
					Disks: []metal.BlockDevice{
						{Name: "/dev/sdb"},
					},
				}
				testDC.FilesystemLayouts = []*adminv2.FilesystemServiceCreateRequest{
					{
						FilesystemLayout: &apiv2.FilesystemLayout{
							Id: "debian-13",
							Constraints: &apiv2.FilesystemLayoutConstraints{
								Sizes: []string{sc.SizeN1Medium},
								Images: map[string]string{
									"debian": ">= 12.0",
								},
							},
							Disks: []*apiv2.Disk{
								{
									Device: "/dev/sda",
								},
							},
						},
					},
				}

				return &testDC
			},
			want:    nil,
			wantErr: errorutil.Internal(`device:/dev/sda does not exist on given hardware`), // TODO InvalidArgument
		},
		{
			name: "no fsl is given and no matching one found",
			req: &apiv2.MachineServiceCreateRequest{
				Project:   sc.Tenant1Project1,
				Partition: sc.Partition1,
				Size:      sc.SizeC1Large,
				Image:     "debian-11",
			},
			want:    nil,
			wantErr: errorutil.InvalidArgument(`could not find a matching filesystemLayout for size:c1-large-x86 and image:debian-11.0.20241220`),
		},
		{
			name: "no fsl is given but present, but no match for image and size",
			req: &apiv2.MachineServiceCreateRequest{
				Project:   sc.Tenant1Project1,
				Partition: sc.Partition1,
				Size:      sc.SizeC1Large,
				Image:     "debian-11",
			},
			createDatacenterFn: func() *sc.DatacenterSpec {
				return &sc.DefaultDatacenter
			},
			want:    nil,
			wantErr: errorutil.InvalidArgument(`could not find a matching filesystemLayout for size:c1-large-x86 and image:debian-11.0.20241220`),
		},
		// Wrong Allocation Types
		{
			name: "allocation type wrong",
			req: &apiv2.MachineServiceCreateRequest{
				Project:        sc.Tenant1Project1,
				Partition:      sc.Partition1,
				Size:           sc.SizeC1Large,
				Image:          "debian-13",
				AllocationType: 0,
			},
			want:    nil,
			wantErr: errorutil.InvalidArgument(`given allocationtype MACHINE_ALLOCATION_TYPE_UNSPECIFIED is not supported`),
		},
		{
			name: "machine with firewall rules",
			req: &apiv2.MachineServiceCreateRequest{
				Project:        sc.Tenant1Project1,
				Partition:      sc.Partition1,
				Size:           sc.SizeC1Large,
				Image:          "debian-13",
				AllocationType: apiv2.MachineAllocationType_MACHINE_ALLOCATION_TYPE_MACHINE,
				FirewallSpec:   &apiv2.FirewallSpec{},
			},
			createDatacenterFn: func() *sc.DatacenterSpec {
				return &sc.DefaultDatacenter
			},
			want:    nil,
			wantErr: errorutil.InvalidArgument(`firewall rules can only be specified on firewalls`),
		},
		// Networks
		{
			name: "machine with no networks",
			req: &apiv2.MachineServiceCreateRequest{
				Project:        sc.Tenant1Project1,
				Partition:      sc.Partition1,
				Size:           sc.SizeC1Large,
				Image:          "debian-13",
				AllocationType: apiv2.MachineAllocationType_MACHINE_ALLOCATION_TYPE_MACHINE,
			},
			createDatacenterFn: func() *sc.DatacenterSpec {
				return &sc.DefaultDatacenter
			},
			want:    nil,
			wantErr: errorutil.InvalidArgument(`networks must not be empty`),
		},
		{
			name: "machine with unknown networks",
			req: &apiv2.MachineServiceCreateRequest{
				Project:        sc.Tenant1Project1,
				Partition:      sc.Partition1,
				Size:           sc.SizeC1Large,
				Image:          "debian-13",
				AllocationType: apiv2.MachineAllocationType_MACHINE_ALLOCATION_TYPE_MACHINE,
				Networks: []*apiv2.MachineAllocationNetwork{
					{Network: "no-internet"},
				},
			},
			createDatacenterFn: func() *sc.DatacenterSpec {
				return &sc.DefaultDatacenter
			},
			want:    nil,
			wantErr: errorutil.NotFound(`no network with id "no-internet" found`),
		},
		{
			name: "machine with private network in wrong network",
			req:  nil, // set below
			createDatacenterAndRequestFn: func() *apiv2.MachineServiceCreateRequest {
				testDC := sc.DefaultDatacenter
				testDC.ProjectsPerTenant = 2
				testDC.FilesystemLayouts = []*adminv2.FilesystemServiceCreateRequest{
					{
						FilesystemLayout: &apiv2.FilesystemLayout{
							Id: "debian",
							Constraints: &apiv2.FilesystemLayoutConstraints{
								Sizes: []string{sc.SizeC1Large},
								Images: map[string]string{
									"debian": ">= 12.0",
								},
							},
						},
					},
				}
				testDC.Networks = append(testDC.Networks, &adminv2.NetworkServiceCreateRequest{
					Name:      new("project network"),
					Project:   new(sc.Tenant1Project1),
					Partition: new(sc.Partition1),
					Type:      apiv2.NetworkType_NETWORK_TYPE_CHILD,
				})
				dc.Create(&testDC)

				projectNetworkId := dc.Networks["project network"].Id
				req := &apiv2.MachineServiceCreateRequest{
					Project:        sc.Tenant1Project2,
					Partition:      sc.Partition1,
					Size:           sc.SizeC1Large,
					Image:          "debian-13",
					AllocationType: apiv2.MachineAllocationType_MACHINE_ALLOCATION_TYPE_MACHINE,
					Networks: []*apiv2.MachineAllocationNetwork{
						{Network: sc.NetworkInternet},
						{Network: projectNetworkId},
					},
				}
				return req
			},

			want:    nil,
			wantErr: errorutil.NotFound(`no network with id "no-internet" found`),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.createDatacenterFn != nil {
				dc.CleanUp()
				dc.Create(tt.createDatacenterFn())
			}
			if tt.createDatacenterAndRequestFn != nil {
				dc.CleanUp()
				tt.req = tt.createDatacenterAndRequestFn()
			}

			m := &machineServiceServer{
				log:  log,
				repo: dc.TestStore.Store,
			}
			if tt.wantErr == nil {
				test.Validate(t, tt.req)
			}
			got, err := m.Create(ctx, tt.req)
			if diff := cmp.Diff(err, tt.wantErr, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("diff = %s", diff)
			}

			if diff := cmp.Diff(
				tt.want, got,
				protocmp.Transform(),
				protocmp.IgnoreFields(
					&apiv2.Meta{}, "created_at", "updated_at",
				),
			); diff != "" {
				t.Errorf("machineServiceServer.Create() = %v, want %v diff: %s", got, tt.want, diff)
			}
		})
	}
}
