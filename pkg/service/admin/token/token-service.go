package admin

import (
	"context"
	"log/slog"

	"github.com/metal-stack/api/go/errorutil"
	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	"github.com/metal-stack/api/go/metalstack/admin/v2/adminv2connect"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/repository"
	tokenutil "github.com/metal-stack/metal-apiserver/pkg/token"
)

type Config struct {
	Log  *slog.Logger
	Repo *repository.Store
}

type tokenService struct {
	log  *slog.Logger
	repo *repository.Store
}

func New(c Config) adminv2connect.TokenServiceHandler {
	return &tokenService{
		log:  c.Log.WithGroup("adminTokenService"),
		repo: c.Repo,
	}
}

func (t *tokenService) List(ctx context.Context, rq *adminv2.TokenServiceListRequest) (*adminv2.TokenServiceListResponse, error) {
	tokens, err := t.repo.UnscopedToken().List(ctx, rq.Query)
	if err != nil {
		return nil, err
	}

	var result []*apiv2.Token

	for _, tok := range tokens {
		result = append(result, tok.Token)
	}

	return &adminv2.TokenServiceListResponse{
		Tokens: result,
	}, nil
}

func (t *tokenService) Revoke(ctx context.Context, req *adminv2.TokenServiceRevokeRequest) (*adminv2.TokenServiceRevokeResponse, error) {
	_, err := t.repo.Token(req.User).Delete(ctx, req.Uuid)
	if err != nil {
		return nil, err
	}

	return &adminv2.TokenServiceRevokeResponse{}, nil
}

func (t *tokenService) Create(ctx context.Context, req *adminv2.TokenServiceCreateRequest) (*adminv2.TokenServiceCreateResponse, error) {
	token, ok := tokenutil.TokenFromContext(ctx)
	if !ok || token == nil {
		return nil, errorutil.Unauthenticated("no token found in request")
	}

	res, err := t.repo.Token(token.User).Create(ctx, req)
	if err != nil {
		return nil, err
	}

	return &adminv2.TokenServiceCreateResponse{
		Token:  res.Token,
		Secret: res.Secret,
	}, nil
}
