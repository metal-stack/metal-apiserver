package admin

import (
	"context"
	"log/slog"

	"github.com/google/go-cmp/cmp"
	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	"github.com/metal-stack/api/go/metalstack/admin/v2/adminv2connect"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/certs"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	ts "github.com/metal-stack/metal-apiserver/pkg/service/api/token"
	tokenutil "github.com/metal-stack/metal-apiserver/pkg/token"
)

type Config struct {
	Log          *slog.Logger
	TokenStore   tokenutil.TokenStore
	CertStore    certs.CertStore
	TokenService ts.TokenService
}

type tokenService struct {
	tokenstore tokenutil.TokenStore
	certs      certs.CertStore
	log        *slog.Logger
	ts         ts.TokenService
}

func New(c Config) adminv2connect.TokenServiceHandler {
	return &tokenService{
		log:        c.Log.WithGroup("adminTokenService"),
		tokenstore: c.TokenStore,
		certs:      c.CertStore,
		ts:         c.TokenService,
	}
}

func (t *tokenService) List(ctx context.Context, req *adminv2.TokenServiceListRequest) (*adminv2.TokenServiceListResponse, error) {
	var (
		result []*apiv2.Token
		err    error
	)

	tokens, err := t.tokenstore.AdminList(ctx)
	if err != nil {
		return nil, errorutil.NewInternal(err)
	}

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

	return &adminv2.TokenServiceListResponse{
		Tokens: result,
	}, nil
}

func (t *tokenService) Revoke(ctx context.Context, req *adminv2.TokenServiceRevokeRequest) (*adminv2.TokenServiceRevokeResponse, error) {
	err := t.tokenstore.Revoke(ctx, req.User, req.Uuid)
	if err != nil {
		return nil, errorutil.NewInternal(err)
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
