package repository

import (
	"context"
	"errors"
	"strings"

	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	"github.com/metal-stack/metal-lib/pkg/tag"
)

func (r *ipRepository) validateCreate(ctx context.Context, req *apiv2.IPServiceCreateRequest) error {
	var errs []error

	errs = validate(errs, req.Project != "", "project should not be empty")
	errs = validate(errs, req.Network != "", "network should not be empty")

	return errors.Join(errs...)
}

func (r *ipRepository) validateUpdate(ctx context.Context, req *apiv2.IPServiceUpdateRequest, _ *metal.IP) error {
	old, err := r.find(ctx, &apiv2.IPQuery{Ip: &req.Ip, Project: &req.Project})
	if err != nil {
		return err
	}

	if req.Type != nil {
		if old.Type == metal.Static && *req.Type != apiv2.IPType_IP_TYPE_STATIC {
			return errorutil.InvalidArgument("cannot change type of ip address from static to ephemeral")
		}
	}

	return nil
}

func (r *ipRepository) validateDelete(ctx context.Context, req *metal.IP) error {
	var errs []error

	errs = validate(errs, req.IPAddress != "", "ipaddress is empty")
	errs = validate(errs, req.AllocationUUID != "", "allocationUUID is empty")
	errs = validate(errs, req.ProjectID != "", "projectId is empty")

	if len(errs) > 0 {
		return errorutil.NewInvalidArgument(errors.Join(errs...))
	}

	ip, err := r.find(ctx, &apiv2.IPQuery{Ip: &req.IPAddress, Uuid: &req.AllocationUUID, Project: &req.ProjectID})
	if err != nil {
		if errorutil.IsNotFound(err) {
			return nil
		}
		return err
	}

	for _, t := range ip.Tags {
		if strings.HasPrefix(t, tag.MachineID) {
			return errorutil.InvalidArgument("ip with machine scope cannot be deleted")
		}
	}

	return nil
}
