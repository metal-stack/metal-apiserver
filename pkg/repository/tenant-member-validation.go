package repository

import (
	"context"
	"fmt"

	"github.com/metal-stack/api/go/errorutil"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
)

func (t *tenantMemberRepository) validateCreate(ctx context.Context, req *apiv2.TenantMemberServiceCreateRequest) error {
	if req.Role == apiv2.TenantRole_TENANT_ROLE_UNSPECIFIED {
		return fmt.Errorf("role must not be %s", req.Role)
	}

	return nil
}

func (t *tenantMemberRepository) validateUpdate(ctx context.Context, req *apiv2.TenantMemberServiceUpdateRequest, membership *tenantMemberEntity) error {
	// TODO: currently the API defines that only owners can update members so there is no possibility to elevate permissions
	// probably, we should still check that no elevation of permissions is possible in case we later change the API
	if req.Role != nil {
		if *req.Role == apiv2.TenantRole_TENANT_ROLE_UNSPECIFIED {
			return fmt.Errorf("role must not be %s", req.Role)
		}

		if membership.MemberId == membership.TenantId && *req.Role != apiv2.TenantRole_TENANT_ROLE_OWNER {
			return errorutil.FailedPrecondition("cannot demote a user's role within their own default tenant")
		}

		lastOwner, err := t.checkIfMemberIsLastOwner(ctx, membership)
		if err != nil {
			return errorutil.NewFailedPrecondition(err)
		}

		if lastOwner && *req.Role != apiv2.TenantRole_TENANT_ROLE_OWNER {
			return errorutil.FailedPrecondition("cannot demote last owner's permissions")
		}
	}

	return nil
}

func (t *tenantMemberRepository) validateDelete(ctx context.Context, req *tenantMemberEntity) error {
	if req.MemberId == req.TenantId {
		return errorutil.FailedPrecondition("cannot remove a member from their own default tenant")
	}

	lastOwner, err := t.checkIfMemberIsLastOwner(ctx, req)
	if err != nil {
		return errorutil.NewFailedPrecondition(err)
	}
	if lastOwner {
		return errorutil.FailedPrecondition("cannot remove last owner of a tenant")
	}

	return nil
}

func (t *tenantMemberRepository) checkIfMemberIsLastOwner(ctx context.Context, req *tenantMemberEntity) (bool, error) {
	isOwner := tenantRoleFromMap(req.Meta.Annotations) == apiv2.TenantRole_TENANT_ROLE_OWNER
	if !isOwner {
		return false, nil
	}

	members, err := t.list(ctx, &apiv2.TenantMemberQuery{
		Role: apiv2.TenantRole_TENANT_ROLE_OWNER.Enum(),
	})
	if err != nil {
		return false, err
	}

	return len(members) < 2, nil
}
