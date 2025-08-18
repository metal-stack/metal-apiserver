package admin

import (
	"context"
	"log/slog"

	"connectrpc.com/connect"
	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	"github.com/metal-stack/api/go/metalstack/admin/v2/adminv2connect"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	"github.com/metal-stack/metal-apiserver/pkg/repository"
)

type Config struct {
	Log  *slog.Logger
	Repo *repository.Store
}

type switchServiceServer struct {
	log  *slog.Logger
	repo *repository.Store
}

func New(c Config) adminv2connect.SwitchServiceHandler {
	return &switchServiceServer{
		log:  c.Log,
		repo: c.Repo,
	}
}

func (s *switchServiceServer) Update(ctx context.Context, rq *connect.Request[adminv2.SwitchServiceUpdateRequest]) (*connect.Response[adminv2.SwitchServiceUpdateResponse], error) {
	return nil, errorutil.Internal("not implemented")
}
