package admin

import (
	"context"
	"log/slog"

	"connectrpc.com/connect"
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

func (i *ipServiceServer) List(ctx context.Context, rq *connect.Request[adminv2.IPServiceListRequest]) (*connect.Response[adminv2.IPServiceListResponse], error) {
	req := rq.Msg

	ips, err := i.repo.UnscopedIP().List(ctx, req.Query)
	if err != nil {
		return nil, err
	}

	return connect.NewResponse(&adminv2.IPServiceListResponse{
		Ips: ips,
	}), nil
}
