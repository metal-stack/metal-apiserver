package repository

import (
	"context"
	"time"

	"github.com/metal-stack/api/go/errorutil"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/repository/api"
	"github.com/metal-stack/metal-apiserver/pkg/tags"
	"github.com/metal-stack/metal-apiserver/pkg/token"
	"github.com/metal-stack/metal-lib/pkg/pointer"
	tenantv1 "github.com/metal-stack/tenant-api/go/tenant/api/v1"
	"github.com/samber/lo"
)

type (
	projectMemberRepository struct {
		s     *Store
		scope *ProjectScope
	}

	projectMemberEntity struct {
		*tenantv1.ProjectMember
	}
)

func (t *projectMemberEntity) SetChanged(time time.Time) {}

func (t *projectMemberRepository) matchScope(e *projectMemberEntity) bool {
	if t.scope == nil {
		return true
	}

	return t.scope.projectID == e.ProjectId
}

func (t *projectMemberRepository) create(ctx context.Context, c *apiv2.ProjectMemberServiceCreateRequest) (*projectMemberEntity, error) {
	if t.scope == nil {
		return nil, errorutil.FailedPrecondition("project member create cannot be called unscoped")
	}

	var labels []string
	if c.Labels != nil {
		labels = tags.ToTags(c.Labels.Labels)
	}

	resp, err := t.s.tc.Apiv1().ProjectMember().Create(ctx, &tenantv1.ProjectMemberServiceCreateRequest{
		ProjectMember: &tenantv1.ProjectMember{
			Meta: &tenantv1.Meta{
				Annotations: map[string]string{
					api.ProjectRoleAnnotation: c.Role.String(),
				},
				Labels: labels,
			},
			TenantId:  c.Member,
			ProjectId: t.scope.projectID,
		},
	})
	if err != nil {
		if errorutil.IsConflict(err) {
			return nil, errorutil.Conflict("%q is already member of project %q", c.Member, t.scope.projectID)
		}

		return nil, errorutil.Convert(err)
	}

	return &projectMemberEntity{
		ProjectMember: resp.ProjectMember,
	}, nil
}

func (t *projectMemberRepository) delete(ctx context.Context, e *projectMemberEntity) (*deleteInfo, error) {
	_, err := t.s.tc.Apiv1().ProjectMember().Delete(ctx, &tenantv1.ProjectMemberServiceDeleteRequest{
		Id: e.Meta.Id,
	})
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return nil, nil
}

func (t *projectMemberRepository) find(ctx context.Context, query *apiv2.ProjectMemberQuery) (*projectMemberEntity, error) {
	if t.scope == nil {
		return nil, errorutil.FailedPrecondition("project member find cannot be called unscoped")
	}

	memberships, err := t.list(ctx, query)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	switch len(memberships) {
	case 0:
		return nil, errorutil.NotFound("member not found in project %q", t.scope.projectID)
	case 1:
		// noop
	default:
		return nil, errorutil.Internal("found multiple membership associations for a member to a project")
	}

	return memberships[0], nil
}

func (t *projectMemberRepository) get(ctx context.Context, id string) (*projectMemberEntity, error) {
	if t.scope == nil {
		return nil, errorutil.InvalidArgument("project member get cannot be called unscoped")
	}

	member, err := t.find(ctx, &apiv2.ProjectMemberQuery{
		Member: &id,
	})
	if err != nil {
		return nil, err
	}

	return member, nil
}

func (t *projectMemberRepository) list(ctx context.Context, query *apiv2.ProjectMemberQuery) ([]*projectMemberEntity, error) {
	if t.scope == nil {
		return nil, errorutil.FailedPrecondition("project member list cannot be called unscoped")
	}

	var (
		annotations      map[string]string
		includeInherited = false
	)

	if query == nil {
		query = &apiv2.ProjectMemberQuery{}
	}

	if query.Role != nil {
		annotations = map[string]string{
			api.ProjectRoleAnnotation: query.Role.String(),
		}
	}

	resp, err := t.s.tc.Apiv1().ProjectMember().List(ctx, &tenantv1.ProjectMemberServiceListRequest{
		ProjectId:   &t.scope.projectID,
		TenantId:    query.Member,
		Annotations: annotations,
	})
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	memberMap := make(map[string]*projectMemberEntity, len(resp.ProjectMembers))
	for _, pm := range resp.ProjectMembers {
		if query.Labels != nil {
			var (
				queryTags  = tags.ToTags(query.Labels.Labels)
				memberTags = pointer.SafeDeref(pm.Meta).Labels
			)

			noMatch, _ := lo.Difference(queryTags, memberTags)
			if len(noMatch) > 0 {
				continue
			}
		}

		memberMap[pm.TenantId] = &projectMemberEntity{
			ProjectMember: pm,
		}
	}

	projectResp, err := t.s.tc.Apiv1().Project().Get(ctx, &tenantv1.ProjectServiceGetRequest{
		Id: t.scope.projectID,
	})
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	if tok, ok := token.TokenFromContext(ctx); ok {
		if role, ok := tok.TenantRoles[projectResp.Project.TenantId]; ok {
			switch role {
			case apiv2.TenantRole_TENANT_ROLE_OWNER, apiv2.TenantRole_TENANT_ROLE_EDITOR, apiv2.TenantRole_TENANT_ROLE_VIEWER:
				includeInherited = true
			}
		}
		if tok.AdminRole != nil {
			switch *tok.AdminRole {
			case apiv2.AdminRole_ADMIN_ROLE_EDITOR, apiv2.AdminRole_ADMIN_ROLE_VIEWER:
				includeInherited = true
			}
		}
	}

	if includeInherited {
		resp, err := t.s.tc.Apiv1().Tenant().ListTenantMembers(ctx, &tenantv1.TenantServiceListTenantMembersRequest{
			TenantId:         projectResp.Project.TenantId,
			IncludeInherited: new(true),
		})
		if err != nil {
			return nil, errorutil.Convert(err)
		}

	NEXT:
		for _, tm := range resp.GetTenants() {
			if query.Member != nil && *query.Member != tm.Tenant.Meta.Id {
				continue
			}

			for k, v := range annotations {
				if ann, ok := tm.TenantAnnotations[k]; !ok || ann != v {
					continue NEXT
				}
			}

			if query.Labels != nil {
				// indirect members do not have labels
				continue
			}

			var indirectRole apiv2.ProjectRole
			switch tenantRoleFromMap(tm.TenantAnnotations) {
			case apiv2.TenantRole_TENANT_ROLE_OWNER:
				indirectRole = apiv2.ProjectRole_PROJECT_ROLE_OWNER
			case apiv2.TenantRole_TENANT_ROLE_EDITOR:
				indirectRole = apiv2.ProjectRole_PROJECT_ROLE_EDITOR
			case apiv2.TenantRole_TENANT_ROLE_VIEWER:
				indirectRole = apiv2.ProjectRole_PROJECT_ROLE_VIEWER
			case apiv2.TenantRole_TENANT_ROLE_GUEST, apiv2.TenantRole_TENANT_ROLE_UNSPECIFIED:
				continue
			default:
				continue
			}

			member, ok := memberMap[tm.Tenant.Meta.Id]
			if !ok {
				memberMap[tm.Tenant.Meta.Id] = &projectMemberEntity{
					ProjectMember: &tenantv1.ProjectMember{
						Meta: &tenantv1.Meta{
							Id: "", // indirect member, no id
							Annotations: map[string]string{
								api.ProjectRoleAnnotation: indirectRole.String(),
							},
							CreatedTime: tm.Tenant.Meta.CreatedTime,
						},
						ProjectId: t.scope.projectID,
						TenantId:  tm.Tenant.Meta.Id,
					},
				}

				continue
			}

			if indirectRole > projectRoleFromMap(member.Meta.Annotations) {
				member.Meta.Annotations[api.ProjectRoleAnnotation] = indirectRole.String()
				memberMap[tm.Tenant.Meta.Id] = member
			}
		}
	}

	pms := make([]*projectMemberEntity, 0, len(memberMap))
	for _, pm := range memberMap {
		pms = append(pms, pm)
	}

	return pms, nil
}

func (t *projectMemberRepository) update(ctx context.Context, member *projectMemberEntity, req *apiv2.ProjectMemberServiceUpdateRequest) (*projectMemberEntity, error) {
	if req.Role != nil && *req.Role != apiv2.ProjectRole_PROJECT_ROLE_UNSPECIFIED {
		member.Meta.Annotations[api.ProjectRoleAnnotation] = req.Role.String()
	}

	if req.Labels != nil {
		member.Meta.Labels = updateLabelsOnSlice(req.Labels, member.Meta.Labels)
	}

	resp, err := t.s.tc.Apiv1().ProjectMember().Update(ctx, &tenantv1.ProjectMemberServiceUpdateRequest{
		ProjectMember: member.ProjectMember,
	})
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return &projectMemberEntity{ProjectMember: resp.ProjectMember}, nil
}

func (t *projectMemberRepository) convertToProto(ctx context.Context, e *projectMemberEntity) (*apiv2.ProjectMember, error) {
	var labels *apiv2.Labels

	if len(e.Meta.Labels) > 0 {
		labels = &apiv2.Labels{
			Labels: tags.ToLabels(e.Meta.Labels),
		}
	}

	return &apiv2.ProjectMember{
		Member:              e.TenantId,
		Role:                projectRoleFromMap(e.Meta.Annotations),
		InheritedMembership: e.Meta.Id == "",
		Project:             e.ProjectId,
		Meta: &apiv2.Meta{
			Labels:     labels,
			CreatedAt:  e.Meta.CreatedTime,
			UpdatedAt:  e.Meta.UpdatedTime,
			Generation: uint64(e.Meta.Version),
		},
	}, nil
}

func projectRoleFromMap(annotations map[string]string) apiv2.ProjectRole {
	if annotations == nil {
		return apiv2.ProjectRole_PROJECT_ROLE_UNSPECIFIED
	}

	var (
		annotation  = annotations[api.ProjectRoleAnnotation]
		projectRole = apiv2.ProjectRole(apiv2.ProjectRole_value[annotation])
	)

	return projectRole
}
