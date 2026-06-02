package token

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"

	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/api/go/permissions"
	"github.com/metal-stack/metal-apiserver/pkg/certs"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	"github.com/metal-stack/metal-apiserver/pkg/repository/api"
	"github.com/metal-stack/metal-apiserver/pkg/request"
)

type (
	TokenWithoutPermissionCheckConfig struct {
		Certs  certs.CertStore
		Tokens TokenStore

		// issuer to sign the JWT Token with
		Issuer string
	}

	TokenWithoutPermissionCheck struct {
		certs  certs.CertStore
		tokens TokenStore

		// issuer to sign the JWT Token with
		issuer string
	}

	TokenWithPermissionCheckConfig struct {
		TokenWithoutPermissionCheckConfig

		Log *slog.Logger

		// adminSubjects are the subjects for which the token service allows the creation of admin api tokens
		AdminSubjects []string

		Authorizer               request.Authorizer
		ProjectsAndTenantsGetter api.ProjectsAndTenantsGetter
	}

	TokenWithPermissionCheck struct {
		TokenWithoutPermissionCheck

		log *slog.Logger

		// adminSubjects are the subjects for which the token service allows the creation of admin api tokens
		adminSubjects []string

		authorizer               request.Authorizer
		projectsAndTenantsGetter api.ProjectsAndTenantsGetter
	}
)

func NewWithoutPermissionCheck(c *TokenWithoutPermissionCheckConfig) *TokenWithoutPermissionCheck {
	return &TokenWithoutPermissionCheck{
		certs:  c.Certs,
		issuer: c.Issuer,
		tokens: c.Tokens,
	}
}

func NewWithPermissionCheck(c *TokenWithPermissionCheckConfig) *TokenWithPermissionCheck {
	return &TokenWithPermissionCheck{
		TokenWithoutPermissionCheck: TokenWithoutPermissionCheck{
			certs:  c.Certs,
			issuer: c.Issuer,
			tokens: c.Tokens,
		},
		adminSubjects:            c.AdminSubjects,
		log:                      c.Log,
		authorizer:               c.Authorizer,
		projectsAndTenantsGetter: c.ProjectsAndTenantsGetter,
	}
}

// CreateUserTokenWithoutPermissionCheck is only called from the auth service during login through console
// No validation against requested roles and permissions is required and implemented here
func (t *TokenWithoutPermissionCheck) CreateUserTokenWithoutPermissionCheck(ctx context.Context, subject string, expiration *time.Duration) (*apiv2.TokenServiceCreateResponse, error) {
	expires := DefaultExpiration
	if expiration != nil {
		expires = *expiration
	}

	privateKey, err := t.certs.LatestPrivate(ctx)
	if err != nil {
		return nil, errorutil.Internal("unable to fetch signing certificate: %w", err)
	}

	secret, token, err := NewJWT(apiv2.TokenType_TOKEN_TYPE_USER, subject, t.issuer, expires, privateKey)
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
func (t *TokenWithoutPermissionCheck) CreateApiTokenWithoutPermissionCheck(ctx context.Context, subject string, req *apiv2.TokenServiceCreateRequest) (*apiv2.TokenServiceCreateResponse, error) {
	expires := DefaultExpiration
	if req.Expires != nil {
		expires = req.Expires.AsDuration()
	}

	privateKey, err := t.certs.LatestPrivate(ctx)
	if err != nil {
		return nil, errorutil.NewInternal(err)
	}

	secret, token, err := NewJWT(apiv2.TokenType_TOKEN_TYPE_API, subject, t.issuer, expires, privateKey)
	if err != nil {
		return nil, errorutil.NewInternal(err)
	}

	token.Description = req.Description
	token.Permissions = req.Permissions
	token.ProjectRoles = req.ProjectRoles
	token.TenantRoles = req.TenantRoles
	token.AdminRole = req.AdminRole
	token.InfraRole = req.InfraRole

	err = t.tokens.Set(ctx, token)
	if err != nil {
		return nil, errorutil.NewInternal(err)
	}

	return &apiv2.TokenServiceCreateResponse{
		Token:  token,
		Secret: secret,
	}, nil
}

func (t *TokenWithPermissionCheck) CreateTokenForUser(ctx context.Context, user *string, req *apiv2.TokenServiceCreateRequest) (*apiv2.TokenServiceCreateResponse, error) {
	token, ok := TokenFromContext(ctx)
	if !ok || token == nil {
		return nil, errorutil.Unauthenticated("no token found in request")
	}

	if req.Expires.AsDuration() > certs.MaxTokenExpiration {
		return nil, fmt.Errorf("requested expiration duration: %q exceeds max expiration: %q", req.Expires.AsDuration(), certs.MaxTokenExpiration)
	}

	if slices.Contains(t.adminSubjects, token.User) {
		if token.AdminRole == nil || *token.AdminRole == apiv2.AdminRole_ADMIN_ROLE_UNSPECIFIED {
			token.AdminRole = req.AdminRole
			token.TokenType = apiv2.TokenType_TOKEN_TYPE_API
		}
		t.log.Debug("user is listed in adminsubjects", "new token.adminrole", token.AdminRole)
	}

	// we first validate token permission elevation for the token used in the token create request,
	// which might be an API token with restricted permissions

	err := t.ValidateTokenRequest(ctx, token, req)
	if err != nil {
		return nil, errorutil.NewPermissionDenied(err)
	}

	// now, we validate if the user is still permitted to create such a token
	// doing this check is not strictly necessary because the resulting token would fail in the auther when being compared
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
		InfraRole:    token.InfraRole,
	}

	tokenUser := token.GetUser()

	if !slices.Contains(t.adminSubjects, token.User) && user != nil {
		return nil, errorutil.PermissionDenied("only admins can specify token user")
	}

	if slices.Contains(t.adminSubjects, token.User) {
		fullUserToken.AdminRole = apiv2.AdminRole_ADMIN_ROLE_EDITOR.Enum()
	}

	if user != nil {
		tokenUser = *user
	}

	err = t.ValidateTokenRequest(ctx, fullUserToken, req)
	if err != nil {
		return nil, errorutil.NewPermissionDenied(err)
	}

	privateKey, err := t.certs.LatestPrivate(ctx)
	if err != nil {
		return nil, errorutil.NewInternal(err)
	}

	secret, token, err := NewJWT(apiv2.TokenType_TOKEN_TYPE_API, tokenUser, t.issuer, req.Expires.AsDuration(), privateKey)
	if err != nil {
		return nil, errorutil.NewInternal(err)
	}

	token.Description = req.Description
	token.Permissions = req.Permissions
	token.ProjectRoles = req.ProjectRoles
	token.TenantRoles = req.TenantRoles
	token.AdminRole = req.AdminRole
	token.InfraRole = req.InfraRole
	token.Meta = &apiv2.Meta{
		Labels:     req.Labels,
		CreatedAt:  token.IssuedAt,
		Generation: 0,
	}

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

type TokenRequest interface {
	GetPermissions() []*apiv2.MethodPermission
	GetProjectRoles() map[string]apiv2.ProjectRole
	GetTenantRoles() map[string]apiv2.TenantRole
	GetAdminRole() apiv2.AdminRole
	GetInfraRole() apiv2.InfraRole
}

func (t *TokenWithPermissionCheck) ValidateTokenRequest(ctx context.Context, currentToken *apiv2.Token, req TokenRequest) error {
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
	requestedPermissions, err := t.authorizer.TokenPermissions(ctx, &apiv2.Token{
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
