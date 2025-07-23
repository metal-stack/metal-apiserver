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
	s     *Store
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

func (r *projectMemberRepository) validateCreate(ctx context.Context, req *ProjectMemberCreateRequest) error {
	return nil
}

func (r *projectMemberRepository) validateUpdate(ctx context.Context, req *ProjectMemberUpdateRequest, _ *mdcv1.ProjectMember) error {
	return nil
}

func (r *projectMemberRepository) validateDelete(ctx context.Context, req *mdcv1.ProjectMember) error {
	return nil
}

func (t *projectMemberRepository) convertToInternal(msg *apiv2.ProjectMember) (*mdcv1.ProjectMember, error) {
	return &mdcv1.ProjectMember{
		Meta: &mdcv1.Meta{
			Id: msg.Id,
			Annotations: map[string]string{
				ProjectRoleAnnotation: msg.Role.String(),
			},
		},
	}, nil
}

func (t *projectMemberRepository) convertToProto(e *mdcv1.ProjectMember) (*apiv2.ProjectMember, error) {
	return &apiv2.ProjectMember{
		Id:        e.TenantId,
		Role:      ProjectRoleFromMap(e.Meta.Annotations),
		CreatedAt: e.Meta.CreatedTime,
	}, nil
}

func (t *projectMemberRepository) create(ctx context.Context, c *ProjectMemberCreateRequest) (*mdcv1.ProjectMember, error) {
	resp, err := t.s.mdc.ProjectMember().Create(ctx, &mdcv1.ProjectMemberCreateRequest{
		ProjectMember: &mdcv1.ProjectMember{
			Meta: &mdcv1.Meta{
				Annotations: map[string]string{
					tutil.TenantRoleAnnotation: c.Role.String(),
				},
			},
			TenantId:  c.MemberID,
			ProjectId: t.scope.projectID,
		},
	})
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return resp.ProjectMember, nil
}

func (t *projectMemberRepository) delete(ctx context.Context, e *mdcv1.ProjectMember) error {
	_, err := t.s.mdc.ProjectMember().Delete(ctx, &mdcv1.ProjectMemberDeleteRequest{
		Id: e.Meta.Id,
	})
	if err != nil {
		return errorutil.Convert(err)
	}

	return nil
}

func (t *projectMemberRepository) find(ctx context.Context, query *ProjectMemberQuery) (*mdcv1.ProjectMember, error) {
	if query.MemberId == nil {
		return nil, fmt.Errorf("member id must be specified")
	}

	memberships, err := t.list(ctx, query)
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

func (t *projectMemberRepository) get(ctx context.Context, id string) (*mdcv1.ProjectMember, error) {
	resp, err := t.s.mdc.ProjectMember().Get(ctx, &mdcv1.ProjectMemberGetRequest{
		Id: id,
	})
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return resp.ProjectMember, nil
}

func (t *projectMemberRepository) list(ctx context.Context, query *ProjectMemberQuery) ([]*mdcv1.ProjectMember, error) {
	resp, err := t.s.mdc.ProjectMember().Find(ctx, &mdcv1.ProjectMemberFindRequest{
		ProjectId:   &t.scope.projectID,
		TenantId:    query.MemberId,
		Annotations: query.Annotations,
	})
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return resp.ProjectMembers, nil
}

func (t *projectMemberRepository) matchScope(e *mdcv1.ProjectMember) bool {
	panic("unimplemented")
}

func (t *projectMemberRepository) update(ctx context.Context, _ *mdcv1.ProjectMember, msg *ProjectMemberUpdateRequest) (*mdcv1.ProjectMember, error) {
	resp, err := t.s.mdc.ProjectMember().Update(ctx, &mdcv1.ProjectMemberUpdateRequest{
		ProjectMember: msg.Member,
	})
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return resp.ProjectMember, nil
}
