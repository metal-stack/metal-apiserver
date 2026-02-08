package repository

import (
	"context"

	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	v1 "github.com/metal-stack/masterdata-api/api/v1"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
)

func (r *sizeReservationRepository) validateCreate(ctx context.Context, req *adminv2.SizeReservationServiceCreateRequest) error {
	if req.SizeReservation == nil {
		return errorutil.InvalidArgument("sizereservation is nil")
	}

	sr := req.SizeReservation

	if sr.Id != "" {
		return errorutil.InvalidArgument("id must not be defined")
	}

	if sr.Amount <= 0 {
		return errorutil.InvalidArgument("amount must be a positive integer")
	}

	if _, err := r.s.ds.Size().Get(ctx, sr.Size); err != nil {
		return errorutil.InvalidArgument("size must exist before creating a size reservation")
	}

	if len(sr.Partitions) == 0 {
		return errorutil.InvalidArgument("at least one partition id must be specified")
	}

	for _, partition := range sr.Partitions {
		if _, err := r.s.ds.Partition().Get(ctx, partition); err != nil {
			return errorutil.InvalidArgument("partition must exist before creating a size reservation")
		}
	}

	if _, err := r.s.mdc.Project().Get(ctx, &v1.ProjectGetRequest{Id: sr.Project}); err != nil {
		return errorutil.InvalidArgument("project must exist before creating a size reservation")
	}

	return nil
}

func (r *sizeReservationRepository) validateUpdate(ctx context.Context, req *adminv2.SizeReservationServiceUpdateRequest, _ *metal.SizeReservation) error {
	for _, partition := range req.Partitions {
		if _, err := r.s.ds.Partition().Get(ctx, partition); err != nil {
			return errorutil.InvalidArgument("partition must exist before creating a size reservation")
		}
	}
	return nil
}
func (r *sizeReservationRepository) validateDelete(ctx context.Context, req *metal.SizeReservation) error {
	// Reservations can be deleted without validation
	return nil
}
