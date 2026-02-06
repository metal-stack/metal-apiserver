package size

import (
	"log/slog"
	"os"
	"testing"

	"github.com/google/go-cmp/cmp"
	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	"github.com/metal-stack/metal-apiserver/pkg/test"
	"github.com/metal-stack/metal-lib/pkg/pointer"
	"google.golang.org/protobuf/testing/protocmp"
)

func Test_sizeServiceServer_Get(t *testing.T) {
	t.Parallel()

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	testStore, closer := test.StartRepositoryWithCleanup(t, log)
	defer closer()

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

	test.CreateSizes(t, testStore, sizes)

	tests := []struct {
		name    string
		rq      *apiv2.SizeServiceGetRequest
		want    *apiv2.SizeServiceGetResponse
		wantErr error
	}{
		{
			name:    "get non existing",
			rq:      &apiv2.SizeServiceGetRequest{Id: "non-existing"},
			want:    nil,
			wantErr: errorutil.NotFound(`no size with id "non-existing" found`),
		},
		{
			name: "get existing",
			rq:   &apiv2.SizeServiceGetRequest{Id: "n1-medium-x86"},
			want: &apiv2.SizeServiceGetResponse{Size: &apiv2.Size{
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
				repo: testStore.Store,
			}
			if tt.wantErr == nil {
				// Execute proto based validation
				test.Validate(t, tt.rq)
			}
			got, err := s.Get(ctx, tt.rq)

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
				t.Errorf("sizeServiceServer.Get() = %v, want %vņdiff: %s", got, tt.want, diff)
			}

		})
	}
}

func Test_sizeServiceServer_List(t *testing.T) {
	t.Parallel()

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	testStore, closer := test.StartRepositoryWithCleanup(t, log)
	defer closer()

	ctx := t.Context()

	sizes := []*adminv2.SizeServiceCreateRequest{
		{
			Size: &apiv2.Size{
				Id: "n1-medium-x86", Name: pointer.Pointer("n1-medium-x86"), Description: pointer.Pointer("firewall"),
				Constraints: []*apiv2.SizeConstraint{
					{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_CORES, Min: 4, Max: 4},
					{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_MEMORY, Min: 1024 * 1024, Max: 1024 * 1024},
					{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_STORAGE, Min: 10 * 1024 * 1024, Max: 10 * 1024 * 1024},
				},
			},
		},
		{
			Size: &apiv2.Size{
				Id: "n2-medium-x86", Name: pointer.Pointer("n2-medium-x86"), Description: pointer.Pointer("worker"),
				Constraints: []*apiv2.SizeConstraint{
					{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_CORES, Min: 8, Max: 8},
					{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_MEMORY, Min: 1024 * 1024, Max: 1024 * 1024},
					{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_STORAGE, Min: 10 * 1024 * 1024, Max: 10 * 1024 * 1024},
				},
			},
		},
		{
			Size: &apiv2.Size{
				Id: "n3-medium-x86", Name: pointer.Pointer("n3-medium-x86"), Description: pointer.Pointer("big worker"),
				Meta: &apiv2.Meta{
					Labels: &apiv2.Labels{
						Labels: map[string]string{"purpose": "worker"},
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

	test.CreateSizes(t, testStore, sizes)

	tests := []struct {
		name    string
		rq      *apiv2.SizeServiceListRequest
		want    *apiv2.SizeServiceListResponse
		wantErr error
	}{
		{
			name:    "list non existing",
			rq:      &apiv2.SizeServiceListRequest{Query: &apiv2.SizeQuery{Id: pointer.Pointer("non-existing")}},
			want:    &apiv2.SizeServiceListResponse{},
			wantErr: nil,
		},
		{
			name: "list one existing",
			rq:   &apiv2.SizeServiceListRequest{Query: &apiv2.SizeQuery{Id: pointer.Pointer("n1-medium-x86")}},
			want: &apiv2.SizeServiceListResponse{
				Sizes: []*apiv2.Size{
					{
						Id: "n1-medium-x86", Name: pointer.Pointer("n1-medium-x86"), Description: pointer.Pointer("firewall"),
						Meta: &apiv2.Meta{},
						Constraints: []*apiv2.SizeConstraint{
							{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_CORES, Min: 4, Max: 4},
							{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_MEMORY, Min: 1024 * 1024, Max: 1024 * 1024},
							{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_STORAGE, Min: 10 * 1024 * 1024, Max: 10 * 1024 * 1024},
						},
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "list by name",
			rq:   &apiv2.SizeServiceListRequest{Query: &apiv2.SizeQuery{Name: pointer.Pointer("n1-medium-x86")}},
			want: &apiv2.SizeServiceListResponse{
				Sizes: []*apiv2.Size{
					{
						Id: "n1-medium-x86", Name: pointer.Pointer("n1-medium-x86"), Description: pointer.Pointer("firewall"),
						Meta: &apiv2.Meta{},
						Constraints: []*apiv2.SizeConstraint{
							{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_CORES, Min: 4, Max: 4},
							{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_MEMORY, Min: 1024 * 1024, Max: 1024 * 1024},
							{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_STORAGE, Min: 10 * 1024 * 1024, Max: 10 * 1024 * 1024},
						},
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "list by description",
			rq:   &apiv2.SizeServiceListRequest{Query: &apiv2.SizeQuery{Description: pointer.Pointer("worker")}},
			want: &apiv2.SizeServiceListResponse{
				Sizes: []*apiv2.Size{
					{
						Id: "n2-medium-x86", Name: pointer.Pointer("n2-medium-x86"), Description: pointer.Pointer("worker"),
						Meta: &apiv2.Meta{},
						Constraints: []*apiv2.SizeConstraint{
							{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_CORES, Min: 8, Max: 8},
							{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_MEMORY, Min: 1024 * 1024, Max: 1024 * 1024},
							{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_STORAGE, Min: 10 * 1024 * 1024, Max: 10 * 1024 * 1024},
						},
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "list by label",
			rq:   &apiv2.SizeServiceListRequest{Query: &apiv2.SizeQuery{Labels: &apiv2.Labels{Labels: map[string]string{"purpose": "worker"}}}},
			want: &apiv2.SizeServiceListResponse{
				Sizes: []*apiv2.Size{
					{
						Id: "n3-medium-x86", Name: pointer.Pointer("n3-medium-x86"), Description: pointer.Pointer("big worker"),
						Meta: &apiv2.Meta{
							Labels: &apiv2.Labels{
								Labels: map[string]string{"purpose": "worker"},
							},
						}, Constraints: []*apiv2.SizeConstraint{
							{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_CORES, Min: 12, Max: 12},
							{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_MEMORY, Min: 1024 * 1024, Max: 1024 * 1024},
							{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_STORAGE, Min: 10 * 1024 * 1024, Max: 10 * 1024 * 1024},
						},
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "list all",
			rq:   &apiv2.SizeServiceListRequest{Query: &apiv2.SizeQuery{}},
			want: &apiv2.SizeServiceListResponse{
				Sizes: []*apiv2.Size{
					{
						Id: "n1-medium-x86", Name: pointer.Pointer("n1-medium-x86"), Description: pointer.Pointer("firewall"),
						Meta: &apiv2.Meta{},
						Constraints: []*apiv2.SizeConstraint{
							{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_CORES, Min: 4, Max: 4},
							{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_MEMORY, Min: 1024 * 1024, Max: 1024 * 1024},
							{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_STORAGE, Min: 10 * 1024 * 1024, Max: 10 * 1024 * 1024},
						},
					},
					{
						Id: "n2-medium-x86", Name: pointer.Pointer("n2-medium-x86"), Description: pointer.Pointer("worker"),
						Meta: &apiv2.Meta{},
						Constraints: []*apiv2.SizeConstraint{
							{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_CORES, Min: 8, Max: 8},
							{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_MEMORY, Min: 1024 * 1024, Max: 1024 * 1024},
							{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_STORAGE, Min: 10 * 1024 * 1024, Max: 10 * 1024 * 1024},
						},
					},
					{
						Id: "n3-medium-x86", Name: pointer.Pointer("n3-medium-x86"), Description: pointer.Pointer("big worker"),
						Meta: &apiv2.Meta{
							Labels: &apiv2.Labels{
								Labels: map[string]string{"purpose": "worker"},
							},
						}, Constraints: []*apiv2.SizeConstraint{
							{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_CORES, Min: 12, Max: 12},
							{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_MEMORY, Min: 1024 * 1024, Max: 1024 * 1024},
							{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_STORAGE, Min: 10 * 1024 * 1024, Max: 10 * 1024 * 1024},
						},
					},
				},
			},
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &sizeServiceServer{
				log:  log,
				repo: testStore.Store,
			}
			if tt.wantErr == nil {
				// Execute proto based validation
				test.Validate(t, tt.rq)
			}
			got, err := s.List(ctx, tt.rq)

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
				t.Errorf("sizeServiceServer.List() = %v, want %vņdiff: %s", got, tt.want, diff)
			}

		})
	}
}
