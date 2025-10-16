package admin

import (
	"context"
	"log/slog"

	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	"github.com/metal-stack/api/go/metalstack/admin/v2/adminv2connect"
	"github.com/metal-stack/metal-apiserver/pkg/repository"
)

type Config struct {
	Log  *slog.Logger
	Repo *repository.Store
}

type ipServiceServer struct {
	log  *slog.Logger
	repo *repository.Store
}

func New(c Config) adminv2connect.IPServiceHandler {
	return &ipServiceServer{
		log:  c.Log.WithGroup("adminIpService"),
		repo: c.Repo,
	}
}

func (i *ipServiceServer) List(ctx context.Context, rq *adminv2.IPServiceListRequest) (*adminv2.IPServiceListResponse, error) {
	req := rq

	ips, err := i.repo.UnscopedIP().List(ctx, req.Query)
	if err != nil {
		return nil, err
	}

	return &adminv2.IPServiceListResponse{
		Ips: ips,
	}, nil
}
