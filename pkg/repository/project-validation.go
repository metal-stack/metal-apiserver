package repository

import (
	"context"

	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	mdcv1 "github.com/metal-stack/masterdata-api/api/v1"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
)

func (r *projectRepository) validateCreate(ctx context.Context, req *apiv2.ProjectServiceCreateRequest) error {
	return nil
}

func (r *projectRepository) validateUpdate(ctx context.Context, req *apiv2.ProjectServiceUpdateRequest, _ *mdcv1.Project) error {
	return nil
}

func (r *projectRepository) validateDelete(ctx context.Context, req *mdcv1.Project) error {
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

	ms, err := r.s.Machine(req.Meta.Id).List(ctx, &apiv2.MachineQuery{
		Allocation: &apiv2.MachineAllocationQuery{
			Project: &req.Meta.Id,
		},
	})
	if err != nil {
		return errorutil.Convert(err)
	}

	if len(ms) > 0 {
		return errorutil.FailedPrecondition("there are still machines associated with this project, you need to delete them first")
	}

	// TODO: ensure project tokens are revoked / cleaned up

	return nil
}
