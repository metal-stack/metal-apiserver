package admin

import (
	"context"
	"log/slog"

	"connectrpc.com/connect"
	"github.com/metal-stack/api-server/pkg/invite"
	tutil "github.com/metal-stack/api-server/pkg/tenant"
	"github.com/metal-stack/api-server/pkg/token"
	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	"github.com/metal-stack/api/go/metalstack/admin/v2/adminv2connect"
	mdcv1 "github.com/metal-stack/masterdata-api/api/v1"

	v1 "github.com/metal-stack/masterdata-api/api/v1"
	mdc "github.com/metal-stack/masterdata-api/pkg/client"
)

type Config struct {
	Log          *slog.Logger
	MasterClient mdc.Client
	InviteStore  invite.TenantInviteStore
	TokenStore   token.TokenStore
}
type tenantServiceServer struct {
	log          *slog.Logger
	masterClient mdc.Client
	inviteStore  invite.TenantInviteStore
	tokenStore   token.TokenStore
}

type TenantService interface {
	adminv2connect.TenantServiceHandler
}

// FIXME use repo where possible

func New(c Config) TenantService {
	return &tenantServiceServer{
		log:          c.Log.WithGroup("adminTenantService"),
		masterClient: c.MasterClient,
		inviteStore:  c.InviteStore,
		tokenStore:   c.TokenStore,
	}
}

// Create implements TenantService.
func (t *tenantServiceServer) Create(ctx context.Context, rq *connect.Request[adminv2.TenantServiceCreateRequest]) (*connect.Response[adminv2.TenantServiceCreateResponse], error) {
	req := rq.Msg
	t.log.Debug("create", "request", req)

	tenant := &mdcv1.Tenant{
		Meta: &v1.Meta{
			Id: req.Name,
		},
	}

	resp, err := t.masterClient.Tenant().Create(ctx, &v1.TenantCreateRequest{Tenant: tenant})
	if err != nil {
		return nil, err
	}

	return connect.NewResponse(&adminv2.TenantServiceCreateResponse{Tenant: tutil.ConvertFromTenant(resp.Tenant)}), nil
}

// List implements TenantService.
func (t *tenantServiceServer) List(context.Context, *connect.Request[adminv2.TenantServiceListRequest]) (*connect.Response[adminv2.TenantServiceListResponse], error) {
	panic("unimplemented")
}
