package request

import (
	"context"
	"fmt"

	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/api/go/permissions"
	"github.com/samber/lo"
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

func (s set) String() string {
	var keys []string
	for k := range s {
		keys = append(keys, k)
	}
	if len(keys) == 0 {
		return ""
	}
	return fmt.Sprintf("%s", keys)
}

const AnySubject = "*"

func (a *authorizer) TokenPermissions(ctx context.Context, token *apiv2.Token) (tokenPermissions, error) {
	return a.getTokenPermissions(ctx, token)
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
			tp[method][AnySubject] = entry{}
		}
		return tp, nil
	}

	pat, err := a.projectsAndTenantsGetter(ctx, token.User)
	if err != nil {
		return nil, err
	}

	if token.TokenType == apiv2.TokenType_TOKEN_TYPE_USER {
		// as we do not store roles in the user token, we set the roles from the information in the tenant-apiserver
		token.ProjectRoles = pat.ProjectRoles
		token.TenantRoles = pat.TenantRoles
		// User token will never get admin roles from the database to prevent interactive logins to directly have admin role.
		token.AdminRole = nil
		// user tokens should never have permissions cause they are not stored in the tenant-apiserver
		token.Permissions = nil
	}

	// Admin Roles have precedence
	if token.AdminRole != nil {

		switch role := *token.AdminRole; role {
		case apiv2.AdminRole_ADMIN_ROLE_EDITOR:
			for method := range servicePermissions.Methods {
				tp[method] = set{AnySubject: entry{}}
			}

			// Return here because all methods are allowed with all permissions
			return tp, nil

		case apiv2.AdminRole_ADMIN_ROLE_VIEWER:
			for _, method := range a.adminViewerMethods {
				tp[method] = set{AnySubject: entry{}}
			}
			// Do not return here because it might be that some permissions are granted later

		default:
			return nil, fmt.Errorf("given admin role:%s is not valid", *token.AdminRole)
		}
	}

	// Infra Roles
	if token.InfraRole != nil {
		for method := range servicePermissions.Roles.Infra[*token.InfraRole] {
			if _, ok := tp[method]; !ok {
				tp[method] = set{}
			}
			tp[method][AnySubject] = entry{}
		}
	}

	// Machine Roles
	for subject, role := range token.MachineRoles {
		machineMethods := servicePermissions.Roles.Machine[role]
		for method := range machineMethods {
			if _, ok := tp[method]; !ok {
				tp[method] = set{}
			}
			tp[method][subject] = entry{}
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
		tenantMethods := servicePermissions.Roles.Tenant[role]
		for method := range tenantMethods {
			if _, ok := tp[method]; !ok {
				tp[method] = set{}
			}
			tp[method][subject] = entry{}
		}
	}

	// Project Roles
	for subject, role := range token.ProjectRoles {
		projectMethods := servicePermissions.Roles.Project[role]
		for method := range projectMethods {
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
			if _, ok := subjects[AnySubject]; !ok {
				continue
			}
			if servicePermissions.Visibility.Project[method] {
				delete(tp[method], AnySubject)
				for project, role := range pat.ProjectRoles {
					if _, ok := servicePermissions.Roles.Project[role][method]; ok {
						tp[method][project] = entry{}
					}
				}
			}
			if servicePermissions.Visibility.Tenant[method] {
				delete(tp[method], AnySubject)
				for tenant, role := range pat.TenantRoles {
					if _, ok := servicePermissions.Roles.Tenant[role][method]; ok {
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
				if subject == AnySubject || subject == "" {
					continue
				}
				if _, ok := pat.ProjectRoles[subject]; !ok {
					delete(subjects, subject)
				}
			}
		case servicePermissions.Visibility.Tenant[method]:
			for subject := range subjects {
				if subject == AnySubject || subject == "" {
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
			tp[method][AnySubject] = entry{}
		}

		for method := range servicePermissions.Visibility.Self {
			if _, ok := tp[method]; !ok {
				tp[method] = set{}
			}
			// Subjects of self service must also be validated inside the service implementation
			tp[method][AnySubject] = entry{}
		}
	}

	return tp, nil
}

func adminViewerMethods() []string {
	var (
		servicePermissions = permissions.GetServicePermissions()
		adminViewerMethods []string
		publicMethods      []string
		selfMethods        []string
	)
	for method := range permissions.GetServicePermissions().Visibility.Public {
		publicMethods = append(publicMethods, method)
	}

	for method := range permissions.GetServicePermissions().Visibility.Self {
		selfMethods = append(selfMethods, method)
	}

	adminViewerMethods = append(adminViewerMethods,
		lo.Keys(servicePermissions.Roles.Tenant[apiv2.TenantRole_TENANT_ROLE_VIEWER])...)
	adminViewerMethods = append(adminViewerMethods,
		lo.Keys(servicePermissions.Roles.Project[apiv2.ProjectRole_PROJECT_ROLE_VIEWER])...)
	adminViewerMethods = append(adminViewerMethods,
		lo.Keys(servicePermissions.Roles.Admin[apiv2.AdminRole_ADMIN_ROLE_VIEWER])...)
	adminViewerMethods = append(adminViewerMethods,
		lo.Keys(servicePermissions.Roles.Infra[apiv2.InfraRole_INFRA_ROLE_VIEWER])...)
	adminViewerMethods = append(adminViewerMethods,
		lo.Keys(servicePermissions.Roles.Machine[apiv2.MachineRole_MACHINE_ROLE_VIEWER])...)
	adminViewerMethods = append(adminViewerMethods, publicMethods...)
	adminViewerMethods = append(adminViewerMethods, selfMethods...)

	return adminViewerMethods
}
