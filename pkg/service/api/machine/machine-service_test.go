package machine

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/avast/retry-go/v4"
	"golang.org/x/crypto/ssh"

	"github.com/google/go-cmp/cmp"
	"github.com/metal-stack/api/go/errorutil"
	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	infrav2 "github.com/metal-stack/api/go/metalstack/infra/v2"
	"github.com/metal-stack/metal-apiserver/pkg/async/task"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	admintask "github.com/metal-stack/metal-apiserver/pkg/service/admin/task"
	"github.com/metal-stack/metal-apiserver/pkg/test"
	sc "github.com/metal-stack/metal-apiserver/pkg/test/scenarios"
	"github.com/stretchr/testify/require"
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

// TODO:
// - convert to datacenter

func Test_machineServiceServer_Get(t *testing.T) {
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
		{ID: m1, PartitionID: "partition-1", SizeID: "c1-large-x86", Allocation: &metal.MachineAllocation{Project: p1, ImageID: "debian-12"}},
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
		{ID: m1, PartitionID: "partition-1", SizeID: "c1-medium-x86"},
		{ID: m2, PartitionID: "partition-1", SizeID: "c1-medium-x86"},
		{ID: m3, PartitionID: "partition-1", SizeID: "c1-large-x86", Allocation: &metal.MachineAllocation{Project: p1, ImageID: "debian-12"}},
		{ID: m4, PartitionID: "partition-1", SizeID: "c1-large-x86", Allocation: &metal.MachineAllocation{Project: p2, ImageID: "debian-12"}},
		{ID: m5, PartitionID: "partition-1", SizeID: "c1-large-x86", Allocation: &metal.MachineAllocation{Project: p2, ImageID: "debian-12"}},
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

func Test_machineServiceServer_Update(t *testing.T) {
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
			ID: m1, PartitionID: "partition-1", SizeID: "c1-medium-x86",
		},
		{
			ID: m2, PartitionID: "partition-1", SizeID: "c1-medium-x86",
		},
		{
			ID: m3, PartitionID: "partition-1", SizeID: "c1-medium-x86",
			Allocation: &metal.MachineAllocation{Project: p1, ImageID: "debian-12"},
		},
		{
			ID: m4, PartitionID: "partition-1", SizeID: "c1-medium-x86",
			Allocation: &metal.MachineAllocation{
				Description: "my-machine",
				Project:     p2,
				ImageID:     "debian-12",
				SSHPubKeys:  []string{"key-1", "key-2"},
			},
		},
		{
			ID: m5, PartitionID: "partition-1", SizeID: "c1-medium-x86",
			Allocation: &metal.MachineAllocation{Project: p2, ImageID: "debian-12"},
		},
	})

	key1 := generateSSHPublicKey(t)
	key2 := generateSSHPublicKey(t)

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
			wantErr: errorutil.FailedPrecondition("only allocated machines can be updated"),
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
					Strategy: &apiv2.UpdateLabels_Patch{
						Patch: &apiv2.LabelsPatch{
							Update: &apiv2.Labels{Labels: map[string]string{"color": "red"}},
						},
					},
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
						Condition:  &apiv2.MachineCondition{State: apiv2.MachineState_MACHINE_STATE_AVAILABLE},
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
				SshPublicKeys: []string{key1, key2},
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
						Condition:  &apiv2.MachineCondition{State: apiv2.MachineState_MACHINE_STATE_AVAILABLE},
						LedState:   &apiv2.MachineChassisIdentifyLEDState{},
						Liveliness: apiv2.MachineLiveliness_MACHINE_LIVELINESS_ALIVE,
					},
					Allocation: &apiv2.MachineAllocation{
						Project:       p2,
						Meta:          &apiv2.Meta{},
						Description:   "my-beloved-machine",
						SshPublicKeys: []string{key1, key2},
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

func generateSSHPublicKey(t *testing.T) string {
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	signer, err := ssh.NewSignerFromKey(privateKey)
	require.NoError(t, err)

	publicKey := signer.PublicKey()
	sshBytes := ssh.MarshalAuthorizedKey(publicKey)

	return strings.TrimSpace(string(sshBytes))
}

func Test_machineServiceServer_GetBMC(t *testing.T) {
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
		{
			ID: m1, PartitionID: "partition-1", RackID: "rack-01", RoomID: "room-01", SizeID: "c1-large-x86",
			Allocation: &metal.MachineAllocation{Project: p1, ImageID: "debian-12"},
			IPMI: metal.IPMI{
				Fru: metal.Fru{
					ChassisPartNumber: "chassis-01",
					BoardPartNumber:   "board-01",
				},
				BMCVersion: "bmc-v1",
			},
			BIOS: metal.BIOS{
				Version: "bios-v1",
				Vendor:  "smc",
			},
		},
	})

	tests := []struct {
		name    string
		rq      *apiv2.MachineServiceGetBMCRequest
		want    *apiv2.MachineServiceGetBMCResponse
		wantErr error
	}{
		{
			name: "get existing",
			rq:   &apiv2.MachineServiceGetBMCRequest{Uuid: m1, Project: p1},
			want: &apiv2.MachineServiceGetBMCResponse{
				BmcDetails: &apiv2.MachineBMCDetails{
					Uuid:      m1,
					Partition: "partition-1",
					Rack:      "rack-01",
					Room:      "room-01",
					Size:      "c1-large-x86",
					BmcReport: &apiv2.MachineBMCReport{
						Bios: &apiv2.MachineBios{
							Version: "bios-v1",
							Vendor:  "smc",
						},
						Bmc: &apiv2.MachineBMC{
							Version: "bmc-v1",
						},
						Fru: &apiv2.MachineFRU{
							ChassisPartNumber:   new("chassis-01"),
							ChassisPartSerial:   new(""),
							BoardPartNumber:     new("board-01"),
							BoardMfg:            new(""),
							BoardMfgSerial:      new(""),
							ProductManufacturer: new(""),
							ProductPartNumber:   new(""),
							ProductSerial:       new(""),
						},
						LedState:  &apiv2.MachineChassisIdentifyLEDState{},
						Uuid:      m1,
						UpdatedAt: timestamppb.New(time.Time{}),
					},
				},
			},
			wantErr: nil,
		},
		{
			name:    "get existing with wrong project",
			rq:      &apiv2.MachineServiceGetBMCRequest{Uuid: m1, Project: p2},
			want:    nil,
			wantErr: errorutil.NotFound(`*metal.Machine with id %q not found`, m1),
		},
		{
			name:    "get non existing",
			rq:      &apiv2.MachineServiceGetBMCRequest{Uuid: "m99"},
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
			got, err := m.GetBMC(ctx, tt.rq)
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

func Test_machineServiceServer_MachineLiveliness(t *testing.T) {
	t.Parallel()

	testStore, closer := test.StartRepositoryWithCleanup(t)
	ctx := t.Context()
	defer closer()

	var (
		recent = time.Now().Add(-1 * time.Minute) // younger than metal.MachineDeadAfter
		old    = time.Now().Add(-1 * time.Hour)   // older than metal.MachineDeadAfter
	)

	test.CreateMachines(t, testStore, []*metal.Machine{
		{ID: m1, PartitionID: "partition-1", SizeID: "c1-large-x86"},
		{ID: m2, PartitionID: "partition-1", SizeID: "c1-large-x86"},
		{ID: m3, PartitionID: "partition-1", SizeID: "c1-large-x86", Allocation: &metal.MachineAllocation{Project: p1}},
		{ID: m4, PartitionID: "partition-1", SizeID: "c1-large-x86"},
	})

	setLastEventTime := func(id string, lastEventTime *time.Time) {
		ec := testStore.GetEventContainer(id)
		ec.LastEventTime = lastEventTime
		err := testStore.GetDatastore().Event().Update(ctx, ec)
		require.NoError(t, err)
	}

	// m1: recent event -> alive
	setLastEventTime(m1, &recent)
	// m2: stale event, not allocated -> dead
	setLastEventTime(m2, &old)
	// m3: stale event, allocated -> unknown, because the customer may have turned off phone home
	setLastEventTime(m3, &old)
	// m4: no event time at all -> liveliness must be left untouched

	_, err := testStore.UnscopedMachine().AdditionalMethods().MachineLiveliness(ctx)
	require.NoError(t, err)

	require.Equal(t, metal.MachineLivelinessAlive, testStore.GetEventContainer(m1).Liveliness)
	require.Equal(t, metal.MachineLivelinessDead, testStore.GetEventContainer(m2).Liveliness)
	require.Equal(t, metal.MachineLivelinessUnknown, testStore.GetEventContainer(m3).Liveliness)
	require.Equal(t, metal.MachineLivelinessAlive, testStore.GetEventContainer(m4).Liveliness)
}

func Test_machineServiceServer_Resurrect(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	// older than metal.MachineResurrectAfter so that a dead machine qualifies for resurrection
	resurrectableLastEventTime := time.Now().Add(-2 * time.Hour)

	tests := []struct {
		name            string
		configureEC     func(ec *metal.ProvisioningEventContainer)
		wantResurrected bool
	}{
		{
			name: "resurrects a dead machine",
			configureEC: func(ec *metal.ProvisioningEventContainer) {
				ec.Liveliness = metal.MachineLivelinessDead
				ec.LastEventTime = &resurrectableLastEventTime
			},
			wantResurrected: true,
		},
		{
			name: "resurrects a machine with failed machine reclaim",
			configureEC: func(ec *metal.ProvisioningEventContainer) {
				ec.FailedMachineReclaim = true
				ec.LastEventTime = &resurrectableLastEventTime
			},
			wantResurrected: true,
		},
		{
			name: "does not resurrect an alive machine",
			configureEC: func(ec *metal.ProvisioningEventContainer) {
				ec.Liveliness = metal.MachineLivelinessAlive
				ec.LastEventTime = &resurrectableLastEventTime
			},
			wantResurrected: false,
		},
		{
			name: "does not resurrect a machine without last event time",
			configureEC: func(ec *metal.ProvisioningEventContainer) {
				ec.Liveliness = metal.MachineLivelinessDead
			},
			wantResurrected: false,
		},
	}

	dc := test.NewDatacenter(t)
	log := dc.GetTestStore().GetLogger()
	defer dc.Close()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dc.Create(&sc.DatacenterWithAllocations)
			defer dc.Cleanup()

			testStore := dc.GetTestStore()

			// Machine2 is not allocated, so it is the one that is evaluated for resurrection.
			ec := testStore.GetEventContainer(sc.Machine2)
			tt.configureEC(ec)
			err := testStore.GetDatastore().Event().Update(ctx, ec)
			require.NoError(t, err)

			var (
				simErr  error
				simDone chan struct{}
			)

			if tt.wantResurrected {
				simDone = make(chan struct{})

				// The decommission workflow triggers a bmc command which needs to be acknowledged for it to complete.
				taskServer := admintask.New(admintask.Config{
					Log:  log,
					Repo: testStore.Store,
				})
				findActiveBMCCommandTask := func() (*adminv2.TaskInfo, error) {
					resp, err := taskServer.List(ctx, &adminv2.TaskServiceListRequest{})
					if err != nil {
						return nil, err
					}

					idx := slices.IndexFunc(resp.Tasks, func(info *adminv2.TaskInfo) bool {
						return info.Type == string(task.TypeMachineBMCCommand) && info.State == adminv2.TaskState_TASK_STATE_ACTIVE
					})
					if idx == -1 {
						return nil, fmt.Errorf("no active bmc command task found")
					}

					return resp.Tasks[idx], nil
				}

				go func() {
					defer close(simDone)

					var bmcPayload *task.MachineBMCCommandPayload

					err := retry.Do(func() error {
						tsk, err := findActiveBMCCommandTask()
						if err != nil {
							return err
						}

						payload, err := task.DecodePayload[*task.MachineBMCCommandPayload](tsk.Payload)
						if err != nil {
							return err
						}

						bmcPayload = payload
						return nil
					}, retry.Attempts(10), retry.Delay(1*time.Second), retry.Context(ctx))
					if err != nil {
						simErr = err
						return
					}

					if _, err = testStore.UnscopedMachine().AdditionalMethods().BMCCommandDone(ctx, &infrav2.BMCCommandDoneRequest{
						CommandId: bmcPayload.CommandID,
					}); err != nil {
						simErr = err
					}
				}()
			}

			_, err = testStore.UnscopedMachine().AdditionalMethods().Resurrect(ctx)
			require.NoError(t, err)

			if tt.wantResurrected {
				<-simDone
				require.NoError(t, simErr)
			}

			ec = testStore.GetEventContainer(sc.Machine2)
			hasReclaimEvent := slices.ContainsFunc(ec.Events, func(e metal.ProvisioningEvent) bool {
				return e.Event == metal.ProvisioningEventMachineReclaim
			})
			require.Equal(t, tt.wantResurrected, hasReclaimEvent, "unexpected machine reclaim event state")
		})
	}
}
