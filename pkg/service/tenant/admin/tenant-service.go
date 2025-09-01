package admin

import (
	"context"
	"log/slog"

	"connectrpc.com/connect"
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

// FIXME use repo where possible

func New(c Config) TenantService {
	return &tenantServiceServer{
		log:         c.Log.WithGroup("adminTenantService"),
		repo:        c.Repo,
		inviteStore: c.InviteStore,
		tokenStore:  c.TokenStore,
	}
}

// Create implements TenantService.
func (t *tenantServiceServer) Create(ctx context.Context, rq *connect.Request[adminv2.TenantServiceCreateRequest]) (*connect.Response[adminv2.TenantServiceCreateResponse], error) {
	req := rq.Msg

	created, err := t.repo.Tenant().Create(ctx, &apiv2.TenantServiceCreateRequest{
		Name:        req.Name,
		Description: req.Description,
		Email:       req.Email,
		AvatarUrl:   req.AvatarUrl,
	})
	if err != nil {
		return nil, err
	}

	converted, err := t.repo.Tenant().ConvertToProto(created)
	if err != nil {
		return nil, err
	}

	return connect.NewResponse(&adminv2.TenantServiceCreateResponse{Tenant: converted}), nil
}

// List implements TenantService.
func (t *tenantServiceServer) List(context.Context, *connect.Request[adminv2.TenantServiceListRequest]) (*connect.Response[adminv2.TenantServiceListResponse], error) {
	panic("unimplemented")
}
