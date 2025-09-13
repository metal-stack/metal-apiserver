package admin

import (
	"log/slog"
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

func Test_sizeServiceServer_Create(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	testStore, closer := test.StartRepositoryWithCleanup(t, log)
	defer closer()
	repo := testStore.Store

	ctx := t.Context()

	sizes := []*adminv2.SizeServiceCreateRequest{
		{Size: &apiv2.Size{
			Id: "n1-medium-x86", Name: pointer.Pointer("n1-medium-x86"),
			Constraints: []*apiv2.SizeConstraint{
				{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_CORES, Min: 4, Max: 4},
				{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_MEMORY, Min: 1024 * 1024, Max: 1024 * 1024},
				{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_STORAGE, Min: 10 * 1024 * 1024, Max: 10 * 1024 * 1024},
			},
		}},
	}

	test.CreateSizes(t, repo, sizes)

	tests := []struct {
		name    string
		rq      *adminv2.SizeServiceCreateRequest
		want    *adminv2.SizeServiceCreateResponse
		wantErr error
	}{
		{
			name: "create a c1-large",
			rq: &adminv2.SizeServiceCreateRequest{
				Size: &apiv2.Size{
					Id: "c1-large-x86", Name: pointer.Pointer("c1-large"),
					Constraints: []*apiv2.SizeConstraint{
						{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_CORES, Min: 12, Max: 12},
						{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_MEMORY, Min: 1024 * 1024, Max: 1024 * 1024},
						{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_STORAGE, Min: 10 * 1024 * 1024, Max: 10 * 1024 * 1024},
					},
				},
			},
			want: &adminv2.SizeServiceCreateResponse{
				Size: &apiv2.Size{
					Id: "c1-large-x86", Name: pointer.Pointer("c1-large"),
					Meta: &apiv2.Meta{},
					Constraints: []*apiv2.SizeConstraint{
						{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_CORES, Min: 12, Max: 12},
						{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_MEMORY, Min: 1024 * 1024, Max: 1024 * 1024},
						{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_STORAGE, Min: 10 * 1024 * 1024, Max: 10 * 1024 * 1024},
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "create a malformed c1-large",
			rq: &adminv2.SizeServiceCreateRequest{
				Size: &apiv2.Size{
					Id: "c1-large-x86", Name: pointer.Pointer("c1-large"),
					Constraints: []*apiv2.SizeConstraint{
						{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_CORES, Min: 14, Max: 10},
						{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_MEMORY, Min: 1024 * 1024, Max: 1024 * 1024},
						{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_STORAGE, Min: 10 * 1024 * 1024, Max: 10 * 1024 * 1024},
					},
				},
			},
			want:    nil,
			wantErr: errorutil.InvalidArgument("constraint at index 0 is invalid: max is smaller than min"),
		},
		{
			name: "create a size already exists",
			rq: &adminv2.SizeServiceCreateRequest{
				Size: &apiv2.Size{
					Id: "n1-medium-x86", Name: pointer.Pointer("n1-medium-x86"),
					Constraints: []*apiv2.SizeConstraint{
						{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_CORES, Min: 4, Max: 4},
						{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_MEMORY, Min: 1024 * 1024, Max: 1024 * 1024},
						{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_STORAGE, Min: 10 * 1024 * 1024, Max: 10 * 1024 * 1024},
					},
				},
			},
			want:    nil,
			wantErr: errorutil.Conflict("cannot create size in database, entity already exists: n1-medium-x86"),
		},
		{
			name: "create a size which overlaps existing",
			rq: &adminv2.SizeServiceCreateRequest{
				Size: &apiv2.Size{
					Id: "n2-medium-x86", Name: pointer.Pointer("n2-medium-x86"),
					Constraints: []*apiv2.SizeConstraint{
						{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_CORES, Min: 4, Max: 4},
						{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_MEMORY, Min: 1024 * 1024, Max: 1024 * 1024},
						{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_STORAGE, Min: 10 * 1024 * 1024, Max: 10 * 1024 * 1024},
					},
				},
			},
			want:    nil,
			wantErr: errorutil.InvalidArgument("already_exists: given size n1-medium-x86 overlaps with existing sizes"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &sizeServiceServer{
				log:  log,
				repo: repo,
			}
			got, err := s.Create(ctx, connect.NewRequest(tt.rq))

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
				t.Errorf("sizeServiceServer.Create() = %v, want %vņdiff: %s", pointer.SafeDeref(got).Msg, tt.want, diff)
			}

		})
	}
}

func Test_sizeServiceServer_Update(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	testStore, closer := test.StartRepositoryWithCleanup(t, log)
	defer closer()
	repo := testStore.Store

	ctx := t.Context()

	sizes := []*adminv2.SizeServiceCreateRequest{
		{
			Size: &apiv2.Size{
				Id: "n1-medium-x86", Name: pointer.Pointer("n1-medium-x86"),
				Constraints: []*apiv2.SizeConstraint{
					{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_CORES, Min: 4, Max: 4},
					{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_MEMORY, Min: 1024 * 1024, Max: 1024 * 1024},
					{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_STORAGE, Min: 10 * 1024 * 1024, Max: 10 * 1024 * 1024},
				},
			},
		},
		{
			Size: &apiv2.Size{
				Id: "n2-medium-x86", Name: pointer.Pointer("n2-medium-x86"),
				Constraints: []*apiv2.SizeConstraint{
					{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_CORES, Min: 8, Max: 8},
					{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_MEMORY, Min: 1024 * 1024, Max: 1024 * 1024},
					{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_STORAGE, Min: 10 * 1024 * 1024, Max: 10 * 1024 * 1024},
				},
			},
		},
		{
			Size: &apiv2.Size{
				Id: "n3-medium-x86", Name: pointer.Pointer("n3-medium-x86"),
				Meta: &apiv2.Meta{
					Labels: &apiv2.Labels{
						Labels: map[string]string{"purpose": "worker", "location": "munich", "architecture": "x86"},
					},
				},
				Constraints: []*apiv2.SizeConstraint{
					{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_CORES, Min: 12, Max: 12},
					{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_MEMORY, Min: 1024 * 1024, Max: 1024 * 1024},
					{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_STORAGE, Min: 10 * 1024 * 1024, Max: 10 * 1024 * 1024},
				},
			},
		},
	}

	sizeMap := test.CreateSizes(t, repo, sizes)

	tests := []struct {
		name    string
		rq      *adminv2.SizeServiceUpdateRequest
		want    *adminv2.SizeServiceUpdateResponse
		wantErr error
	}{
		{
			name: "update n1-medium name and description",
			rq: &adminv2.SizeServiceUpdateRequest{
				Id: "n1-medium-x86", Name: pointer.Pointer("n1-medium"), Description: pointer.Pointer("best for firewalls"),
				UpdatedAt: timestamppb.New(sizeMap["n1-medium-x86"].Changed),
				Constraints: []*apiv2.SizeConstraint{
					{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_CORES, Min: 4, Max: 4},
					{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_MEMORY, Min: 1024 * 1024, Max: 1024 * 1024},
					{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_STORAGE, Min: 10 * 1024 * 1024, Max: 10 * 1024 * 1024},
				},
			},
			want: &adminv2.SizeServiceUpdateResponse{
				Size: &apiv2.Size{
					Id: "n1-medium-x86", Name: pointer.Pointer("n1-medium"), Description: pointer.Pointer("best for firewalls"),
					Meta: &apiv2.Meta{Generation: 1},
					Constraints: []*apiv2.SizeConstraint{
						{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_CORES, Min: 4, Max: 4},
						{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_MEMORY, Min: 1024 * 1024, Max: 1024 * 1024},
						{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_STORAGE, Min: 10 * 1024 * 1024, Max: 10 * 1024 * 1024},
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "update n2-medium constraints",
			rq: &adminv2.SizeServiceUpdateRequest{
				Id:        "n2-medium-x86",
				UpdatedAt: timestamppb.New(sizeMap["n2-medium-x86"].Changed),
				Constraints: []*apiv2.SizeConstraint{
					{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_CORES, Min: 6, Max: 12},
					{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_MEMORY, Min: 1024 * 1024, Max: 2 * 1024 * 1024},
					{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_STORAGE, Min: 20 * 1024 * 1024, Max: 30 * 1024 * 1024},
				},
			},
			want: &adminv2.SizeServiceUpdateResponse{
				Size: &apiv2.Size{
					Id: "n2-medium-x86", Name: pointer.Pointer("n2-medium-x86"),
					Meta: &apiv2.Meta{Generation: 1},
					Constraints: []*apiv2.SizeConstraint{
						{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_CORES, Min: 6, Max: 12},
						{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_MEMORY, Min: 1024 * 1024, Max: 2 * 1024 * 1024},
						{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_STORAGE, Min: 20 * 1024 * 1024, Max: 30 * 1024 * 1024},
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "update n3-medium labels",
			rq: &adminv2.SizeServiceUpdateRequest{
				Id:        "n3-medium-x86",
				UpdatedAt: timestamppb.New(sizeMap["n3-medium-x86"].Changed),
				Labels: &apiv2.UpdateLabels{
					Update: &apiv2.Labels{Labels: map[string]string{"purpose": "big worker"}},
					Remove: []string{"location"},
				},
			},
			want: &adminv2.SizeServiceUpdateResponse{
				Size: &apiv2.Size{
					Id: "n3-medium-x86", Name: pointer.Pointer("n3-medium-x86"),
					Meta: &apiv2.Meta{
						Labels: &apiv2.Labels{
							Labels: map[string]string{"purpose": "big worker", "architecture": "x86"},
						},
						Generation: 1,
					},
					Constraints: []*apiv2.SizeConstraint{
						{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_CORES, Min: 12, Max: 12},
						{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_MEMORY, Min: 1024 * 1024, Max: 1024 * 1024},
						{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_STORAGE, Min: 10 * 1024 * 1024, Max: 10 * 1024 * 1024},
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "update n3-medium which will overlap",
			rq: &adminv2.SizeServiceUpdateRequest{
				Id: "n3-medium-x86",
				Constraints: []*apiv2.SizeConstraint{
					{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_CORES, Min: 4, Max: 6},
					{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_MEMORY, Min: 1024 * 1024, Max: 1024 * 1024},
					{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_STORAGE, Min: 10 * 1024 * 1024, Max: 10 * 1024 * 1024},
				},
			},
			want:    nil,
			wantErr: errorutil.InvalidArgument("already_exists: given size n1-medium-x86 overlaps with existing sizes"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &sizeServiceServer{
				log:  log,
				repo: repo,
			}
			got, err := s.Update(ctx, connect.NewRequest(tt.rq))

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
				t.Errorf("sizeServiceServer.Update() = %v, want %vņdiff: %s", pointer.SafeDeref(got).Msg, tt.want, diff)
			}

		})
	}
}

func Test_sizeServiceServer_Delete(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	testStore, closer := test.StartRepositoryWithCleanup(t, log)
	defer closer()
	repo := testStore.Store

	ctx := t.Context()

	sizes := []*adminv2.SizeServiceCreateRequest{
		{
			Size: &apiv2.Size{
				Id: "n1-medium-x86", Name: pointer.Pointer("n1-medium-x86"),
				Constraints: []*apiv2.SizeConstraint{
					{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_CORES, Min: 4, Max: 4},
					{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_MEMORY, Min: 1024 * 1024, Max: 1024 * 1024},
					{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_STORAGE, Min: 10 * 1024 * 1024, Max: 10 * 1024 * 1024},
				},
			},
		},
		{
			Size: &apiv2.Size{
				Id: "c1-large-x86", Name: pointer.Pointer("c1-large-x86"),
				Constraints: []*apiv2.SizeConstraint{
					{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_CORES, Min: 12, Max: 12},
					{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_MEMORY, Min: 1024 * 1024, Max: 1024 * 1024},
					{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_STORAGE, Min: 10 * 1024 * 1024, Max: 10 * 1024 * 1024},
				},
			},
		},
	}

	test.CreateSizes(t, repo, sizes)
	test.CreateMachines(t, testStore, []*metal.Machine{
		{
			Base: metal.Base{ID: "m1"}, PartitionID: "partition-one",
			SizeID: "c1-large-x86",
			Allocation: &metal.MachineAllocation{
				FilesystemLayout: &metal.FilesystemLayout{
					Base: metal.Base{ID: "m1-large"},
				},
			},
		},
	})

	tests := []struct {
		name    string
		rq      *adminv2.SizeServiceDeleteRequest
		want    *adminv2.SizeServiceDeleteResponse
		wantErr error
	}{
		{
			name:    "delete non existing",
			rq:      &adminv2.SizeServiceDeleteRequest{Id: "non-existing"},
			want:    nil,
			wantErr: errorutil.NotFound(`no size with id "non-existing" found`),
		},
		{
			name:    "delete existing with attached machines",
			rq:      &adminv2.SizeServiceDeleteRequest{Id: "c1-large-x86"},
			want:    nil,
			wantErr: errorutil.InvalidArgument(`cannot remove size with existing machines of this size`),
		},
		{
			name: "delete n1-medium",
			rq:   &adminv2.SizeServiceDeleteRequest{Id: "n1-medium-x86"},
			want: &adminv2.SizeServiceDeleteResponse{Size: &apiv2.Size{
				Id: "n1-medium-x86", Name: pointer.Pointer("n1-medium-x86"),
				Meta: &apiv2.Meta{},
				Constraints: []*apiv2.SizeConstraint{
					{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_CORES, Min: 4, Max: 4},
					{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_MEMORY, Min: 1024 * 1024, Max: 1024 * 1024},
					{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_STORAGE, Min: 10 * 1024 * 1024, Max: 10 * 1024 * 1024},
				},
			}},
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &sizeServiceServer{
				log:  log,
				repo: repo,
			}
			got, err := s.Delete(ctx, connect.NewRequest(tt.rq))

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
				t.Errorf("sizeServiceServer.Delete() = %v, want %vņdiff: %s", pointer.SafeDeref(got).Msg, tt.want, diff)
			}

		})
	}
}
