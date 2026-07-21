package repository

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/metal-stack/api/go/errorutil"
	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/api/go/permissions"
	"github.com/metal-stack/metal-apiserver/pkg/repository/api"
	"github.com/metal-stack/metal-apiserver/pkg/request"
	"github.com/metal-stack/metal-apiserver/pkg/token"
	"github.com/samber/lo"
)

func (t *tokenRepository) validateCreate(ctx context.Context, req *adminv2.TokenServiceCreateRequest) error {
	if t.scope == nil {
		return errorutil.FailedPrecondition("tokens cannot be created unscoped")
	}

	var (
		rq   = req.TokenCreateRequest
		user = t.scope.user
	)

	sessionToken, ok := token.TokenFromContext(ctx)
	if !ok || sessionToken == nil {
		return errorutil.Unauthenticated("no token found in request")
	}

	switch sessionToken.TokenType {
	case apiv2.TokenType_TOKEN_TYPE_API, apiv2.TokenType_TOKEN_TYPE_USER:
		// noop
	default:
		return errorutil.FailedPrecondition("invalid token type for token creation: %q", sessionToken.TokenType)
	}

	projectsAndTenants, err := t.patg(ctx, user)
	if err != nil {
		return errorutil.NewInternal(err)
	}
	var (
		isAdmin   bool
		adminRole apiv2.AdminRole
	)

	if role, ok := t.hasAdminRole(projectsAndTenants); ok {
		if sessionToken.AdminRole == nil || *sessionToken.AdminRole == apiv2.AdminRole_ADMIN_ROLE_UNSPECIFIED {
			if err := t.isAdminRoleRequestAllowed(projectsAndTenants, rq.AdminRole); err != nil {
				return errorutil.NewPermissionDenied(err)
			}
			sessionToken.AdminRole = rq.AdminRole
			sessionToken.TokenType = apiv2.TokenType_TOKEN_TYPE_API
		}

		adminRole = *role
		isAdmin = true

		t.s.log.Debug("user is member of the provider-tenant", "admin-role", sessionToken.AdminRole)
	}

	if !isAdmin && req.User != nil {
		return errorutil.PermissionDenied("only admins can specify token user")
	}

	rq.Permissions = compactPermissions(rq.Permissions)
	if err := t.validatePermissions(ctx, rq.Permissions, projectsAndTenants); err != nil {
		return errorutil.PermissionDenied("invalid permissions requested: %w", err)
	}

	var (
		requestedToken = &apiv2.Token{
			User:         user,
			ProjectRoles: rq.ProjectRoles,
			TenantRoles:  rq.TenantRoles,
			AdminRole:    rq.AdminRole,
			InfraRole:    rq.InfraRole,
			MachineRoles: rq.MachineRoles,
			Permissions:  flattenPermissions(rq.Permissions),
			TokenType:    apiv2.TokenType_TOKEN_TYPE_API,
		}
		userToken = &apiv2.Token{
			User:         user,
			ProjectRoles: projectsAndTenants.ProjectRoles,
			TenantRoles:  projectsAndTenants.TenantRoles,
			AdminRole:    nil,
			InfraRole:    sessionToken.InfraRole,
			MachineRoles: sessionToken.MachineRoles,
		}
	)

	if isAdmin {
		userToken.AdminRole = &adminRole
	}

	// we first validate token permission elevation for the token used in the token create request,
	// which might be an API token with restricted permissions

	err = t.validateTokenRequest(ctx, sessionToken, requestedToken)
	if err != nil {
		return errorutil.NewPermissionDenied(err)
	}

	// now, we validate if the user is still permitted to create such a token
	// doing this check is not strictly necessary because the resulting token would fail in the auther when being compared
	// to the actual user permissions, but it's nicer for the user to already prevent token creation immediately in this place

	err = t.validateTokenRequest(ctx, userToken, requestedToken)
	if err != nil {
		return errorutil.NewPermissionDenied(err)
	}

	return nil
}

func (t *tokenRepository) validateUpdate(ctx context.Context, req *apiv2.TokenServiceUpdateRequest, tokenToUpdate *api.TokenWithSecret) error {
	if t.scope == nil {
		return errorutil.FailedPrecondition("tokens cannot be updated unscoped")
	}

	if req.UpdateMeta != nil && req.UpdateMeta.UpdatedAt != nil {
		return errorutil.InvalidArgument("optimistic locking is not yet implemented, please do not provide updated_at in update meta")
	}

	sessionToken, ok := token.TokenFromContext(ctx)
	if !ok || sessionToken == nil {
		return errorutil.Unauthenticated("no token found in request")
	}

	var (
		user = t.scope.user
	)

	projectsAndTenants, err := t.patg(ctx, user)
	if err != nil {
		return errorutil.NewInternal(err)
	}

	req.Permissions = compactPermissions(req.Permissions)
	if err := t.validatePermissions(ctx, req.Permissions, projectsAndTenants); err != nil {
		return errorutil.PermissionDenied("invalid permissions requested: %w", err)
	}

	var (
		requestedToken = &apiv2.Token{
			User:         tokenToUpdate.Token.User,
			ProjectRoles: req.ProjectRoles,
			TenantRoles:  req.TenantRoles,
			AdminRole:    req.AdminRole,
			InfraRole:    req.InfraRole,
			MachineRoles: req.MachineRoles,
			Permissions:  flattenPermissions(req.Permissions),
			TokenType:    apiv2.TokenType_TOKEN_TYPE_API,
		}
		userToken = &apiv2.Token{
			User:         sessionToken.User,
			ProjectRoles: projectsAndTenants.ProjectRoles,
			TenantRoles:  projectsAndTenants.TenantRoles,
			AdminRole:    nil,
			InfraRole:    sessionToken.InfraRole,
			MachineRoles: sessionToken.MachineRoles,
		}
	)

	if role, ok := t.hasAdminRole(projectsAndTenants); ok {
		userToken.AdminRole = role
	}

	// we first validate token permission elevation for the token used in the token update request,
	// which might be an API token with restricted permissions

	err = t.validateTokenRequest(ctx, sessionToken, requestedToken)
	if err != nil {
		return errorutil.NewPermissionDenied(err)
	}

	// now, we validate if the user is still permitted to update the token
	// doing this check is not strictly necessary because the resulting token would fail in the auther when being compared
	// to the actual user permissions, but it's nicer for the user to already prevent token update immediately in this place

	err = t.validateTokenRequest(ctx, userToken, requestedToken)
	if err != nil {
		return errorutil.NewPermissionDenied(err)
	}

	return nil
}

func (t *tokenRepository) validateDelete(ctx context.Context, req *api.TokenWithSecret) error {
	// token scope match is already checked before this func
	// apart from this a token can always be revoked
	return nil
}

func (t *tokenRepository) validateTokenRequest(ctx context.Context, currentToken *apiv2.Token, requestedToken *apiv2.Token) error {
	// Calculate the permission from the token in the request
	currentPermissions, err := t.authorizer.TokenPermissions(ctx, currentToken)
	if err != nil {
		return err
	}

	var (
		adminRole *apiv2.AdminRole
		infraRole *apiv2.InfraRole
	)

	// Ensure no unspecified roles are requested.
	if requestedToken.GetAdminRole() != apiv2.AdminRole_ADMIN_ROLE_UNSPECIFIED {
		adminRole = requestedToken.GetAdminRole().Enum()
	}
	if requestedToken.GetInfraRole() != apiv2.InfraRole_INFRA_ROLE_UNSPECIFIED {
		infraRole = requestedToken.GetInfraRole().Enum()
	}

	var (
		requestedTenants    = lo.Keys(requestedToken.GetTenantRoles())
		allowedTenants      = lo.Keys(currentToken.TenantRoles)
		forbiddenTenants, _ = lo.Difference(requestedTenants, allowedTenants)
	)

	if len(forbiddenTenants) > 0 && !slices.Contains(allowedTenants, "*") {
		return fmt.Errorf("requested tenant roles are not allowed: %v", forbiddenTenants)
	}

	for _, tr := range requestedToken.GetTenantRoles() {
		if tr == apiv2.TenantRole_TENANT_ROLE_UNSPECIFIED {
			return fmt.Errorf("requested tenant role: %q is not allowed", tr)
		}
	}

	var (
		requestedProjects    = lo.Keys(requestedToken.GetProjectRoles())
		allowedProjects      = lo.Keys(currentToken.ProjectRoles)
		forbiddenProjects, _ = lo.Difference(requestedProjects, allowedProjects)
	)

	if len(forbiddenProjects) > 0 && !slices.Contains(allowedProjects, "*") {
		return fmt.Errorf("requested project roles are not allowed: %v", forbiddenProjects)
	}

	for _, pr := range requestedToken.GetProjectRoles() {
		if pr == apiv2.ProjectRole_PROJECT_ROLE_UNSPECIFIED {
			return fmt.Errorf("requested project role: %q is not allowed", pr)
		}
	}

	// Ensure no permissions pointing to unknown methods are requested
	for _, permission := range requestedToken.GetPermissions() {
		for _, method := range permission.Methods {
			if _, found := permissions.GetServicePermissions().Methods[method]; !found {
				return fmt.Errorf("unknown method %q", method)
			}
		}
	}

	var (
		requestedMachines    = lo.Keys(requestedToken.GetMachineRoles())
		allowedMachines      = lo.Keys(currentToken.MachineRoles)
		forbiddenMachines, _ = lo.Difference(requestedMachines, allowedMachines)
	)

	if len(forbiddenMachines) > 0 && !slices.Contains(allowedMachines, "*") {
		return fmt.Errorf("requested machine roles are not allowed: %v", forbiddenMachines)
	}

	// Calculate the permission from the token request (either create/update or refresh)
	// and the methods which are coming from roles only.
	requestedPermissions, err := t.authorizer.TokenPermissions(ctx, &apiv2.Token{
		User:         currentToken.User,
		Permissions:  requestedToken.GetPermissions(),
		ProjectRoles: requestedToken.GetProjectRoles(),
		TenantRoles:  requestedToken.GetTenantRoles(),
		MachineRoles: requestedToken.GetMachineRoles(),
		AdminRole:    adminRole,
		InfraRole:    infraRole,
	})
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
		currentSubjects, ok := currentPermissions[method]
		if !ok {
			errMsg := fmt.Sprintf("the following method %q is not allowed", method)
			if len(subjects) > 0 {
				errMsg += fmt.Sprintf(" on any of the requested subjects: %s", subjects)
			}
			return errors.New(errMsg)
		}

		if _, ok := currentSubjects[request.AnySubject]; ok {
			continue
		}
		// It is possible to request any subjects to be able to have a token
		// which is able to make calls to projects which will be created in the future.
		// The actually possible subjects are calculated at request time.
		if _, ok := subjects[request.AnySubject]; ok {
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

func (t *tokenRepository) validatePermissions(ctx context.Context, perms []*apiv2.PermissionsByVisibility, projectsAndTenants *api.ProjectsAndTenants) error {
	type visibility string

	const (
		adminVisibility   visibility = "admin"
		infraVisibility   visibility = "infra"
		machineVisibility visibility = "machine"
		projectVisibility visibility = "project"
		publicVisibility  visibility = "public"
		selfVisibility    visibility = "self"
		tenantVisibility  visibility = "tenant"
	)

	errorVisibility := func(requestedVisibility visibility, method string) error {
		var actualVisibility visibility

		if _, ok := permissions.GetServicePermissions().Visibility.Admin[method]; ok {
			actualVisibility = adminVisibility
		}
		if _, ok := permissions.GetServicePermissions().Visibility.Infra[method]; ok {
			actualVisibility = infraVisibility
		}
		if _, ok := permissions.GetServicePermissions().Visibility.Machine[method]; ok {
			actualVisibility = machineVisibility
		}
		if _, ok := permissions.GetServicePermissions().Visibility.Project[method]; ok {
			actualVisibility = projectVisibility
		}
		if _, ok := permissions.GetServicePermissions().Visibility.Public[method]; ok {
			actualVisibility = publicVisibility
		}
		if _, ok := permissions.GetServicePermissions().Visibility.Self[method]; ok {
			actualVisibility = selfVisibility
		}
		if _, ok := permissions.GetServicePermissions().Visibility.Tenant[method]; ok {
			actualVisibility = tenantVisibility
		}

		if actualVisibility == "" {
			return fmt.Errorf("requested method %q is not contained in the api", method)
		}

		return fmt.Errorf("requested method %q is of visibility %q, not of visibility %q", method, actualVisibility, requestedVisibility)
	}

	for _, p := range perms {
		switch p.Visibility.(type) {
		case *apiv2.PermissionsByVisibility_Admin:
			if _, has := t.hasAdminRole(projectsAndTenants); !has {
				return fmt.Errorf("admin api permission are only allowed for admins")
			}

			for _, method := range p.GetAdmin().GetMethods() {
				if _, ok := permissions.GetServicePermissions().Visibility.Admin[method]; !ok {
					return errorVisibility(adminVisibility, method)
				}
			}

		case *apiv2.PermissionsByVisibility_Infra:
			for _, method := range p.GetInfra().GetMethods() {
				if _, ok := permissions.GetServicePermissions().Visibility.Infra[method]; !ok {
					return errorVisibility(infraVisibility, method)
				}
			}

		case *apiv2.PermissionsByVisibility_Machine:
			// every machine is allowed to be scoped, so no check necessary
			// if p.GetMachine().Uuid != request.AnySubject {}

			for _, method := range p.GetMachine().GetMethods() {
				if _, ok := permissions.GetServicePermissions().Visibility.Machine[method]; !ok {
					return errorVisibility(machineVisibility, method)
				}
			}

		case *apiv2.PermissionsByVisibility_Project:
			if project := p.GetProject().GetProject(); project != request.AnySubject {
				if _, has := t.hasAdminRole(projectsAndTenants); has {
					if _, err := t.s.UnscopedProject().Get(ctx, project); err != nil {
						if errorutil.IsNotFound(err) {
							return fmt.Errorf("requesting method for project %q but this does not exist in the database", project)
						}

						return err
					}
				} else {
					if _, ok := projectsAndTenants.ProjectRoles[project]; !ok {
						return fmt.Errorf("requesting method for project %q but available projects are: %v", project, lo.Keys(projectsAndTenants.ProjectRoles))
					}
				}
			}

			for _, method := range p.GetProject().GetMethods() {
				if _, ok := permissions.GetServicePermissions().Visibility.Project[method]; !ok {
					return errorVisibility(projectVisibility, method)
				}
			}

		case *apiv2.PermissionsByVisibility_Public:
			for _, method := range p.GetPublic().GetMethods() {
				if _, ok := permissions.GetServicePermissions().Visibility.Public[method]; !ok {
					return errorVisibility(publicVisibility, method)
				}
			}

		case *apiv2.PermissionsByVisibility_Self:
			for _, method := range p.GetSelf().GetMethods() {
				if _, ok := permissions.GetServicePermissions().Visibility.Self[method]; !ok {
					return errorVisibility(selfVisibility, method)
				}
			}

		case *apiv2.PermissionsByVisibility_Tenant:
			if login := p.GetTenant().GetLogin(); login != request.AnySubject {
				if _, has := t.hasAdminRole(projectsAndTenants); has {
					if _, err := t.s.Tenant().Get(ctx, login); err != nil {
						if errorutil.IsNotFound(err) {
							return fmt.Errorf("requesting method for tenant %q but this does not exist in the database", login)
						}

						return err
					}
				} else {
					if _, ok := projectsAndTenants.TenantRoles[login]; !ok {
						return fmt.Errorf("requesting method for tenant %q but available tenants are: %v", login, lo.Keys(projectsAndTenants.TenantRoles))
					}
				}
			}

			for _, method := range p.GetTenant().GetMethods() {
				if _, ok := permissions.GetServicePermissions().Visibility.Tenant[method]; !ok {
					return errorVisibility(tenantVisibility, method)
				}
			}

		}
	}

	return nil
}

func (t *tokenRepository) hasAdminRole(projectsAndTenants *api.ProjectsAndTenants) (*apiv2.AdminRole, bool) {
	if role, ok := projectsAndTenants.TenantRoles[t.s.providerTenant]; ok {
		switch role {
		case apiv2.TenantRole_TENANT_ROLE_OWNER:
			return apiv2.AdminRole_ADMIN_ROLE_EDITOR.Enum(), true
		case apiv2.TenantRole_TENANT_ROLE_EDITOR, apiv2.TenantRole_TENANT_ROLE_VIEWER:
			return apiv2.AdminRole_ADMIN_ROLE_VIEWER.Enum(), true
		}
	}

	return nil, false
}

func (t *tokenRepository) isAdminRoleRequestAllowed(projectsAndTenants *api.ProjectsAndTenants, requestedRole *apiv2.AdminRole) error {
	if requestedRole == nil {
		return nil
	}

	role, ok := t.hasAdminRole(projectsAndTenants)
	if !ok {
		return fmt.Errorf("requested adminrole %q is not allowed because you are not member of provider tenant", *requestedRole)
	}

	if *role == apiv2.AdminRole_ADMIN_ROLE_VIEWER && *requestedRole == apiv2.AdminRole_ADMIN_ROLE_EDITOR {
		return fmt.Errorf("your provider tenant membership only allows %q, but you requested %q", *role, *requestedRole)
	}

	return nil
}
