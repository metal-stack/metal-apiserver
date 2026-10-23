package projectmember

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

	projectMemberServiceServer struct {
		log  *slog.Logger
		repo *repository.Store
	}
)

func New(c Config) apiv2connect.ProjectMemberServiceHandler {
	return &projectMemberServiceServer{
		log:  c.Log.WithGroup("projectMemberService"),
		repo: c.Repo,
	}
}

func (p *projectMemberServiceServer) Create(ctx context.Context, req *apiv2.ProjectMemberServiceCreateRequest) (*apiv2.ProjectMemberServiceCreateResponse, error) {
	member, err := p.repo.ProjectMember(req.Project).Create(ctx, req)
	if err != nil {
		return nil, err
	}

	return &apiv2.ProjectMemberServiceCreateResponse{
		Member: member,
	}, nil
}

func (p *projectMemberServiceServer) Delete(ctx context.Context, req *apiv2.ProjectMemberServiceDeleteRequest) (*apiv2.ProjectMemberServiceDeleteResponse, error) {
	member, err := p.repo.ProjectMember(req.Project).Delete(ctx, req.Member)
	if err != nil {
		return nil, err
	}

	return &apiv2.ProjectMemberServiceDeleteResponse{
		Member: member,
	}, nil
}

func (p *projectMemberServiceServer) Get(ctx context.Context, req *apiv2.ProjectMemberServiceGetRequest) (*apiv2.ProjectMemberServiceGetResponse, error) {
	member, err := p.repo.ProjectMember(req.Project).Get(ctx, req.Member)
	if err != nil {
		return nil, err
	}

	return &apiv2.ProjectMemberServiceGetResponse{
		Member: member,
	}, nil
}

func (p *projectMemberServiceServer) Leave(ctx context.Context, req *apiv2.ProjectMemberServiceLeaveRequest) (*apiv2.ProjectMemberServiceLeaveResponse, error) {
	tok, ok := token.TokenFromContext(ctx)
	if !ok || tok == nil {
		return nil, errorutil.Unauthenticated("no token found in request")
	}

	member, err := p.repo.ProjectMember(req.Project).Delete(ctx, tok.User)
	if err != nil {
		return nil, err
	}

	return &apiv2.ProjectMemberServiceLeaveResponse{
		Member: member,
	}, nil
}

func (p *projectMemberServiceServer) List(ctx context.Context, req *apiv2.ProjectMemberServiceListRequest) (*apiv2.ProjectMemberServiceListResponse, error) {
	members, err := p.repo.ProjectMember(req.Project).List(ctx, req.Query)
	if err != nil {
		return nil, err
	}

	return &apiv2.ProjectMemberServiceListResponse{
		Members: members,
	}, nil
}

func (p *projectMemberServiceServer) Update(ctx context.Context, req *apiv2.ProjectMemberServiceUpdateRequest) (*apiv2.ProjectMemberServiceUpdateResponse, error) {
	member, err := p.repo.ProjectMember(req.Project).Update(ctx, req.Member, req)
	if err != nil {
		return nil, err
	}

	return &apiv2.ProjectMemberServiceUpdateResponse{
		Member: member,
	}, nil
}
