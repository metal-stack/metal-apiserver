package admin

import (
	"context"
	"log/slog"

	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	"github.com/metal-stack/api/go/metalstack/admin/v2/adminv2connect"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/repository"
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
	projects, err := p.repo.UnscopedProject().List(ctx, &apiv2.ProjectServiceListRequest{
		Tenant: req.Tenant,
		Labels: req.Labels,
	})

	if err != nil {
		return nil, err
	}

	return &adminv2.ProjectServiceListResponse{
		Projects: projects,
	}, nil
}
