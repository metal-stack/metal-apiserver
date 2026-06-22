package request

import (
	"context"
	"errors"
	"fmt"
	"slices"

	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/api/go/permissions"
	"github.com/metal-stack/metal-apiserver/pkg/repository/api"
)

type (
	// entry is a single value stored in a set
	entry struct{}
	// set emulates a slice with unique entries by using a map internally to achieve a O(1) access
	set map[string]entry
	// flattenedPerms contain flattened method permissions for every subject, all roles were expanded.
	// It maps the method to the allowed subjects per method.
	// The subject will be set to "*" in case of admin roles.
	// This works because a certain method can either be tenant or project scoped.
	// Therefore a single subject is enough to decide
	//
	// eg. map[method]set[subject]
	flattenedPerms map[string]set
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

func (a *authorizer) TokenPermissions(ctx context.Context, token *apiv2.Token) (flattenedPerms, error) {
	switch {
	case token == nil:
		return a.anonymousRequestPermissions()
	case token.TokenType == apiv2.TokenType_TOKEN_TYPE_API:
		return a.tokenPermissions(ctx, token)
	case token.TokenType == apiv2.TokenType_TOKEN_TYPE_USER:
		return a.databasePermissions(ctx, token.User)
	default:
		return nil, fmt.Errorf("unexpected token type %q", token.TokenType.String())
	}
}

func (a *authorizer) ValidateTokenAgainstDatabase(ctx context.Context, currentToken, requestedToken *apiv2.Token) error {
	userPermissions, err := a.databasePermissions(ctx, currentToken.User)
	if err != nil {
		return err
	}

	// FIXME: check that no roles are requested for tenants and projects that the user does not belong to!
	// for tenant, role := range requestedToken.GetTenantRoles() {
	// 	// you do not belong to this tenant!
	// }

	// for project, role := range requestedToken.GetProjectRoles() {
	// 	// you do not belong to this project!
	// }

	// // you cannot request machine, infra or admin roles!

	// now expand the permissions from the requested token to check if the methods are allowed
	requestedPermissions, err := a.TokenPermissions(ctx, requestedToken)
	if err != nil {
		return err
	}

	// sort requestedPermissions by method
	var methods []string
	for method := range requestedPermissions {
		methods = append(methods, method)
	}
	slices.Sort(methods)

	for _, method := range methods {
		subjects, ok := requestedPermissions[method]
		if !ok {
			continue
		}
		currentSubjects, ok := userPermissions[method]
		if !ok {
			errMsg := fmt.Sprintf("the following method %q is not allowed", method)
			if len(subjects) > 0 {
				errMsg += fmt.Sprintf(" on any of the requested subjects: %s", subjects)
			}
			return errors.New(errMsg)
		}

		if _, ok := currentSubjects[AnySubject]; ok {
			continue
		}
		// It is possible to request any subjects to be able to have a token
		// which is able to make calls to projects which will be created in the future.
		// The actually possible subjects are calculated at request time.
		if _, ok := subjects[AnySubject]; ok {
			continue
		}

		for subject := range subjects {
			if _, ok := currentSubjects[subject]; !ok {
				return fmt.Errorf("method %q is not allowed on subject %q with your current user permissions", method, subject)
			}
		}
	}

	return nil
}

// anonymousRequestPermissions returns a flat permission set for anonymous user requests.
func (a *authorizer) anonymousRequestPermissions() (flattenedPerms, error) {
	return a.expandRoles(nil, nil)
}

// tokenPermissions returns a flat permission set from a token.
// It expands the roles stored inside the token.
func (a *authorizer) tokenPermissions(ctx context.Context, token *apiv2.Token) (flattenedPerms, error) {
	pat, err := a.projectsAndTenantsGetter(ctx, token.User)
	if err != nil {
		return nil, err
	}

	return a.expandRoles(pat, token)
}

// databasePermissions returns a flat permission set from the database for the given user.
// It expands the roles stored inside the token.
func (a *authorizer) databasePermissions(ctx context.Context, user string) (flattenedPerms, error) {
	pat, err := a.projectsAndTenantsGetter(ctx, user)
	if err != nil {
		return nil, err
	}

	return a.expandRoles(pat, &apiv2.Token{
		Uuid:         user,
		User:         user,
		TokenType:    apiv2.TokenType_TOKEN_TYPE_USER,
		ProjectRoles: pat.ProjectRoles,
		TenantRoles:  pat.TenantRoles,
		// User token does not gain admin roles from the database because
		// we want to grant admin permissions only explicitly through API token
		AdminRole:    nil,
		InfraRole:    nil,
		MachineRoles: nil,
	})
}

func (a *authorizer) expandRoles(pat *api.ProjectsAndTenants, token *apiv2.Token) (flattenedPerms, error) {
	var (
		tp                 = flattenedPerms{}
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
				tp[method] = set{AnySubject: entry{}}
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
			tp[method][AnySubject] = entry{}
		}
	}

	// Machine Roles
	for subject, role := range token.MachineRoles {
		machineMethods := servicePermissions.Roles.Machine[role.Enum().String()]
		for _, method := range machineMethods {
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
			if _, ok := subjects[AnySubject]; !ok {
				continue
			}
			if servicePermissions.Visibility.Project[method] {
				delete(tp[method], AnySubject)
				for project, role := range pat.ProjectRoles {
					if slices.Contains(servicePermissions.Roles.Project[role.Enum().String()], method) {
						tp[method][project] = entry{}
					}
				}
			}
			if servicePermissions.Visibility.Tenant[method] {
				delete(tp[method], AnySubject)
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
