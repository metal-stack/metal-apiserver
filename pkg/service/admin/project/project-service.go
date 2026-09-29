package admin

import (
	"context"
	"log/slog"

	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	"github.com/metal-stack/api/go/metalstack/admin/v2/adminv2connect"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/repository"
	"github.com/metal-stack/metal-apiserver/pkg/repository/api"
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

func (p *projectServiceServer) Create(ctx context.Context, req *adminv2.ProjectServiceCreateRequest) (*adminv2.ProjectServiceCreateResponse, error) {
	project, err := p.repo.UnscopedProject().AdditionalMethods().CreateWithID(ctx, &apiv2.ProjectServiceCreateRequest{
		Login:       req.Login,
		Name:        req.Name,
		Description: req.Description,
		AvatarUrl:   req.AvatarUrl,
		Labels:      req.Labels,
	}, pointer.SafeDeref(req.Project))
	if err != nil {
		return nil, err
	}

	_, err = p.repo.Project(project.Uuid).AdditionalMethods().Member().Create(ctx, &api.ProjectMemberCreateRequest{
		TenantId: req.Login,
		Role:     apiv2.ProjectRole_PROJECT_ROLE_OWNER,
	})
	if err != nil {
		return nil, err
	}

	return &adminv2.ProjectServiceCreateResponse{Project: project}, nil
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
