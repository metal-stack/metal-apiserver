package repository

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Masterminds/semver/v3"
	metalcommon "github.com/metal-stack/metal-lib/pkg/metal"
	"github.com/metal-stack/metal-lib/pkg/pointer"
	"google.golang.org/protobuf/types/known/timestamppb"

	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-apiserver/pkg/db/queries"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
)

type imageRepository struct {
	s *Store
}

func (r *imageRepository) validateCreate(ctx context.Context, req *adminv2.ImageServiceCreateRequest) error {
	// FIXME use validate helper
	image := req.Image
	if image.Id == "" {
		return errorutil.InvalidArgument("image id must not be empty")
	}
	if image.Url == "" {
		return errorutil.InvalidArgument("image url must not be empty")
	}
	if err := checkIfUrlExists(ctx, "image", image.Id, image.Url); err != nil {
		return errorutil.NewInvalidArgument(err)
	}
	if len(image.Features) == 0 {
		return errorutil.InvalidArgument("image features must not be empty")
	}
	if _, err := metal.ImageFeaturesFrom(image.Features); err != nil {
		return errorutil.NewInvalidArgument(err)
	}
	if _, err := metal.VersionClassificationFrom(image.Classification); err != nil {
		return errorutil.NewInvalidArgument(err)
	}
	if _, _, err := metalcommon.GetOsAndSemverFromImage(image.Id); err != nil {
		return errorutil.NewInvalidArgument(err)
	}
	if image.ExpiresAt != nil && !image.ExpiresAt.AsTime().IsZero() {
		if image.ExpiresAt.AsTime().Before(time.Now()) {
			return errorutil.InvalidArgument("image expiresAt must be in the future")
		}
	}
	// FIXME implement: https://github.com/metal-stack/metal-api/issues/92
	return nil
}

func (r *imageRepository) validateUpdate(ctx context.Context, req *adminv2.ImageServiceUpdateRequest, _ *metal.Image) error {
	// FIXME use validate helper
	image := req.Image
	if image.Id == "" {
		return errorutil.InvalidArgument("image id must not be empty")
	}
	if image.Url != "" {
		if err := checkIfUrlExists(ctx, "image", image.Id, image.Url); err != nil {
			return errorutil.NewInvalidArgument(err)
		}
	}
	if len(image.Features) >= 0 {
		if _, err := metal.ImageFeaturesFrom(image.Features); err != nil {
			return errorutil.NewInvalidArgument(err)
		}
	}
	if _, err := metal.VersionClassificationFrom(image.Classification); err != nil {
		return errorutil.NewInvalidArgument(err)
	}
	if _, _, err := metalcommon.GetOsAndSemverFromImage(image.Id); err != nil {
		return errorutil.NewInvalidArgument(err)
	}
	if image.ExpiresAt != nil && !image.ExpiresAt.AsTime().IsZero() {
		if image.ExpiresAt.AsTime().Before(time.Now()) {
			return errorutil.InvalidArgument("image expiresAt must be in the future")
		}
	}

	return nil
}

func (r *imageRepository) validateDelete(ctx context.Context, img *metal.Image) error {
	machines, err := r.s.UnscopedMachine().List(ctx, &apiv2.MachineQuery{
		Allocation: &apiv2.MachineAllocationQuery{Image: &img.ID},
	})
	if err != nil {
		return err
	}
	if len(machines) > 0 {
		return errorutil.InvalidArgument("cannot remove image with existing machine allocations")
	}
	return nil
}

func (r *imageRepository) get(ctx context.Context, id string) (*metal.Image, error) {
	fsl, err := r.s.ds.Image().Get(ctx, id)
	if err != nil {
		return nil, err
	}

	return fsl, nil
}

// Image is not project scoped
func (r *imageRepository) matchScope(_ *metal.Image) bool {
	return true
}

func (r *imageRepository) create(ctx context.Context, rq *adminv2.ImageServiceCreateRequest) (*metal.Image, error) {
	fsl, err := r.convertToInternal(rq.Image)
	if err != nil {
		return nil, err
	}

	resp, err := r.s.ds.Image().Create(ctx, fsl)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

func (r *imageRepository) update(ctx context.Context, e *metal.Image, rq *adminv2.ImageServiceUpdateRequest) (*metal.Image, error) {
	image := rq.Image

	old, err := r.get(ctx, image.Id)
	if err != nil {
		return nil, err
	}
	new := *old

	if image.Name != nil {
		new.Name = *image.Name
	}
	if image.Description != nil {
		new.Description = *image.Description
	}
	if image.ExpiresAt != nil {
		new.ExpirationDate = image.ExpiresAt.AsTime()
	}
	if image.Classification != apiv2.ImageClassification_IMAGE_CLASSIFICATION_UNSPECIFIED {
		classification, err := metal.VersionClassificationFrom(image.Classification)
		if err != nil {
			return nil, err
		}
		new.Classification = classification
	}
	if len(image.Features) != 0 {
		features, err := metal.ImageFeaturesFrom(image.Features)
		if err != nil {
			return nil, err
		}
		new.Features = features
	}
	if image.Url != "" {
		new.URL = image.Url
	}

	err = r.s.ds.Image().Update(ctx, &new)
	if err != nil {
		return nil, err
	}

	return &new, nil
}

func (r *imageRepository) delete(ctx context.Context, e *metal.Image) error {
	err := r.s.ds.Image().Delete(ctx, e)
	if err != nil {
		return err
	}

	return nil
}

func (r *imageRepository) find(ctx context.Context, rq *apiv2.ImageQuery) (*metal.Image, error) {
	image, err := r.s.ds.Image().Find(ctx, queries.ImageFilter(rq))
	if err != nil {
		return nil, err
	}

	return image, nil
}

func (r *imageRepository) list(ctx context.Context, rq *apiv2.ImageQuery) ([]*metal.Image, error) {
	images, err := r.s.ds.Image().List(ctx, queries.ImageFilter(rq))
	if err != nil {
		return nil, err
	}

	return images, nil
}

func (r *imageRepository) convertToInternal(msg *apiv2.Image) (*metal.Image, error) {
	features, err := metal.ImageFeaturesFrom(msg.Features)
	if err != nil {
		return nil, err
	}
	classification, err := metal.VersionClassificationFrom(msg.Classification)
	if err != nil {
		return nil, err
	}
	expiresAt := time.Now().Add(metal.DefaultImageExpiration)
	if msg.ExpiresAt != nil {
		expiresAt = msg.ExpiresAt.AsTime()
	}
	os, v, err := metalcommon.GetOsAndSemverFromImage(msg.Id)
	if err != nil {
		return nil, err
	}

	image := &metal.Image{
		Base: metal.Base{
			ID:          msg.Id,
			Name:        pointer.SafeDeref(msg.Name),
			Description: pointer.SafeDeref(msg.Description),
		},
		URL:            msg.Url,
		Features:       features,
		OS:             os,
		Version:        v.String(),
		ExpirationDate: expiresAt,
		Classification: classification,
	}
	return image, nil
}
func (r *imageRepository) convertToProto(in *metal.Image) (*apiv2.Image, error) {
	var features []apiv2.ImageFeature
	for feature := range in.Features {
		switch feature {
		case metal.ImageFeatureMachine:
			features = append(features, apiv2.ImageFeature_IMAGE_FEATURE_MACHINE)
		case metal.ImageFeatureFirewall:
			features = append(features, apiv2.ImageFeature_IMAGE_FEATURE_FIREWALL)
		default:
			return nil, fmt.Errorf("invalid image feature:%s", feature)
		}
	}
	var classification apiv2.ImageClassification
	switch in.Classification {
	case metal.ClassificationDeprecated:
		classification = apiv2.ImageClassification_IMAGE_CLASSIFICATION_DEPRECATED
	case metal.ClassificationPreview:
		classification = apiv2.ImageClassification_IMAGE_CLASSIFICATION_PREVIEW
	case metal.ClassificationSupported:
		classification = apiv2.ImageClassification_IMAGE_CLASSIFICATION_SUPPORTED
	default:
		return nil, fmt.Errorf("invalid image classification:%s", classification)
	}

	image := &apiv2.Image{
		Id:          in.ID,
		Name:        &in.Name,
		Description: &in.Description,
		Meta: &apiv2.Meta{
			CreatedAt: timestamppb.New(in.GetCreated()),
			UpdatedAt: timestamppb.New(in.GetChanged()),
		},
		Url:            in.URL,
		Features:       features,
		Classification: classification,
		ExpiresAt:      timestamppb.New(in.ExpirationDate),
	}
	return image, nil
}

// GetMostRecentImageFor
// the id is in the form of: <name>-<version>
// where name is for example ubuntu or firewall
// version must be a semantic version, see https://semver.org/
// we decided to specify the version in the form of major.minor.patch,
// where patch is in the form of YYYYMMDD
// If version is not fully specified, e.g. ubuntu-19.10 or ubuntu-19.10
// then the most recent ubuntu image (ubuntu-19.10.20200407) is returned
// If patch is specified e.g. ubuntu-20.04.20200502 then this exact image is searched.
func (r *imageRepository) GetMostRecentImageFor(id string, images []*metal.Image) (*metal.Image, error) {
	os, sv, err := metalcommon.GetOsAndSemverFromImage(id)
	if err != nil {
		return nil, err
	}

	matcher := "~"
	// if patch is given return a exact match
	if sv.Patch() > 0 {
		matcher = "="
	}
	constraint, err := semver.NewConstraint(matcher + sv.String())
	if err != nil {
		return nil, fmt.Errorf("could not create constraint of image version:%s err:%w", sv, err)
	}

	var latestImage *metal.Image
	sortedImages := r.SortImages(images)
	for i := range sortedImages {
		image := sortedImages[i]
		if os != image.OS {
			continue
		}
		v, err := semver.NewVersion(image.Version)
		if err != nil {
			continue
		}
		if constraint.Check(v) {
			latestImage = image
			break
		}
	}
	if latestImage != nil {
		return latestImage, nil
	}
	return nil, errorutil.NotFound("no image for os:%s version:%s found", os, sv)
}

func (r *imageRepository) SortImages(images []*metal.Image) []*metal.Image {
	sort.SliceStable(images, func(i, j int) bool {
		c := strings.Compare(images[i].OS, images[j].OS)
		// OS is equal
		if c == 0 {
			iv, err := semver.NewVersion(images[i].Version)
			if err != nil {
				return false
			}
			jv, err := semver.NewVersion(images[j].Version)
			if err != nil {
				return true
			}
			return iv.GreaterThan(jv)
		}
		return c <= 0
	})
	return images
}
