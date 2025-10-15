package infra

import (
	"context"
	"log/slog"

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

// Get implements infrav2connect.SwitchServiceHandler.
func (s *switchServiceServer) Get(context.Context, *infrav2.SwitchServiceGetRequest) (*infrav2.SwitchServiceGetResponse, error) {
	panic("unimplemented")
}

func (s *switchServiceServer) Register(ctx context.Context, rq *infrav2.SwitchServiceRegisterRequest) (*infrav2.SwitchServiceRegisterResponse, error) {
	sw, err := s.repo.Switch().AdditionalMethods().Register(ctx, rq)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	converted, err := s.repo.Switch().ConvertToProto(ctx, sw)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return &infrav2.SwitchServiceRegisterResponse{Switch: converted}, nil
}

func (s *switchServiceServer) Heartbeat(ctx context.Context, rq *infrav2.SwitchServiceHeartbeatRequest) (*infrav2.SwitchServiceHeartbeatResponse, error) {
	panic("unimplemented")
}
