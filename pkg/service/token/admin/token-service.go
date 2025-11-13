package admin

import (
	"context"
	"log/slog"

	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	"github.com/metal-stack/api/go/metalstack/admin/v2/adminv2connect"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/certs"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	"github.com/metal-stack/metal-apiserver/pkg/repository"
	tokenutil "github.com/metal-stack/metal-apiserver/pkg/token"
)

type Config struct {
	Log        *slog.Logger
	TokenStore tokenutil.TokenStore
	CertStore  certs.CertStore
	Repo       *repository.Store
}

type tokenService struct {
	tokenstore tokenutil.TokenStore
	certs      certs.CertStore
	log        *slog.Logger
}

func New(c Config) adminv2connect.TokenServiceHandler {
	return &tokenService{
		log:        c.Log.WithGroup("adminTokenService"),
		tokenstore: c.TokenStore,
		certs:      c.CertStore,
	}
}

func (t *tokenService) Create(context.Context, *adminv2.TokenServiceCreateRequest) (*adminv2.TokenServiceCreateResponse, error) {
	panic("unimplemented")
}

func (t *tokenService) List(ctx context.Context, req *adminv2.TokenServiceListRequest) (*adminv2.TokenServiceListResponse, error) {
	var (
		tokens []*apiv2.Token
		err    error
	)
	if req.User != nil {
		tokens, err = t.tokenstore.List(ctx, *req.User)
		if err != nil {
			return nil, errorutil.NewInternal(err)
		}
	} else {
		tokens, err = t.tokenstore.AdminList(ctx)
		if err != nil {
			return nil, errorutil.NewInternal(err)
		}
	}

	return &adminv2.TokenServiceListResponse{
		Tokens: tokens,
	}, nil
}

func (t *tokenService) Revoke(ctx context.Context, req *adminv2.TokenServiceRevokeRequest) (*adminv2.TokenServiceRevokeResponse, error) {
	err := t.tokenstore.Revoke(ctx, req.User, req.Uuid)
	if err != nil {
		return nil, errorutil.NewInternal(err)
	}

	return &adminv2.TokenServiceRevokeResponse{}, nil
}
