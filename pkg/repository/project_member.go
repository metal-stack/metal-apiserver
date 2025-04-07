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

type projectMemberRepository struct {
	r     *Store
	scope *ProjectScope
}

type (
	ProjectMemberCreateRequest struct {
		MemberID string
		Role     apiv2.ProjectRole
	}
	ProjectMemberUpdateRequest struct {
		Member *mdcv1.ProjectMember
	}
	ProjectMemberQuery struct {
		MemberId    *string
		Annotations map[string]string
	}
)

func (t *projectMemberRepository) ConvertToInternal(msg *apiv2.ProjectMember) (*mdcv1.ProjectMember, error) {
	return &mdcv1.ProjectMember{
		Meta: &mdcv1.Meta{
			Id: msg.Id,
			Annotations: map[string]string{
				ProjectRoleAnnotation: msg.Role.String(),
			},
		},
	}, nil
}

func (t *projectMemberRepository) ConvertToProto(e *mdcv1.ProjectMember) (*apiv2.ProjectMember, error) {
	return &apiv2.ProjectMember{
		Id:        e.TenantId,
		Role:      ProjectRoleFromMap(e.Meta.Annotations),
		CreatedAt: e.Meta.CreatedTime,
	}, nil
}

func (t *projectMemberRepository) Create(ctx context.Context, c *Validated[*ProjectMemberCreateRequest]) (*mdcv1.ProjectMember, error) {
	resp, err := t.r.mdc.ProjectMember().Create(ctx, &mdcv1.ProjectMemberCreateRequest{
		ProjectMember: &mdcv1.ProjectMember{
			Meta: &mdcv1.Meta{
				Annotations: map[string]string{
					tutil.TenantRoleAnnotation: c.message.Role.String(),
				},
			},
			TenantId:  c.message.MemberID,
			ProjectId: t.scope.projectID,
		},
	})
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return resp.ProjectMember, nil
}

func (t *projectMemberRepository) Delete(ctx context.Context, e *Validated[*mdcv1.ProjectMember]) (*mdcv1.ProjectMember, error) {
	resp, err := t.r.mdc.ProjectMember().Delete(ctx, &mdcv1.ProjectMemberDeleteRequest{
		Id: e.message.Meta.Id,
	})
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return resp.ProjectMember, nil
}

func (t *projectMemberRepository) Find(ctx context.Context, query *ProjectMemberQuery) (*mdcv1.ProjectMember, error) {
	if query.MemberId == nil {
		return nil, fmt.Errorf("member id must be specified")
	}

	memberships, err := t.List(ctx, query)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	switch len(memberships) {
	case 0:
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("tenant %s is not a member of project %s", *query.MemberId, t.scope.projectID))
	case 1:
		// fallthrough
	default:
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("found multiple membership associations for a member to a tenant"))
	}

	return memberships[0], nil
}

func (t *projectMemberRepository) Get(ctx context.Context, id string) (*mdcv1.ProjectMember, error) {
	resp, err := t.r.mdc.ProjectMember().Get(ctx, &mdcv1.ProjectMemberGetRequest{
		Id: id,
	})
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return resp.ProjectMember, nil
}

func (t *projectMemberRepository) List(ctx context.Context, query *ProjectMemberQuery) ([]*mdcv1.ProjectMember, error) {
	resp, err := t.r.mdc.ProjectMember().Find(ctx, &mdcv1.ProjectMemberFindRequest{
		ProjectId:   &t.scope.projectID,
		TenantId:    query.MemberId,
		Annotations: query.Annotations,
	})
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return resp.ProjectMembers, nil
}

func (t *projectMemberRepository) MatchScope(e *mdcv1.ProjectMember) error {
	panic("unimplemented")
}

func (t *projectMemberRepository) Update(ctx context.Context, msg *Validated[*ProjectMemberUpdateRequest]) (*mdcv1.ProjectMember, error) {
	resp, err := t.r.mdc.ProjectMember().Update(ctx, &mdcv1.ProjectMemberUpdateRequest{
		ProjectMember: msg.message.Member,
	})
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return resp.ProjectMember, nil
}

func (t *projectMemberRepository) ValidateCreate(ctx context.Context, create *ProjectMemberCreateRequest) (*Validated[*ProjectMemberCreateRequest], error) {
	return &Validated[*ProjectMemberCreateRequest]{
		message: create,
	}, nil
}

func (t *projectMemberRepository) ValidateDelete(ctx context.Context, e *mdcv1.ProjectMember) (*Validated[*mdcv1.ProjectMember], error) {
	return &Validated[*mdcv1.ProjectMember]{
		message: e,
	}, nil
}

func (t *projectMemberRepository) ValidateUpdate(ctx context.Context, msg *ProjectMemberUpdateRequest) (*Validated[*ProjectMemberUpdateRequest], error) {
	return &Validated[*ProjectMemberUpdateRequest]{
		message: msg,
	}, nil
}
