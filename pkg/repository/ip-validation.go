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

func (r *ipRepository) ValidateCreate(ctx context.Context, req *apiv2.IPServiceCreateRequest) (*Validated[*apiv2.IPServiceCreateRequest], error) {
	var errs []error

	errs = validate(errs, req.Project != "", "project should not be empty")
	errs = validate(errs, req.Network != "", "network should not be empty")

	if len(errs) > 0 {
		return nil, errorutil.NewInvalidArgument(errors.Join(errs...))
	}

	return &Validated[*apiv2.IPServiceCreateRequest]{
		message: req,
	}, nil
}

func (r *ipRepository) ValidateUpdate(ctx context.Context, req *apiv2.IPServiceUpdateRequest) (*Validated[*apiv2.IPServiceUpdateRequest], error) {
	old, err := r.Find(ctx, &apiv2.IPQuery{Ip: &req.Ip, Project: &req.Project})
	if err != nil {
		return nil, err
	}

	if req.Type != nil {
		if old.Type == metal.Static && *req.Type != apiv2.IPType_IP_TYPE_STATIC {
			return nil, errorutil.InvalidArgument("cannot change type of ip address from static to ephemeral")
		}
	}

	return &Validated[*apiv2.IPServiceUpdateRequest]{
		message: req,
	}, nil
}

func (r *ipRepository) ValidateDelete(ctx context.Context, req *metal.IP) (*Validated[*metal.IP], error) {
	var errs []error

	errs = validate(errs, req.IPAddress != "", "ipaddress is empty")
	errs = validate(errs, req.AllocationUUID != "", "allocationUUID is empty")
	errs = validate(errs, req.ProjectID != "", "projectId is empty")

	if len(errs) > 0 {
		return nil, errorutil.NewInvalidArgument(errors.Join(errs...))
	}

	ip, err := r.Find(ctx, &apiv2.IPQuery{Ip: &req.IPAddress, Uuid: &req.AllocationUUID, Project: &req.ProjectID})
	if err != nil {
		if errorutil.IsNotFound(err) {
			return &Validated[*metal.IP]{
				message: req,
			}, nil
		}
		return nil, err
	}

	for _, t := range ip.Tags {
		if strings.HasPrefix(t, tag.MachineID) {
			return nil, errorutil.InvalidArgument("ip with machine scope cannot be deleted")
		}
	}

	return &Validated[*metal.IP]{
		message: req,
	}, nil
}
