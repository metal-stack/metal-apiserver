package queries_test

import (
	"log/slog"
	"os"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/stretchr/testify/require"

	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-apiserver/pkg/db/queries"
	"github.com/metal-stack/metal-apiserver/pkg/test"
)

var (
	img1 = &metal.Image{
		Base:           metal.Base{ID: "debian-11", Name: "debian-11", Description: "Debian 11"},
		OS:             "debian",
		Version:        "11",
		URL:            "https://example.com/debian-11.tgz",
		Classification: metal.ClassificationSupported,
		Features:       map[metal.ImageFeatureType]bool{metal.ImageFeatureMachine: true},
	}
	img2 = &metal.Image{
		Base:           metal.Base{ID: "debian-12", Name: "debian-12", Description: "Debian 12"},
		OS:             "debian",
		Version:        "12",
		URL:            "https://example.com/debian-12.tgz",
		Classification: metal.ClassificationSupported,
		Features:       map[metal.ImageFeatureType]bool{metal.ImageFeatureMachine: true},
	}
	img3 = &metal.Image{
		Base:           metal.Base{ID: "ubuntu-22", Name: "ubuntu-22", Description: "Ubuntu 22"},
		OS:             "ubuntu",
		Version:        "22",
		URL:            "https://example.com/ubuntu-22.tgz",
		Classification: metal.ClassificationSupported,
		Features:       map[metal.ImageFeatureType]bool{metal.ImageFeatureMachine: true, metal.ImageFeatureFirewall: true},
	}
	img4 = &metal.Image{
		Base:           metal.Base{ID: "debian-10", Name: "debian-10", Description: "Old Debian"},
		OS:             "debian",
		Version:        "10",
		URL:            "https://example.com/debian-10.tgz",
		Classification: metal.ClassificationDeprecated,
		Features:       map[metal.ImageFeatureType]bool{metal.ImageFeatureMachine: true},
	}
	img5 = &metal.Image{
		Base:           metal.Base{ID: "firewall-image", Name: "firewall-only", Description: "Firewall Image"},
		OS:             "metalstack",
		Version:        "1",
		URL:            "https://example.com/firewall.tgz",
		Classification: metal.ClassificationSupported,
		Features:       map[metal.ImageFeatureType]bool{metal.ImageFeatureFirewall: true},
	}
	img6 = &metal.Image{
		Base:           metal.Base{ID: "preview-image", Name: "preview-os", Description: "Preview Image"},
		OS:             "previewos",
		Version:        "1",
		URL:            "https://example.com/preview.tgz",
		Classification: metal.ClassificationPreview,
		Features:       map[metal.ImageFeatureType]bool{metal.ImageFeatureMachine: true},
	}
	imgs = []*metal.Image{img1, img2, img3, img4, img5, img6}
)

func TestImageFilter(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	ds, _, rethinkCloser := test.StartRethink(t, log)
	defer func() {
		rethinkCloser()
	}()

	ctx := t.Context()

	for _, img := range imgs {
		_, err := ds.Image().Create(ctx, img)
		require.NoError(t, err)
	}

	tests := []struct {
		name string
		rq   *apiv2.ImageQuery
		want []*metal.Image
	}{
		{
			name: "empty request returns unfiltered",
			rq:   nil,
			want: []*metal.Image{img1, img2, img3, img4, img5, img6},
		},
		{
			name: "by id",
			rq:   &apiv2.ImageQuery{Id: &img1.ID},
			want: []*metal.Image{img1},
		},
		{
			name: "by os",
			rq:   &apiv2.ImageQuery{Os: &img1.OS},
			want: []*metal.Image{img1, img2, img4},
		},
		{
			name: "by os 2",
			rq:   &apiv2.ImageQuery{Os: &img3.OS},
			want: []*metal.Image{img3},
		},
		{
			name: "by version",
			rq:   &apiv2.ImageQuery{Version: &img2.Version},
			want: []*metal.Image{img2},
		},
		{
			name: "by name",
			rq:   &apiv2.ImageQuery{Name: &img3.Name},
			want: []*metal.Image{img3},
		},
		{
			name: "by description",
			rq:   &apiv2.ImageQuery{Description: &img4.Description},
			want: []*metal.Image{img4},
		},
		{
			name: "by machine feature",
			rq:   &apiv2.ImageQuery{Feature: apiv2.ImageFeature_IMAGE_FEATURE_MACHINE.Enum()},
			want: []*metal.Image{img1, img2, img3, img4, img6},
		},
		{
			name: "by firewall feature",
			rq:   &apiv2.ImageQuery{Feature: apiv2.ImageFeature_IMAGE_FEATURE_FIREWALL.Enum()},
			want: []*metal.Image{img3, img5},
		},
		{
			name: "by supported classification",
			rq:   &apiv2.ImageQuery{Classification: apiv2.ImageClassification_IMAGE_CLASSIFICATION_SUPPORTED.Enum()},
			want: []*metal.Image{img1, img2, img3, img5},
		},
		{
			name: "by deprecated classification",
			rq:   &apiv2.ImageQuery{Classification: apiv2.ImageClassification_IMAGE_CLASSIFICATION_DEPRECATED.Enum()},
			want: []*metal.Image{img4},
		},
		{
			name: "by preview classification",
			rq:   &apiv2.ImageQuery{Classification: apiv2.ImageClassification_IMAGE_CLASSIFICATION_PREVIEW.Enum()},
			want: []*metal.Image{img6},
		},
		{
			name: "by os and version combined",
			rq:   &apiv2.ImageQuery{Os: &img1.OS, Version: &img1.Version},
			want: []*metal.Image{img1},
		},
		{
			name: "by os and classification combined",
			rq:   &apiv2.ImageQuery{Os: &img1.OS, Classification: apiv2.ImageClassification_IMAGE_CLASSIFICATION_PREVIEW.Enum()},
			want: nil,
		},
		{
			name: "by machine feature and deprecated classification",
			rq:   &apiv2.ImageQuery{Feature: apiv2.ImageFeature_IMAGE_FEATURE_MACHINE.Enum(), Classification: apiv2.ImageClassification_IMAGE_CLASSIFICATION_DEPRECATED.Enum()},
			want: []*metal.Image{img4},
		},
		{
			name: "by machine feature and ubuntu os",
			rq:   &apiv2.ImageQuery{Os: &img3.OS, Feature: apiv2.ImageFeature_IMAGE_FEATURE_MACHINE.Enum()},
			want: []*metal.Image{img3},
		},
		{
			name: "no result by wrong os",
			rq:   &apiv2.ImageQuery{Os: new("nonexistent")},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ds.Image().List(ctx, queries.ImageFilter(tt.rq))
			require.NoError(t, err)

			sliceCmpOpts := []cmp.Option{
				cmpopts.SortSlices(func(a, b *metal.Image) bool { return a.ID < b.ID }),
				cmpopts.IgnoreFields(metal.Image{}, "Created", "Changed", "ExpirationDate"),
				cmpopts.IgnoreFields(metal.Base{}, "Created", "Changed", "Generation"),
			}

			if diff := cmp.Diff(tt.want, got, sliceCmpOpts...); diff != "" {
				t.Errorf("ImageFilter() = %v, want %v, diff: %s", got, tt.want, diff)
			}
		})
	}
}
