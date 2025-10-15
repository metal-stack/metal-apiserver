package infra

import (
	"context"
	"log/slog"

	"connectrpc.com/connect"
	infrav2 "github.com/metal-stack/api/go/metalstack/infra/v2"
	"github.com/metal-stack/api/go/metalstack/infra/v2/infrav2connect"
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

func New(c Config) infrav2connect.SwitchServiceHandler {
	return &switchServiceServer{
		log:  c.Log.WithGroup("switchService"),
		repo: c.Repo,
	}
}

func (s *switchServiceServer) Register(ctx context.Context, rq *connect.Request[infrav2.SwitchServiceRegisterRequest]) (*connect.Response[infrav2.SwitchServiceRegisterResponse], error) {
	sw, err := s.repo.Switch().AdditionalMethods().Register(ctx, rq.Msg)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return connect.NewResponse(&infrav2.SwitchServiceRegisterResponse{Switch: sw}), nil
}

func (s *switchServiceServer) Heartbeat(ctx context.Context, rq *connect.Request[infrav2.SwitchServiceHeartbeatRequest]) (*connect.Response[infrav2.SwitchServiceHeartbeatResponse], error) {
	panic("unimplemented")
}
