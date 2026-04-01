package repository

import (
	"context"

	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
)

func (r *sizeImageConstraintRepository) validateCreate(ctx context.Context, req *adminv2.SizeImageConstraintServiceCreateRequest) error {
	sic, err := r.convertToInternal(ctx, &apiv2.SizeImageConstraint{
		Size:             req.Size,
		ImageConstraints: req.ImageConstraints,
	})
	if err != nil {
		return err
	}

	// TODO: move validation code here?
	err = sic.Validate()
	if err != nil {
		return err
	}

	return nil
}

func (r *sizeImageConstraintRepository) validateUpdate(ctx context.Context, req *adminv2.SizeImageConstraintServiceUpdateRequest, _ *metal.SizeImageConstraint) error {
	sic, err := r.convertToInternal(ctx, &apiv2.SizeImageConstraint{
		Size:             req.Size,
		ImageConstraints: req.ImageConstraints,
	})
	if err != nil {
		return err
	}

	err = sic.Validate()
	if err != nil {
		return err
	}

	return nil
}

func (r *sizeImageConstraintRepository) validateDelete(ctx context.Context, req *metal.SizeImageConstraint) error {
	return nil
}
