package admin

import (
	"context"
	"log/slog"

	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	"github.com/metal-stack/api/go/metalstack/admin/v2/adminv2connect"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	"github.com/metal-stack/metal-apiserver/pkg/repository"
	"github.com/metal-stack/metal-apiserver/pkg/token"
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

func (t *tokenService) List(ctx context.Context, req *adminv2.TokenServiceListRequest) (*adminv2.TokenServiceListResponse, error) {
	var (
		result []*apiv2.Token
		err    error
	)

	tokens, err := t.repo.UnscopedToken().List(ctx, req.Query)
	if err != nil {
		return nil, err
	}

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
	user := req.User

	if user == nil {
		token, ok := token.TokenFromContext(ctx)
		if !ok || token == nil {
			return nil, errorutil.Unauthenticated("no token found in request")
		}

		user = &token.User
	}

	resp, err := t.repo.Token(*user).Create(ctx, req)

	if err != nil {
		return nil, err
	}

	return &adminv2.TokenServiceCreateResponse{
		Token:  resp.Token,
		Secret: resp.Secret,
	}, nil
}
