package admin

import (
	"context"
	"log/slog"

	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	"github.com/metal-stack/api/go/metalstack/admin/v2/adminv2connect"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/invite"
	"github.com/metal-stack/metal-apiserver/pkg/repository"
	"github.com/metal-stack/metal-apiserver/pkg/token"
)

type Config struct {
	Log         *slog.Logger
	Repo        *repository.Store
	InviteStore invite.TenantInviteStore
	TokenStore  token.TokenStore
}
type tenantServiceServer struct {
	log         *slog.Logger
	repo        *repository.Store
	inviteStore invite.TenantInviteStore
	tokenStore  token.TokenStore
}

type TenantService interface {
	adminv2connect.TenantServiceHandler
}

func New(c Config) TenantService {
	return &tenantServiceServer{
		log:         c.Log.WithGroup("adminTenantService"),
		repo:        c.Repo,
		inviteStore: c.InviteStore,
		tokenStore:  c.TokenStore,
	}
}

func (t *tenantServiceServer) Create(ctx context.Context, req *adminv2.TenantServiceCreateRequest) (*adminv2.TenantServiceCreateResponse, error) {
	tenant, err := t.repo.Tenant().Create(ctx, &apiv2.TenantServiceCreateRequest{
		Name:        req.Name,
		Description: req.Description,
		Email:       req.Email,
		AvatarUrl:   req.AvatarUrl,
	})
	if err != nil {
		return nil, err
	}

	return &adminv2.TenantServiceCreateResponse{Tenant: tenant}, nil
}

func (t *tenantServiceServer) List(ctx context.Context, req *adminv2.TenantServiceListRequest) (*adminv2.TenantServiceListResponse, error) {
	tenants, err := t.repo.Tenant().List(ctx, &apiv2.TenantServiceListRequest{
		Id:   req.Login,
		Name: req.Name,
	})
	if err != nil {
		return nil, err
	}

	return &adminv2.TenantServiceListResponse{Tenants: tenants}, nil
}
