package admin

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
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	"github.com/metal-stack/metal-apiserver/pkg/test"
	"google.golang.org/protobuf/testing/protocmp"
)

var (
	partition1 = "partition-1"
	partition2 = "partition-2"
	p1         = "00000000-0000-0000-0000-000000000001"
	p2         = "00000000-0000-0000-0000-000000000002"
)

func Test_sizeReservationServiceServer_Create(t *testing.T) {
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

	sizes := []*adminv2.SizeServiceCreateRequest{
		{Size: &apiv2.Size{
			Id: "n1-medium-x86", Name: new("n1-medium-x86"),
			Constraints: []*apiv2.SizeConstraint{
				{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_CORES, Min: 4, Max: 4},
				{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_MEMORY, Min: 1024 * 1024, Max: 1024 * 1024},
				{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_STORAGE, Min: 10 * 1024 * 1024, Max: 10 * 1024 * 1024},
			},
		}},
	}

	test.CreateSizes(t, testStore, sizes)
	test.CreatePartitions(t, testStore, []*adminv2.PartitionServiceCreateRequest{
		{
			Partition: &apiv2.Partition{Id: partition1, BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}},
		},
	})
	test.CreateTenants(t, testStore, []*apiv2.TenantServiceCreateRequest{{Name: "t1"}})
	test.CreateProjects(t, testStore, []*apiv2.ProjectServiceCreateRequest{{Name: p1, Login: "t1"}, {Name: p2, Login: "t1"}})

	tests := []struct {
		name    string
		req     *adminv2.SizeReservationServiceCreateRequest
		want    *adminv2.SizeReservationServiceCreateResponse
		wantErr error
	}{
		{
			name: "Create with errors",
			req: &adminv2.SizeReservationServiceCreateRequest{
				SizeReservation: &apiv2.SizeReservation{},
			},
			want:    nil,
			wantErr: errorutil.InvalidArgument("amount must be a positive integer"),
		},
		{
			name: "Create with errors, size not present",
			req: &adminv2.SizeReservationServiceCreateRequest{
				SizeReservation: &apiv2.SizeReservation{
					Name:   "no size",
					Amount: 1,
				},
			},
			want:    nil,
			wantErr: errorutil.InvalidArgument(`size must exist before creating a size reservation: not_found: no size with id "" found`),
		},
		{
			name: "Create with errors, partitions empty",
			req: &adminv2.SizeReservationServiceCreateRequest{
				SizeReservation: &apiv2.SizeReservation{
					Name:    "no size",
					Amount:  1,
					Size:    "n1-medium-x86",
					Project: p1,
				},
			},
			want:    nil,
			wantErr: errorutil.InvalidArgument("at least one partition id must be specified"),
		},
		{
			name: "Create with errors, partition does not exist",
			req: &adminv2.SizeReservationServiceCreateRequest{
				SizeReservation: &apiv2.SizeReservation{
					Name:       "no size",
					Amount:     1,
					Size:       "n1-medium-x86",
					Project:    p1,
					Partitions: []string{"partition-0"},
				},
			},
			want:    nil,
			wantErr: errorutil.InvalidArgument(`partition must exist before creating a size reservation: not_found: no partition with id "partition-0" found`),
		},
		{
			name: "Create with errors, partition does not exist",
			req: &adminv2.SizeReservationServiceCreateRequest{
				SizeReservation: &apiv2.SizeReservation{
					Name:       "no size",
					Amount:     1,
					Size:       "n1-medium-x86",
					Partitions: []string{partition1},
				},
			},
			want:    nil,
			wantErr: errorutil.InvalidArgument(`project must exist before creating a size reservation: rpc error: code = NotFound desc = get of project with id `),
		},
		{
			name: "Create with errors, id specified",
			req: &adminv2.SizeReservationServiceCreateRequest{
				SizeReservation: &apiv2.SizeReservation{
					Id:         "00000000-0000-0000-0000-000000000000",
					Name:       "no size",
					Amount:     1,
					Size:       "n1-medium-x86",
					Partitions: []string{partition1},
				},
			},
			want:    nil,
			wantErr: errorutil.InvalidArgument("id must not be defined"),
		},
		{
			name: "Create valid reservation",
			req: &adminv2.SizeReservationServiceCreateRequest{
				SizeReservation: &apiv2.SizeReservation{
					Name:       "no size",
					Amount:     1,
					Size:       "n1-medium-x86",
					Partitions: []string{partition1},
					Project:    p1,
				},
			},
			want: &adminv2.SizeReservationServiceCreateResponse{
				SizeReservation: &apiv2.SizeReservation{
					Amount:     1,
					Meta:       &apiv2.Meta{},
					Name:       "no size",
					Size:       "n1-medium-x86",
					Project:    p1,
					Partitions: []string{partition1},
				},
			},
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &sizeReservationServiceServer{
				log:  log,
				repo: testStore.Store,
			}
			if tt.wantErr == nil {
				// Execute proto based validation
				test.Validate(t, tt.req)
			}

			got, err := s.Create(ctx, tt.req)

			if diff := cmp.Diff(err, tt.wantErr, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("diff = %s", diff)
			}

			if diff := cmp.Diff(
				tt.want, got,
				protocmp.Transform(),
				protocmp.IgnoreFields(
					&apiv2.SizeReservation{}, "id",
				),
				protocmp.IgnoreFields(
					&apiv2.Meta{}, "created_at", "updated_at",
				),
			); diff != "" {
				t.Errorf("sizeReservationServiceServer.Create() = %v, want %vņdiff: %s", got, tt.want, diff)
			}
		})
	}
}

func Test_sizeReservationServiceServer_Update(t *testing.T) {
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

	sizes := []*adminv2.SizeServiceCreateRequest{
		{Size: &apiv2.Size{
			Id: "n1-medium-x86", Name: new("n1-medium-x86"),
			Constraints: []*apiv2.SizeConstraint{
				{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_CORES, Min: 4, Max: 4},
				{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_MEMORY, Min: 1024 * 1024, Max: 1024 * 1024},
				{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_STORAGE, Min: 10 * 1024 * 1024, Max: 10 * 1024 * 1024},
			},
		}},
	}
	sizeReservations := []*adminv2.SizeReservationServiceCreateRequest{
		{SizeReservation: &apiv2.SizeReservation{
			Name:        "sz-n1",
			Description: "N1 Reservation for project-1 in partition-1",
			Project:     p1,
			Size:        "n1-medium-x86",
			Partitions:  []string{partition1},
			Amount:      2,
		}},
	}

	test.CreateSizes(t, testStore, sizes)
	test.CreatePartitions(t, testStore, []*adminv2.PartitionServiceCreateRequest{
		{
			Partition: &apiv2.Partition{Id: partition1, BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}},
		},
		{
			Partition: &apiv2.Partition{Id: partition2, BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}},
		},
	})
	test.CreateTenants(t, testStore, []*apiv2.TenantServiceCreateRequest{{Name: "t1"}})
	test.CreateProjects(t, testStore, []*apiv2.ProjectServiceCreateRequest{{Name: p1, Login: "t1"}, {Name: p2, Login: "t1"}})
	sizeReservationMap := test.CreateSizeReservations(t, testStore, sizeReservations)

	tests := []struct {
		name    string
		req     *adminv2.SizeReservationServiceUpdateRequest
		want    *adminv2.SizeReservationServiceUpdateResponse
		wantErr error
	}{
		{
			name: "Update without errors",
			req: &adminv2.SizeReservationServiceUpdateRequest{
				Id:          sizeReservationMap["sz-n1"].Id,
				Name:        new("size-reservation-n1-medium"),
				Partitions:  []string{partition1, partition2},
				Amount:      new(int32(3)),
				Description: new("N1 Reservation for project-1 in partition-1 and partition-2"),
				UpdateMeta:  &apiv2.UpdateMeta{UpdatedAt: sizeReservationMap["sz-n1"].Meta.UpdatedAt},
			},
			want: &adminv2.SizeReservationServiceUpdateResponse{
				SizeReservation: &apiv2.SizeReservation{
					Id:          sizeReservationMap["sz-n1"].Id,
					Meta:        &apiv2.Meta{Generation: 1},
					Name:        "size-reservation-n1-medium",
					Description: "N1 Reservation for project-1 in partition-1 and partition-2",
					Project:     p1,
					Size:        "n1-medium-x86",
					Amount:      3,
					Partitions:  []string{partition1, partition2},
				},
			},
			wantErr: nil,
		},
		{
			name: "Update with errors, partition does not exist",
			req: &adminv2.SizeReservationServiceUpdateRequest{
				Id:          sizeReservationMap["sz-n1"].Id,
				Name:        new("size-reservation-n1-medium"),
				Partitions:  []string{partition1, "partition-3"},
				Amount:      new(int32(3)),
				Description: new("N1 Reservation for project-1 in partition-1 and partition-2"),
				UpdateMeta:  &apiv2.UpdateMeta{UpdatedAt: sizeReservationMap["sz-n1"].Meta.UpdatedAt},
			},
			want:    nil,
			wantErr: errorutil.InvalidArgument(`partition must exist before creating a size reservation: not_found: no partition with id "partition-3" found`),
		},
		{
			name: "Update with errors, null amount",
			req: &adminv2.SizeReservationServiceUpdateRequest{
				Id:          sizeReservationMap["sz-n1"].Id,
				Name:        new("size-reservation-n1-medium"),
				Partitions:  []string{partition1},
				Amount:      new(int32(0)),
				Description: new("N1 Reservation for project-1 in partition-1 and partition-2"),
				UpdateMeta:  &apiv2.UpdateMeta{UpdatedAt: sizeReservationMap["sz-n1"].Meta.UpdatedAt},
			},
			want:    nil,
			wantErr: errorutil.InvalidArgument("amount must be a positive integer"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &sizeReservationServiceServer{
				log:  log,
				repo: testStore.Store,
			}
			if tt.wantErr == nil {
				// Execute proto based validation
				test.Validate(t, tt.req)
			}

			got, err := s.Update(ctx, tt.req)

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
				t.Errorf("sizeReservationServiceServer.Update() = %v, want %vņdiff: %s", got, tt.want, diff)
			}
		})
	}
}

func Test_sizeReservationServiceServer_Delete(t *testing.T) {
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

	sizes := []*adminv2.SizeServiceCreateRequest{
		{Size: &apiv2.Size{
			Id: "n1-medium-x86", Name: new("n1-medium-x86"),
			Constraints: []*apiv2.SizeConstraint{
				{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_CORES, Min: 4, Max: 4},
				{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_MEMORY, Min: 1024 * 1024, Max: 1024 * 1024},
				{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_STORAGE, Min: 10 * 1024 * 1024, Max: 10 * 1024 * 1024},
			},
		}},
	}
	sizeReservations := []*adminv2.SizeReservationServiceCreateRequest{
		{SizeReservation: &apiv2.SizeReservation{
			Name:        "sz-n1",
			Description: "N1 Reservation for project-1 in partition-1",
			Project:     p1,
			Size:        "n1-medium-x86",
			Partitions:  []string{partition1},
			Amount:      2,
		}},
	}

	test.CreateSizes(t, testStore, sizes)
	test.CreatePartitions(t, testStore, []*adminv2.PartitionServiceCreateRequest{
		{
			Partition: &apiv2.Partition{Id: partition1, BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}},
		},
		{
			Partition: &apiv2.Partition{Id: partition2, BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}},
		},
	})
	test.CreateTenants(t, testStore, []*apiv2.TenantServiceCreateRequest{{Name: "t1"}})
	test.CreateProjects(t, testStore, []*apiv2.ProjectServiceCreateRequest{{Name: p1, Login: "t1"}, {Name: p2, Login: "t1"}})
	sizeReservationMap := test.CreateSizeReservations(t, testStore, sizeReservations)

	tests := []struct {
		name    string
		req     *adminv2.SizeReservationServiceDeleteRequest
		want    *adminv2.SizeReservationServiceDeleteResponse
		wantErr error
	}{
		{
			name: "Delete with errors",
			req: &adminv2.SizeReservationServiceDeleteRequest{
				Id: "non-existing",
			},
			want:    nil,
			wantErr: errorutil.NotFound(`no sizereservation with id "non-existing" found`),
		},
		{
			name: "Delete without error",
			req: &adminv2.SizeReservationServiceDeleteRequest{
				Id: sizeReservationMap["sz-n1"].Id,
			},
			want: &adminv2.SizeReservationServiceDeleteResponse{
				SizeReservation: &apiv2.SizeReservation{
					Id:          sizeReservationMap["sz-n1"].Id,
					Meta:        &apiv2.Meta{},
					Name:        "sz-n1",
					Description: "N1 Reservation for project-1 in partition-1",
					Project:     p1,
					Size:        "n1-medium-x86",
					Partitions:  []string{partition1},
					Amount:      2,
				},
			},
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &sizeReservationServiceServer{
				log:  log,
				repo: testStore.Store,
			}
			if tt.wantErr == nil {
				// Execute proto based validation
				test.Validate(t, tt.req)
			}

			got, err := s.Delete(ctx, tt.req)

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
				t.Errorf("sizeReservationServiceServer.Delete() = %v, want %vņdiff: %s", got, tt.want, diff)
			}
		})
	}
}

func Test_sizeReservationServiceServer_List(t *testing.T) {
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

	sizes := []*adminv2.SizeServiceCreateRequest{
		{Size: &apiv2.Size{
			Id: "n1-medium-x86", Name: new("n1-medium-x86"),
			Constraints: []*apiv2.SizeConstraint{
				{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_CORES, Min: 4, Max: 4},
				{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_MEMORY, Min: 1024 * 1024, Max: 1024 * 1024},
				{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_STORAGE, Min: 10 * 1024 * 1024, Max: 10 * 1024 * 1024},
			},
		}},
		{Size: &apiv2.Size{
			Id: "n2-medium-x86", Name: new("n2-medium-x86"),
			Constraints: []*apiv2.SizeConstraint{
				{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_CORES, Min: 8, Max: 8},
				{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_MEMORY, Min: 1024 * 1024, Max: 1024 * 1024},
				{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_STORAGE, Min: 10 * 1024 * 1024, Max: 10 * 1024 * 1024},
			},
		}},
	}
	sizeReservations := []*adminv2.SizeReservationServiceCreateRequest{
		{
			SizeReservation: &apiv2.SizeReservation{
				Name:        "sz-n1",
				Description: "N1 Reservation for project-1 in partition-1",
				Project:     p1,
				Size:        "n1-medium-x86",
				Partitions:  []string{partition1},
				Amount:      2,
			},
		},
		{
			SizeReservation: &apiv2.SizeReservation{
				Name:        "sz-n2",
				Description: "N2 Reservation for project-2 in partition-2",
				Project:     p2,
				Size:        "n2-medium-x86",
				Partitions:  []string{partition2},
				Amount:      5,
			},
		},
	}

	test.CreateSizes(t, testStore, sizes)
	test.CreatePartitions(t, testStore, []*adminv2.PartitionServiceCreateRequest{
		{
			Partition: &apiv2.Partition{Id: partition1, BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}},
		},
		{
			Partition: &apiv2.Partition{Id: partition2, BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}},
		},
	})
	test.CreateTenants(t, testStore, []*apiv2.TenantServiceCreateRequest{{Name: "t1"}})
	test.CreateProjects(t, testStore, []*apiv2.ProjectServiceCreateRequest{{Name: p1, Login: "t1"}, {Name: p2, Login: "t1"}})
	sizeReservationMap := test.CreateSizeReservations(t, testStore, sizeReservations)

	tests := []struct {
		name    string
		req     *adminv2.SizeReservationServiceListRequest
		want    *adminv2.SizeReservationServiceListResponse
		wantErr error
	}{
		{
			name: "List in partition-1",
			req: &adminv2.SizeReservationServiceListRequest{
				Query: &apiv2.SizeReservationQuery{
					Partition: &partition1,
				},
			},
			want: &adminv2.SizeReservationServiceListResponse{
				SizeReservations: []*apiv2.SizeReservation{
					{
						Id:          sizeReservationMap["sz-n1"].Id,
						Meta:        &apiv2.Meta{},
						Name:        "sz-n1",
						Description: "N1 Reservation for project-1 in partition-1",
						Project:     p1,
						Size:        "n1-medium-x86",
						Partitions:  []string{partition1},
						Amount:      2,
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "List in project-1",
			req: &adminv2.SizeReservationServiceListRequest{
				Query: &apiv2.SizeReservationQuery{
					Project: &p1,
				},
			},
			want: &adminv2.SizeReservationServiceListResponse{
				SizeReservations: []*apiv2.SizeReservation{
					{
						Id:          sizeReservationMap["sz-n1"].Id,
						Meta:        &apiv2.Meta{},
						Name:        "sz-n1",
						Description: "N1 Reservation for project-1 in partition-1",
						Project:     p1,
						Size:        "n1-medium-x86",
						Partitions:  []string{partition1},
						Amount:      2,
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "List size n2",
			req: &adminv2.SizeReservationServiceListRequest{
				Query: &apiv2.SizeReservationQuery{
					Size: new("n2-medium-x86"),
				},
			},
			want: &adminv2.SizeReservationServiceListResponse{
				SizeReservations: []*apiv2.SizeReservation{
					{
						Id:          sizeReservationMap["sz-n2"].Id,
						Meta:        &apiv2.Meta{},
						Name:        "sz-n2",
						Description: "N2 Reservation for project-2 in partition-2",
						Project:     p2,
						Size:        "n2-medium-x86",
						Partitions:  []string{partition2},
						Amount:      5,
					},
				},
			},
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &sizeReservationServiceServer{
				log:  log,
				repo: testStore.Store,
			}
			if tt.wantErr == nil {
				// Execute proto based validation
				test.Validate(t, tt.req)
			}

			got, err := s.List(ctx, tt.req)

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
				t.Errorf("sizeReservationServiceServer.List() = %v, want %vņdiff: %s", got, tt.want, diff)
			}
		})
	}
}
