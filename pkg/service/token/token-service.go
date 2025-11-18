package token

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"

	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/api/go/metalstack/api/v2/apiv2connect"
	"github.com/metal-stack/api/go/permissions"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"

	"github.com/metal-stack/metal-apiserver/pkg/certs"
	"github.com/metal-stack/metal-apiserver/pkg/repository"
	"github.com/metal-stack/metal-apiserver/pkg/service/method"
	tokenutil "github.com/metal-stack/metal-apiserver/pkg/token"
	"github.com/metal-stack/metal-lib/pkg/pointer"
)

type Config struct {
	Log        *slog.Logger
	TokenStore tokenutil.TokenStore
	CertStore  certs.CertStore
	Repo       *repository.Store

	// AdminSubjects are the subjects for which the token service allows the creation of admin api tokens
	AdminSubjects []string

	// Issuer to sign the JWT Token with
	Issuer string
}

type tokenService struct {
	issuer             string
	adminSubjects      []string
	tokens             tokenutil.TokenStore
	certs              certs.CertStore
	log                *slog.Logger
	servicePermissions *permissions.ServicePermissions

	projectsAndTenantsGetter func(ctx context.Context, userId string) (*repository.ProjectsAndTenants, error)
}

type TokenService interface {
	apiv2connect.TokenServiceHandler
	CreateUserTokenWithoutPermissionCheck(ctx context.Context, subject string, expiration *time.Duration) (*apiv2.TokenServiceCreateResponse, error)
	CreateApiTokenWithoutPermissionCheck(ctx context.Context, subject string, rq *apiv2.TokenServiceCreateRequest) (*apiv2.TokenServiceCreateResponse, error)
	CreateTokenForUser(ctx context.Context, user *string, req *apiv2.TokenServiceCreateRequest) (*apiv2.TokenServiceCreateResponse, error)
}

func New(c Config) TokenService {
	servicePermissions := permissions.GetServicePermissions()

	return &tokenService{
		tokens:             c.TokenStore,
		certs:              c.CertStore,
		issuer:             c.Issuer,
		log:                c.Log.WithGroup("tokenService"),
		servicePermissions: servicePermissions,
		adminSubjects:      c.AdminSubjects,

		projectsAndTenantsGetter: func(ctx context.Context, userId string) (*repository.ProjectsAndTenants, error) {
			return c.Repo.UnscopedProject().AdditionalMethods().GetProjectsAndTenants(ctx, userId)
		},
	}
}

// CreateUserTokenWithoutPermissionCheck is only called from the auth service during login through console
// No validation against requested roles and permissions is required and implemented here
func (t *tokenService) CreateUserTokenWithoutPermissionCheck(ctx context.Context, subject string, expiration *time.Duration) (*apiv2.TokenServiceCreateResponse, error) {
	expires := tokenutil.DefaultExpiration
	if expiration != nil {
		expires = *expiration
	}

	privateKey, err := t.certs.LatestPrivate(ctx)
	if err != nil {
		return nil, errorutil.Internal("unable to fetch signing certificate: %w", err)
	}

	secret, token, err := tokenutil.NewJWT(apiv2.TokenType_TOKEN_TYPE_USER, subject, t.issuer, expires, privateKey)
	if err != nil {
		return nil, errorutil.Internal("unable to create console token: %w", err)
	}

	err = t.tokens.Set(ctx, token)
	if err != nil {
		return nil, errorutil.NewInternal(err)
	}

	return &apiv2.TokenServiceCreateResponse{
		Token:  token,
		Secret: secret,
	}, nil
}

// CreateApiTokenWithoutPermissionCheck is only called from the api-server command line interface
// No validation against requested roles and permissions is required and implemented here
func (t *tokenService) CreateApiTokenWithoutPermissionCheck(ctx context.Context, subject string, req *apiv2.TokenServiceCreateRequest) (*apiv2.TokenServiceCreateResponse, error) {
	expires := tokenutil.DefaultExpiration
	if req.Expires != nil {
		expires = req.Expires.AsDuration()
	}

	privateKey, err := t.certs.LatestPrivate(ctx)
	if err != nil {
		return nil, errorutil.NewInternal(err)
	}

	secret, token, err := tokenutil.NewJWT(apiv2.TokenType_TOKEN_TYPE_API, subject, t.issuer, expires, privateKey)
	if err != nil {
		return nil, errorutil.NewInternal(err)
	}

	token.Description = req.Description
	token.Permissions = req.Permissions
	token.ProjectRoles = req.ProjectRoles
	token.TenantRoles = req.TenantRoles
	token.AdminRole = req.AdminRole

	err = t.tokens.Set(ctx, token)
	if err != nil {
		return nil, errorutil.NewInternal(err)
	}

	return &apiv2.TokenServiceCreateResponse{
		Token:  token,
		Secret: secret,
	}, nil
}

// Get returns the token by a given uuid for the user who requests it.
func (t *tokenService) Get(ctx context.Context, rq *apiv2.TokenServiceGetRequest) (*apiv2.TokenServiceGetResponse, error) {
	token, ok := tokenutil.TokenFromContext(ctx)
	if !ok || token == nil {
		return nil, errorutil.Unauthenticated("no token found in request")
	}

	res, err := t.tokens.Get(ctx, token.User, rq.Uuid)
	if err != nil {
		if errors.Is(err, tokenutil.ErrTokenNotFound) {
			return nil, errorutil.NotFound("token not found")
		}
		return nil, errorutil.NewInternal(err)
	}

	return &apiv2.TokenServiceGetResponse{
		Token: res,
	}, nil
}

// Update updates a given token of a user.
// We need to prevent a user from elevating permissions here.
func (t *tokenService) Update(ctx context.Context, req *apiv2.TokenServiceUpdateRequest) (*apiv2.TokenServiceUpdateResponse, error) {
	token, ok := tokenutil.TokenFromContext(ctx)
	if !ok || token == nil {
		return nil, errorutil.Unauthenticated("no token found in request")
	}

	// we first validate token permission elevation for the token used in the token update request,
	// which might be an API token with restricted permissions

	createRequest := &apiv2.TokenServiceCreateRequest{
		Permissions:  req.Permissions,
		ProjectRoles: req.ProjectRoles,
		TenantRoles:  req.TenantRoles,
		AdminRole:    req.AdminRole,
	}

	err := validateTokenCreate(token, createRequest, t.servicePermissions, t.adminSubjects)
	if err != nil {
		return nil, errorutil.NewPermissionDenied(err)
	}

	// now, we validate if the user is still permitted to update the token
	// doing this check is not strictly necessary because the resulting token would fail in the opa auther when being compared
	// to the actual user permissions, but it's nicer for the user to already prevent token update immediately in this place

	projectsAndTenants, err := t.projectsAndTenantsGetter(ctx, token.GetUser())
	if err != nil {
		return nil, errorutil.NewInternal(err)
	}
	fullUserToken := &apiv2.Token{
		User:         token.User,
		ProjectRoles: projectsAndTenants.ProjectRoles,
		TenantRoles:  projectsAndTenants.TenantRoles,
		AdminRole:    nil,
	}
	if slices.Contains(t.adminSubjects, token.User) {
		fullUserToken.AdminRole = apiv2.AdminRole_ADMIN_ROLE_EDITOR.Enum()
	}
	err = validateTokenCreate(fullUserToken, createRequest, t.servicePermissions, t.adminSubjects)
	if err != nil {
		return nil, errorutil.PermissionDenied("outdated token: %w", err)
	}

	// now follows the update

	tokenToUpdate, err := t.tokens.Get(ctx, token.User, req.Uuid)
	if err != nil {
		if errors.Is(err, tokenutil.ErrTokenNotFound) {
			return nil, errorutil.NotFound("token not found")
		}
		return nil, errorutil.NewInternal(err)
	}

	if tokenToUpdate.TokenType != apiv2.TokenType_TOKEN_TYPE_API {
		return nil, errorutil.FailedPrecondition("only updating API tokens is currently supported")
	}

	if req.Description != nil {
		tokenToUpdate.Description = *req.Description
	}
	if req.AdminRole != nil {
		if *req.AdminRole == apiv2.AdminRole_ADMIN_ROLE_UNSPECIFIED {
			tokenToUpdate.AdminRole = nil
		} else {
			tokenToUpdate.AdminRole = req.AdminRole
		}
	}

	tokenToUpdate.Permissions = req.Permissions
	tokenToUpdate.ProjectRoles = req.ProjectRoles
	tokenToUpdate.TenantRoles = req.TenantRoles

	err = t.tokens.Set(ctx, tokenToUpdate)
	if err != nil {
		return nil, errorutil.NewInternal(err)
	}

	return &apiv2.TokenServiceUpdateResponse{
		Token: tokenToUpdate,
	}, nil
}

// Create is called by users to issue new API tokens. This can be done from console tokens but also from other API tokens which have the permission to call token create.
// We need to prevent a user from elevating permissions here.
func (t *tokenService) Create(ctx context.Context, req *apiv2.TokenServiceCreateRequest) (*apiv2.TokenServiceCreateResponse, error) {
	token, ok := tokenutil.TokenFromContext(ctx)
	if !ok || token == nil {
		return nil, errorutil.Unauthenticated("no token found in request")
	}
	return t.CreateTokenForUser(ctx, &token.User, req)
}

func (t *tokenService) CreateTokenForUser(ctx context.Context, user *string, req *apiv2.TokenServiceCreateRequest) (*apiv2.TokenServiceCreateResponse, error) {
	token, ok := tokenutil.TokenFromContext(ctx)
	if !ok || token == nil {
		return nil, errorutil.Unauthenticated("no token found in request")
	}

	// we first validate token permission elevation for the token used in the token create request,
	// which might be an API token with restricted permissions

	err := validateTokenCreate(token, req, t.servicePermissions, t.adminSubjects)
	if err != nil {
		return nil, errorutil.NewPermissionDenied(err)
	}

	// now, we validate if the user is still permitted to create such a token
	// doing this check is not strictly necessary because the resulting token would fail in the opa auther when being compared
	// to the actual user permissions, but it's nicer for the user to already prevent token creation immediately in this place

	projectsAndTenants, err := t.projectsAndTenantsGetter(ctx, token.GetUser())
	if err != nil {
		return nil, errorutil.NewInternal(err)
	}
	fullUserToken := &apiv2.Token{
		User:         token.User,
		ProjectRoles: projectsAndTenants.ProjectRoles,
		TenantRoles:  projectsAndTenants.TenantRoles,
		AdminRole:    nil,
	}

	tokenUser := token.GetUser()

	if slices.Contains(t.adminSubjects, token.User) {
		fullUserToken.AdminRole = apiv2.AdminRole_ADMIN_ROLE_EDITOR.Enum()
		if user != nil {
			tokenUser = *user
		}
	} else {
		if user != nil && token.GetUser() != *user {
			return nil, errorutil.PermissionDenied("only admins can specify token user")
		}
	}

	err = validateTokenCreate(fullUserToken, req, t.servicePermissions, t.adminSubjects)
	if err != nil {
		return nil, errorutil.PermissionDenied("outdated token: %w", err)
	}

	privateKey, err := t.certs.LatestPrivate(ctx)
	if err != nil {
		return nil, errorutil.NewInternal(err)
	}

	secret, token, err := tokenutil.NewJWT(apiv2.TokenType_TOKEN_TYPE_API, tokenUser, t.issuer, req.Expires.AsDuration(), privateKey)
	if err != nil {
		return nil, errorutil.NewInternal(err)
	}

	token.Description = req.Description
	token.Permissions = req.Permissions
	token.ProjectRoles = req.ProjectRoles
	token.TenantRoles = req.TenantRoles
	token.AdminRole = req.AdminRole

	err = t.tokens.Set(ctx, token)
	if err != nil {
		return nil, errorutil.NewInternal(err)
	}

	resp := &apiv2.TokenServiceCreateResponse{
		Token:  token,
		Secret: secret,
	}

	return resp, nil
}

// List lists the tokens of a specific user.
func (t *tokenService) List(ctx context.Context, _ *apiv2.TokenServiceListRequest) (*apiv2.TokenServiceListResponse, error) {
	token, ok := tokenutil.TokenFromContext(ctx)
	if !ok || token == nil {
		return nil, errorutil.Unauthenticated("no token found in request")
	}

	tokens, err := t.tokens.List(ctx, token.User)
	if err != nil {
		return nil, errorutil.NewInternal(err)
	}

	return &apiv2.TokenServiceListResponse{
		Tokens: tokens,
	}, nil
}

// Revoke revokes a token of a given user and token ID.
func (t *tokenService) Revoke(ctx context.Context, rq *apiv2.TokenServiceRevokeRequest) (*apiv2.TokenServiceRevokeResponse, error) {
	token, ok := tokenutil.TokenFromContext(ctx)
	if !ok || token == nil {
		return nil, errorutil.Unauthenticated("no token found in request")
	}

	err := t.tokens.Revoke(ctx, token.User, rq.Uuid)
	if err != nil {
		return nil, errorutil.NewInternal(err)
	}

	return &apiv2.TokenServiceRevokeResponse{}, nil
}

func (t *tokenService) Refresh(ctx context.Context, _ *apiv2.TokenServiceRefreshRequest) (*apiv2.TokenServiceRefreshResponse, error) {
	token, ok := tokenutil.TokenFromContext(ctx)
	if !ok || token == nil {
		return nil, errorutil.Unauthenticated("no token found in request")
	}

	oldtoken, err := t.tokens.Get(ctx, token.User, token.Uuid)
	if err != nil {
		if errors.Is(err, tokenutil.ErrTokenNotFound) {
			return nil, errorutil.NotFound("token not found")
		}
		return nil, errorutil.NewInternal(err)
	}

	// we first copy the token permission from the old token
	createRequest := &apiv2.TokenServiceCreateRequest{
		Permissions:  oldtoken.Permissions,
		ProjectRoles: oldtoken.ProjectRoles,
		TenantRoles:  oldtoken.TenantRoles,
		AdminRole:    oldtoken.AdminRole,
	}

	err = validateTokenCreate(token, createRequest, t.servicePermissions, t.adminSubjects)
	if err != nil {
		return nil, errorutil.NewPermissionDenied(err)
	}

	// now, we validate if the user is still permitted to refresh the token
	// doing this check is not strictly necessary because the resulting token would fail in the opa auther when being compared
	// to the actual user permissions, but it's nicer for the user to already prevent token update immediately in this place

	projectsAndTenants, err := t.projectsAndTenantsGetter(ctx, token.GetUser())
	if err != nil {
		return nil, errorutil.NewInternal(err)
	}
	fullUserToken := &apiv2.Token{
		User:         token.User,
		ProjectRoles: projectsAndTenants.ProjectRoles,
		TenantRoles:  projectsAndTenants.TenantRoles,
		AdminRole:    nil,
	}
	if slices.Contains(t.adminSubjects, token.User) {
		fullUserToken.AdminRole = apiv2.AdminRole_ADMIN_ROLE_EDITOR.Enum()
	}
	err = validateTokenCreate(fullUserToken, createRequest, t.servicePermissions, t.adminSubjects)
	if err != nil {
		return nil, errorutil.PermissionDenied("outdated token: %w", err)
	}

	// now follows the refresh, aka create a new token

	privateKey, err := t.certs.LatestPrivate(ctx)
	if err != nil {
		return nil, errorutil.NewInternal(err)
	}

	// New duration is calculated from the old token
	exp := oldtoken.Expires.AsTime().Sub(oldtoken.IssuedAt.AsTime())

	secret, newToken, err := tokenutil.NewJWT(apiv2.TokenType_TOKEN_TYPE_API, token.GetUser(), t.issuer, exp, privateKey)
	if err != nil {
		return nil, errorutil.NewInternal(err)
	}

	newToken.Description = oldtoken.Description
	newToken.Permissions = oldtoken.Permissions
	newToken.ProjectRoles = oldtoken.ProjectRoles
	newToken.TenantRoles = oldtoken.TenantRoles
	newToken.AdminRole = oldtoken.AdminRole

	err = t.tokens.Set(ctx, newToken)
	if err != nil {
		return nil, errorutil.NewInternal(err)
	}

	return &apiv2.TokenServiceRefreshResponse{
		Token:  newToken,
		Secret: secret,
	}, nil
}

// FIXME check if this logic can be refactored using the existing logic in tokenpermissions.go
func validateTokenCreate(currentToken *apiv2.Token, req *apiv2.TokenServiceCreateRequest, servicePermissions *permissions.ServicePermissions, adminIDs []string) error {
	var (
		tokenPermissionsMap = method.PermissionsBySubject(currentToken)
		tokenProjectRoles   = currentToken.ProjectRoles
		tokenTenantRoles    = currentToken.TenantRoles

		requestedProjectRoles = req.ProjectRoles
		requestedTenantRoles  = req.TenantRoles
		requestedAdminRole    = req.AdminRole
		requestedPermissions  = req.Permissions
	)

	// First check all requested permissions are defined in servicePermissions

	// if token permissions are empty fill them from token roles
	// methods restrictions are inherited from the user role for every project
	if len(tokenPermissionsMap) == 0 {
		tokenPermissionsMap = method.AllowedMethodsFromRoles(servicePermissions, currentToken)
	}

	for _, reqSubjectPermission := range requestedPermissions {
		reqSubjectID := reqSubjectPermission.Subject
		if reqSubjectID == "*" {
			continue
		}

		// If admin skip this checks
		// FIXME is this correct
		if slices.Contains(adminIDs, currentToken.User) && currentToken.AdminRole != nil {
			continue
		}

		// Check if the requested subject, e.g. project or organization can be accessed
		tokenProjectPermissions, ok := tokenPermissionsMap[reqSubjectID]
		if !ok {
			return fmt.Errorf("requested subject: %q access is not allowed", reqSubjectID)
		}

		for _, reqMethod := range reqSubjectPermission.Methods {
			// Check if the requested permissions are part of all available methods
			if !servicePermissions.Methods[reqMethod] {
				return fmt.Errorf("requested method: %q is not allowed", reqMethod)
			}

			// Check if the requested permissions are part of the token
			if !slices.Contains(tokenProjectPermissions.Methods, reqMethod) {
				return fmt.Errorf("requested method: %q is not allowed for subject: %q", reqMethod, reqSubjectID)
			}
		}
	}

	// derive if a user has admin privileges in case he belongs to a certain id, which was preconfigured in the deployment
	for _, subject := range adminIDs {
		if currentToken.User != subject {
			// we exclude invited members of an admin tenant
			continue
		}

		role, ok := currentToken.TenantRoles[subject]
		if !ok {
			continue
		}

		switch role {
		case apiv2.TenantRole_TENANT_ROLE_EDITOR, apiv2.TenantRole_TENANT_ROLE_OWNER:
			currentToken.AdminRole = pointer.Pointer(apiv2.AdminRole_ADMIN_ROLE_EDITOR)
		case apiv2.TenantRole_TENANT_ROLE_VIEWER:
			currentToken.AdminRole = pointer.Pointer(apiv2.AdminRole_ADMIN_ROLE_VIEWER)
		case apiv2.TenantRole_TENANT_ROLE_GUEST, apiv2.TenantRole_TENANT_ROLE_UNSPECIFIED:
			// noop
		default:
			// noop
		}
	}

	// Check if requested roles do not exceed existing roles

	// first check if the requested role subject is part of the token subject
	for reqProjectID, reqRole := range requestedProjectRoles {
		if reqRole == apiv2.ProjectRole_PROJECT_ROLE_UNSPECIFIED {
			return fmt.Errorf("requested project role: %q is not allowed", reqRole.String())
		}

		projectRole, ok := tokenProjectRoles[reqProjectID]
		if !ok {
			return fmt.Errorf("requested project: %q is not allowed", reqProjectID)
		}

		// OWNER has the lowest index
		if reqRole < projectRole {
			return fmt.Errorf("requested role: %q is higher than allowed role: %q", reqRole.String(), projectRole.String())
		}
	}

	for reqTenantID, reqRole := range requestedTenantRoles {
		if reqRole == apiv2.TenantRole_TENANT_ROLE_UNSPECIFIED {
			return fmt.Errorf("requested tenant role: %q is not allowed", reqRole.String())
		}

		tenantRole, ok := tokenTenantRoles[reqTenantID]
		if !ok {
			return fmt.Errorf("requested tenant: %q is not allowed", reqTenantID)
		}

		// OWNER has the lowest index
		if reqRole < tenantRole {
			return fmt.Errorf("requested role: %q is higher than allowed role: %q", reqRole.String(), tenantRole.String())
		}
	}

	if requestedAdminRole != nil {
		if currentToken.AdminRole == nil {
			return fmt.Errorf("requested admin role: %q is not allowed", requestedAdminRole.String())
		}

		if *requestedAdminRole == apiv2.AdminRole_ADMIN_ROLE_UNSPECIFIED {
			return fmt.Errorf("requested admin role: %q is not allowed", requestedAdminRole.String())
		}

		if *requestedAdminRole < *currentToken.AdminRole {
			return fmt.Errorf("requested admin role: %q is not allowed", requestedAdminRole.String())
		}
	}

	// Validate Expire
	if req.Expires.AsDuration() > tokenutil.MaxExpiration {
		return fmt.Errorf("requested expiration duration: %q exceeds max expiration: %q", req.Expires.AsDuration(), tokenutil.MaxExpiration)
	}

	return nil
}
