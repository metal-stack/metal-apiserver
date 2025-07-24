package repository

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	mdcv1 "github.com/metal-stack/masterdata-api/api/v1"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
)

type (
	tenantMemberRepository struct {
		s     *Store
		scope *TenantScope
	}

	TenantMemberCreateRequest struct {
		MemberID string
		Role     apiv2.TenantRole
	}
	TenantMemberUpdateRequest struct {
		Member *mdcv1.TenantMember
	}
	TenantMemberQuery struct {
		MemberId    *string
		Annotations map[string]string
	}
)

func (t *tenantMemberRepository) validateCreate(ctx context.Context, req *TenantMemberCreateRequest) error {
	return nil
}

func (t *tenantMemberRepository) validateUpdate(ctx context.Context, req *TenantMemberUpdateRequest, _ *mdcv1.TenantMember) error {
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

func (t *tenantMemberRepository) checkIfMemberIsLastOwner(ctx context.Context, req *mdcv1.TenantMember) (bool, error) {
	isOwner := TenantRoleFromMap(req.Meta.Annotations) == apiv2.TenantRole_TENANT_ROLE_OWNER
	if !isOwner {
		return false, nil
	}

	members, err := t.list(ctx, &TenantMemberQuery{
		Annotations: map[string]string{
			TenantRoleAnnotation: apiv2.TenantRole_TENANT_ROLE_OWNER.String(),
		},
	})
	if err != nil {
		return false, err
	}

	return len(members) < 2, nil
}

func (t *tenantMemberRepository) convertToInternal(msg *mdcv1.TenantMember) (*mdcv1.TenantMember, error) {
	return msg, nil
}

func (t *tenantMemberRepository) convertToProto(e *mdcv1.TenantMember) (*mdcv1.TenantMember, error) {
	return e, nil
}

func (t *tenantMemberRepository) create(ctx context.Context, c *TenantMemberCreateRequest) (*mdcv1.TenantMember, error) {
	resp, err := t.s.mdc.TenantMember().Create(ctx, &mdcv1.TenantMemberCreateRequest{
		TenantMember: &mdcv1.TenantMember{
			Meta: &mdcv1.Meta{
				Annotations: map[string]string{
					TenantRoleAnnotation: c.Role.String(),
				},
			},
			MemberId: c.MemberID,
			TenantId: t.scope.tenantID,
		},
	})
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return resp.TenantMember, nil
}

func (t *tenantMemberRepository) delete(ctx context.Context, e *mdcv1.TenantMember) error {
	_, err := t.s.mdc.TenantMember().Delete(ctx, &mdcv1.TenantMemberDeleteRequest{
		Id: e.MemberId,
	})
	if err != nil {
		return errorutil.Convert(err)
	}

	return nil
}

func (t *tenantMemberRepository) find(ctx context.Context, query *TenantMemberQuery) (*mdcv1.TenantMember, error) {
	if query.MemberId == nil {
		return nil, fmt.Errorf("member id must be specified")
	}

	memberships, err := t.list(ctx, query)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	switch len(memberships) {
	case 0:
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("tenant %s is not a member of tenant %s", t.scope.tenantID, *query.MemberId))
	case 1:
		// fallthrough
	default:
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("found multiple membership associations for a member to a tenant"))
	}

	return memberships[0], nil
}

func (t *tenantMemberRepository) get(ctx context.Context, id string) (*mdcv1.TenantMember, error) {
	resp, err := t.s.mdc.TenantMember().Get(ctx, &mdcv1.TenantMemberGetRequest{
		Id: id,
	})
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return resp.TenantMember, nil
}

func (t *tenantMemberRepository) list(ctx context.Context, query *TenantMemberQuery) ([]*mdcv1.TenantMember, error) {
	resp, err := t.s.mdc.TenantMember().Find(ctx, &mdcv1.TenantMemberFindRequest{
		TenantId:    &t.scope.tenantID,
		MemberId:    query.MemberId,
		Annotations: query.Annotations,
	})
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return resp.TenantMembers, nil
}

func (t *tenantMemberRepository) matchScope(e *mdcv1.TenantMember) bool {
	if t.scope == nil {
		return true
	}

	return t.scope.tenantID == e.TenantId
}

func (t *tenantMemberRepository) update(ctx context.Context, _ *mdcv1.TenantMember, msg *TenantMemberUpdateRequest) (*mdcv1.TenantMember, error) {
	resp, err := t.s.mdc.TenantMember().Update(ctx, &mdcv1.TenantMemberUpdateRequest{
		TenantMember: msg.Member,
	})
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return resp.TenantMember, nil
}
