package admin

import (
	"context"
	"log/slog"

	"github.com/google/go-cmp/cmp"
	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	"github.com/metal-stack/api/go/metalstack/admin/v2/adminv2connect"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/certs"
	"github.com/metal-stack/metal-apiserver/pkg/repository"
	"github.com/metal-stack/metal-apiserver/pkg/repository/api"
	"github.com/metal-stack/metal-apiserver/pkg/request"
	tokencommon "github.com/metal-stack/metal-apiserver/pkg/token"
)

type Config struct {
	Log           *slog.Logger
	TokenStore    tokencommon.TokenStore
	CertStore     certs.CertStore
	Issuer        string
	AdminSubjects []string
	Repo          *repository.Store
}

type tokenService struct {
	tokens       tokencommon.TokenStore
	tokenCreator tokencommon.TokenWithPermissionCheck
}

func New(c Config) adminv2connect.TokenServiceHandler {
	var (
		log = c.Log.WithGroup("adminTokenService")
	)

	projectsAndTenantsGetter := func(ctx context.Context, userId string) (*api.ProjectsAndTenants, error) {
		return c.Repo.UnscopedProject().AdditionalMethods().GetProjectsAndTenants(ctx, userId)
	}

	return &tokenService{
		tokens: c.TokenStore,
		tokenCreator: *tokencommon.NewWithPermissionCheck(&tokencommon.TokenWithPermissionCheckConfig{
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

func (t *tokenService) List(ctx context.Context, req *adminv2.TokenServiceListRequest) (*adminv2.TokenServiceListResponse, error) {
	var (
		result []*apiv2.Token
		err    error
	)

	tokens, err := t.tokens.AdminList(ctx)
	if err != nil {
		return nil, err
	}

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

	return &adminv2.TokenServiceListResponse{
		Tokens: result,
	}, nil
}

func (t *tokenService) Revoke(ctx context.Context, req *adminv2.TokenServiceRevokeRequest) (*adminv2.TokenServiceRevokeResponse, error) {
	err := t.tokens.Revoke(ctx, req.User, req.Uuid)
	if err != nil {
		return nil, err
	}

	return &adminv2.TokenServiceRevokeResponse{}, nil
}

func (t *tokenService) Create(ctx context.Context, req *adminv2.TokenServiceCreateRequest) (*adminv2.TokenServiceCreateResponse, error) {
	resp, err := t.tokenCreator.CreateTokenForUser(ctx, req.User, req.TokenCreateRequest)

	if err != nil {
		return nil, err
	}

	return &adminv2.TokenServiceCreateResponse{
		Token:  resp.Token,
		Secret: resp.Secret,
	}, nil
}
