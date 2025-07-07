package repository

import (
	"context"
	"errors"
	"fmt"

	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
)

func (r *sizeRepository) validateCreate(ctx context.Context, req *adminv2.SizeServiceCreateRequest) error {
	var (
		errs []error
	)

	err := r.validateSizeConstraints(req.Size)
	if err != nil {
		errs = append(errs, err)
	}
	err = r.valideteSizesNotOverlapping(ctx, req.Size)
	if err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

func (r *sizeRepository) validateUpdate(ctx context.Context, req *adminv2.SizeServiceUpdateRequest, _ *metal.Size) error {
	var (
		errs []error
	)

	err := r.validateSizeConstraints(req.Size)
	if err != nil {
		errs = append(errs, err)
	}
	err = r.valideteSizesNotOverlapping(ctx, req.Size)
	if err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

func (r *sizeRepository) validateDelete(ctx context.Context, req *metal.Size) error {
	var errs []error

	// FIXME find all machines with this size

	if len(errs) > 0 {
		return errorutil.NewInvalidArgument(errors.Join(errs...))
	}

	return nil
}

func (r *sizeRepository) validateSizeConstraints(size *apiv2.Size) error {
	var (
		errs       []error
		typeCounts = map[apiv2.SizeConstraintType]uint{}
	)

	for i, c := range size.Constraints {
		typeCounts[c.Type]++

		metalConstraint, err := metal.ToConstraint(c)
		if err != nil {
			errs = append(errs, err)
			continue
		}

		err = metalConstraint.Validate()
		if err != nil {
			errs = append(errs, fmt.Errorf("constraint at index %d is invalid: %w", i, err))
		}

		switch t := c.Type; t {
		case apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_GPU, apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_STORAGE:
		case apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_MEMORY, apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_CORES:
			if typeCounts[t] > 1 {
				errs = append(errs, fmt.Errorf("constraint at index %d is invalid: type duplicates are not allowed for type %q", i, t))
			}
		case apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_UNSPECIFIED:
			fallthrough
		default:
			errs = append(errs, fmt.Errorf("invalid constraint type:%q", t))
		}
	}

	return errors.Join(errs...)
}

func (r *sizeRepository) valideteSizesNotOverlapping(ctx context.Context, size *apiv2.Size) error {
	sizes, err := r.s.ds.Size().List(ctx)
	if err != nil {
		return err
	}

	metalSize, err := r.s.ds.Size().Get(ctx, size.Id)
	if err != nil {
		return err
	}

	var ss metal.Sizes
	for _, s := range sizes {
		ss = append(ss, *s)
	}
	overlapping := metalSize.Overlaps(&ss)
	if overlapping != nil {
		return errorutil.Conflict("given size %s overlaps with existing sizes", overlapping.ID)
	}
	return nil
}
