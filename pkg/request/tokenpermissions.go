package request

import (
	"context"
	"fmt"

	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/api/go/permissions"
)

type (
	// tokenPermission represents the unflattened permissions from a token.
	// It maps the method to the allowed subjects per method.
	// The subject will be set to "*" in case of admin roles.
	// This works because a certain method can either be tenant or project scoped.
	// Therefore a single subject is enough to decide
	//
	// eg. map[method]map[subject]
	tokenPermissions map[string]map[string]bool
)

const anySubject = "*"

func (a *authorizer) getTokenPermissions(ctx context.Context, token *apiv2.Token) (tokenPermissions, error) {
	var (
		tp                 = tokenPermissions{}
		adminRole          *apiv2.AdminRole
		servicePermissions = permissions.GetServicePermissions()
	)

	if token == nil {
		for method := range servicePermissions.Visibility.Public {
			if _, ok := tp[method]; !ok {
				tp[method] = map[string]bool{}
			}
			tp[method][anySubject] = true
		}
		return tp, nil
	}

	if token.TokenType == apiv2.TokenType_TOKEN_TYPE_USER {
		pat, err := a.projectsAndTenantsGetter(ctx, token.User)
		if err != nil {
			return nil, err
		}
		// as we do not store roles in the user token, we set the roles from the information in the masterdata-db
		token.ProjectRoles = pat.ProjectRoles
		token.TenantRoles = pat.TenantRoles
		token.AdminRole = adminRole
		// user tokens should never have permissions cause they are not stored in the masterdata-db
		token.Permissions = nil
	}

	// Admin Roles have precedence
	if token.AdminRole != nil {
		var allMethods []string
		for method := range servicePermissions.Methods {
			allMethods = append(allMethods, method)
		}

		switch *token.AdminRole {
		case apiv2.AdminRole_ADMIN_ROLE_EDITOR:
			for _, method := range allMethods {
				tp[method] = map[string]bool{anySubject: true}
			}
			// Return here because all methods are allowed with all permissions
			return tp, nil

		case apiv2.AdminRole_ADMIN_ROLE_VIEWER:
			var (
				adminViewerMethods []string
			)

			adminViewerMethods = append(adminViewerMethods,
				servicePermissions.Roles.Tenant[apiv2.TenantRole_TENANT_ROLE_VIEWER.String()]...)
			adminViewerMethods = append(adminViewerMethods,
				servicePermissions.Roles.Project[apiv2.ProjectRole_PROJECT_ROLE_VIEWER.String()]...)
			adminViewerMethods = append(adminViewerMethods,
				servicePermissions.Roles.Admin[apiv2.AdminRole_ADMIN_ROLE_VIEWER.String()]...)
			adminViewerMethods = append(adminViewerMethods,
				servicePermissions.Roles.Infra[apiv2.InfraRole_INFRA_ROLE_VIEWER.String()]...)
			adminViewerMethods = append(adminViewerMethods, publicMethods()...)
			adminViewerMethods = append(adminViewerMethods, selfMethods()...)

			for _, method := range adminViewerMethods {
				tp[method] = map[string]bool{anySubject: true}
			}
			// Do not return here because it might be that some permissions are granted later

		default:
			return nil, fmt.Errorf("given admin role:%s is not valid", *token.AdminRole)
		}
	}

	// Permission
	for _, permission := range token.Permissions {
		subject := permission.Subject
		for _, method := range permission.Methods {
			if _, ok := tp[method]; !ok {
				tp[method] = map[string]bool{}
			}
			tp[method][subject] = true
		}
	}

	// Tenant Roles
	for subject, role := range token.TenantRoles {
		tenantMethods := servicePermissions.Roles.Tenant[role.Enum().String()]
		for _, method := range tenantMethods {
			if _, ok := tp[method]; !ok {
				tp[method] = map[string]bool{}
			}
			tp[method][subject] = true
		}
	}

	// Project Roles
	for subject, role := range token.ProjectRoles {
		projectMethods := servicePermissions.Roles.Project[role.Enum().String()]
		for _, method := range projectMethods {
			if _, ok := tp[method]; !ok {
				tp[method] = map[string]bool{}
			}
			tp[method][subject] = true
		}
	}

	// Public and Self Methods only on user tokens
	if token.TokenType == apiv2.TokenType_TOKEN_TYPE_USER {
		for method := range servicePermissions.Visibility.Public {
			if _, ok := tp[method]; !ok {
				tp[method] = map[string]bool{}
			}
			tp[method][anySubject] = true
		}

		for method := range servicePermissions.Visibility.Self {
			if _, ok := tp[method]; !ok {
				tp[method] = map[string]bool{}
			}
			// Subjects of self service must also be validated inside the service implementation
			tp[method][anySubject] = true
		}
	}

	// TODO infra roles not yet possible in a token

	return tp, nil
}

func publicMethods() []string {
	var m []string
	for method := range permissions.GetServicePermissions().Visibility.Public {
		m = append(m, method)
	}
	return m
}

func selfMethods() []string {
	var m []string
	for method := range permissions.GetServicePermissions().Visibility.Self {
		m = append(m, method)
	}
	return m
}
