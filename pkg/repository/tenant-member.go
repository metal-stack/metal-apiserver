package repository

import (
	"context"
	"time"

	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	mdcv1 "github.com/metal-stack/masterdata-api/api/v1"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
)

type (
	tenantMemberEntity struct {
		*mdcv1.TenantMember
	}

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

func (t *tenantMemberEntity) SetChanged(time time.Time) {}

func (*TenantMemberUpdateRequest) GetUpdateMeta() *apiv2.UpdateMeta {
	return &apiv2.UpdateMeta{}
}

func (t *tenantMemberRepository) checkIfMemberIsLastOwner(ctx context.Context, req *tenantMemberEntity) (bool, error) {
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

func (t *tenantMemberRepository) convertToInternal(ctx context.Context, msg *apiv2.TenantMember) (*tenantMemberEntity, error) {
	// this is an internal interface, so no implementation here
	panic("unimplemented")
}

func (t *tenantMemberRepository) convertToProto(ctx context.Context, e *tenantMemberEntity) (*apiv2.TenantMember, error) {
	return &apiv2.TenantMember{
		Id:        e.TenantId,
		Role:      TenantRoleFromMap(e.Meta.Annotations),
		CreatedAt: e.Meta.CreatedTime,
	}, nil
}

func (t *tenantMemberRepository) create(ctx context.Context, c *TenantMemberCreateRequest) (*tenantMemberEntity, error) {
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

	return &tenantMemberEntity{TenantMember: resp.TenantMember}, nil
}

func (t *tenantMemberRepository) delete(ctx context.Context, e *tenantMemberEntity) error {
	_, err := t.s.mdc.TenantMember().Delete(ctx, &mdcv1.TenantMemberDeleteRequest{
		Id: e.Meta.Id,
	})
	if err != nil {
		return errorutil.Convert(err)
	}

	return nil
}

func (t *tenantMemberRepository) find(ctx context.Context, query *TenantMemberQuery) (*tenantMemberEntity, error) {
	if query.MemberId == nil {
		return nil, errorutil.InvalidArgument("member id must be specified")
	}

	memberships, err := t.list(ctx, query)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	switch len(memberships) {
	case 0:
		return nil, errorutil.NotFound("tenant %s is not a member of tenant %s", *query.MemberId, t.scope.tenantID)
	case 1:
		// noop
	default:
		return nil, errorutil.Internal("found multiple membership associations for a member to a tenant")
	}

	return memberships[0], nil
}

func (t *tenantMemberRepository) get(ctx context.Context, id string) (*tenantMemberEntity, error) {
	member, err := t.find(ctx, &TenantMemberQuery{
		MemberId: &id,
	})
	if err != nil {
		return nil, err
	}

	return member, nil
}

func (t *tenantMemberRepository) list(ctx context.Context, query *TenantMemberQuery) ([]*tenantMemberEntity, error) {
	resp, err := t.s.mdc.TenantMember().Find(ctx, &mdcv1.TenantMemberFindRequest{
		TenantId:    &t.scope.tenantID,
		MemberId:    query.MemberId,
		Annotations: query.Annotations,
	})
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	tms := make([]*tenantMemberEntity, 0, len(resp.TenantMembers))
	for _, tm := range resp.TenantMembers {
		tms = append(tms, &tenantMemberEntity{TenantMember: tm})
	}

	return tms, nil
}

func (t *tenantMemberRepository) matchScope(e *tenantMemberEntity) bool {
	if t.scope == nil {
		return true
	}

	return t.scope.tenantID == e.TenantId
}

func (t *tenantMemberRepository) update(ctx context.Context, member *tenantMemberEntity, msg *TenantMemberUpdateRequest) (*tenantMemberEntity, error) {
	if msg.Role != apiv2.TenantRole_TENANT_ROLE_UNSPECIFIED {
		member.Meta.Annotations[TenantRoleAnnotation] = msg.Role.String()
	}

	resp, err := t.s.mdc.TenantMember().Update(ctx, &mdcv1.TenantMemberUpdateRequest{
		TenantMember: member.TenantMember,
	})
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return &tenantMemberEntity{TenantMember: resp.TenantMember}, nil
}
