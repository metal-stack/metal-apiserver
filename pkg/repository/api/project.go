package api

import (
	"context"

	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
)

const (
	ProjectRoleAnnotation = "metal-stack.io/project-role"
)

type (
	ProjectMemberCreateRequest struct {
		TenantId string
		Role     apiv2.ProjectRole
	}

	ProjectMemberUpdateRequest struct {
		Role apiv2.ProjectRole
		Meta apiv2.Meta
	}

	ProjectMemberQuery struct {
		TenantId    *string
		Annotations map[string]string
	}

	ProjectsAndTenants struct {
		Projects      []*apiv2.Project
		Tenants       []*apiv2.Tenant
		DefaultTenant *apiv2.Tenant
		ProjectRoles  map[string]apiv2.ProjectRole
		TenantRoles   map[string]apiv2.TenantRole
	}

	ProjectsAndTenantsGetter func(ctx context.Context, userId string) (*ProjectsAndTenants, error)
)

func (*ProjectMemberUpdateRequest) GetUpdateMeta() *apiv2.UpdateMeta {
	return &apiv2.UpdateMeta{}
}
