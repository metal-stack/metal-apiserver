package repository

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	mdcv1 "github.com/metal-stack/masterdata-api/api/v1"
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
		return err
	}

	if len(ips) > 0 {
		return connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("there are still ips associated with this project, you need to delete them first"))
	}

	networks, err := r.s.Network(req.Meta.Id).List(ctx, &apiv2.NetworkQuery{Project: &req.Meta.Id})
	if err != nil {
		return err
	}

	if len(networks) > 0 {
		return connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("there are still networks associated with this project, you need to delete them first"))
	}

	ms, err := r.s.Machine(req.Meta.Id).List(ctx, &apiv2.MachineQuery{
		Allocation: &apiv2.MachineAllocationQuery{
			Project: &req.Meta.Id,
		},
	})
	if err != nil {
		return err
	}

	if len(ms) > 0 {
		return connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("there are still machines associated with this project, you need to delete them first"))
	}

	// TODO: ensure project tokens are revoked / cleaned up

	return nil
}
