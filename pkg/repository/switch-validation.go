package repository

import (
	"context"
	"errors"
	"fmt"

	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	"github.com/samber/lo"
)

func (r *switchRepository) validateCreate(ctx context.Context, req *SwitchServiceCreateRequest) error {
	var errs []error

	_, err := r.s.ds.Partition().Get(ctx, req.Switch.Partition)
	if err != nil {
		errs = append(errs, err)
	}

	sw, err := r.convertToInternal(req.Switch)
	if err != nil {
		errs = append(errs, err)
	}

	duplicateIdentifiers := lo.FindDuplicates(lo.Map(sw.Nics, func(n metal.Nic, i int) string {
		return n.Identifier
	}))
	duplicateNames := lo.FindDuplicates(lo.Map(sw.Nics, func(n metal.Nic, i int) string {
		return n.Name
	}))

	if len(duplicateIdentifiers) > 0 {
		errs = append(errs, fmt.Errorf("switch nics contain duplicate identifiers:%v", duplicateIdentifiers))
	}
	if len(duplicateNames) > 0 {
		errs = append(errs, fmt.Errorf("switch nics contain duplicate name:%v", duplicateNames))
	}

	// TODO: is this enough validation?

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
