package admin

import (
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
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	"github.com/metal-stack/metal-apiserver/pkg/test"
	"github.com/metal-stack/metal-lib/pkg/pointer"
	"google.golang.org/protobuf/testing/protocmp"
)

func Test_imageServiceServer_Create(t *testing.T) {
	log := slog.Default()

	testStore, closer := test.StartRepositoryWithCleanup(t, log)
	defer closer()
	repo := testStore.Store

	ctx := t.Context()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintln(w, "a image")
	}))
	defer ts.Close()
	url := ts.URL
	url2 := url + "/url2"
	url3 := url + "/url3"

	test.CreateImages(t, repo, []*adminv2.ImageServiceCreateRequest{
		{
			Image: &apiv2.Image{Id: "debian-13.0.20241231", Url: url2, Features: []apiv2.ImageFeature{apiv2.ImageFeature_IMAGE_FEATURE_MACHINE}},
		},
		{
			Image: &apiv2.Image{Id: "ubuntu-24.4.20241231", Url: url3, Features: []apiv2.ImageFeature{apiv2.ImageFeature_IMAGE_FEATURE_MACHINE}},
		},
	})

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
			name:    "image url is already set on a other image",
			request: &adminv2.ImageServiceCreateRequest{Image: &apiv2.Image{Id: "debian-12.0.20241231", Url: url2, Features: []apiv2.ImageFeature{apiv2.ImageFeature_IMAGE_FEATURE_MACHINE}}},
			want:    nil,
			wantErr: errorutil.InvalidArgument(`image url already configured for debian-13.0.20241231`),
		},
		{
			name:    "image url is already set on a other image but different os, which is allowed",
			request: &adminv2.ImageServiceCreateRequest{Image: &apiv2.Image{Id: "debian-13.0.20251231", Url: url3, Features: []apiv2.ImageFeature{apiv2.ImageFeature_IMAGE_FEATURE_MACHINE}}},
			want: &adminv2.ImageServiceCreateResponse{
				Image: &apiv2.Image{
					Id:             "debian-13.0.20251231",
					Url:            url3,
					Name:           pointer.Pointer(""),
					Description:    pointer.Pointer(""),
					Features:       []apiv2.ImageFeature{apiv2.ImageFeature_IMAGE_FEATURE_MACHINE},
					Classification: apiv2.ImageClassification_IMAGE_CLASSIFICATION_PREVIEW,
				},
			},
			wantErr: nil,
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
				protocmp.Transform(),
				protocmp.IgnoreFields(
					&apiv2.Image{}, "meta", "expires_at",
				),
				protocmp.IgnoreFields(
					&apiv2.Meta{}, "created_at", "updated_at",
				),
			); diff != "" {
				t.Errorf("imageServiceServer.Create() = %v, want %vņdiff: %s", got.Msg, tt.want, diff)
			}
		})
	}
}

func Test_imageServiceServer_Update(t *testing.T) {
	log := slog.Default()

	testStore, closer := test.StartRepositoryWithCleanup(t, log)
	defer closer()
	repo := testStore.Store

	ctx := t.Context()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.String(), "/invalid") {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		_, _ = fmt.Fprintln(w, "a image")
	}))

	validURL := ts.URL
	invalidURL := ts.URL + "/invalid"
	url2 := validURL + "/url2"

	defer ts.Close()

	test.CreateImages(t, repo, []*adminv2.ImageServiceCreateRequest{
		{
			Image: &apiv2.Image{Id: "debian-12.0.20241231", Url: validURL, Features: []apiv2.ImageFeature{apiv2.ImageFeature_IMAGE_FEATURE_MACHINE}},
		},
		{
			Image: &apiv2.Image{Id: "debian-13.0.20241231", Url: url2, Features: []apiv2.ImageFeature{apiv2.ImageFeature_IMAGE_FEATURE_MACHINE}},
		},
	})

	tests := []struct {
		name    string
		request *adminv2.ImageServiceUpdateRequest
		want    *adminv2.ImageServiceUpdateResponse
		wantErr error
	}{
		{
			name:    "update on non existing",
			request: &adminv2.ImageServiceUpdateRequest{Image: &apiv2.Image{Id: "debian-12.0.20250101"}},
			want:    nil,
			wantErr: errorutil.NotFound(`no image with id "debian-12.0.20250101" found`),
		},
		{
			name:    "update on existing, invalid url",
			request: &adminv2.ImageServiceUpdateRequest{Image: &apiv2.Image{Id: "debian-12.0.20241231", Url: invalidURL}},
			want:    nil,
			wantErr: errorutil.InvalidArgument(`image:debian-12.0.20241231 is not accessible under:%s statuscode:404`, invalidURL),
		},
		{
			name:    "update on existing, existing url",
			request: &adminv2.ImageServiceUpdateRequest{Image: &apiv2.Image{Id: "debian-13.0.20241231", Url: validURL}},
			want:    nil,
			wantErr: errorutil.InvalidArgument(`image url already configured for debian-12.0.20241231`),
		},
		{
			name:    "update on existing name",
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
			name:    "update on existing feature",
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
			name:    "update on existing classification",
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
				protocmp.Transform(),
				protocmp.IgnoreFields(
					&apiv2.Image{}, "meta", "expires_at",
				),
				protocmp.IgnoreFields(
					&apiv2.Meta{}, "created_at", "updated_at",
				),
			); diff != "" {
				t.Errorf("imageServiceServer.Update() = %v, want %vņdiff: %s", got.Msg, tt.want, diff)
			}
		})
	}
}

func Test_imageServiceServer_Delete(t *testing.T) {
	log := slog.Default()

	testStore, closer := test.StartRepositoryWithCleanup(t, log)
	defer closer()
	repo := testStore.Store

	ctx := t.Context()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintln(w, "a image")
	}))
	url := ts.URL
	url2 := url + "/url2"
	defer ts.Close()

	test.CreateImages(t, repo, []*adminv2.ImageServiceCreateRequest{
		{
			Image: &apiv2.Image{Id: "debian-12.0.20241231", Url: url, Features: []apiv2.ImageFeature{apiv2.ImageFeature_IMAGE_FEATURE_MACHINE}},
		},
		{
			Image: &apiv2.Image{Id: "debian-11.0.20221231", Url: url2, Features: []apiv2.ImageFeature{apiv2.ImageFeature_IMAGE_FEATURE_MACHINE}},
		},
	})

	test.CreateMachines(t, testStore, []*metal.Machine{
		{
			Base: metal.Base{ID: "m1"},
			Allocation: &metal.MachineAllocation{
				ImageID: "debian-11.0.20221231",
			},
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
		{
			name:    "delete image with existing allocated machine",
			request: &adminv2.ImageServiceDeleteRequest{Id: "debian-11.0.20221231"},
			want:    nil,
			wantErr: errorutil.InvalidArgument(`cannot remove image with existing machine allocations`),
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
				protocmp.Transform(),
				protocmp.IgnoreFields(
					&apiv2.Image{}, "meta", "expires_at",
				),
				protocmp.IgnoreFields(
					&apiv2.Meta{}, "created_at", "updated_at",
				),
			); diff != "" {
				t.Errorf("imageServiceServer.Delete() = %v, want %vņdiff: %s", got.Msg, tt.want, diff)
			}
		})
	}
}
