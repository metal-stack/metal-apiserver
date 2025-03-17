package image

import (
	"context"
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
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	"github.com/metal-stack/metal-apiserver/pkg/test"
	"github.com/metal-stack/metal-lib/pkg/pointer"
	"google.golang.org/protobuf/testing/protocmp"
)

func Test_imageServiceServer_Get(t *testing.T) {
	log := slog.Default()
	repo, closer := test.StartRepository(t, log)
	defer func() {
		closer()
	}()
	ctx := context.Background()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "a image")
	}))
	url := ts.URL
	defer ts.Close()

	test.CreateImages(t, ctx, repo, []*adminv2.ImageServiceCreateRequest{
		{
			Image: &apiv2.Image{Id: "debian-12.0.20241231", Url: url, Features: []apiv2.ImageFeature{apiv2.ImageFeature_IMAGE_FEATURE_MACHINE}},
		},
	})

	tests := []struct {
		name    string
		request *apiv2.ImageServiceGetRequest
		want    *apiv2.ImageServiceGetResponse
		wantErr error
	}{
		{
			name:    "simple get non existing",
			request: &apiv2.ImageServiceGetRequest{Id: "debian-12.0.20250101"},
			want:    nil,
			wantErr: errorutil.NotFound(`no image with id "debian-12.0.20250101" found`),
		},
		{
			name:    "simple get existing",
			request: &apiv2.ImageServiceGetRequest{Id: "debian-12.0.20241231"},
			want: &apiv2.ImageServiceGetResponse{
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
			got, err := i.Get(ctx, connect.NewRequest(tt.request))
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
				t.Errorf("imageServiceServer.Get() = %v, want %vņdiff: %s", got.Msg, tt.want, diff)
			}
		})
	}
}

func Test_imageServiceServer_List(t *testing.T) {
	log := slog.Default()
	repo, closer := test.StartRepository(t, log)
	defer func() {
		closer()
	}()
	ctx := context.Background()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "a image")
	}))
	url := ts.URL
	defer ts.Close()

	test.CreateImages(t, ctx, repo, []*adminv2.ImageServiceCreateRequest{
		{
			Image: &apiv2.Image{Id: "debian-12.0.20241231", Url: url, Features: []apiv2.ImageFeature{apiv2.ImageFeature_IMAGE_FEATURE_MACHINE}, Classification: apiv2.ImageClassification_IMAGE_CLASSIFICATION_SUPPORTED},
		},
		{
			Image: &apiv2.Image{Id: "debian-12.0.20250101", Url: url, Features: []apiv2.ImageFeature{apiv2.ImageFeature_IMAGE_FEATURE_MACHINE}},
		},
		{
			Image: &apiv2.Image{Id: "firewall-12.0.20241231", Url: url, Features: []apiv2.ImageFeature{apiv2.ImageFeature_IMAGE_FEATURE_FIREWALL}},
		},
		{
			Image: &apiv2.Image{Id: "ubuntu-24.4.20241231", Url: url, Features: []apiv2.ImageFeature{apiv2.ImageFeature_IMAGE_FEATURE_MACHINE}},
		},
	})

	tests := []struct {
		name    string
		request *apiv2.ImageServiceListRequest
		want    *apiv2.ImageServiceListResponse
		wantErr error
	}{
		{
			name:    "list all",
			request: &apiv2.ImageServiceListRequest{},
			want: &apiv2.ImageServiceListResponse{
				Images: []*apiv2.Image{
					{
						Id:             "debian-12.0.20250101",
						Url:            url,
						Name:           pointer.Pointer(""),
						Description:    pointer.Pointer(""),
						Features:       []apiv2.ImageFeature{apiv2.ImageFeature_IMAGE_FEATURE_MACHINE},
						Classification: apiv2.ImageClassification_IMAGE_CLASSIFICATION_PREVIEW,
					},
					{
						Id:             "debian-12.0.20241231",
						Url:            url,
						Name:           pointer.Pointer(""),
						Description:    pointer.Pointer(""),
						Features:       []apiv2.ImageFeature{apiv2.ImageFeature_IMAGE_FEATURE_MACHINE},
						Classification: apiv2.ImageClassification_IMAGE_CLASSIFICATION_SUPPORTED,
					},
					{
						Id:             "firewall-12.0.20241231",
						Url:            url,
						Name:           pointer.Pointer(""),
						Description:    pointer.Pointer(""),
						Features:       []apiv2.ImageFeature{apiv2.ImageFeature_IMAGE_FEATURE_FIREWALL},
						Classification: apiv2.ImageClassification_IMAGE_CLASSIFICATION_PREVIEW,
					},
					{
						Id:             "ubuntu-24.4.20241231",
						Url:            url,
						Name:           pointer.Pointer(""),
						Description:    pointer.Pointer(""),
						Features:       []apiv2.ImageFeature{apiv2.ImageFeature_IMAGE_FEATURE_MACHINE},
						Classification: apiv2.ImageClassification_IMAGE_CLASSIFICATION_PREVIEW,
					},
				},
			},
		},
		{
			name:    "list firewall",
			request: &apiv2.ImageServiceListRequest{Query: &apiv2.ImageQuery{Feature: apiv2.ImageFeature_IMAGE_FEATURE_FIREWALL.Enum()}},
			want: &apiv2.ImageServiceListResponse{
				Images: []*apiv2.Image{
					{
						Id:             "firewall-12.0.20241231",
						Url:            url,
						Name:           pointer.Pointer(""),
						Description:    pointer.Pointer(""),
						Features:       []apiv2.ImageFeature{apiv2.ImageFeature_IMAGE_FEATURE_FIREWALL},
						Classification: apiv2.ImageClassification_IMAGE_CLASSIFICATION_PREVIEW,
					},
				},
			},
		},
		{
			name:    "list supported machine",
			request: &apiv2.ImageServiceListRequest{Query: &apiv2.ImageQuery{Feature: apiv2.ImageFeature_IMAGE_FEATURE_MACHINE.Enum(), Classification: apiv2.ImageClassification_IMAGE_CLASSIFICATION_SUPPORTED.Enum()}},
			want: &apiv2.ImageServiceListResponse{
				Images: []*apiv2.Image{
					{
						Id:             "debian-12.0.20241231",
						Url:            url,
						Name:           pointer.Pointer(""),
						Description:    pointer.Pointer(""),
						Features:       []apiv2.ImageFeature{apiv2.ImageFeature_IMAGE_FEATURE_MACHINE},
						Classification: apiv2.ImageClassification_IMAGE_CLASSIFICATION_SUPPORTED,
					},
				},
			},
		},
		{
			name:    "list debian image",
			request: &apiv2.ImageServiceListRequest{Query: &apiv2.ImageQuery{Feature: apiv2.ImageFeature_IMAGE_FEATURE_MACHINE.Enum(), Os: pointer.Pointer("debian")}},
			want: &apiv2.ImageServiceListResponse{
				Images: []*apiv2.Image{
					{
						Id:             "debian-12.0.20250101",
						Url:            url,
						Name:           pointer.Pointer(""),
						Description:    pointer.Pointer(""),
						Features:       []apiv2.ImageFeature{apiv2.ImageFeature_IMAGE_FEATURE_MACHINE},
						Classification: apiv2.ImageClassification_IMAGE_CLASSIFICATION_PREVIEW,
					},
					{
						Id:             "debian-12.0.20241231",
						Url:            url,
						Name:           pointer.Pointer(""),
						Description:    pointer.Pointer(""),
						Features:       []apiv2.ImageFeature{apiv2.ImageFeature_IMAGE_FEATURE_MACHINE},
						Classification: apiv2.ImageClassification_IMAGE_CLASSIFICATION_SUPPORTED,
					},
				},
			},
		},
		{
			name:    "list non existing image",
			request: &apiv2.ImageServiceListRequest{Query: &apiv2.ImageQuery{Feature: apiv2.ImageFeature_IMAGE_FEATURE_FIREWALL.Enum(), Os: pointer.Pointer("debian")}},
			want: &apiv2.ImageServiceListResponse{
				Images: []*apiv2.Image{},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			i := &imageServiceServer{
				log:  log,
				repo: repo,
			}
			got, err := i.List(ctx, connect.NewRequest(tt.request))
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
				t.Errorf("imageServiceServer.List() = %v, want %vņdiff: %s", got.Msg, tt.want, diff)
			}
		})
	}
}

func Test_imageServiceServer_Latest(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	repo, closer := test.StartRepository(t, log)
	defer func() {
		closer()
	}()
	ctx := context.Background()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "a image")
	}))
	url := ts.URL
	defer ts.Close()

	test.CreateImages(t, ctx, repo, []*adminv2.ImageServiceCreateRequest{
		{
			Image: &apiv2.Image{Id: "debian-12.0.20241231", Url: url, Features: []apiv2.ImageFeature{apiv2.ImageFeature_IMAGE_FEATURE_MACHINE}},
		},
		{
			Image: &apiv2.Image{Id: "debian-12.0.20250101", Url: url, Features: []apiv2.ImageFeature{apiv2.ImageFeature_IMAGE_FEATURE_MACHINE}},
		},
		{
			Image: &apiv2.Image{Id: "debian-11.0.20250101", Url: url, Features: []apiv2.ImageFeature{apiv2.ImageFeature_IMAGE_FEATURE_MACHINE}},
		},
		{
			Image: &apiv2.Image{Id: "debian-12.0.20250201", Url: url, Features: []apiv2.ImageFeature{apiv2.ImageFeature_IMAGE_FEATURE_MACHINE}},
		},
	})

	tests := []struct {
		name    string
		request *apiv2.ImageServiceLatestRequest
		want    *apiv2.ImageServiceLatestResponse
		wantErr error
	}{
		{
			name:    "list latest debian",
			request: &apiv2.ImageServiceLatestRequest{Os: "debian-12"},
			want: &apiv2.ImageServiceLatestResponse{
				Image: &apiv2.Image{
					Id:             "debian-12.0.20250201",
					Url:            url,
					Name:           pointer.Pointer(""),
					Description:    pointer.Pointer(""),
					Features:       []apiv2.ImageFeature{apiv2.ImageFeature_IMAGE_FEATURE_MACHINE},
					Classification: apiv2.ImageClassification_IMAGE_CLASSIFICATION_PREVIEW,
				},
			},
		},
		{
			name:    "list latest ubuntu which does not match",
			request: &apiv2.ImageServiceLatestRequest{Os: "ubuntu-24.4"},
			want:    nil,
			wantErr: errorutil.NotFound(`no image for os:ubuntu version:24.4.0 found`),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			i := &imageServiceServer{
				log:  log,
				repo: repo,
			}
			got, err := i.Latest(ctx, connect.NewRequest(tt.request))
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
				t.Errorf("imageServiceServer.Latest() = %v, want %vņdiff: %s", got.Msg, tt.want, diff)
			}
		})
	}
}
