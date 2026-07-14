package token

import (
	"context"
	"log/slog"

	"github.com/metal-stack/api/go/errorutil"
	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/api/go/metalstack/api/v2/apiv2connect"
	"github.com/metal-stack/metal-apiserver/pkg/repository/api"
	"github.com/metal-stack/metal-apiserver/pkg/request"

	"github.com/metal-stack/metal-apiserver/pkg/certs"
	"github.com/metal-stack/metal-apiserver/pkg/repository"
	tokenutil "github.com/metal-stack/metal-apiserver/pkg/token"
)

type Config struct {
	Log        *slog.Logger
	TokenStore tokenutil.TokenStore
	CertStore  certs.CertStore
	Repo       *repository.Store

	// provider tenant, other tenants which are tenant member with owner rights of this tenant can request admin-role-editor,
	// if they have editor or viewer rights, they can request admin-role-viewer.
	ProviderTenant string

	// Issuer to sign the JWT Token with
	Issuer string
}

type tokenService struct {
	issuer         string
	providerTenant string
	tokens         tokenutil.TokenStore
	certs          certs.CertStore
	log            *slog.Logger
	repo           *repository.Store

	projectsAndTenantsGetter api.ProjectsAndTenantsGetter
	authorizer               request.Authorizer
}

type TokenService interface {
	apiv2connect.TokenServiceHandler
}

func New(c Config) TokenService {
	projectsAndTenantsGetter := func(ctx context.Context, userId string) (*api.ProjectsAndTenants, error) {
		return c.Repo.UnscopedProject().AdditionalMethods().GetProjectsAndTenants(ctx, userId)
	}
	log := c.Log.WithGroup("tokenService")

	return &tokenService{
		tokens:         c.TokenStore,
		certs:          c.CertStore,
		issuer:         c.Issuer,
		log:            log,
		providerTenant: c.ProviderTenant,
		repo:           c.Repo,

		projectsAndTenantsGetter: projectsAndTenantsGetter,
		authorizer:               request.NewAuthorizer(log, projectsAndTenantsGetter),
	}
}

// Get returns the token by a given uuid for the user who requests it.
func (t *tokenService) Get(ctx context.Context, rq *apiv2.TokenServiceGetRequest) (*apiv2.TokenServiceGetResponse, error) {
	token, ok := tokenutil.TokenFromContext(ctx)
	if !ok || token == nil {
		return nil, errorutil.Unauthenticated("no token found in request")
	}

	res, err := t.repo.Token(token.User).Get(ctx, rq.Uuid)
	if err != nil {
		return nil, err
	}

	return &apiv2.TokenServiceGetResponse{
		Token: res.Token,
	}, nil
}

// Update updates a given token of a user.
// We need to prevent a user from elevating permissions here.
func (t *tokenService) Update(ctx context.Context, req *apiv2.TokenServiceUpdateRequest) (*apiv2.TokenServiceUpdateResponse, error) {
	token, ok := tokenutil.TokenFromContext(ctx)
	if !ok || token == nil {
		return nil, errorutil.Unauthenticated("no token found in request")
	}

	res, err := t.repo.Token(token.User).Update(ctx, req.Uuid, req)
	if err != nil {
		return nil, err
	}

	return &apiv2.TokenServiceUpdateResponse{
		Token: res.Token,
	}, nil
}

// Create is called by users to issue new API tokens. This can be done from console tokens but also from other API tokens which have the permission to call token create.
// We need to prevent a user from elevating permissions here.
func (t *tokenService) Create(ctx context.Context, req *apiv2.TokenServiceCreateRequest) (*apiv2.TokenServiceCreateResponse, error) {
	token, ok := tokenutil.TokenFromContext(ctx)
	if !ok || token == nil {
		return nil, errorutil.Unauthenticated("no token found in request")
	}

	res, err := t.repo.Token(token.User).Create(ctx, &adminv2.TokenServiceCreateRequest{
		TokenCreateRequest: req,
	})
	if err != nil {
		return nil, err
	}

	return &apiv2.TokenServiceCreateResponse{
		Token:  res.Token,
		Secret: res.Secret,
	}, nil
}

// List lists the tokens of a specific user.
func (t *tokenService) List(ctx context.Context, rq *apiv2.TokenServiceListRequest) (*apiv2.TokenServiceListResponse, error) {
	token, ok := tokenutil.TokenFromContext(ctx)
	if !ok || token == nil {
		return nil, errorutil.Unauthenticated("no token found in request")
	}

	tokens, err := t.repo.Token(token.User).List(ctx, rq.Query)
	if err != nil {
		return nil, err
	}

	var result []*apiv2.Token

	for _, tok := range tokens {
		result = append(result, tok.Token)
	}

	return &apiv2.TokenServiceListResponse{
		Tokens: result,
	}, nil
}

// Revoke revokes a token of a given user and token ID.
func (t *tokenService) Revoke(ctx context.Context, rq *apiv2.TokenServiceRevokeRequest) (*apiv2.TokenServiceRevokeResponse, error) {
	token, ok := tokenutil.TokenFromContext(ctx)
	if !ok || token == nil {
		return nil, errorutil.Unauthenticated("no token found in request")
	}

	_, err := t.repo.Token(token.User).Delete(ctx, rq.Uuid)
	if err != nil {
		return nil, err
	}

	return &apiv2.TokenServiceRevokeResponse{}, nil
}

func (t *tokenService) Refresh(ctx context.Context, _ *apiv2.TokenServiceRefreshRequest) (*apiv2.TokenServiceRefreshResponse, error) {
	token, ok := tokenutil.TokenFromContext(ctx)
	if !ok || token == nil {
		return nil, errorutil.Unauthenticated("no token found in request")
	}

	return t.repo.Token(token.User).AdditionalMethods().Refresh(ctx, token.Uuid)
}
