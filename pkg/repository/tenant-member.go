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
	tenantMemberEntity struct {
		*tenantv1.TenantMember
		*tenantv1.TenantWithMembershipAnnotations
	}

	tenantMemberRepository struct {
		s     *Store
		scope *TenantScope
	}
)

func (t *tenantMemberEntity) SetChanged(time time.Time) {}

func (t *tenantMemberRepository) matchScope(e *tenantMemberEntity) bool {
	if t.scope == nil {
		return true
	}

	return t.scope.tenantID == e.TenantId
}

func (t *tenantMemberRepository) create(ctx context.Context, c *apiv2.TenantMemberServiceCreateRequest) (*tenantMemberEntity, error) {
	if t.scope == nil {
		return nil, errorutil.FailedPrecondition("tenant member create cannot be called unscoped")
	}

	var labels []string
	if c.Labels != nil {
		labels = tags.ToTags(c.Labels.Labels)
	}

	resp, err := t.s.tc.Apiv1().TenantMember().Create(ctx, &tenantv1.TenantMemberServiceCreateRequest{
		TenantMember: &tenantv1.TenantMember{
			Meta: &tenantv1.Meta{
				Annotations: map[string]string{
					api.TenantRoleAnnotation: c.Role.String(),
				},
				Labels: labels,
			},
			MemberId: c.Member,
			TenantId: t.scope.tenantID,
		},
	})
	if err != nil {
		if errorutil.IsConflict(err) {
			return nil, errorutil.Conflict("%q is already member of tenant %q", c.Member, t.scope.tenantID)
		}

		return nil, errorutil.Convert(err)
	}

	return &tenantMemberEntity{
		TenantMember: resp.TenantMember,
	}, nil
}

func (t *tenantMemberRepository) delete(ctx context.Context, e *tenantMemberEntity) (*deleteInfo, error) {
	_, err := t.s.tc.Apiv1().TenantMember().Delete(ctx, &tenantv1.TenantMemberServiceDeleteRequest{
		Id: e.Meta.Id,
	})
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return nil, nil
}

func (t *tenantMemberRepository) find(ctx context.Context, query *apiv2.TenantMemberQuery) (*tenantMemberEntity, error) {
	if t.scope == nil {
		return nil, errorutil.FailedPrecondition("tenant member find cannot be called unscoped")
	}

	memberships, err := t.list(ctx, query)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	switch len(memberships) {
	case 0:
		return nil, errorutil.NotFound("member not found in tenant %q", t.scope.tenantID)
	case 1:
		// noop
	default:
		return nil, errorutil.Internal("found multiple membership associations for a member to a tenant")
	}

	return memberships[0], nil
}

func (t *tenantMemberRepository) get(ctx context.Context, id string) (*tenantMemberEntity, error) {
	if t.scope == nil {
		return nil, errorutil.InvalidArgument("tenant member get cannot be called unscoped")
	}

	member, err := t.find(ctx, &apiv2.TenantMemberQuery{
		Member: &id,
	})
	if err != nil {
		return nil, err
	}

	return member, nil
}

func (t *tenantMemberRepository) list(ctx context.Context, query *apiv2.TenantMemberQuery) ([]*tenantMemberEntity, error) {
	if t.scope == nil {
		return nil, errorutil.InvalidArgument("tenant member list get cannot be called unscoped")
	}

	var (
		annotations map[string]string
	)

	if query == nil {
		query = &apiv2.TenantMemberQuery{}
	}

	if query.Role != nil {
		annotations = map[string]string{
			api.TenantRoleAnnotation: query.Role.String(),
		}
	}

	directMembers, err := t.s.tc.Apiv1().TenantMember().List(ctx, &tenantv1.TenantMemberServiceListRequest{
		TenantId:    &t.scope.tenantID,
		Annotations: annotations,
		MemberId:    query.Member,
	})
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	memberMap := make(map[string]*tenantMemberEntity, len(directMembers.TenantMembers))

	for _, tm := range directMembers.TenantMembers {
		if query.Labels != nil {
			var (
				queryTags  = tags.ToTags(query.Labels.Labels)
				memberTags = pointer.SafeDeref(tm.Meta).Labels
			)

			noMatch, _ := lo.Difference(queryTags, memberTags)
			if len(noMatch) > 0 {
				continue
			}
		}

		memberMap[tm.MemberId] = &tenantMemberEntity{TenantMember: tm}
	}

	resp, err := t.s.tc.Apiv1().Tenant().ListTenantMembers(ctx, &tenantv1.TenantServiceListTenantMembersRequest{
		TenantId:         t.scope.tenantID,
		IncludeInherited: new(true),
	})
	if err != nil {
		return nil, errorutil.Convert(err)
	}

NEXT:
	for _, tm := range resp.Tenants {
		if query.Member != nil && *query.Member != tm.Tenant.Meta.Id {
			continue
		}

		for k, v := range annotations {
			if ann, ok := tm.TenantAnnotations[k]; !ok || ann != v {
				if !ok && (k == api.TenantRoleAnnotation && v == apiv2.TenantRole_TENANT_ROLE_GUEST.String()) {
					continue
				}

				continue NEXT
			}
		}

		if query.Labels != nil {
			// indirect members do not have labels
			continue
		}

		member, ok := memberMap[tm.Tenant.Meta.Id]
		if !ok {
			memberMap[tm.Tenant.Meta.Id] = &tenantMemberEntity{
				TenantMember: &tenantv1.TenantMember{
					Meta: &tenantv1.Meta{
						Id:          "", // indirect member, no id
						CreatedTime: tm.Tenant.Meta.CreatedTime,
						Annotations: tm.TenantAnnotations,
					},
					TenantId: t.scope.tenantID,
					MemberId: tm.Tenant.Meta.Id,
				},
				TenantWithMembershipAnnotations: tm,
			}

			continue
		}

		member.TenantWithMembershipAnnotations = tm
	}

	tms := make([]*tenantMemberEntity, 0, len(memberMap))
	for _, member := range memberMap {
		tms = append(tms, member)
	}

	return tms, nil
}

func (t *tenantMemberRepository) update(ctx context.Context, member *tenantMemberEntity, req *apiv2.TenantMemberServiceUpdateRequest) (*tenantMemberEntity, error) {
	if req.Role != nil && *req.Role != apiv2.TenantRole_TENANT_ROLE_UNSPECIFIED {
		member.Meta.Annotations[api.TenantRoleAnnotation] = req.Role.String()
	}

	if req.Labels != nil {
		member.Meta.Labels = updateLabelsOnSlice(req.Labels, member.Meta.Labels)
	}

	resp, err := t.s.tc.Apiv1().TenantMember().Update(ctx, &tenantv1.TenantMemberServiceUpdateRequest{
		TenantMember: member.TenantMember,
	})
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return &tenantMemberEntity{TenantMember: resp.TenantMember}, nil
}

func (t *tenantMemberRepository) convertToProto(ctx context.Context, e *tenantMemberEntity) (*apiv2.TenantMember, error) {
	var (
		labels          *apiv2.Labels
		includeProjects = false
		projects        []string
	)

	if tok, ok := token.TokenFromContext(ctx); ok {
		if role, ok := tok.TenantRoles[e.TenantId]; ok {
			switch role {
			case apiv2.TenantRole_TENANT_ROLE_OWNER, apiv2.TenantRole_TENANT_ROLE_EDITOR, apiv2.TenantRole_TENANT_ROLE_VIEWER:
				includeProjects = true
			}
		}
		if tok.AdminRole != nil {
			switch *tok.AdminRole {
			case apiv2.AdminRole_ADMIN_ROLE_EDITOR, apiv2.AdminRole_ADMIN_ROLE_VIEWER:
				includeProjects = true
			}
		}
	}

	if len(e.Meta.Labels) > 0 {
		labels = &apiv2.Labels{
			Labels: tags.ToLabels(e.Meta.Labels),
		}
	}

	if e.TenantWithMembershipAnnotations != nil && includeProjects {
		projects = e.ProjectIds
	}

	return &apiv2.TenantMember{
		Member:   e.MemberId,
		Role:     tenantRoleFromMap(e.Meta.Annotations),
		Projects: projects,
		Meta: &apiv2.Meta{
			Labels:     labels,
			CreatedAt:  e.Meta.CreatedTime,
			UpdatedAt:  e.Meta.UpdatedTime,
			Generation: uint64(e.Meta.Version),
		},
		Tenant: e.TenantId,
	}, nil
}

func tenantRoleFromMap(annotations map[string]string) apiv2.TenantRole {
	if annotations == nil {
		return apiv2.TenantRole_TENANT_ROLE_GUEST
	}

	var (
		annotation = annotations[api.TenantRoleAnnotation]
		tenantRole = apiv2.TenantRole(apiv2.TenantRole_value[annotation])
	)

	return tenantRole
}
