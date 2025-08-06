package repository

import (
	"context"

	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
)

func (r *machineRepository) validateCreate(ctx context.Context, req *apiv2.MachineServiceCreateRequest) error {
	panic("unimplemented")
}

func (r *machineRepository) validateUpdate(ctx context.Context, req *apiv2.MachineServiceUpdateRequest, _ *metal.Machine) error {
	// FIXME implement with admin machine update
	return nil
}

func (r *machineRepository) validateDelete(ctx context.Context, req *metal.Machine) error {
	panic("unimplemented")

}
