package admin

import (
	"context"
	"log/slog"

	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	"github.com/metal-stack/api/go/metalstack/admin/v2/adminv2connect"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"

	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	"github.com/metal-stack/metal-apiserver/pkg/invite"
	"github.com/metal-stack/metal-apiserver/pkg/repository"
	"github.com/metal-stack/metal-apiserver/pkg/repository/api"
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
		Labels:      req.Labels,
	})
	if err != nil {
		return nil, err
	}

	_, err = t.repo.Tenant().AdditionalMethods().Member(tenant.Login).Create(ctx, &api.TenantMemberCreateRequest{
		MemberID: tenant.Login,
		Role:     apiv2.TenantRole_TENANT_ROLE_OWNER,
	})
	if err != nil {
		return nil, err
	}

	return &adminv2.TenantServiceCreateResponse{Tenant: tenant}, nil
}

func (t *tenantServiceServer) List(ctx context.Context, req *adminv2.TenantServiceListRequest) (*adminv2.TenantServiceListResponse, error) {
	tenants, err := t.repo.Tenant().List(ctx, req.Query)
	if err != nil {
		return nil, err
	}

	return &adminv2.TenantServiceListResponse{Tenants: tenants}, nil
}

func (t *tenantServiceServer) AddMember(ctx context.Context, req *adminv2.TenantServiceAddMemberRequest) (*adminv2.TenantServiceAddMemberResponse, error) {
	tms, err := t.repo.Tenant().AdditionalMethods().Member(req.Tenant).List(ctx, &api.TenantMemberQuery{MemberId: &req.Member})

	if err != nil {
		return nil, errorutil.Internal("error reading tenant member:%v", err)
	}
	if len(tms) > 0 {
		return nil, errorutil.Conflict("tenant with id %q already is member in tenant: %q", req.Member, req.Tenant)
	}

	_, err = t.repo.Tenant().AdditionalMethods().Member(req.Tenant).Create(ctx, &api.TenantMemberCreateRequest{
		MemberID: req.Member,
		Role:     req.Role,
	})
	if err != nil {
		return nil, errorutil.Internal("failed to add member to tenant: %w", err)
	}

	t.log.Debug("member added successfully", "memberId", req.Member)
	return &adminv2.TenantServiceAddMemberResponse{}, nil
}
func (t *tenantServiceServer) RemoveMember(ctx context.Context, req *adminv2.TenantServiceRemoveMemberRequest) (*adminv2.TenantServiceRemoveMemberResponse, error) {
	_, err := t.repo.Tenant().AdditionalMethods().Member(req.Tenant).Delete(ctx, req.Member)
	if err != nil {
		return nil, err
	}

	t.log.Debug("member removed successfully", "memberId", req.Member)
	return &adminv2.TenantServiceRemoveMemberResponse{}, nil
}
