package repository

import (
	"context"
	"errors"

	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/generic"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-apiserver/pkg/db/queries"
	"github.com/metal-stack/metal-lib/pkg/pointer"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type (
	sizeRepository struct {
		s     *Store
		scope *ProjectScope
	}
)

func (r *sizeRepository) get(ctx context.Context, id string) (*metal.Size, error) {
	size, err := r.s.ds.Size().Get(ctx, id)
	if err != nil {
		return nil, err
	}

	return size, nil
}

func (r *sizeRepository) matchScope(size *metal.Size) bool {
	// Is not project scoped
	return true
}

func (r *sizeRepository) create(ctx context.Context, req *adminv2.SizeServiceCreateRequest) (*metal.Size, error) {
	if req.Size == nil {
		return nil, nil
	}
	size, err := r.convertToInternal(ctx, req.Size)
	if err != nil {
		return nil, err
	}

	resp, err := r.s.ds.Size().Create(ctx, size)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

func (r *sizeRepository) update(ctx context.Context, e *metal.Size, req *adminv2.SizeServiceUpdateRequest) (*metal.Size, error) {
	if req.Description != nil {
		e.Description = *req.Description
	}
	if req.Name != nil {
		e.Name = *req.Name
	}

	var constraints []metal.Constraint
	if req.Constraints != nil {
		for _, c := range req.Constraints {
			metalConstraint, err := metal.ToConstraint(c)
			if err != nil {
				return nil, err
			}
			constraints = append(constraints, *metalConstraint)
		}
		e.Constraints = constraints
	}

	if req.Labels != nil {
		e.Labels = updateLabelsOnMap(req.Labels, e.Labels)
	}

	err := r.s.ds.Size().Update(ctx, e)
	if err != nil {
		return nil, err
	}

	return e, nil
}

func (r *sizeRepository) delete(ctx context.Context, e *metal.Size) error {
	err := r.s.ds.Size().Delete(ctx, e)
	if err != nil {
		return err
	}

	return nil
}

func (r *sizeRepository) find(ctx context.Context, rq *apiv2.SizeQuery) (*metal.Size, error) {
	size, err := r.s.ds.Size().Find(ctx, r.sizeFilters(queries.SizeFilter(rq))...)
	if err != nil {
		return nil, err
	}

	return size, nil
}

func (r *sizeRepository) list(ctx context.Context, rq *apiv2.SizeQuery) ([]*metal.Size, error) {
	sizes, err := r.s.ds.Size().List(ctx, r.sizeFilters(queries.SizeFilter(rq))...)
	if err != nil {
		return nil, err
	}

	return sizes, nil
}

func (r *sizeRepository) convertToInternal(ctx context.Context, e *apiv2.Size) (*metal.Size, error) {
	if e == nil {
		return nil, nil
	}
	var constraints []metal.Constraint
	for _, c := range e.Constraints {
		metalConstraint, err := metal.ToConstraint(c)
		if err != nil {
			return nil, err
		}
		constraints = append(constraints, *metalConstraint)
	}

	var labels map[string]string
	if e.Meta != nil && e.Meta.Labels != nil {
		labels = e.Meta.Labels.Labels
	}

	size := &metal.Size{
		Base: metal.Base{
			ID:          e.Id,
			Name:        pointer.SafeDeref(e.Name),
			Description: pointer.SafeDeref(e.Description),
		},
		Labels:      labels,
		Constraints: constraints,
	}

	return size, nil
}

func (r *sizeRepository) convertToProto(ctx context.Context, e *metal.Size) (*apiv2.Size, error) {
	if e == nil {
		return nil, nil
	}

	var (
		constraints []*apiv2.SizeConstraint
		labels      *apiv2.Labels
	)

	if e.Labels != nil {
		labels = &apiv2.Labels{
			Labels: e.Labels,
		}
	}

	for _, c := range e.Constraints {
		apiv2Constraint, err := metal.FromConstraint(c)
		if err != nil {
			return nil, err
		}
		constraints = append(constraints, apiv2Constraint)
	}

	size := &apiv2.Size{
		Id:          e.ID,
		Name:        pointer.PointerOrNil(e.Name),
		Description: pointer.PointerOrNil(e.Description),
		Constraints: constraints,
		Meta: &apiv2.Meta{
			Labels:     labels,
			CreatedAt:  timestamppb.New(e.Created),
			UpdatedAt:  timestamppb.New(e.Changed),
			Generation: e.Generation,
		},
	}

	return size, nil
}

func (r *sizeRepository) sizeFilters(filter generic.EntityQuery) []generic.EntityQuery {
	var qs []generic.EntityQuery
	if filter != nil {
		qs = append(qs, filter)
	}

	return qs
}

// FromHardware tries to find a size which matches the given hardware specs.
func (r *sizeRepository) FromHardware(ctx context.Context, hw metal.MachineHardware) (*metal.Size, error) {
	sz, err := r.s.ds.Size().List(ctx)
	if err != nil {
		return nil, err
	}
	if len(sz) < 1 {
		// this should not happen, so we do not return a notfound
		return nil, errors.New("no sizes found in database")
	}
	var sizes metal.Sizes
	for _, s := range sz {
		if len(s.Constraints) < 1 {
			r.s.log.Error("missing constraints", "size", s)
			continue
		}
		sizes = append(sizes, *s)
	}
	return sizes.FromHardware(hw)
}
