package repository

import (
	"context"
	"strings"
	"time"

	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	metalcommon "github.com/metal-stack/metal-lib/pkg/metal"
)

func (r *imageRepository) validateCreate(ctx context.Context, req *adminv2.ImageServiceCreateRequest) error {
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
	os, _, err := metalcommon.GetOsAndSemverFromImage(image.Id)
	if err != nil {
		return errorutil.NewInvalidArgument(err)
	}

	if err := r.ensureUniqueUrlPerOs(ctx, os, req.Image.Url, nil); err != nil {
		return err
	}

	if image.ExpiresAt != nil && !image.ExpiresAt.AsTime().IsZero() {
		if image.ExpiresAt.AsTime().Before(time.Now()) {
			return errorutil.InvalidArgument("image expiresAt must be in the future")
		}
	}
	return nil
}

func (r *imageRepository) validateUpdate(ctx context.Context, req *adminv2.ImageServiceUpdateRequest, _ *metal.Image) error {
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
	os, _, err := metalcommon.GetOsAndSemverFromImage(image.Id)
	if err != nil {
		return errorutil.NewInvalidArgument(err)
	}

	if err := r.ensureUniqueUrlPerOs(ctx, os, req.Image.Url, &image.Id); err != nil {
		return err
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

// Solves https://github.com/metal-stack/metal-api/issues/92
func (r *imageRepository) ensureUniqueUrlPerOs(ctx context.Context, os, url string, imageId *string) error {
	images, err := r.s.Image().List(ctx, &apiv2.ImageQuery{
		Os:  &os,
		Url: &url,
	})
	if err != nil {
		return errorutil.NewInternal(err)
	}
	var imageIds []string
	for _, img := range images {
		if imageId != nil && *imageId == img.ID {
			continue
		}
		imageIds = append(imageIds, img.ID)
	}
	if len(imageIds) > 0 {
		return errorutil.InvalidArgument("image url already configured for %s", strings.Join(imageIds, ","))
	}
	return nil
}
