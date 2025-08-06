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
)

func Test_machineServiceServer_Get(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	ds, opts, rethinkcloser := test.StartRethink(t, log)

	testStore, closer := test.StartRepositoryWithRethinkDB(t, log, ds, opts, rethinkcloser)
	defer closer()
	repo := testStore.Store

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintln(w, "a image")
	}))

	validURL := ts.URL
	defer ts.Close()

	ctx := t.Context()

	test.CreateTenants(t, repo, []*apiv2.TenantServiceCreateRequest{{Name: "t1"}})
	test.CreateProjects(t, repo, []*apiv2.ProjectServiceCreateRequest{{Name: "p1", Login: "t1"}, {Name: "p2", Login: "t1"}})
	test.CreatePartitions(t, repo, []*adminv2.PartitionServiceCreateRequest{
		{
			Partition: &apiv2.Partition{Id: "partition-1", BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}},
		},
	})

	// We need to create machines directly on the database because there is no MachineCreateRequest available and never will.
	// Once the boot-service is available we can simulate a pxe booting machine the actually create a machine from the api level.
	test.CreateMachines(t, testStore, []*metal.Machine{
		{Base: metal.Base{ID: "m1"}, PartitionID: "partition-1"},
	})

	tests := []struct {
		name    string
		rq      *apiv2.MachineServiceGetRequest
		want    *apiv2.MachineServiceGetResponse
		wantErr error
	}{
		{
			name: "get existing",
			rq:   &apiv2.MachineServiceGetRequest{Uuid: "m1"},
			want: &apiv2.MachineServiceGetResponse{
				Machine: &apiv2.Machine{
					Uuid:                     "m1",
					Meta:                     &apiv2.Meta{},
					Partition:                &apiv2.Partition{Id: "partition-1", BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}, Meta: &apiv2.Meta{}},
					Bios:                     &apiv2.MachineBios{},
					Hardware:                 &apiv2.MachineHardware{},
					LedState:                 &apiv2.MachineChassisIdentifyLEDState{},
					RecentProvisioningEvents: &apiv2.MachineRecentProvisioningEvents{},
					State:                    &apiv2.MachineStateDetails{},
				},
			},
			wantErr: nil,
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
			got, err := m.Get(ctx, connect.NewRequest(tt.rq))
			if diff := cmp.Diff(err, tt.wantErr, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("diff = %s", diff)
			}

			if diff := cmp.Diff(
				tt.want, pointer.SafeDeref(got).Msg,
				protocmp.Transform(),
				protocmp.IgnoreFields(
					&apiv2.Meta{}, "created_at", "updated_at",
				),
			); diff != "" {
				t.Errorf("machineServiceServer.Get() = %v, want %vņdiff: %s", got.Msg, tt.want, diff)
			}
		})
	}
}

func Test_machineServiceServer_List(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	ds, opts, rethinkcloser := test.StartRethink(t, log)

	testStore, closer := test.StartRepositoryWithRethinkDB(t, log, ds, opts, rethinkcloser)
	defer closer()
	repo := testStore.Store

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintln(w, "a image")
	}))

	validURL := ts.URL
	defer ts.Close()

	ctx := t.Context()

	test.CreateTenants(t, repo, []*apiv2.TenantServiceCreateRequest{{Name: "t1"}})
	test.CreateProjects(t, repo, []*apiv2.ProjectServiceCreateRequest{{Name: "p1", Login: "t1"}, {Name: "p2", Login: "t1"}})
	test.CreatePartitions(t, repo, []*adminv2.PartitionServiceCreateRequest{
		{
			Partition: &apiv2.Partition{Id: "partition-1", BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}},
		},
	})
	test.CreateImages(t, repo, []*adminv2.ImageServiceCreateRequest{
		{Image: &apiv2.Image{Id: "debian-12", Url: validURL, Features: []apiv2.ImageFeature{apiv2.ImageFeature_IMAGE_FEATURE_MACHINE}}},
	})

	// We need to create machines directly on the database because there is no MachineCreateRequest available and never will.
	// Once the boot-service is available we can simulate a pxe booting machine the actually create a machine from the api level.
	test.CreateMachines(t, testStore, []*metal.Machine{
		{Base: metal.Base{ID: "m1"}, PartitionID: "partition-1"},
		{Base: metal.Base{ID: "m2"}, PartitionID: "partition-1"},
		{Base: metal.Base{ID: "m3"}, PartitionID: "partition-1", Allocation: &metal.MachineAllocation{Project: "p1", ImageID: "debian-12"}},
		{Base: metal.Base{ID: "m4"}, PartitionID: "partition-1", Allocation: &metal.MachineAllocation{Project: "p2", ImageID: "debian-12"}},
		{Base: metal.Base{ID: "m5"}, PartitionID: "partition-1", Allocation: &metal.MachineAllocation{Project: "p2", ImageID: "debian-12"}},
	})

	tests := []struct {
		name    string
		rq      *apiv2.MachineServiceListRequest
		want    *apiv2.MachineServiceListResponse
		wantErr error
	}{
		{
			name: "List from p1",
			rq:   &apiv2.MachineServiceListRequest{Project: "p1"},
			want: &apiv2.MachineServiceListResponse{
				Machines: []*apiv2.Machine{
					{
						Uuid:                     "m3",
						Meta:                     &apiv2.Meta{},
						Partition:                &apiv2.Partition{Id: "partition-1", BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}, Meta: &apiv2.Meta{}},
						Bios:                     &apiv2.MachineBios{},
						Hardware:                 &apiv2.MachineHardware{},
						LedState:                 &apiv2.MachineChassisIdentifyLEDState{},
						RecentProvisioningEvents: &apiv2.MachineRecentProvisioningEvents{},
						State:                    &apiv2.MachineStateDetails{},
						Allocation: &apiv2.MachineAllocation{
							Project: "p1",
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
			rq:   &apiv2.MachineServiceListRequest{Project: "p2", Query: &apiv2.MachineQuery{Uuid: pointer.Pointer("m4")}},
			want: &apiv2.MachineServiceListResponse{
				Machines: []*apiv2.Machine{
					{
						Uuid:                     "m4",
						Meta:                     &apiv2.Meta{},
						Partition:                &apiv2.Partition{Id: "partition-1", BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}, Meta: &apiv2.Meta{}},
						Bios:                     &apiv2.MachineBios{},
						Hardware:                 &apiv2.MachineHardware{},
						LedState:                 &apiv2.MachineChassisIdentifyLEDState{},
						RecentProvisioningEvents: &apiv2.MachineRecentProvisioningEvents{},
						State:                    &apiv2.MachineStateDetails{},
						Allocation: &apiv2.MachineAllocation{
							Project: "p2",
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
			); diff != "" {
				t.Errorf("machineServiceServer.Get() = %v, want %vņdiff: %s", got.Msg, tt.want, diff)
			}
		})
	}
}
