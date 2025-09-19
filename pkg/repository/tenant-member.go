package repository

import (
	"context"

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
		Role apiv2.TenantRole
	}
	TenantMemberQuery struct {
		MemberId    *string
		Annotations map[string]string
	}
)

func (*TenantMemberUpdateRequest) GetUpdateMeta() *apiv2.UpdateMeta {
	return &apiv2.UpdateMeta{}
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

func (t *tenantMemberRepository) convertToInternal(ctx context.Context, msg *mdcv1.TenantMember) (*mdcv1.TenantMember, error) {
	// this is an internal interface, so no implementation here
	panic("unimplemented")
}

func (t *tenantMemberRepository) convertToProto(ctx context.Context, e *mdcv1.TenantMember) (*mdcv1.TenantMember, error) {
	// this is an internal interface, so no implementation here
	panic("unimplemented")
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
		Id: e.Meta.Id,
	})
	if err != nil {
		return errorutil.Convert(err)
	}

	return nil
}

func (t *tenantMemberRepository) find(ctx context.Context, query *TenantMemberQuery) (*mdcv1.TenantMember, error) {
	if query.MemberId == nil {
		return nil, errorutil.InvalidArgument("member id must be specified")
	}

	memberships, err := t.list(ctx, query)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	switch len(memberships) {
	case 0:
		return nil, errorutil.NotFound("tenant %s is not a member of tenant %s", t.scope.tenantID, *query.MemberId)
	case 1:
		// noop
	default:
		return nil, errorutil.Internal("found multiple membership associations for a member to a tenant")
	}

	return memberships[0], nil
}

func (t *tenantMemberRepository) get(ctx context.Context, id string) (*mdcv1.TenantMember, error) {
	member, err := t.find(ctx, &TenantMemberQuery{
		MemberId: &id,
	})
	if err != nil {
		return nil, err
	}

	return member, nil
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

func (t *tenantMemberRepository) update(ctx context.Context, member *mdcv1.TenantMember, msg *TenantMemberUpdateRequest) (*mdcv1.TenantMember, error) {
	if msg.Role != apiv2.TenantRole_TENANT_ROLE_UNSPECIFIED {
		member.Meta.Annotations[TenantRoleAnnotation] = msg.Role.String()
	}

	resp, err := t.s.mdc.TenantMember().Update(ctx, &mdcv1.TenantMemberUpdateRequest{
		TenantMember: member,
	})
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return resp.TenantMember, nil
}
