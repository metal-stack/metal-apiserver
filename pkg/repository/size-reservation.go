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
	sizeReservationRepository struct {
		s     *Store
		scope *ProjectScope
	}
)

func (r *sizeReservationRepository) get(ctx context.Context, id string) (*metal.SizeReservation, error) {
	sizeReservation, err := r.s.ds.SizeReservation().Get(ctx, id)
	if err != nil {
		return nil, err
	}

	return sizeReservation, nil
}

func (r *sizeReservationRepository) matchScope(sizeReservation *metal.SizeReservation) bool {
	if r.scope == nil {
		return true
	}

	return r.scope.projectID == pointer.SafeDeref(sizeReservation).ProjectID
}

func (r *sizeReservationRepository) create(ctx context.Context, req *adminv2.SizeReservationServiceCreateRequest) (*metal.SizeReservation, error) {
	if req.SizeReservation == nil {
		return nil, nil
	}
	sizeReservation, err := r.convertToInternal(ctx, req.SizeReservation)
	if err != nil {
		return nil, err
	}

	resp, err := r.s.ds.SizeReservation().Create(ctx, sizeReservation)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

func (r *sizeReservationRepository) update(ctx context.Context, e *metal.SizeReservation, req *adminv2.SizeReservationServiceUpdateRequest) (*metal.SizeReservation, error) {
	if req.Description != nil {
		e.Description = *req.Description
	}
	if req.Name != nil {
		e.Name = *req.Name
	}
	if req.Amount != nil {
		e.Amount = int(*req.Amount)
	}
	if len(req.Partitions) != 0 {
		e.PartitionIDs = req.Partitions
	}
	if req.Labels != nil {
		e.Labels = updateLabelsOnMap(req.Labels, e.Labels)
	}

	err := r.s.ds.SizeReservation().Update(ctx, e)
	if err != nil {
		return nil, err
	}

	return e, nil
}

func (r *sizeReservationRepository) delete(ctx context.Context, e *metal.SizeReservation) error {
	err := r.s.ds.SizeReservation().Delete(ctx, e)
	if err != nil {
		return err
	}

	return nil
}

func (r *sizeReservationRepository) find(ctx context.Context, rq *apiv2.SizeReservationQuery) (*metal.SizeReservation, error) {
	sizeReservation, err := r.s.ds.SizeReservation().Find(ctx, r.sizeReservationFilters(queries.SizeReservationFilter(rq))...)
	if err != nil {
		return nil, err
	}

	return sizeReservation, nil
}

func (r *sizeReservationRepository) list(ctx context.Context, rq *apiv2.SizeReservationQuery) ([]*metal.SizeReservation, error) {
	sizeReservations, err := r.s.ds.SizeReservation().List(ctx, r.sizeReservationFilters(queries.SizeReservationFilter(rq))...)
	if err != nil {
		return nil, err
	}

	return sizeReservations, nil
}

func (r *sizeReservationRepository) convertToInternal(ctx context.Context, e *apiv2.SizeReservation) (*metal.SizeReservation, error) {
	if e == nil {
		return nil, nil
	}
	var labels map[string]string
	if e.Meta != nil && e.Meta.Labels != nil {
		labels = e.Meta.Labels.Labels
	}

	sizeReservation := &metal.SizeReservation{
		Base: metal.Base{
			ID:          e.Id,
			Name:        e.Name,
			Description: e.Description,
		},
		Labels:       labels,
		SizeID:       e.Size,
		Amount:       int(e.Amount),
		ProjectID:    e.Project,
		PartitionIDs: e.Partitions,
	}

	return sizeReservation, nil
}

func (r *sizeReservationRepository) convertToProto(ctx context.Context, e *metal.SizeReservation) (*apiv2.SizeReservation, error) {
	if e == nil {
		return nil, errors.New("sizeReservation is nil")
	}

	var (
		labels *apiv2.Labels
	)

	if e.Labels != nil {
		labels = &apiv2.Labels{
			Labels: e.Labels,
		}
	}

	sizeReservation := &apiv2.SizeReservation{
		Id:          e.ID,
		Name:        e.Name,
		Description: e.Description,
		Project:     e.ProjectID,
		Partitions:  e.PartitionIDs,
		Size:        e.SizeID,
		Amount:      int32(e.Amount),
		Meta: &apiv2.Meta{
			Labels:     labels,
			CreatedAt:  timestamppb.New(e.Created),
			UpdatedAt:  timestamppb.New(e.Changed),
			Generation: e.Generation,
		},
	}

	return sizeReservation, nil
}

func (r *sizeReservationRepository) sizeReservationFilters(filter generic.EntityQuery) []generic.EntityQuery {
	var qs []generic.EntityQuery
	if filter != nil {
		qs = append(qs, filter)
	}

	return qs
}
