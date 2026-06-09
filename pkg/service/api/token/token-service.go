package token

import (
	"context"
	"log/slog"

	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/api/go/metalstack/api/v2/apiv2connect"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"

	"github.com/metal-stack/metal-apiserver/pkg/repository"
	tokencommon "github.com/metal-stack/metal-apiserver/pkg/token"
)

type Config struct {
	Log  *slog.Logger
	Repo *repository.Store
}

type tokenService struct {
	log  *slog.Logger
	repo *repository.Store
}

func New(c Config) apiv2connect.TokenServiceHandler {
	return &tokenService{
		repo: c.Repo,
		log:  c.Log.WithGroup("tokenService"),
	}
}

// Get returns the token by a given uuid for the user who requests it.
func (t *tokenService) Get(ctx context.Context, rq *apiv2.TokenServiceGetRequest) (*apiv2.TokenServiceGetResponse, error) {
	token, ok := tokencommon.TokenFromContext(ctx)
	if !ok || token == nil {
		return nil, errorutil.Unauthenticated("no token found in request")
	}

	tok, err := t.repo.Token(token.User).Get(ctx, rq.Uuid)
	if err != nil {
		return nil, err
	}

	return &apiv2.TokenServiceGetResponse{
		Token: tok.Token,
	}, nil
}

// Update updates a given token of a user.
func (t *tokenService) Update(ctx context.Context, rq *apiv2.TokenServiceUpdateRequest) (*apiv2.TokenServiceUpdateResponse, error) {
	token, ok := tokencommon.TokenFromContext(ctx)
	if !ok || token == nil {
		return nil, errorutil.Unauthenticated("no token found in request")
	}

	tok, err := t.repo.Token(token.User).Update(ctx, rq.Uuid, rq)
	if err != nil {
		return nil, err
	}

	return &apiv2.TokenServiceUpdateResponse{
		Token: tok.Token,
	}, nil
}

// Create is called by users to issue new API tokens. This can be done from console tokens but also from other API tokens which have the permission to call token create.
// We need to prevent a user from elevating permissions here.
func (t *tokenService) Create(ctx context.Context, req *apiv2.TokenServiceCreateRequest) (*apiv2.TokenServiceCreateResponse, error) {
	token, ok := tokencommon.TokenFromContext(ctx)
	if !ok || token == nil {
		return nil, errorutil.Unauthenticated("no token found in request")
	}

	tok, err := t.repo.Token(token.User).Create(ctx, &adminv2.TokenServiceCreateRequest{
		TokenCreateRequest: req,
	})
	if err != nil {
		return nil, err
	}

	return &apiv2.TokenServiceCreateResponse{
		Token:  tok.Token,
		Secret: tok.Secret,
	}, nil
}

// List lists the tokens of a specific user.
func (t *tokenService) List(ctx context.Context, req *apiv2.TokenServiceListRequest) (*apiv2.TokenServiceListResponse, error) {
	token, ok := tokencommon.TokenFromContext(ctx)
	if !ok || token == nil {
		return nil, errorutil.Unauthenticated("no token found in request")
	}

	tokens, err := t.repo.Token(token.User).List(ctx, req.Query)
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
	token, ok := tokencommon.TokenFromContext(ctx)
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
	token, ok := tokencommon.TokenFromContext(ctx)
	if !ok || token == nil {
		return nil, errorutil.Unauthenticated("no token found in request")
	}

	resp, err := t.repo.Token(token.User).AdditionalMethods().Refresh(ctx)
	if err != nil {
		return nil, err
	}

	return &apiv2.TokenServiceRefreshResponse{
		Token:  resp.Token,
		Secret: resp.Secret,
	}, nil
}
