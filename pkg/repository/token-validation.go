package repository

import (
	"context"
	"errors"
	"fmt"
	"slices"

	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/api/go/permissions"
	"github.com/metal-stack/metal-apiserver/pkg/certs"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	"github.com/metal-stack/metal-apiserver/pkg/repository/api"
	"github.com/metal-stack/metal-apiserver/pkg/request"
	"github.com/metal-stack/metal-apiserver/pkg/token"
)

func (t *tokenRepository) validateCreate(ctx context.Context, req *adminv2.TokenServiceCreateRequest) error {
	tok, ok := token.TokenFromContext(ctx)
	if !ok || tok == nil {
		return errorutil.Unauthenticated("no token found in request")
	}

	if req.TokenCreateRequest.Expires.AsDuration() > certs.MaxTokenExpiration {
		return fmt.Errorf("requested expiration duration: %q exceeds max expiration: %q", req.TokenCreateRequest.Expires.AsDuration(), certs.MaxTokenExpiration)
	}

	if slices.Contains(t.s.adminSubjects, tok.User) {
		if tok.AdminRole == nil || *tok.AdminRole == apiv2.AdminRole_ADMIN_ROLE_UNSPECIFIED {
			tok.AdminRole = req.TokenCreateRequest.AdminRole
			tok.TokenType = apiv2.TokenType_TOKEN_TYPE_API
		}

		t.s.log.Debug("user is listed in adminsubjects", "user", tok.User, "new token.adminrole", tok.AdminRole)
	}

	// we first validate token permission elevation for the token used in the token create request,
	// which might be an API token with restricted permissions

	err := t.validateTokenRequest(ctx, tok, req.TokenCreateRequest)
	if err != nil {
		return errorutil.NewPermissionDenied(err)
	}

	// now, we validate if the user is still permitted to create such a token
	// doing this check is not strictly necessary because the resulting token would fail in the auther when being compared
	// to the actual user permissions, but it's nicer for the user to already prevent token creation immediately in this place

	projectsAndTenants, err := t.patg(ctx, tok.GetUser())
	if err != nil {
		return errorutil.NewInternal(err)
	}

	fullUserToken := &apiv2.Token{
		User:         tok.User,
		ProjectRoles: projectsAndTenants.ProjectRoles,
		TenantRoles:  projectsAndTenants.TenantRoles,
		AdminRole:    nil,
		InfraRole:    tok.InfraRole,
	}

	if !slices.Contains(t.s.adminSubjects, tok.User) && req.User != nil {
		return errorutil.PermissionDenied("only admins can specify token user")
	}

	if slices.Contains(t.s.adminSubjects, tok.User) {
		fullUserToken.AdminRole = apiv2.AdminRole_ADMIN_ROLE_EDITOR.Enum()
	}

	err = t.validateTokenRequest(ctx, fullUserToken, req.TokenCreateRequest)
	if err != nil {
		return errorutil.NewPermissionDenied(err)
	}

	return nil
}

func (t *tokenRepository) validateUpdate(ctx context.Context, req *apiv2.TokenServiceUpdateRequest, tokenToUpdate *api.TokenWithSecret) error {
	tok, ok := token.TokenFromContext(ctx)
	if !ok || tok == nil {
		return errorutil.Unauthenticated("no token found in request")
	}

	if tokenToUpdate.Token.TokenType != apiv2.TokenType_TOKEN_TYPE_API {
		return errorutil.FailedPrecondition("only updating API tokens is currently supported")
	}

	// we first validate token permission elevation for the token used in the token update request,
	// which might be an API token with restricted permissions

	err := t.validateTokenRequest(ctx, tok, req)
	if err != nil {
		return errorutil.NewPermissionDenied(err)
	}

	// now, we validate if the user is still permitted to update the token
	// doing this check is not strictly necessary because the resulting token would fail in the auther when being compared
	// to the actual user permissions, but it's nicer for the user to already prevent token update immediately in this place

	projectsAndTenants, err := t.patg(ctx, tok.GetUser())
	if err != nil {
		return errorutil.NewInternal(err)
	}

	fullUserToken := &apiv2.Token{
		User:         tok.GetUser(),
		ProjectRoles: projectsAndTenants.ProjectRoles,
		TenantRoles:  projectsAndTenants.TenantRoles,
		AdminRole:    nil,
		InfraRole:    tok.InfraRole,
	}

	if slices.Contains(t.s.adminSubjects, tok.GetUser()) {
		fullUserToken.AdminRole = apiv2.AdminRole_ADMIN_ROLE_EDITOR.Enum()
	}

	err = t.validateTokenRequest(ctx, fullUserToken, req)
	if err != nil {
		return errorutil.NewPermissionDenied(err)
	}

	return nil
}

func (t *tokenRepository) validateDelete(ctx context.Context, req *api.TokenWithSecret) error {
	return nil
}

type tokenRequest interface {
	GetPermissions() []*apiv2.MethodPermission
	GetProjectRoles() map[string]apiv2.ProjectRole
	GetTenantRoles() map[string]apiv2.TenantRole
	GetAdminRole() apiv2.AdminRole
	GetInfraRole() apiv2.InfraRole
}

func (t *tokenRepository) validateTokenRequest(ctx context.Context, currentToken *apiv2.Token, req tokenRequest) error {
	authorizer := request.NewAuthorizer(t.s.log, func(ctx context.Context, userId string) (*api.ProjectsAndTenants, error) {
		return t.patg(ctx, userId)
	})

	// Calculate the permission from the token in the request
	currentPermissions, err := authorizer.TokenPermissions(ctx, currentToken)
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

	for _, tr := range req.GetTenantRoles() {
		if tr == apiv2.TenantRole_TENANT_ROLE_UNSPECIFIED {
			return fmt.Errorf("requested tenant role: %q is not allowed", tr)
		}
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

	// Calculate the permission from the token request (either create/update or refresh)
	// and the methods which are coming from roles only.
	requestedPermissions, err := authorizer.TokenPermissions(ctx, &apiv2.Token{
		User:         currentToken.User,
		Permissions:  req.GetPermissions(),
		ProjectRoles: req.GetProjectRoles(),
		TenantRoles:  req.GetTenantRoles(),
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
