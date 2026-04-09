package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"

	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/api/go/tag"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
)

func (r *ipRepository) validateCreate(ctx context.Context, req *apiv2.IPServiceCreateRequest) error {
	var errs []error

	errs = validate(errs, req.Project != "", "project should not be empty")
	errs = validate(errs, req.Network != "", "network should not be empty")

	return errors.Join(errs...)
}

func (r *ipRepository) validateUpdate(ctx context.Context, req *apiv2.IPServiceUpdateRequest, ip *metal.IP) error {
	if req.Type != nil {
		if ip.Type == metal.Static && *req.Type != apiv2.IPType_IP_TYPE_STATIC {
			return fmt.Errorf("cannot change type of ip address from static to ephemeral")
		}
	}

	return nil
}

func (r *ipRepository) validateDelete(ctx context.Context, ip *metal.IP) error {
	var errs []error

	errs = validate(errs, ip.IPAddress != "", "ipaddress is empty")
	errs = validate(errs, ip.AllocationUUID != "", "allocationUUID is empty")
	errs = validate(errs, ip.ProjectID != "", "projectId is empty")

	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	for _, t := range ip.Tags {
		if strings.HasPrefix(t, tag.MachineID) {
			return fmt.Errorf("ip with machine scope cannot be deleted")
		}
	}

	return nil
}
