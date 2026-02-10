package repository

import (
	"context"

	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/queries"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
)

func (r *projectRepository) validateCreate(ctx context.Context, req *apiv2.ProjectServiceCreateRequest) error {
	return nil
}

func (r *projectRepository) validateUpdate(ctx context.Context, req *apiv2.ProjectServiceUpdateRequest, _ *projectEntity) error {
	return nil
}

func (r *projectRepository) validateDelete(ctx context.Context, req *projectEntity) error {
	ips, err := r.s.IP(req.Meta.Id).List(ctx, &apiv2.IPQuery{Project: &req.Meta.Id})
	if err != nil {
		return errorutil.Convert(err)
	}

	if len(ips) > 0 {
		return errorutil.FailedPrecondition("there are still ips associated with this project, you need to delete them first")
	}

	networks, err := r.s.Network(req.Meta.Id).List(ctx, &apiv2.NetworkQuery{Project: &req.Meta.Id})
	if err != nil {
		return errorutil.Convert(err)
	}

	if len(networks) > 0 {
		return errorutil.FailedPrecondition("there are still networks associated with this project, you need to delete them first")
	}

	ms, err := r.s.ds.Machine().List(ctx, queries.MachineFilter(&apiv2.MachineQuery{
		Allocation: &apiv2.MachineAllocationQuery{
			Project: &req.Meta.Id,
		},
	}))
	if err != nil {
		return errorutil.Convert(err)
	}

	if len(ms) > 0 {
		return errorutil.FailedPrecondition("there are still machines associated with this project, you need to delete them first")
	}

	sizeReservations, err := r.s.ds.SizeReservation().List(ctx, queries.SizeReservationFilter(&apiv2.SizeReservationQuery{
		Project: &req.Meta.Id,
	}))
	if err != nil {
		return err
	}
	if len(sizeReservations) > 0 {
		return errorutil.InvalidArgument("cannot remove project with existing size reservations of this project")
	}

	// TODO: ensure project tokens are revoked / cleaned up

	return nil
}
