package repository

import (
	"context"
	"errors"

	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	infrav2 "github.com/metal-stack/api/go/metalstack/infra/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
)

func (r *switchRepository) validateCreate(ctx context.Context, req *infrav2.SwitchServiceCreateRequest) error {
	var errs []error

	_, err := r.s.ds.Partition().Get(ctx, req.Switch.Partition)
	if err != nil {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return errorutil.NewInvalidArgument(errors.Join(errs...))
	}

	return nil
}

func (r *switchRepository) validateUpdate(ctx context.Context, req *adminv2.SwitchServiceUpdateRequest, oldSwitch *metal.Switch) error {
	panic("unimplemented")
}

func (r *switchRepository) validateDelete(ctx context.Context, sw *metal.Switch) error {
	panic("unimplemented")
}
