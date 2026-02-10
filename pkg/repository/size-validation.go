package repository

import (
	"context"
	"errors"
	"fmt"

	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-apiserver/pkg/db/queries"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
)

func (r *sizeRepository) validateCreate(ctx context.Context, req *adminv2.SizeServiceCreateRequest) error {
	var (
		errs []error
	)

	err := r.validateSizeConstraints(req.Size.Constraints)
	if err != nil {
		errs = append(errs, err)
	}
	err = r.validateSizesNotOverlapping(ctx, req.Size)
	if err != nil {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return errorutil.NewInvalidArgument(errors.Join(errs...))
	}

	return nil
}

func (r *sizeRepository) validateUpdate(ctx context.Context, req *adminv2.SizeServiceUpdateRequest, _ *metal.Size) error {
	var (
		errs []error
	)

	err := r.validateSizeConstraints(req.Constraints)
	if err != nil {
		errs = append(errs, err)
	}

	size := &apiv2.Size{
		Id:          req.Id,
		Name:        req.Name,
		Description: req.Description,
		Constraints: req.Constraints,
	}

	if req.Constraints != nil {
		err = r.validateSizesNotOverlapping(ctx, size)
		if err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return errorutil.NewInvalidArgument(errors.Join(errs...))
	}

	return nil
}

func (r *sizeRepository) validateDelete(ctx context.Context, req *metal.Size) error {
	machines, err := r.s.ds.Machine().List(ctx, queries.MachineFilter(&apiv2.MachineQuery{
		Size: &req.ID,
	}))
	if err != nil {
		return err
	}
	if len(machines) > 0 {
		return errorutil.InvalidArgument("cannot remove size with existing machines of this size")
	}

	sizeReservations, err := r.s.ds.SizeReservation().List(ctx, queries.SizeReservationFilter(&apiv2.SizeReservationQuery{
		Size: &req.ID,
	}))
	if err != nil {
		return err
	}
	if len(sizeReservations) > 0 {
		return errorutil.InvalidArgument("cannot remove size with existing size reservations of this size")
	}

	return nil
}

func (r *sizeRepository) validateSizeConstraints(constraints []*apiv2.SizeConstraint) error {
	var (
		errs       []error
		typeCounts = map[apiv2.SizeConstraintType]uint{}
	)

	for i, c := range constraints {
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

		switch c.Type {
		case apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_GPU, apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_STORAGE:
		case apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_MEMORY, apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_CORES:
			if typeCounts[c.Type] > 1 {
				errs = append(errs, fmt.Errorf("constraint at index %d is invalid: type duplicates are not allowed for type %q", i, c.Type))
			}
		case apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_UNSPECIFIED:
			fallthrough
		default:
			errs = append(errs, fmt.Errorf("invalid constraint type:%q", c.Type))
		}
	}

	return errors.Join(errs...)
}

func (r *sizeRepository) validateSizesNotOverlapping(ctx context.Context, size *apiv2.Size) error {
	sizes, err := r.s.ds.Size().List(ctx)
	if err != nil {
		return err
	}
	metalSize, err := r.convertToInternal(ctx, size)
	if err != nil {
		return err
	}

	var ss metal.Sizes
	for _, s := range sizes {
		ss = append(ss, *s)
	}
	overlapping := metalSize.Overlaps(ss)
	if overlapping != nil {
		return errorutil.Conflict("given size %s overlaps with existing sizes", overlapping.ID)
	}
	return nil
}
