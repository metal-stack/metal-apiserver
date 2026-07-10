package admin

import (
	"context"
	"log/slog"

	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	"github.com/metal-stack/api/go/metalstack/admin/v2/adminv2connect"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/certs"
	"github.com/metal-stack/metal-apiserver/pkg/repository"
	ts "github.com/metal-stack/metal-apiserver/pkg/service/api/token"
	tokenutil "github.com/metal-stack/metal-apiserver/pkg/token"
)

type Config struct {
	Log          *slog.Logger
	TokenStore   tokenutil.TokenStore
	CertStore    certs.CertStore
	TokenService ts.TokenService
	Repo         *repository.Store
}

type tokenService struct {
	tokenstore tokenutil.TokenStore
	certs      certs.CertStore
	log        *slog.Logger
	ts         ts.TokenService
	repo       *repository.Store
}

func New(c Config) adminv2connect.TokenServiceHandler {
	return &tokenService{
		log:        c.Log.WithGroup("adminTokenService"),
		tokenstore: c.TokenStore,
		certs:      c.CertStore,
		ts:         c.TokenService,
		repo:       c.Repo,
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
	resp, err := t.ts.CreateTokenForUser(ctx, req.User, req.TokenCreateRequest)

	if err != nil {
		return nil, err
	}

	return &adminv2.TokenServiceCreateResponse{
		Token:  resp.Token,
		Secret: resp.Secret,
	}, nil
}
