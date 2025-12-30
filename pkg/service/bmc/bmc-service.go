package bmc

import (
	"context"
	"log/slog"

	"connectrpc.com/connect"
	infrav2 "github.com/metal-stack/api/go/metalstack/infra/v2"
	"github.com/metal-stack/api/go/metalstack/infra/v2/infrav2connect"
	"github.com/metal-stack/metal-apiserver/pkg/repository"
)

type Config struct {
	Log  *slog.Logger
	Repo *repository.Store
}

type bmcServiceServer struct {
	log  *slog.Logger
	repo *repository.Store
}

func New(c Config) infrav2connect.BMCServiceHandler {
	return &bmcServiceServer{
		log:  c.Log.WithGroup("bmcService"),
		repo: c.Repo,
	}
}

func (b *bmcServiceServer) UpdateBMCInfo(ctx context.Context, req *infrav2.UpdateBMCInfoRequest) (*infrav2.UpdateBMCInfoResponse, error) {
	return b.repo.UnscopedMachine().AdditionalMethods().UpdateBMCInfo(ctx, req)
}

func (b *bmcServiceServer) WaitForBMCCommand(ctx context.Context, req *infrav2.WaitForBMCCommandRequest, srv *connect.ServerStream[infrav2.WaitForBMCCommandResponse]) error {
	b.log.Info("waitforbmccommand", "req", req)
	return b.repo.UnscopedMachine().AdditionalMethods().WaitForBMCCommand(ctx, req, srv)
}
