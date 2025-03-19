package admin

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"github.com/google/go-cmp/cmp"
	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	"github.com/metal-stack/metal-apiserver/pkg/test"
	"github.com/metal-stack/metal-lib/pkg/pointer"
	"google.golang.org/protobuf/testing/protocmp"
)

func Test_imageServiceServer_Create(t *testing.T) {
	log := slog.Default()
	repo, closer := test.StartRepository(t, log)
	defer closer()

	ctx := context.Background()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "a image")
	}))
	url := ts.URL
	defer ts.Close()

	tests := []struct {
		name    string
		request *adminv2.ImageServiceCreateRequest
		want    *adminv2.ImageServiceCreateResponse
		wantErr error
	}{
		{
			name:    "image url is empty",
			request: &adminv2.ImageServiceCreateRequest{Image: &apiv2.Image{Id: "debian-12.0.20241231"}},
			want:    nil,
			wantErr: errorutil.InvalidArgument(`image url must not be empty`),
		},
		{
			name:    "image feature is empty",
			request: &adminv2.ImageServiceCreateRequest{Image: &apiv2.Image{Id: "debian-12.0.20241231", Url: url}},
			want:    nil,
			wantErr: errorutil.InvalidArgument(`image features must not be empty`),
		},
		{
			name:    "valid image",
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
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			i := &imageServiceServer{
				log:  log,
				repo: repo,
			}
			got, err := i.Create(ctx, connect.NewRequest(tt.request))
			if diff := cmp.Diff(err, tt.wantErr, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("diff = %s", diff)
			}
			if diff := cmp.Diff(
				tt.want, pointer.SafeDeref(got).Msg,
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
	repo, closer := test.StartRepository(t, log)
	defer closer()

	ctx := context.Background()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.String(), "/invalid") {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		fmt.Fprintln(w, "a image")
	}))

	validURL := ts.URL
	invalidURL := ts.URL + "/invalid"

	defer ts.Close()

	test.CreateImages(t, repo, []*adminv2.ImageServiceCreateRequest{
		{
			Image: &apiv2.Image{Id: "debian-12.0.20241231", Url: validURL, Features: []apiv2.ImageFeature{apiv2.ImageFeature_IMAGE_FEATURE_MACHINE}},
		},
	})

	tests := []struct {
		name    string
		request *adminv2.ImageServiceUpdateRequest
		want    *adminv2.ImageServiceUpdateResponse
		wantErr error
	}{
		{
			name:    "simple update on non existing",
			request: &adminv2.ImageServiceUpdateRequest{Image: &apiv2.Image{Id: "debian-12.0.20250101"}},
			want:    nil,
			wantErr: errorutil.NotFound(`no image with id "debian-12.0.20250101" found`),
		},
		{
			name:    "simple update on existing, invalid url",
			request: &adminv2.ImageServiceUpdateRequest{Image: &apiv2.Image{Id: "debian-12.0.20241231", Url: invalidURL}},
			want:    nil,
			wantErr: errorutil.InvalidArgument(`image:debian-12.0.20241231 is not accessible under:%s statuscode:404`, invalidURL),
		},
		{
			name:    "simple update on existing update name",
			request: &adminv2.ImageServiceUpdateRequest{Image: &apiv2.Image{Id: "debian-12.0.20241231", Url: validURL, Name: pointer.Pointer("NewName")}},
			want: &adminv2.ImageServiceUpdateResponse{
				Image: &apiv2.Image{
					Id:             "debian-12.0.20241231",
					Url:            validURL,
					Name:           pointer.Pointer("NewName"),
					Description:    pointer.Pointer(""),
					Features:       []apiv2.ImageFeature{apiv2.ImageFeature_IMAGE_FEATURE_MACHINE},
					Classification: apiv2.ImageClassification_IMAGE_CLASSIFICATION_PREVIEW,
				},
			},
		},
		{
			name:    "simple update on existing update feature",
			request: &adminv2.ImageServiceUpdateRequest{Image: &apiv2.Image{Id: "debian-12.0.20241231", Url: validURL, Name: pointer.Pointer("NewName"), Features: []apiv2.ImageFeature{apiv2.ImageFeature_IMAGE_FEATURE_FIREWALL}}},
			want: &adminv2.ImageServiceUpdateResponse{
				Image: &apiv2.Image{
					Id:             "debian-12.0.20241231",
					Url:            validURL,
					Name:           pointer.Pointer("NewName"),
					Description:    pointer.Pointer(""),
					Features:       []apiv2.ImageFeature{apiv2.ImageFeature_IMAGE_FEATURE_FIREWALL},
					Classification: apiv2.ImageClassification_IMAGE_CLASSIFICATION_PREVIEW,
				},
			},
		},
		{
			name:    "simple update on existing update classification",
			request: &adminv2.ImageServiceUpdateRequest{Image: &apiv2.Image{Id: "debian-12.0.20241231", Url: validURL, Name: pointer.Pointer("NewName"), Features: []apiv2.ImageFeature{apiv2.ImageFeature_IMAGE_FEATURE_FIREWALL}, Classification: apiv2.ImageClassification_IMAGE_CLASSIFICATION_SUPPORTED}},
			want: &adminv2.ImageServiceUpdateResponse{
				Image: &apiv2.Image{
					Id:             "debian-12.0.20241231",
					Url:            validURL,
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
			if diff := cmp.Diff(err, tt.wantErr, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("diff = %s", diff)
			}

			if diff := cmp.Diff(
				tt.want, pointer.SafeDeref(got).Msg,
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
	repo, closer := test.StartRepository(t, log)
	defer closer()

	ctx := context.Background()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "a image")
	}))
	url := ts.URL
	defer ts.Close()

	test.CreateImages(t, repo, []*adminv2.ImageServiceCreateRequest{
		{
			Image: &apiv2.Image{Id: "debian-12.0.20241231", Url: url, Features: []apiv2.ImageFeature{apiv2.ImageFeature_IMAGE_FEATURE_MACHINE}},
		},
	})

	tests := []struct {
		name    string
		request *adminv2.ImageServiceDeleteRequest
		want    *adminv2.ImageServiceDeleteResponse
		wantErr error
	}{
		{
			name:    "simple delete on non existing",
			request: &adminv2.ImageServiceDeleteRequest{Id: "debian-12.0.20250101"},
			want:    nil,
			wantErr: errorutil.NotFound(`no image with id "debian-12.0.20250101" found`),
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
			if diff := cmp.Diff(err, tt.wantErr, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("diff = %s", diff)
			}
			if diff := cmp.Diff(
				tt.want, pointer.SafeDeref(got).Msg,
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
