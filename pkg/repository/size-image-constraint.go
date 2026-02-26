package repository

import (
	"context"
	"errors"
	"sort"

	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/generic"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-apiserver/pkg/db/queries"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	"github.com/metal-stack/metal-lib/pkg/pointer"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type (
	sizeImageConstraintRepository struct {
		s *Store
	}
)

func (r *sizeImageConstraintRepository) get(ctx context.Context, id string) (*metal.SizeImageConstraint, error) {
	sizeReservation, err := r.s.ds.SizeImageConstraint().Get(ctx, id)
	if err != nil {
		return nil, err
	}

	return sizeReservation, nil
}

func (r *sizeImageConstraintRepository) matchScope(sizeReservation *metal.SizeImageConstraint) bool {
	return true
}

func (r *sizeImageConstraintRepository) create(ctx context.Context, req *adminv2.SizeImageConstraintServiceCreateRequest) (*metal.SizeImageConstraint, error) {
	images := make(map[string]string)

	for _, imageConstraint := range req.ImageConstraints {
		images[imageConstraint.Image] = imageConstraint.SemverMatch
	}

	sic := &metal.SizeImageConstraint{
		Base: metal.Base{
			ID:          req.Size,
			Name:        pointer.SafeDerefOrDefault(req.Name, ""),
			Description: pointer.SafeDerefOrDefault(req.Description, ""),
		},
		Images: images,
	}

	resp, err := r.s.ds.SizeImageConstraint().Create(ctx, sic)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

func (r *sizeImageConstraintRepository) update(ctx context.Context, e *metal.SizeImageConstraint, req *adminv2.SizeImageConstraintServiceUpdateRequest) (*metal.SizeImageConstraint, error) {
	if req.Description != nil {
		e.Description = *req.Description
	}
	if req.Name != nil {
		e.Name = *req.Name
	}

	images := make(map[string]string)
	for _, imageConstraint := range req.ImageConstraints {
		images[imageConstraint.Image] = imageConstraint.SemverMatch
	}
	e.Images = images
	e.ID = req.Size

	err := r.s.ds.SizeImageConstraint().Update(ctx, e)
	if err != nil {
		return nil, err
	}

	return e, nil
}

func (r *sizeImageConstraintRepository) delete(ctx context.Context, e *metal.SizeImageConstraint) error {
	err := r.s.ds.SizeImageConstraint().Delete(ctx, e)
	if err != nil {
		return err
	}

	return nil
}

func (r *sizeImageConstraintRepository) find(ctx context.Context, rq *apiv2.SizeImageConstraintQuery) (*metal.SizeImageConstraint, error) {
	sizeReservation, err := r.s.ds.SizeImageConstraint().Find(ctx, r.sizeImageConstraintFilters(queries.SizeImageConstraintFilter(rq))...)
	if err != nil {
		return nil, err
	}

	return sizeReservation, nil
}

func (r *sizeImageConstraintRepository) list(ctx context.Context, rq *apiv2.SizeImageConstraintQuery) ([]*metal.SizeImageConstraint, error) {
	sizeReservations, err := r.s.ds.SizeImageConstraint().List(ctx, r.sizeImageConstraintFilters(queries.SizeImageConstraintFilter(rq))...)
	if err != nil {
		return nil, err
	}

	sort.SliceStable(sizeReservations, func(i, j int) bool {
		return sizeReservations[i].ID < sizeReservations[j].ID
	})

	return sizeReservations, nil
}

func (r *sizeImageConstraintRepository) convertToInternal(ctx context.Context, e *apiv2.SizeImageConstraint) (*metal.SizeImageConstraint, error) {
	if e == nil {
		return nil, nil
	}
	images := make(map[string]string)
	for _, imageConstraint := range e.ImageConstraints {
		images[imageConstraint.Image] = imageConstraint.SemverMatch
	}

	sizeImageConstraint := &metal.SizeImageConstraint{
		Base: metal.Base{
			ID:          e.Size,
			Name:        pointer.SafeDerefOrDefault(e.Name, ""),
			Description: pointer.SafeDerefOrDefault(e.Description, ""),
		},
		Images: images,
	}

	return sizeImageConstraint, nil
}

func (r *sizeImageConstraintRepository) convertToProto(ctx context.Context, e *metal.SizeImageConstraint) (*apiv2.SizeImageConstraint, error) {
	if e == nil {
		return nil, errors.New("sizeImageConstraint is nil")
	}

	var imageConstraints []*apiv2.ImageConstraint
	for image, semverMatch := range e.Images {
		imageConstraints = append(imageConstraints, &apiv2.ImageConstraint{
			Image:       image,
			SemverMatch: semverMatch,
		})
	}

	sizeImageConstraint := &apiv2.SizeImageConstraint{
		Size:        e.ID,
		Name:        &e.Name,
		Description: &e.Description,
		Meta: &apiv2.Meta{
			CreatedAt:  timestamppb.New(e.Created),
			UpdatedAt:  timestamppb.New(e.Changed),
			Generation: e.Generation,
		},
		ImageConstraints: imageConstraints,
	}

	return sizeImageConstraint, nil
}

func (r *sizeImageConstraintRepository) sizeImageConstraintFilters(filter generic.EntityQuery) []generic.EntityQuery {
	var qs []generic.EntityQuery
	if filter != nil {
		qs = append(qs, filter)
	}

	return qs
}

func (r sizeImageConstraintRepository) Try(ctx context.Context, req *apiv2.SizeImageConstraintServiceTryRequest) error {
	sic, err := r.get(ctx, req.Size)
	if err != nil {
		return err
	}

	size, err := r.s.ds.Size().Get(ctx, req.Size)
	if err != nil {
		return err
	}

	image, err := r.s.ds.Image().Get(ctx, req.Image)
	if err != nil {
		return err
	}

	err = sic.Matches(*size, *image)
	if err != nil {
		return errorutil.NewInvalidArgument(err)
	}
	return nil
}

func (r sizeImageConstraintRepository) Validate(ctx context.Context, sic *metal.SizeImageConstraint) error {
	return sic.Validate()
}
