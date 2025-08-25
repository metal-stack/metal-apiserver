package repository

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	mdcv1 "github.com/metal-stack/masterdata-api/api/v1"
)

func (t *tenantMemberRepository) validateCreate(ctx context.Context, req *TenantMemberCreateRequest) error {
	return nil
}

func (t *tenantMemberRepository) validateUpdate(ctx context.Context, req *TenantMemberUpdateRequest, membership *mdcv1.TenantMember) error {
	// TODO: currently the API defines that only owners can update members so there is no possibility to elevate permissions
	// probably, we should still check that no elevation of permissions is possible in case we later change the API

	if membership.MemberId == membership.TenantId && req.Role != apiv2.TenantRole_TENANT_ROLE_OWNER {
		return connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("cannot demote a user's role within their own default tenant"))
	}

	lastOwner, err := t.checkIfMemberIsLastOwner(ctx, membership)
	if err != nil {
		return connect.NewError(connect.CodeInternal, err)
	}

	if lastOwner && req.Role != apiv2.TenantRole_TENANT_ROLE_OWNER {
		return connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("cannot demote last owner's permissions"))
	}

	return nil
}

func (t *tenantMemberRepository) validateDelete(ctx context.Context, req *mdcv1.TenantMember) error {
	if req.MemberId == req.TenantId {
		return connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("cannot remove a member from their own default tenant"))
	}

	lastOwner, err := t.checkIfMemberIsLastOwner(ctx, req)
	if err != nil {
		return err
	}
	if lastOwner {
		return connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("cannot remove last owner of a tenant"))
	}

	return nil
}
