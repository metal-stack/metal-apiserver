package api

import (
	"context"

	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
)

const (
	ProjectRoleAnnotation = "metal-stack.io/project-role"
)

type (
	ProjectsAndTenants struct {
		Projects      []*apiv2.Project
		Tenants       []*apiv2.Tenant
		DefaultTenant *apiv2.Tenant
		ProjectRoles  map[string]apiv2.ProjectRole
		TenantRoles   map[string]apiv2.TenantRole
	}

	ProjectsAndTenantsGetter func(ctx context.Context, userId string) (*ProjectsAndTenants, error)
)
