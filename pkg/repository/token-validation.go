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

	user := t.scope.user

	tok, ok := token.TokenFromContext(ctx)
	if !ok || tok == nil {
		return errorutil.Unauthenticated("no token found in request")
	}

	switch tok.TokenType {
	case apiv2.TokenType_TOKEN_TYPE_API, apiv2.TokenType_TOKEN_TYPE_USER:
		// noop
	default:
		return errorutil.FailedPrecondition("invalid token type for token creation: %q", tok.TokenType)
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
		if tok.AdminRole == nil || *tok.AdminRole == apiv2.AdminRole_ADMIN_ROLE_UNSPECIFIED {
			if err := t.isAdminRoleRequestAllowed(projectsAndTenants, req.TokenCreateRequest.AdminRole); err != nil {
				return errorutil.NewPermissionDenied(err)
			}
			tok.AdminRole = req.TokenCreateRequest.AdminRole
			tok.TokenType = apiv2.TokenType_TOKEN_TYPE_API
		}

		adminRole = *role
		isAdmin = true

		t.s.log.Debug("user is member of the provider-tenant", "admin-role", tok.AdminRole)
	}

	if !isAdmin && req.User != nil {
		return errorutil.PermissionDenied("only admins can specify token user")
	}

	// we first validate token permission elevation for the token used in the token create request,
	// which might be an API token with restricted permissions

	err = t.validateTokenRequest(ctx, tok, req.TokenCreateRequest)
	if err != nil {
		return errorutil.NewPermissionDenied(err)
	}

	// now, we validate if the user is still permitted to create such a token
	// doing this check is not strictly necessary because the resulting token would fail in the auther when being compared
	// to the actual user permissions, but it's nicer for the user to already prevent token creation immediately in this place

	fullUserToken := &apiv2.Token{
		User:         user,
		ProjectRoles: projectsAndTenants.ProjectRoles,
		TenantRoles:  projectsAndTenants.TenantRoles,
		AdminRole:    nil,
		InfraRole:    tok.InfraRole,
		MachineRoles: tok.MachineRoles,
	}

	if isAdmin {
		fullUserToken.AdminRole = &adminRole
	}

	err = t.validateTokenRequest(ctx, fullUserToken, req.TokenCreateRequest)
	if err != nil {
		return errorutil.NewPermissionDenied(err)
	}

	return nil
}

func (t *tokenRepository) validateUpdate(ctx context.Context, req *apiv2.TokenServiceUpdateRequest, tokenToUpdate *api.TokenWithSecret) error {
	panic("unimplemented")
}

func (t *tokenRepository) validateDelete(ctx context.Context, req *api.TokenWithSecret) error {
	// token scope match is already checked before this func
	// apart from this a token can always be revoked
	return nil
}

type tokenRequest interface {
	GetPermissions() []*apiv2.MethodPermission
	GetProjectRoles() map[string]apiv2.ProjectRole
	GetTenantRoles() map[string]apiv2.TenantRole
	GetMachineRoles() map[string]apiv2.MachineRole
	GetAdminRole() apiv2.AdminRole
	GetInfraRole() apiv2.InfraRole
}

func (t *tokenRepository) validateTokenRequest(ctx context.Context, currentToken *apiv2.Token, req tokenRequest) error {
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
	if req.GetAdminRole() != apiv2.AdminRole_ADMIN_ROLE_UNSPECIFIED {
		adminRole = req.GetAdminRole().Enum()
	}
	if req.GetInfraRole() != apiv2.InfraRole_INFRA_ROLE_UNSPECIFIED {
		infraRole = req.GetInfraRole().Enum()
	}

	var (
		requestedTenants    = lo.Keys(req.GetTenantRoles())
		allowedTenants      = lo.Keys(currentToken.TenantRoles)
		forbiddenTenants, _ = lo.Difference(requestedTenants, allowedTenants)
	)

	if len(forbiddenTenants) > 0 && !slices.Contains(allowedTenants, "*") {
		return fmt.Errorf("requested tenant roles are not allowed: %v", forbiddenTenants)
	}

	for _, tr := range req.GetTenantRoles() {
		if tr == apiv2.TenantRole_TENANT_ROLE_UNSPECIFIED {
			return fmt.Errorf("requested tenant role: %q is not allowed", tr)
		}
	}

	var (
		requestedProjects    = lo.Keys(req.GetProjectRoles())
		allowedProjects      = lo.Keys(currentToken.ProjectRoles)
		forbiddenProjects, _ = lo.Difference(requestedProjects, allowedProjects)
	)

	if len(forbiddenProjects) > 0 && !slices.Contains(allowedProjects, "*") {
		return fmt.Errorf("requested project roles are not allowed: %v", forbiddenProjects)
	}

	for _, pr := range req.GetProjectRoles() {
		if pr == apiv2.ProjectRole_PROJECT_ROLE_UNSPECIFIED {
			return fmt.Errorf("requested project role: %q is not allowed", pr)
		}
	}

	// Ensure no permissions pointing to unknown methods are requested
	for _, permission := range req.GetPermissions() {
		for _, method := range permission.Methods {
			if _, found := permissions.GetServicePermissions().Methods[method]; !found {
				return fmt.Errorf("unknown method %q", method)
			}
		}
	}

	var (
		requestedMachines    = lo.Keys(req.GetMachineRoles())
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
		Permissions:  req.GetPermissions(),
		ProjectRoles: req.GetProjectRoles(),
		TenantRoles:  req.GetTenantRoles(),
		MachineRoles: req.GetMachineRoles(),
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
