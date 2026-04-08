package admin

import (
	"log/slog"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	"github.com/metal-stack/metal-apiserver/pkg/test"
	sc "github.com/metal-stack/metal-apiserver/pkg/test/scenarios"
	"google.golang.org/protobuf/testing/protocmp"
)

func Test_sizeImageConstraintServiceServer_Create(t *testing.T) {
	t.Parallel()

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	dc := test.NewDatacenter(t, log)
	defer dc.Close()

	tests := []struct {
		name    string
		req     *adminv2.SizeImageConstraintServiceCreateRequest
		want    *adminv2.SizeImageConstraintServiceCreateResponse
		wantErr error
	}{
		{
			name: "simple",
			req: &adminv2.SizeImageConstraintServiceCreateRequest{
				Size: "debian",
				ImageConstraints: []*apiv2.ImageConstraint{
					{
						Image:       "debian",
						SemverMatch: ">= 12.0",
					},
				},
			},
			want: &adminv2.SizeImageConstraintServiceCreateResponse{
				SizeImageConstraint: &apiv2.SizeImageConstraint{
					Size:        "debian",
					Name:        new(""),
					Description: new(""),
					Meta:        &apiv2.Meta{},
					ImageConstraints: []*apiv2.ImageConstraint{
						{
							Image:       "debian",
							SemverMatch: ">= 12.0",
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &sizeImageConstraintServiceServer{
				log:  log,
				repo: dc.GetTestStore().Store,
			}
			got, err := s.Create(t.Context(), tt.req)
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
				t.Errorf("sizeImageConstraintServiceServer.Create() = %v, want %vņdiff: %s", got, tt.want, diff)
			}

		})
	}
}

func Test_sizeImageConstraintServiceServer_Delete(t *testing.T) {
	t.Parallel()

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	dc := test.NewDatacenter(t, log)
	defer dc.Close()

	dc.Create(&sc.DatacenterSpec{
		Sizes:  sc.DefaultDatacenter.Sizes,
		Images: sc.DefaultDatacenter.Images,
		SizeImageConstraints: []*adminv2.SizeImageConstraintServiceCreateRequest{
			{
				Size: sc.SizeC1Large,
				ImageConstraints: []*apiv2.ImageConstraint{
					{
						Image:       "debian",
						SemverMatch: ">= 13.0",
					},
				},
			},
		},
	})

	tests := []struct {
		name    string
		req     *adminv2.SizeImageConstraintServiceDeleteRequest
		want    *adminv2.SizeImageConstraintServiceDeleteResponse
		wantErr error
	}{
		{
			name: "non existing",
			req: &adminv2.SizeImageConstraintServiceDeleteRequest{
				Size: "unknown size",
			},
			want:    nil,
			wantErr: errorutil.NotFound(`no sizeimageconstraint with id "unknown size" found`),
		},
		{
			name: "existing",
			req: &adminv2.SizeImageConstraintServiceDeleteRequest{
				Size: sc.SizeC1Large,
			},
			want: &adminv2.SizeImageConstraintServiceDeleteResponse{
				SizeImageConstraint: &apiv2.SizeImageConstraint{
					Size:        sc.SizeC1Large,
					Name:        new(""),
					Description: new(""),
					Meta:        &apiv2.Meta{},
					ImageConstraints: []*apiv2.ImageConstraint{
						{
							Image:       "debian",
							SemverMatch: ">= 13.0",
						},
					},
				},
			},
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &sizeImageConstraintServiceServer{
				log:  log,
				repo: dc.GetTestStore().Store,
			}
			got, err := s.Delete(t.Context(), tt.req)
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
				t.Errorf("sizeImageConstraintServiceServer.Delete() = %v, want %vņdiff: %s", got, tt.want, diff)
			}
		})
	}
}

func Test_sizeImageConstraintServiceServer_Get(t *testing.T) {
	t.Parallel()

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	dc := test.NewDatacenter(t, log)
	defer dc.Close()

	dc.Create(&sc.DatacenterSpec{
		Sizes:  sc.DefaultDatacenter.Sizes,
		Images: sc.DefaultDatacenter.Images,
		SizeImageConstraints: []*adminv2.SizeImageConstraintServiceCreateRequest{
			{
				Size: sc.SizeC1Large,
				ImageConstraints: []*apiv2.ImageConstraint{
					{
						Image:       "debian",
						SemverMatch: ">= 13.0",
					},
				},
			},
		},
	})
	tests := []struct {
		name    string
		req     *adminv2.SizeImageConstraintServiceGetRequest
		want    *adminv2.SizeImageConstraintServiceGetResponse
		wantErr error
	}{
		{
			name: "non existing",
			req: &adminv2.SizeImageConstraintServiceGetRequest{
				Size: "unknown size",
			},
			want:    nil,
			wantErr: errorutil.NotFound(`no sizeimageconstraint with id "unknown size" found`),
		},
		{
			name: "existing",
			req: &adminv2.SizeImageConstraintServiceGetRequest{
				Size: sc.SizeC1Large,
			},
			want: &adminv2.SizeImageConstraintServiceGetResponse{
				SizeImageConstraint: &apiv2.SizeImageConstraint{
					Size:        sc.SizeC1Large,
					Name:        new(""),
					Description: new(""),
					Meta:        &apiv2.Meta{},
					ImageConstraints: []*apiv2.ImageConstraint{
						{
							Image:       "debian",
							SemverMatch: ">= 13.0",
						},
					},
				},
			},
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &sizeImageConstraintServiceServer{
				log:  log,
				repo: dc.GetTestStore().Store,
			}
			got, err := s.Get(t.Context(), tt.req)
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
				t.Errorf("sizeImageConstraintServiceServer.Get() = %v, want %vņdiff: %s", got, tt.want, diff)
			}
		})
	}
}

func Test_sizeImageConstraintServiceServer_Update(t *testing.T) {
	t.Parallel()

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	dc := test.NewDatacenter(t, log)
	defer dc.Close()

	dc.Create(&sc.DatacenterSpec{
		Sizes:  sc.DefaultDatacenter.Sizes,
		Images: sc.DefaultDatacenter.Images,
		SizeImageConstraints: []*adminv2.SizeImageConstraintServiceCreateRequest{
			{
				Size: sc.SizeC1Large,
				ImageConstraints: []*apiv2.ImageConstraint{
					{
						Image:       "debian",
						SemverMatch: ">= 13.0",
					},
				},
			},
		},
	})
	tests := []struct {
		name    string
		req     *adminv2.SizeImageConstraintServiceUpdateRequest
		want    *adminv2.SizeImageConstraintServiceUpdateResponse
		wantErr error
	}{
		{
			name: "non existing",
			req: &adminv2.SizeImageConstraintServiceUpdateRequest{
				Size: "unknown size",
			},
			want:    nil,
			wantErr: errorutil.NotFound(`no sizeimageconstraint with id "unknown size" found`),
		},
		{
			name: "existing",
			req: &adminv2.SizeImageConstraintServiceUpdateRequest{
				Size:        sc.SizeC1Large,
				Name:        new("C1 Size Image Constraint"),
				Description: new("C1 Size Image Constraint"),
				UpdateMeta: &apiv2.UpdateMeta{
					LockingStrategy: apiv2.OptimisticLockingStrategy_OPTIMISTIC_LOCKING_STRATEGY_SERVER,
				},
				ImageConstraints: []*apiv2.ImageConstraint{
					{
						Image:       "debian",
						SemverMatch: ">= 13.0",
					},
					{
						Image:       "ubuntu",
						SemverMatch: ">= 24.4",
					},
				},
			},
			want: &adminv2.SizeImageConstraintServiceUpdateResponse{
				SizeImageConstraint: &apiv2.SizeImageConstraint{
					Size:        sc.SizeC1Large,
					Name:        new("C1 Size Image Constraint"),
					Description: new("C1 Size Image Constraint"),
					Meta: &apiv2.Meta{
						Generation: uint64(1),
					},
					ImageConstraints: []*apiv2.ImageConstraint{
						{
							Image:       "debian",
							SemverMatch: ">= 13.0",
						},
						{
							Image:       "ubuntu",
							SemverMatch: ">= 24.4",
						},
					},
				},
			},
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &sizeImageConstraintServiceServer{
				log:  log,
				repo: dc.GetTestStore().Store,
			}
			got, err := s.Update(t.Context(), tt.req)
			if diff := cmp.Diff(err, tt.wantErr, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("diff = %s", diff)
			}

			if got != nil {
				slices.SortFunc(got.SizeImageConstraint.ImageConstraints, func(c1, c2 *apiv2.ImageConstraint) int {
					return strings.Compare(c1.Image, c2.Image)
				})
			}

			if diff := cmp.Diff(
				tt.want, got,
				protocmp.Transform(),
				protocmp.IgnoreFields(
					&apiv2.Meta{}, "created_at", "updated_at",
				),
			); diff != "" {
				t.Errorf("sizeImageConstraintServiceServer.Get() = %v, want %vņdiff: %s", got, tt.want, diff)
			}
		})
	}
}

func Test_sizeImageConstraintServiceServer_List(t *testing.T) {
	t.Parallel()

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	dc := test.NewDatacenter(t, log)
	defer dc.Close()

	dc.Create(&sc.DatacenterSpec{
		Sizes:  sc.DefaultDatacenter.Sizes,
		Images: sc.DefaultDatacenter.Images,
		SizeImageConstraints: []*adminv2.SizeImageConstraintServiceCreateRequest{
			{
				Size: sc.SizeC1Large,
				ImageConstraints: []*apiv2.ImageConstraint{
					{
						Image:       "debian",
						SemverMatch: ">= 13.0",
					},
				},
			},
			{
				Size: sc.SizeN1Medium,
				ImageConstraints: []*apiv2.ImageConstraint{
					{
						Image:       "ubuntu",
						SemverMatch: ">= 24.4",
					},
				},
			},
		},
	})
	tests := []struct {
		name    string
		req     *adminv2.SizeImageConstraintServiceListRequest
		want    *adminv2.SizeImageConstraintServiceListResponse
		wantErr error
	}{
		{
			name: "non existing",
			req: &adminv2.SizeImageConstraintServiceListRequest{
				Query: &apiv2.SizeImageConstraintQuery{Size: new("non existing")},
			},
			want:    &adminv2.SizeImageConstraintServiceListResponse{},
			wantErr: nil,
		},
		{
			name: "one existing",
			req: &adminv2.SizeImageConstraintServiceListRequest{
				Query: &apiv2.SizeImageConstraintQuery{Size: new(sc.SizeC1Large)},
			},
			want: &adminv2.SizeImageConstraintServiceListResponse{
				SizeImageConstraints: []*apiv2.SizeImageConstraint{
					{
						Size:        sc.SizeC1Large,
						Name:        new(""),
						Description: new(""),
						Meta:        &apiv2.Meta{},
						ImageConstraints: []*apiv2.ImageConstraint{
							{
								Image:       "debian",
								SemverMatch: ">= 13.0",
							},
						},
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "all existing",
			req: &adminv2.SizeImageConstraintServiceListRequest{
				Query: &apiv2.SizeImageConstraintQuery{},
			},
			want: &adminv2.SizeImageConstraintServiceListResponse{
				SizeImageConstraints: []*apiv2.SizeImageConstraint{
					{
						Size:        sc.SizeC1Large,
						Name:        new(""),
						Description: new(""),
						Meta:        &apiv2.Meta{},
						ImageConstraints: []*apiv2.ImageConstraint{
							{
								Image:       "debian",
								SemverMatch: ">= 13.0",
							},
						},
					},
					{
						Size:        sc.SizeN1Medium,
						Name:        new(""),
						Description: new(""),
						Meta:        &apiv2.Meta{},
						ImageConstraints: []*apiv2.ImageConstraint{
							{
								Image:       "ubuntu",
								SemverMatch: ">= 24.4",
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
			s := &sizeImageConstraintServiceServer{
				log:  log,
				repo: dc.GetTestStore().Store,
			}
			got, err := s.List(t.Context(), tt.req)
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
				t.Errorf("sizeImageConstraintServiceServer.List() = %v, want %vņdiff: %s", got, tt.want, diff)
			}
		})
	}
}
