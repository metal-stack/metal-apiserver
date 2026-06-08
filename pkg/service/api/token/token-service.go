package token

import (
	"context"
	"log/slog"
	"slices"

	"github.com/google/go-cmp/cmp"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/api/go/metalstack/api/v2/apiv2connect"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	"github.com/metal-stack/metal-apiserver/pkg/repository/api"
	"github.com/metal-stack/metal-apiserver/pkg/request"

	"github.com/metal-stack/metal-apiserver/pkg/certs"
	"github.com/metal-stack/metal-apiserver/pkg/repository"
	tokencommon "github.com/metal-stack/metal-apiserver/pkg/token"
)

type Config struct {
	Log        *slog.Logger
	TokenStore tokencommon.TokenStore
	CertStore  certs.CertStore
	Repo       *repository.Store

	// AdminSubjects are the subjects for which the token service allows the creation of admin api tokens
	AdminSubjects []string

	// Issuer to sign the JWT Token with
	Issuer string
}

type tokenService struct {
	issuer        string
	adminSubjects []string
	tokens        tokencommon.TokenStore
	certs         certs.CertStore
	log           *slog.Logger

	tokenCreator             *tokencommon.TokenWithPermissionCheck
	projectsAndTenantsGetter api.ProjectsAndTenantsGetter
}

func New(c Config) apiv2connect.TokenServiceHandler {
	projectsAndTenantsGetter := func(ctx context.Context, userId string) (*api.ProjectsAndTenants, error) {
		return c.Repo.UnscopedProject().AdditionalMethods().GetProjectsAndTenants(ctx, userId)
	}

	log := c.Log.WithGroup("tokenService")

	return &tokenService{
		tokens:                   c.TokenStore,
		certs:                    c.CertStore,
		issuer:                   c.Issuer,
		log:                      log,
		adminSubjects:            c.AdminSubjects,
		projectsAndTenantsGetter: projectsAndTenantsGetter,
		tokenCreator: tokencommon.NewWithPermissionCheck(&tokencommon.TokenWithPermissionCheckConfig{
			TokenWithoutPermissionCheckConfig: tokencommon.TokenWithoutPermissionCheckConfig{
				Certs:  c.CertStore,
				Tokens: c.TokenStore,
				Issuer: c.Issuer,
			},
			Log:                      log,
			AdminSubjects:            c.AdminSubjects,
			Authorizer:               request.NewAuthorizer(log, projectsAndTenantsGetter),
			ProjectsAndTenantsGetter: projectsAndTenantsGetter,
		}),
	}
}

// Get returns the token by a given uuid for the user who requests it.
func (t *tokenService) Get(ctx context.Context, rq *apiv2.TokenServiceGetRequest) (*apiv2.TokenServiceGetResponse, error) {
	token, ok := tokencommon.TokenFromContext(ctx)
	if !ok || token == nil {
		return nil, errorutil.Unauthenticated("no token found in request")
	}

	res, err := t.tokens.Get(ctx, token.User, rq.Uuid)
	if err != nil {
		return nil, err
	}

	return &apiv2.TokenServiceGetResponse{
		Token: res,
	}, nil
}

// Update updates a given token of a user.
// We need to prevent a user from elevating permissions here.
func (t *tokenService) Update(ctx context.Context, req *apiv2.TokenServiceUpdateRequest) (*apiv2.TokenServiceUpdateResponse, error) {
	token, ok := tokencommon.TokenFromContext(ctx)
	if !ok || token == nil {
		return nil, errorutil.Unauthenticated("no token found in request")
	}

	// we first validate token permission elevation for the token used in the token update request,
	// which might be an API token with restricted permissions

	err := t.tokenCreator.ValidateTokenRequest(ctx, token, req)
	if err != nil {
		return nil, errorutil.NewPermissionDenied(err)
	}

	// now, we validate if the user is still permitted to update the token
	// doing this check is not strictly necessary because the resulting token would fail in the auther when being compared
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
		InfraRole:    token.InfraRole,
	}
	if slices.Contains(t.adminSubjects, token.User) {
		fullUserToken.AdminRole = apiv2.AdminRole_ADMIN_ROLE_EDITOR.Enum()
	}
	err = t.tokenCreator.ValidateTokenRequest(ctx, fullUserToken, req)
	if err != nil {
		return nil, errorutil.NewPermissionDenied(err)
	}

	// now follows the update

	tokenToUpdate, err := t.tokens.Get(ctx, token.User, req.Uuid)
	if err != nil {
		return nil, err
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
	if req.Labels != nil {
		if tokenToUpdate.Meta == nil {
			tokenToUpdate.Meta = &apiv2.Meta{}
		}
		if tokenToUpdate.Meta.Labels == nil {
			tokenToUpdate.Meta.Labels = &apiv2.Labels{}
		}

		tokenToUpdate.Meta.Labels.Labels = repository.UpdateLabelsOnMap(req.Labels, token.Meta.Labels.Labels)
	}

	tokenToUpdate.Permissions = req.Permissions
	tokenToUpdate.ProjectRoles = req.ProjectRoles
	tokenToUpdate.TenantRoles = req.TenantRoles

	err = t.tokens.Set(ctx, tokenToUpdate)
	if err != nil {
		return nil, err
	}

	return &apiv2.TokenServiceUpdateResponse{
		Token: tokenToUpdate,
	}, nil
}

// Create is called by users to issue new API tokens. This can be done from console tokens but also from other API tokens which have the permission to call token create.
// We need to prevent a user from elevating permissions here.
func (t *tokenService) Create(ctx context.Context, req *apiv2.TokenServiceCreateRequest) (*apiv2.TokenServiceCreateResponse, error) {
	token, ok := tokencommon.TokenFromContext(ctx)
	if !ok || token == nil {
		return nil, errorutil.Unauthenticated("no token found in request")
	}
	return t.tokenCreator.CreateTokenForUser(ctx, nil, req)
}

// List lists the tokens of a specific user.
func (t *tokenService) List(ctx context.Context, req *apiv2.TokenServiceListRequest) (*apiv2.TokenServiceListResponse, error) {
	token, ok := tokencommon.TokenFromContext(ctx)
	if !ok || token == nil {
		return nil, errorutil.Unauthenticated("no token found in request")
	}

	tokens, err := t.tokens.List(ctx, token.User)
	if err != nil {
		return nil, err
	}

	var result []*apiv2.Token

	if req.Query == nil {
		result = tokens
	} else {
		for _, tok := range tokens {
			match := true

			if req.Query.Description != nil {
				match = match && *req.Query.Description == tok.Description
			}
			if req.Query.TokenType != nil {
				match = match && *req.Query.TokenType == tok.TokenType
			}
			if req.Query.User != nil {
				match = match && *req.Query.User == tok.User
			}
			if req.Query.Uuid != nil {
				match = match && *req.Query.Uuid == tok.Uuid
			}
			if req.Query.Labels != nil {
				if tok.Meta == nil || tok.Meta.Labels == nil {
					continue
				}

				match = match && cmp.Equal(req.Query.Labels.Labels, tok.Meta.Labels.Labels)
			}

			if match {
				result = append(result, tok)
			}
		}
	}

	return &apiv2.TokenServiceListResponse{
		Tokens: result,
	}, nil
}

// Revoke revokes a token of a given user and token ID.
func (t *tokenService) Revoke(ctx context.Context, rq *apiv2.TokenServiceRevokeRequest) (*apiv2.TokenServiceRevokeResponse, error) {
	token, ok := tokencommon.TokenFromContext(ctx)
	if !ok || token == nil {
		return nil, errorutil.Unauthenticated("no token found in request")
	}

	err := t.tokens.Revoke(ctx, token.User, rq.Uuid)
	if err != nil {
		return nil, err
	}

	return &apiv2.TokenServiceRevokeResponse{}, nil
}

func (t *tokenService) Refresh(ctx context.Context, _ *apiv2.TokenServiceRefreshRequest) (*apiv2.TokenServiceRefreshResponse, error) {
	token, ok := tokencommon.TokenFromContext(ctx)
	if !ok || token == nil {
		return nil, errorutil.Unauthenticated("no token found in request")
	}

	oldtoken, err := t.tokens.Get(ctx, token.User, token.Uuid)
	if err != nil {
		return nil, err
	}

	// we first copy the token permission from the old token
	createRequest := &apiv2.TokenServiceCreateRequest{
		Permissions:  oldtoken.Permissions,
		ProjectRoles: oldtoken.ProjectRoles,
		TenantRoles:  oldtoken.TenantRoles,
		AdminRole:    oldtoken.AdminRole,
		InfraRole:    oldtoken.InfraRole,
	}

	err = t.tokenCreator.ValidateTokenRequest(ctx, token, createRequest)
	if err != nil {
		return nil, errorutil.NewPermissionDenied(err)
	}

	// now, we validate if the user is still permitted to refresh the token
	// doing this check is not strictly necessary because the resulting token would fail in the auther when being compared
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
		InfraRole:    token.InfraRole,
	}
	if slices.Contains(t.adminSubjects, token.User) {
		fullUserToken.AdminRole = apiv2.AdminRole_ADMIN_ROLE_EDITOR.Enum()
	}
	err = t.tokenCreator.ValidateTokenRequest(ctx, fullUserToken, createRequest)
	if err != nil {
		return nil, errorutil.NewPermissionDenied(err)
	}

	// now follows the refresh, aka create a new token

	privateKey, err := t.certs.LatestPrivate(ctx)
	if err != nil {
		return nil, errorutil.NewInternal(err)
	}

	// New duration is calculated from the old token
	exp := oldtoken.Expires.AsTime().Sub(oldtoken.IssuedAt.AsTime())

	secret, newToken, err := tokencommon.NewJWT(apiv2.TokenType_TOKEN_TYPE_API, token.GetUser(), t.issuer, exp, privateKey)
	if err != nil {
		return nil, errorutil.NewInternal(err)
	}

	newToken.Description = oldtoken.Description
	newToken.Permissions = oldtoken.Permissions
	newToken.ProjectRoles = oldtoken.ProjectRoles
	newToken.TenantRoles = oldtoken.TenantRoles
	newToken.AdminRole = oldtoken.AdminRole
	newToken.InfraRole = oldtoken.InfraRole

	err = t.tokens.Set(ctx, newToken)
	if err != nil {
		return nil, err
	}

	return &apiv2.TokenServiceRefreshResponse{
		Token:  newToken,
		Secret: secret,
	}, nil
}
