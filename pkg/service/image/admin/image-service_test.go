package admin

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"connectrpc.com/connect"
	"github.com/google/go-cmp/cmp"
	"github.com/metal-stack/api-server/pkg/db/repository"
	"github.com/metal-stack/api-server/pkg/test"
	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-lib/pkg/pointer"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/testing/protocmp"
)

func Test_imageServiceServer_Create(t *testing.T) {
	log := slog.Default()
	repo, container := test.StartRepository(t, log)
	defer func() {
		_ = container.Terminate(context.Background())
	}()
	ctx := context.Background()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "a image")
	}))
	url := ts.URL
	defer ts.Close()

	tests := []struct {
		name           string
		request        *adminv2.ImageServiceCreateRequest
		want           *adminv2.ImageServiceCreateResponse
		wantReturnCode connect.Code
		wantErrMessage string
	}{
		{
			name:           "image url is empty",
			request:        &adminv2.ImageServiceCreateRequest{Image: &apiv2.Image{Id: "debian-12.0.20241231"}},
			want:           nil,
			wantReturnCode: connect.CodeNotFound,
			wantErrMessage: "invalid_argument: image url must not be empty",
		},
		{
			name:           "image url is empty",
			request:        &adminv2.ImageServiceCreateRequest{Image: &apiv2.Image{Id: "debian-12.0.20241231", Url: url}},
			want:           nil,
			wantReturnCode: connect.CodeNotFound,
			wantErrMessage: "invalid_argument: image features must not be empty",
		},
		{
			name:    "image url is empty",
			request: &adminv2.ImageServiceCreateRequest{Image: &apiv2.Image{Id: "debian-12.0.20241231", Url: url, Features: []apiv2.ImageFeature{apiv2.ImageFeature_IMAGE_FEATURE_MACHINE}}},
			want: &adminv2.ImageServiceCreateResponse{
				Image: &apiv2.Image{
					Id:             "debian-12.0.20241231",
					Url:            url,
					Name:           pointer.Pointer(""),
					Description:    pointer.Pointer(""),
					Features:       []apiv2.ImageFeature{apiv2.ImageFeature_IMAGE_FEATURE_MACHINE},
					Classification: apiv2.ImageClassification_IMAGE_CLASSIFICATION_PREVIEW,
				},
			},
			wantReturnCode: connect.CodeNotFound,
			wantErrMessage: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			i := &imageServiceServer{
				log:  log,
				repo: repo,
			}
			got, err := i.Create(ctx, connect.NewRequest(tt.request))
			if (err != nil) && tt.wantErrMessage == "" {
				t.Errorf("imageServiceServer.Create() error = %v, wantErr %s", err, tt.wantErrMessage)
				return
			}
			if (err != nil) && tt.wantErrMessage != err.Error() {
				t.Errorf("imageServiceServer.Create() error = %s, wantErr %s", err.Error(), tt.wantErrMessage)
				return
			}
			if tt.want == nil && got == nil {
				return
			}
			if tt.want == nil && got != nil {
				t.Error("tt.want is nil but got is not")
				return
			}
			if diff := cmp.Diff(
				tt.want, got.Msg,
				cmp.Options{
					protocmp.Transform(),
					protocmp.IgnoreFields(
						&apiv2.Image{}, "meta", "expires_at",
					),
					protocmp.IgnoreFields(
						&apiv2.Meta{}, "created_at", "updated_at",
					),
				},
			); diff != "" {
				t.Errorf("imageServiceServer.Create() = %v, want %vņdiff: %s", got.Msg, tt.want, diff)
			}
		})
	}
}

func Test_imageServiceServer_Update(t *testing.T) {
	log := slog.Default()
	repo, container := test.StartRepository(t, log)
	defer func() {
		_ = container.Terminate(context.Background())
	}()
	ctx := context.Background()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "a image")
	}))
	url := ts.URL
	defer ts.Close()

	createImages(t, ctx, repo, []*adminv2.ImageServiceCreateRequest{
		{
			Image: &apiv2.Image{Id: "debian-12.0.20241231", Url: url, Features: []apiv2.ImageFeature{apiv2.ImageFeature_IMAGE_FEATURE_MACHINE}},
		},
	})

	tests := []struct {
		name           string
		request        *adminv2.ImageServiceUpdateRequest
		want           *adminv2.ImageServiceUpdateResponse
		wantReturnCode connect.Code
		wantErrMessage string
	}{
		{
			name:           "simple update on non existing",
			request:        &adminv2.ImageServiceUpdateRequest{Image: &apiv2.Image{Id: "debian-12.0.20250101"}},
			want:           nil,
			wantReturnCode: connect.CodeNotFound,
			wantErrMessage: "not_found: no image with id \"debian-12.0.20250101\" found",
		},
		{
			name:           "simple update on existing, invalid url",
			request:        &adminv2.ImageServiceUpdateRequest{Image: &apiv2.Image{Id: "debian-12.0.20241231", Url: "http://nonexisting"}},
			want:           nil,
			wantReturnCode: connect.CodeInvalidArgument,
			wantErrMessage: "invalid_argument: image:debian-12.0.20241231 is not accessible under:http://nonexisting error:Head \"http://nonexisting\": dial tcp: lookup nonexisting: Temporary failure in name resolution",
		},
		{
			name:    "simple update on existing update name",
			request: &adminv2.ImageServiceUpdateRequest{Image: &apiv2.Image{Id: "debian-12.0.20241231", Url: url, Name: pointer.Pointer("NewName")}},
			want: &adminv2.ImageServiceUpdateResponse{
				Image: &apiv2.Image{
					Id:             "debian-12.0.20241231",
					Url:            url,
					Name:           pointer.Pointer("NewName"),
					Description:    pointer.Pointer(""),
					Features:       []apiv2.ImageFeature{apiv2.ImageFeature_IMAGE_FEATURE_MACHINE},
					Classification: apiv2.ImageClassification_IMAGE_CLASSIFICATION_PREVIEW,
				},
			},
		},
		{
			name:    "simple update on existing update feature",
			request: &adminv2.ImageServiceUpdateRequest{Image: &apiv2.Image{Id: "debian-12.0.20241231", Url: url, Name: pointer.Pointer("NewName"), Features: []apiv2.ImageFeature{apiv2.ImageFeature_IMAGE_FEATURE_FIREWALL}}},
			want: &adminv2.ImageServiceUpdateResponse{
				Image: &apiv2.Image{
					Id:             "debian-12.0.20241231",
					Url:            url,
					Name:           pointer.Pointer("NewName"),
					Description:    pointer.Pointer(""),
					Features:       []apiv2.ImageFeature{apiv2.ImageFeature_IMAGE_FEATURE_FIREWALL},
					Classification: apiv2.ImageClassification_IMAGE_CLASSIFICATION_PREVIEW,
				},
			},
		},
		{
			name:    "simple update on existing update classification",
			request: &adminv2.ImageServiceUpdateRequest{Image: &apiv2.Image{Id: "debian-12.0.20241231", Url: url, Name: pointer.Pointer("NewName"), Features: []apiv2.ImageFeature{apiv2.ImageFeature_IMAGE_FEATURE_FIREWALL}, Classification: apiv2.ImageClassification_IMAGE_CLASSIFICATION_SUPPORTED}},
			want: &adminv2.ImageServiceUpdateResponse{
				Image: &apiv2.Image{
					Id:             "debian-12.0.20241231",
					Url:            url,
					Name:           pointer.Pointer("NewName"),
					Description:    pointer.Pointer(""),
					Features:       []apiv2.ImageFeature{apiv2.ImageFeature_IMAGE_FEATURE_FIREWALL},
					Classification: apiv2.ImageClassification_IMAGE_CLASSIFICATION_SUPPORTED,
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			i := &imageServiceServer{
				log:  log,
				repo: repo,
			}
			got, err := i.Update(ctx, connect.NewRequest(tt.request))
			if (err != nil) && tt.wantErrMessage == "" {
				t.Errorf("imageServiceServer.Create() error = %v, wantErr %s", err, tt.wantErrMessage)
				return
			}
			if (err != nil) && tt.wantErrMessage != err.Error() {
				t.Errorf("imageServiceServer.Create() error = %s, wantErr %s", err.Error(), tt.wantErrMessage)
				return
			}
			if err != nil {
				var connectErr *connect.Error
				if errors.As(err, &connectErr) {
					if tt.wantReturnCode != connectErr.Code() {
						t.Errorf("imageServiceServer.Create() code = %s, wantCode %s", connectErr.Code(), tt.wantReturnCode)
						return
					}
				}
			}
			if tt.want == nil && got == nil {
				return
			}
			if tt.want == nil && got != nil {
				t.Error("tt.want is nil but got is not")
				return
			}
			if diff := cmp.Diff(
				tt.want, got.Msg,
				cmp.Options{
					protocmp.Transform(),
					protocmp.IgnoreFields(
						&apiv2.Image{}, "meta", "expires_at",
					),
					protocmp.IgnoreFields(
						&apiv2.Meta{}, "created_at", "updated_at",
					),
				},
			); diff != "" {
				t.Errorf("imageServiceServer.Update() = %v, want %vņdiff: %s", got.Msg, tt.want, diff)
			}
		})
	}
}

func Test_imageServiceServer_Delete(t *testing.T) {
	log := slog.Default()
	repo, container := test.StartRepository(t, log)
	defer func() {
		_ = container.Terminate(context.Background())
	}()
	ctx := context.Background()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "a image")
	}))
	url := ts.URL
	defer ts.Close()

	createImages(t, ctx, repo, []*adminv2.ImageServiceCreateRequest{
		{
			Image: &apiv2.Image{Id: "debian-12.0.20241231", Url: url, Features: []apiv2.ImageFeature{apiv2.ImageFeature_IMAGE_FEATURE_MACHINE}},
		},
	})

	tests := []struct {
		name           string
		request        *adminv2.ImageServiceDeleteRequest
		want           *adminv2.ImageServiceDeleteResponse
		wantReturnCode connect.Code
		wantErrMessage string
	}{
		{
			name:           "simple delete on non existing",
			request:        &adminv2.ImageServiceDeleteRequest{Id: "debian-12.0.20250101"},
			want:           nil,
			wantReturnCode: connect.CodeNotFound,
			wantErrMessage: "not_found: no image with id \"debian-12.0.20250101\" found",
		},
		{
			name:    "simple delete on existing update name",
			request: &adminv2.ImageServiceDeleteRequest{Id: "debian-12.0.20241231"},
			want: &adminv2.ImageServiceDeleteResponse{
				Image: &apiv2.Image{
					Id:             "debian-12.0.20241231",
					Url:            url,
					Name:           pointer.Pointer(""),
					Description:    pointer.Pointer(""),
					Features:       []apiv2.ImageFeature{apiv2.ImageFeature_IMAGE_FEATURE_MACHINE},
					Classification: apiv2.ImageClassification_IMAGE_CLASSIFICATION_PREVIEW,
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			i := &imageServiceServer{
				log:  log,
				repo: repo,
			}
			got, err := i.Delete(ctx, connect.NewRequest(tt.request))
			if (err != nil) && tt.wantErrMessage == "" {
				t.Errorf("imageServiceServer.Delete() error = %v, wantErr %s", err, tt.wantErrMessage)
				return
			}
			if (err != nil) && tt.wantErrMessage != err.Error() {
				t.Errorf("imageServiceServer.Delete() error = %s, wantErr %s", err.Error(), tt.wantErrMessage)
				return
			}
			if err != nil {
				var connectErr *connect.Error
				if errors.As(err, &connectErr) {
					if tt.wantReturnCode != connectErr.Code() {
						t.Errorf("imageServiceServer.Delete() code = %s, wantCode %s", connectErr.Code(), tt.wantReturnCode)
						return
					}
				}
			}
			if tt.want == nil && got == nil {
				return
			}
			if tt.want == nil && got != nil {
				t.Error("tt.want is nil but got is not")
				return
			}
			if diff := cmp.Diff(
				tt.want, got.Msg,
				cmp.Options{
					protocmp.Transform(),
					protocmp.IgnoreFields(
						&apiv2.Image{}, "meta", "expires_at",
					),
					protocmp.IgnoreFields(
						&apiv2.Meta{}, "created_at", "updated_at",
					),
				},
			); diff != "" {
				t.Errorf("imageServiceServer.Delete() = %v, want %vņdiff: %s", got.Msg, tt.want, diff)
			}
		})
	}
}

func createImages(t *testing.T, ctx context.Context, repo *repository.Store, images []*adminv2.ImageServiceCreateRequest) {
	for _, img := range images {
		validated, err := repo.Image().ValidateCreate(ctx, img)
		require.NoError(t, err)
		_, err = repo.Image().Create(ctx, validated)
		require.NoError(t, err)
	}
}
