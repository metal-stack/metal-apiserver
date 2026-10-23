package admin

import (
	"context"
	"log/slog"

	"github.com/metal-stack/api/go/errorutil"
	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	"github.com/metal-stack/api/go/metalstack/admin/v2/adminv2connect"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/repository"
	"github.com/metal-stack/metal-lib/pkg/pointer"
)

type Config struct {
	Log  *slog.Logger
	Repo *repository.Store
}

type projectServiceServer struct {
	log  *slog.Logger
	repo *repository.Store
}

func New(c Config) adminv2connect.ProjectServiceHandler {
	return &projectServiceServer{
		log:  c.Log.WithGroup("adminProjectService"),
		repo: c.Repo,
	}
}

func (p *projectServiceServer) List(ctx context.Context, req *adminv2.ProjectServiceListRequest) (*adminv2.ProjectServiceListResponse, error) {
	projects, err := p.repo.UnscopedProject().List(ctx, req.Query)

	if err != nil {
		return nil, err
	}

	return &adminv2.ProjectServiceListResponse{
		Projects: projects,
	}, nil
}

func (p *projectServiceServer) Create(ctx context.Context, req *adminv2.ProjectServiceCreateRequest) (*adminv2.ProjectServiceCreateResponse, error) {
	project, err := p.repo.UnscopedProject().AdditionalMethods().CreateWithID(ctx, req.Project, pointer.SafeDeref(req.Id))
	if err != nil {
		return nil, err
	}

	converted, err := p.repo.UnscopedProject().Get(ctx, project.Meta.Id)
	if err != nil {
		return nil, err
	}

	return &adminv2.ProjectServiceCreateResponse{
		Project: converted,
	}, nil
}

func (p *projectServiceServer) AddMember(ctx context.Context, req *adminv2.ProjectServiceAddMemberRequest) (*adminv2.ProjectServiceAddMemberResponse, error) {
	member, err := p.repo.ProjectMember(req.Project).Create(ctx, &apiv2.ProjectMemberServiceCreateRequest{
		Project: req.Project,
		Member:  req.Member,
		Role:    req.Role,
	})
	if err != nil {
		return nil, errorutil.Internal("failed to add member to project: %w", err)
	}

	p.log.Debug("member added successfully", "memberId", req.Member)

	return &adminv2.ProjectServiceAddMemberResponse{
		Member: member,
	}, nil
}

func (p *projectServiceServer) RemoveMember(ctx context.Context, req *adminv2.ProjectServiceRemoveMemberRequest) (*adminv2.ProjectServiceRemoveMemberResponse, error) {
	member, err := p.repo.ProjectMember(req.Project).Delete(ctx, req.Member)
	if err != nil {
		return nil, err
	}

	p.log.Debug("member removed successfully", "memberId", req.Member)

	return &adminv2.ProjectServiceRemoveMemberResponse{
		Member: member,
	}, nil
}
