package q_test

import (
	"log/slog"
	"os"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/generic/pg"
	"github.com/metal-stack/metal-apiserver/pkg/db/generic/pg/q"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-apiserver/pkg/test"
	"github.com/stretchr/testify/require"
)

var (
	pgImage1 = &metal.Image{
		ID: "debian-11", Name: "debian-11", Description: "Debian 11",
		OS:             "debian",
		Version:        "11",
		URL:            "https://example.com/debian-11.tgz",
		Classification: metal.ClassificationSupported,
		Features:       map[metal.ImageFeatureType]bool{metal.ImageFeatureMachine: true},
	}
	pgImage2 = &metal.Image{
		ID: "debian-12", Name: "debian-12", Description: "Debian 12",
		OS:             "debian",
		Version:        "12",
		URL:            "https://example.com/debian-12.tgz",
		Classification: metal.ClassificationSupported,
		Features:       map[metal.ImageFeatureType]bool{metal.ImageFeatureMachine: true},
	}
	pgImage3 = &metal.Image{
		ID: "ubuntu-22", Name: "ubuntu-22", Description: "Ubuntu 22",
		OS:             "ubuntu",
		Version:        "22",
		URL:            "https://example.com/ubuntu-22.tgz",
		Classification: metal.ClassificationSupported,
		Features:       map[metal.ImageFeatureType]bool{metal.ImageFeatureMachine: true, metal.ImageFeatureFirewall: true},
		Labels:         map[string]string{"a": "b"},
	}
	pgImage4 = &metal.Image{
		ID: "debian-10", Name: "debian-10", Description: "Old Debian",
		OS:             "debian",
		Version:        "10",
		URL:            "https://example.com/debian-10.tgz",
		Classification: metal.ClassificationDeprecated,
		Features:       map[metal.ImageFeatureType]bool{metal.ImageFeatureMachine: true},
	}
	pgImage5 = &metal.Image{
		ID: "firewall-image", Name: "firewall-only", Description: "Firewall Image",
		OS:             "metalstack",
		Version:        "1",
		URL:            "https://example.com/firewall.tgz",
		Classification: metal.ClassificationSupported,
		Features:       map[metal.ImageFeatureType]bool{metal.ImageFeatureFirewall: true},
	}
	pgImage6 = &metal.Image{
		ID: "preview-image", Name: "preview-os", Description: "Preview Image",
		OS:             "previewos",
		Version:        "1",
		URL:            "https://example.com/preview.tgz",
		Classification: metal.ClassificationPreview,
		Features:       map[metal.ImageFeatureType]bool{metal.ImageFeatureMachine: true},
	}
	pgImages = []*metal.Image{pgImage1, pgImage2, pgImage3, pgImage4, pgImage5, pgImage6}
)

// TestImageFilterPostgres mirrors the RethinkDB-backed TestImageFilter from
// pkg/db/queries, but runs against the Postgres backend using q.ImageFilter.
func TestImageFilterPostgres(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	db, closer := test.StartPostgres(t, log)
	defer closer()

	repo, err := pg.NewGenericRepository[*metal.Image](log, db)
	require.NoError(t, err)

	ctx := t.Context()

	for _, img := range pgImages {
		require.NoError(t, repo.Create(ctx, img.GetID(), img))
	}

	tests := []struct {
		name string
		rq   *apiv2.ImageQuery
		want []*metal.Image
	}{
		{name: "empty request returns unfiltered", rq: nil, want: pgImages},
		{name: "by id", rq: &apiv2.ImageQuery{Id: &pgImage1.ID}, want: []*metal.Image{pgImage1}},
		{name: "by os", rq: &apiv2.ImageQuery{Os: &pgImage1.OS}, want: []*metal.Image{pgImage1, pgImage2, pgImage4}},
		{name: "by os 2", rq: &apiv2.ImageQuery{Os: &pgImage3.OS}, want: []*metal.Image{pgImage3}},
		{name: "by version", rq: &apiv2.ImageQuery{Version: &pgImage2.Version}, want: []*metal.Image{pgImage2}},
		{name: "by name", rq: &apiv2.ImageQuery{Name: &pgImage3.Name}, want: []*metal.Image{pgImage3}},
		{name: "by description", rq: &apiv2.ImageQuery{Description: &pgImage4.Description}, want: []*metal.Image{pgImage4}},
		{name: "by label", rq: &apiv2.ImageQuery{Labels: &apiv2.Labels{Labels: pgImage3.Labels}}, want: []*metal.Image{pgImage3}},
		{name: "by machine feature", rq: &apiv2.ImageQuery{Feature: apiv2.ImageFeature_IMAGE_FEATURE_MACHINE.Enum()}, want: []*metal.Image{pgImage1, pgImage2, pgImage3, pgImage4, pgImage6}},
		{name: "by firewall feature", rq: &apiv2.ImageQuery{Feature: apiv2.ImageFeature_IMAGE_FEATURE_FIREWALL.Enum()}, want: []*metal.Image{pgImage3, pgImage5}},
		{name: "by supported classification", rq: &apiv2.ImageQuery{Classification: apiv2.ImageClassification_IMAGE_CLASSIFICATION_SUPPORTED.Enum()}, want: []*metal.Image{pgImage1, pgImage2, pgImage3, pgImage5}},
		{name: "by deprecated classification", rq: &apiv2.ImageQuery{Classification: apiv2.ImageClassification_IMAGE_CLASSIFICATION_DEPRECATED.Enum()}, want: []*metal.Image{pgImage4}},
		{name: "by preview classification", rq: &apiv2.ImageQuery{Classification: apiv2.ImageClassification_IMAGE_CLASSIFICATION_PREVIEW.Enum()}, want: []*metal.Image{pgImage6}},
		{name: "by os and version combined", rq: &apiv2.ImageQuery{Os: &pgImage1.OS, Version: &pgImage1.Version}, want: []*metal.Image{pgImage1}},
		{name: "by os and classification combined", rq: &apiv2.ImageQuery{Os: &pgImage1.OS, Classification: apiv2.ImageClassification_IMAGE_CLASSIFICATION_PREVIEW.Enum()}, want: nil},
		{name: "by machine feature and deprecated classification", rq: &apiv2.ImageQuery{Feature: apiv2.ImageFeature_IMAGE_FEATURE_MACHINE.Enum(), Classification: apiv2.ImageClassification_IMAGE_CLASSIFICATION_DEPRECATED.Enum()}, want: []*metal.Image{pgImage4}},
		{name: "by machine feature and ubuntu os", rq: &apiv2.ImageQuery{Os: &pgImage3.OS, Feature: apiv2.ImageFeature_IMAGE_FEATURE_MACHINE.Enum()}, want: []*metal.Image{pgImage3}},
		{name: "no result by wrong os", rq: &apiv2.ImageQuery{Os: new("nonexistent")}, want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := queryData(t, ctx, repo, q.ImageFilter(tt.rq))
			if diff := cmp.Diff(
				tt.want, got,
				cmpopts.SortSlices(func(a, b *metal.Image) bool { return a.ID < b.ID }),
				cmpopts.IgnoreFields(metal.Image{}, "Created", "Changed", "ExpirationDate"),
				cmpopts.IgnoreFields(metal.Base{}, "Created", "Changed", "Generation"),
			); diff != "" {
				t.Errorf("ImageFilter() = %v, want %v diff: %s", got, tt.want, diff)
			}
		})
	}
}
