package repository

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	mdcv1 "github.com/metal-stack/masterdata-api/api/v1"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	tutil "github.com/metal-stack/metal-apiserver/pkg/tenant"
)

type tenantMemberRepository struct {
	r     *Store
	scope *TenantScope
}

type (
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

func (t *tenantMemberRepository) ConvertToInternal(msg *mdcv1.TenantMember) (*mdcv1.TenantMember, error) {
	return msg, nil
}

func (t *tenantMemberRepository) ConvertToProto(e *mdcv1.TenantMember) (*mdcv1.TenantMember, error) {
	return e, nil
}

func (t *tenantMemberRepository) Create(ctx context.Context, c *Validated[*TenantMemberCreateRequest]) (*mdcv1.TenantMember, error) {
	resp, err := t.r.mdc.TenantMember().Create(ctx, &mdcv1.TenantMemberCreateRequest{
		TenantMember: &mdcv1.TenantMember{
			Meta: &mdcv1.Meta{
				Annotations: map[string]string{
					tutil.TenantRoleAnnotation: c.message.Role.String(),
				},
			},
			MemberId: c.message.MemberID,
			TenantId: t.scope.tenantID,
		},
	})
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return resp.TenantMember, nil
}

func (t *tenantMemberRepository) Delete(ctx context.Context, e *Validated[*mdcv1.TenantMember]) (*mdcv1.TenantMember, error) {
	resp, err := t.r.mdc.TenantMember().Delete(ctx, &mdcv1.TenantMemberDeleteRequest{
		Id: e.message.MemberId,
	})
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return resp.TenantMember, nil
}

func (t *tenantMemberRepository) Find(ctx context.Context, query *TenantMemberQuery) (*mdcv1.TenantMember, error) {
	if query.MemberId == nil {
		return nil, fmt.Errorf("member id must be specified")
	}

	memberships, err := t.List(ctx, query)
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

func (t *tenantMemberRepository) Get(ctx context.Context, id string) (*mdcv1.TenantMember, error) {
	resp, err := t.r.mdc.TenantMember().Get(ctx, &mdcv1.TenantMemberGetRequest{
		Id: id,
	})
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return resp.TenantMember, nil
}

func (t *tenantMemberRepository) List(ctx context.Context, query *TenantMemberQuery) ([]*mdcv1.TenantMember, error) {
	resp, err := t.r.mdc.TenantMember().Find(ctx, &mdcv1.TenantMemberFindRequest{
		TenantId:    &t.scope.tenantID,
		MemberId:    query.MemberId,
		Annotations: query.Annotations,
	})
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return resp.TenantMembers, nil
}

func (t *tenantMemberRepository) MatchScope(e *mdcv1.TenantMember) error {
	panic("unimplemented")
}

func (t *tenantMemberRepository) Update(ctx context.Context, msg *Validated[*TenantMemberUpdateRequest]) (*mdcv1.TenantMember, error) {
	resp, err := t.r.mdc.TenantMember().Update(ctx, &mdcv1.TenantMemberUpdateRequest{
		TenantMember: msg.message.Member,
	})
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return resp.TenantMember, nil
}

func (t *tenantMemberRepository) ValidateCreate(ctx context.Context, create *TenantMemberCreateRequest) (*Validated[*TenantMemberCreateRequest], error) {
	return &Validated[*TenantMemberCreateRequest]{
		message: create,
	}, nil
}

func (t *tenantMemberRepository) ValidateDelete(ctx context.Context, e *mdcv1.TenantMember) (*Validated[*mdcv1.TenantMember], error) {
	return &Validated[*mdcv1.TenantMember]{
		message: e,
	}, nil
}

func (t *tenantMemberRepository) ValidateUpdate(ctx context.Context, msg *TenantMemberUpdateRequest) (*Validated[*TenantMemberUpdateRequest], error) {
	return &Validated[*TenantMemberUpdateRequest]{
		message: msg,
	}, nil
}
