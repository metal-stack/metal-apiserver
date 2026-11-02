package tenantmember

import (
	"context"
	"log/slog"

	"github.com/metal-stack/api/go/errorutil"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/api/go/metalstack/api/v2/apiv2connect"
	"github.com/metal-stack/metal-apiserver/pkg/repository"
	"github.com/metal-stack/metal-apiserver/pkg/token"
)

type (
	Config struct {
		Log  *slog.Logger
		Repo *repository.Store
	}

	tenantMemberServiceServer struct {
		log  *slog.Logger
		repo *repository.Store
	}
)

func New(c Config) apiv2connect.TenantMemberServiceHandler {
	return &tenantMemberServiceServer{
		log:  c.Log.WithGroup("tenantMemberService"),
		repo: c.Repo,
	}
}

func (t *tenantMemberServiceServer) Create(ctx context.Context, req *apiv2.TenantMemberServiceCreateRequest) (*apiv2.TenantMemberServiceCreateResponse, error) {
	member, err := t.repo.TenantMember(req.Login).Create(ctx, req)
	if err != nil {
		return nil, err
	}

	return &apiv2.TenantMemberServiceCreateResponse{
		Member: member,
	}, nil
}

func (t *tenantMemberServiceServer) Delete(ctx context.Context, req *apiv2.TenantMemberServiceDeleteRequest) (*apiv2.TenantMemberServiceDeleteResponse, error) {
	member, err := t.repo.TenantMember(req.Login).Delete(ctx, req.Member)
	if err != nil {
		return nil, err
	}

	return &apiv2.TenantMemberServiceDeleteResponse{
		Member: member,
	}, nil
}

func (t *tenantMemberServiceServer) Get(ctx context.Context, req *apiv2.TenantMemberServiceGetRequest) (*apiv2.TenantMemberServiceGetResponse, error) {
	member, err := t.repo.TenantMember(req.Login).Get(ctx, req.Member)
	if err != nil {
		return nil, err
	}

	return &apiv2.TenantMemberServiceGetResponse{
		Member: member,
	}, nil
}

func (t *tenantMemberServiceServer) Leave(ctx context.Context, req *apiv2.TenantMemberServiceLeaveRequest) (*apiv2.TenantMemberServiceLeaveResponse, error) {
	tok, ok := token.TokenFromContext(ctx)
	if !ok || tok == nil {
		return nil, errorutil.Unauthenticated("no token found in request")
	}

	member, err := t.repo.TenantMember(req.Login).Delete(ctx, tok.User)
	if err != nil {
		return nil, err
	}

	return &apiv2.TenantMemberServiceLeaveResponse{
		Member: member,
	}, nil
}

func (t *tenantMemberServiceServer) List(ctx context.Context, req *apiv2.TenantMemberServiceListRequest) (*apiv2.TenantMemberServiceListResponse, error) {
	members, err := t.repo.TenantMember(req.Login).List(ctx, req.Query)
	if err != nil {
		return nil, err
	}

	return &apiv2.TenantMemberServiceListResponse{
		Members: members,
	}, nil
}

func (t *tenantMemberServiceServer) Update(ctx context.Context, req *apiv2.TenantMemberServiceUpdateRequest) (*apiv2.TenantMemberServiceUpdateResponse, error) {
	member, err := t.repo.TenantMember(req.Login).Update(ctx, req.Member, req)
	if err != nil {
		return nil, err
	}

	return &apiv2.TenantMemberServiceUpdateResponse{
		Member: member,
	}, nil
}
