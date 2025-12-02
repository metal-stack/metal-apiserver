package request

import (
	"context"
	"fmt"
	"slices"

	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/api/go/permissions"
)

type (
	// entry is a single value stored in a set
	entry struct{}
	// set emulates a slice with unique entries by using a map internally to achieve a O(1) access
	set map[string]entry
	// tokenPermission represents the unflattened permissions from a token.
	// It maps the method to the allowed subjects per method.
	// The subject will be set to "*" in case of admin roles.
	// This works because a certain method can either be tenant or project scoped.
	// Therefore a single subject is enough to decide
	//
	// eg. map[method]set[subject]
	tokenPermissions map[string]set
)

const anySubject = "*"

func (a *authorizer) TokenMethods(ctx context.Context, token *apiv2.Token) ([]string, error) {
	tp, err := a.getTokenPermissions(ctx, token)
	if err != nil {
		return nil, err
	}
	var methods []string
	for method := range tp {
		methods = append(methods, method)
	}
	slices.Sort(methods)
	return methods, nil
}

func (a *authorizer) getTokenPermissions(ctx context.Context, token *apiv2.Token) (tokenPermissions, error) {
	var (
		tp                 = tokenPermissions{}
		servicePermissions = permissions.GetServicePermissions()
	)

	if token == nil || token.User == "" {
		for method := range servicePermissions.Visibility.Public {
			if _, ok := tp[method]; !ok {
				tp[method] = set{}
			}
			tp[method][anySubject] = entry{}
		}
		return tp, nil
	}

	pat, err := a.projectsAndTenantsGetter(ctx, token.User)
	if err != nil {
		return nil, err
	}

	if token.TokenType == apiv2.TokenType_TOKEN_TYPE_USER {
		// as we do not store roles in the user token, we set the roles from the information in the masterdata-db
		token.ProjectRoles = pat.ProjectRoles
		token.TenantRoles = pat.TenantRoles
		// User token will never get admin roles from the database
		token.AdminRole = nil
		// user tokens should never have permissions cause they are not stored in the masterdata-db
		token.Permissions = nil
	}

	// Admin Roles have precedence
	if token.AdminRole != nil {

		switch *token.AdminRole {
		case apiv2.AdminRole_ADMIN_ROLE_EDITOR:
			for method := range servicePermissions.Methods {
				tp[method] = set{anySubject: entry{}}
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
				tp[method] = set{anySubject: entry{}}
			}
			// Do not return here because it might be that some permissions are granted later

		default:
			return nil, fmt.Errorf("given admin role:%s is not valid", *token.AdminRole)
		}
	}

	// Infra Roles
	if token.InfraRole != nil {
		for _, method := range servicePermissions.Roles.Infra[token.InfraRole.String()] {
			if _, ok := tp[method]; !ok {
				tp[method] = set{}
			}
			tp[method][anySubject] = entry{}
		}
	}

	// Permission
	for _, permission := range token.Permissions {
		subject := permission.Subject
		for _, method := range permission.Methods {
			if _, ok := tp[method]; !ok {
				tp[method] = set{}
			}
			tp[method][subject] = entry{}
		}
	}

	// Tenant Roles
	for subject, role := range token.TenantRoles {
		tenantMethods := servicePermissions.Roles.Tenant[role.Enum().String()]
		for _, method := range tenantMethods {
			if _, ok := tp[method]; !ok {
				tp[method] = set{}
			}
			tp[method][subject] = entry{}
		}
	}

	// Project Roles
	for subject, role := range token.ProjectRoles {
		projectMethods := servicePermissions.Roles.Project[role.Enum().String()]
		for _, method := range projectMethods {
			if _, ok := tp[method]; !ok {
				tp[method] = set{}
			}
			tp[method][subject] = entry{}
		}
	}

	// Now reduce "*" permissions of non admin users to allowed subjects
	if token.AdminRole == nil {
		for method, subjects := range tp {
			// only "*" subject is considered
			if _, ok := subjects[anySubject]; !ok {
				continue
			}
			if servicePermissions.Visibility.Project[method] {
				delete(tp[method], anySubject)
				for project, role := range pat.ProjectRoles {
					if slices.Contains(servicePermissions.Roles.Project[role.Enum().String()], method) {
						tp[method][project] = entry{}
					}
				}
			}
			if servicePermissions.Visibility.Tenant[method] {
				delete(tp[method], anySubject)
				for tenant, role := range pat.TenantRoles {
					if slices.Contains(servicePermissions.Roles.Tenant[role.Enum().String()], method) {
						tp[method][tenant] = entry{}
					}
				}
			}
		}
	}

	for method, subjects := range tp {
		switch {
		case servicePermissions.Visibility.Project[method]:
			for subject := range subjects {
				if subject == anySubject || subject == "" {
					continue
				}
				if _, ok := pat.ProjectRoles[subject]; !ok {
					delete(subjects, subject)
				}
			}
		case servicePermissions.Visibility.Tenant[method]:
			for subject := range subjects {
				if subject == anySubject || subject == "" {
					continue
				}
				if _, ok := pat.TenantRoles[subject]; !ok {
					delete(subjects, subject)
				}
			}
		default:
			// noop
		}
	}

	// Public and Self Methods only on user tokens
	if token.TokenType == apiv2.TokenType_TOKEN_TYPE_USER {
		for method := range servicePermissions.Visibility.Public {
			if _, ok := tp[method]; !ok {
				tp[method] = set{}
			}
			tp[method][anySubject] = entry{}
		}

		for method := range servicePermissions.Visibility.Self {
			if _, ok := tp[method]; !ok {
				tp[method] = set{}
			}
			// Subjects of self service must also be validated inside the service implementation
			tp[method][anySubject] = entry{}
		}
	}

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
