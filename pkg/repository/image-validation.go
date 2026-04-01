package repository

import (
	"context"
	"fmt"
	"time"

	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-apiserver/pkg/db/queries"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	metalcommon "github.com/metal-stack/metal-lib/pkg/metal"
)

func (r *imageRepository) validateCreate(ctx context.Context, req *adminv2.ImageServiceCreateRequest) error {
	// FIXME use validate helper
	image := req.Image

	if image.Id == "" {
		return fmt.Errorf("image id must not be empty")
	}

	if image.Url == "" {
		return fmt.Errorf("image url must not be empty")
	}

	if err := checkIfUrlExists(ctx, "image", image.Id, image.Url); err != nil {
		return err
	}

	if len(image.Features) == 0 {
		return fmt.Errorf("image features must not be empty")
	}

	if _, err := metal.ImageFeaturesFrom(image.Features); err != nil {
		return err
	}

	if _, err := metal.VersionClassificationFrom(image.Classification); err != nil {
		return err
	}

	if _, _, err := metalcommon.GetOsAndSemverFromImage(image.Id); err != nil {
		return err
	}

	if image.ExpiresAt != nil && !image.ExpiresAt.AsTime().IsZero() {
		if image.ExpiresAt.AsTime().Before(time.Now()) {
			return fmt.Errorf("image expiresAt must be in the future")
		}
	}

	// FIXME implement: https://github.com/metal-stack/metal-api/issues/92
	return nil
}

func (r *imageRepository) validateUpdate(ctx context.Context, req *adminv2.ImageServiceUpdateRequest, _ *metal.Image) error {
	// FIXME use validate helper
	if req.Id == "" {
		return fmt.Errorf("image id must not be empty")
	}

	if req.Url != nil {
		if err := checkIfUrlExists(ctx, "image", req.Id, *req.Url); err != nil {
			return err
		}
	}

	if len(req.Features) >= 0 {
		if _, err := metal.ImageFeaturesFrom(req.Features); err != nil {
			return err
		}
	}

	if _, err := metal.VersionClassificationFrom(req.Classification); err != nil {
		return err
	}

	if _, _, err := metalcommon.GetOsAndSemverFromImage(req.Id); err != nil {
		return err
	}

	if req.ExpiresAt != nil && !req.ExpiresAt.AsTime().IsZero() {
		if req.ExpiresAt.AsTime().Before(time.Now()) {
			return fmt.Errorf("image expiresAt must be in the future")
		}
	}

	return nil
}

func (r *imageRepository) validateDelete(ctx context.Context, img *metal.Image) error {
	machines, err := r.s.ds.Machine().List(ctx, queries.MachineFilter(&apiv2.MachineQuery{
		Allocation: &apiv2.MachineAllocationQuery{Image: &img.ID},
	}))
	if err != nil {
		return errorutil.NewInternal(err)
	}

	if len(machines) > 0 {
		return errorutil.FailedPrecondition("cannot remove image with existing machine allocations")
	}

	return nil
}
