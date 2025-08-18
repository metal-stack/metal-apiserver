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
	panic("unimplemented")
}

func (s *switchServiceServer) Delete(ctx context.Context, rq *connect.Request[adminv2.SwitchServiceDeleteRequest]) (*connect.Response[adminv2.SwitchServiceDeleteResponse], error) {
	panic("unimplemented")
}

func (s *switchServiceServer) Migrate(ctx context.Context, rq *connect.Request[adminv2.SwitchServiceMigrateRequest]) (*connect.Response[adminv2.SwitchServiceMigrateResponse], error) {
	panic("unimplemented")
}

func (s *switchServiceServer) Port(ctx context.Context, rq *connect.Request[adminv2.SwitchServicePortRequest]) (*connect.Response[adminv2.SwitchServicePortResponse], error) {
	panic("unimplemented")
}
