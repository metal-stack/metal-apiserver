package repository

import (
	"context"
	"time"

	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	mdcv1 "github.com/metal-stack/masterdata-api/api/v1"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	"github.com/metal-stack/metal-apiserver/pkg/repository/api"
)

type (
	projectMemberRepository struct {
		s     *Store
		scope *ProjectScope
	}

	projectMemberEntity struct {
		*mdcv1.ProjectMember
	}
)

func (t *projectMemberEntity) SetChanged(time time.Time) {}

func (t *projectMemberRepository) convertToInternal(ctx context.Context, msg *apiv2.ProjectMember) (*projectMemberEntity, error) {
	return &projectMemberEntity{
		ProjectMember: &mdcv1.ProjectMember{
			Meta: &mdcv1.Meta{
				Id: msg.Id,
				Annotations: map[string]string{
					api.ProjectRoleAnnotation: msg.Role.String(),
				},
			},
		},
	}, nil
}

func (t *projectMemberRepository) convertToProto(ctx context.Context, e *projectMemberEntity) (*apiv2.ProjectMember, error) {
	if e.Meta.Annotations == nil {
		e.Meta.Annotations = map[string]string{}
	}

	return &apiv2.ProjectMember{
		Id:        e.TenantId,
		Role:      projectRoleFromMap(e.Meta.Annotations),
		CreatedAt: e.Meta.CreatedTime,
	}, nil
}

func (t *projectMemberRepository) create(ctx context.Context, c *api.ProjectMemberCreateRequest) (*projectMemberEntity, error) {
	resp, err := t.s.mdc.ProjectMember().Create(ctx, &mdcv1.ProjectMemberCreateRequest{
		ProjectMember: &mdcv1.ProjectMember{
			Meta: &mdcv1.Meta{
				Annotations: map[string]string{
					api.ProjectRoleAnnotation: c.Role.String(),
				},
			},
			TenantId:  c.TenantId,
			ProjectId: t.scope.projectID,
		},
	})
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return &projectMemberEntity{
		ProjectMember: resp.ProjectMember,
	}, nil
}

func (t *projectMemberRepository) delete(ctx context.Context, e *projectMemberEntity) error {
	_, err := t.s.mdc.ProjectMember().Delete(ctx, &mdcv1.ProjectMemberDeleteRequest{
		Id: e.Meta.Id,
	})
	if err != nil {
		return errorutil.Convert(err)
	}

	return nil
}

func (t *projectMemberRepository) find(ctx context.Context, query *api.ProjectMemberQuery) (*projectMemberEntity, error) {
	if query.TenantId == nil {
		return nil, errorutil.InvalidArgument("tenant id must be specified")
	}

	memberships, err := t.list(ctx, query)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	switch len(memberships) {
	case 0:
		return nil, errorutil.NotFound("tenant %s is not a member of project %s", *query.TenantId, t.scope.projectID)
	case 1:
		// noop
	default:
		return nil, errorutil.Internal("found multiple membership associations for a member to a project")
	}

	return memberships[0], nil
}

func (t *projectMemberRepository) get(ctx context.Context, id string) (*projectMemberEntity, error) {
	member, err := t.find(ctx, &api.ProjectMemberQuery{
		TenantId: &id,
	})
	if err != nil {
		return nil, err
	}

	return member, nil
}

func (t *projectMemberRepository) list(ctx context.Context, query *api.ProjectMemberQuery) ([]*projectMemberEntity, error) {
	resp, err := t.s.mdc.ProjectMember().Find(ctx, &mdcv1.ProjectMemberFindRequest{
		ProjectId:   &t.scope.projectID,
		TenantId:    query.TenantId,
		Annotations: query.Annotations,
	})
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	pms := make([]*projectMemberEntity, 0, len(resp.ProjectMembers))
	for _, pm := range resp.ProjectMembers {
		pms = append(pms, &projectMemberEntity{ProjectMember: pm})
	}

	return pms, nil
}

func (t *projectMemberRepository) matchScope(e *projectMemberEntity) bool {
	if t.scope == nil {
		return true
	}

	return t.scope.projectID == e.ProjectId
}

func (t *projectMemberRepository) update(ctx context.Context, member *projectMemberEntity, msg *api.ProjectMemberUpdateRequest) (*projectMemberEntity, error) {
	if msg.Role != apiv2.ProjectRole_PROJECT_ROLE_UNSPECIFIED {
		member.Meta.Annotations[api.ProjectRoleAnnotation] = msg.Role.String()
	}

	resp, err := t.s.mdc.ProjectMember().Update(ctx, &mdcv1.ProjectMemberUpdateRequest{
		ProjectMember: member.ProjectMember,
	})
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return &projectMemberEntity{ProjectMember: resp.ProjectMember}, nil
}

func (t *projectMemberRepository) checkIfMemberIsLastOwner(ctx context.Context, membership *projectMemberEntity) (bool, error) {
	isOwner := membership.Meta.Annotations[api.ProjectRoleAnnotation] == apiv2.ProjectRole_PROJECT_ROLE_OWNER.String()
	if !isOwner {
		return false, nil
	}

	memberships, err := t.list(ctx, &api.ProjectMemberQuery{
		Annotations: map[string]string{
			api.ProjectRoleAnnotation: apiv2.ProjectRole_PROJECT_ROLE_OWNER.String(),
		},
	})
	if err != nil {
		return false, err
	}

	return len(memberships) < 2, nil
}
