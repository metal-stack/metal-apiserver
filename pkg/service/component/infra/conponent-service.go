package infra

import (
	"context"
	"log/slog"

	"github.com/google/uuid"
	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	infrav2 "github.com/metal-stack/api/go/metalstack/infra/v2"
	"github.com/metal-stack/api/go/metalstack/infra/v2/infrav2connect"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	"github.com/metal-stack/metal-apiserver/pkg/repository"
	"github.com/metal-stack/metal-apiserver/pkg/token"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type Config struct {
	Log  *slog.Logger
	Repo *repository.Store
}

type componentServiceServer struct {
	log  *slog.Logger
	repo *repository.Store
}

func New(config Config) infrav2connect.ComponentServiceHandler {
	return &componentServiceServer{
		log:  config.Log,
		repo: config.Repo,
	}
}

func (c *componentServiceServer) Ping(ctx context.Context, req *infrav2.ComponentServicePingRequest) (*infrav2.ComponentServicePingResponse, error) {
	var (
		t, ok = token.TokenFromContext(ctx)
	)

	if !ok || t == nil {
		return nil, errorutil.Unauthenticated("no token found in request")
	}

	uid, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}

	component := &adminv2.Component{
		Uuid:       uid.String(),
		Type:       req.Type,
		Identifier: req.Identifier,
		StartedAt:  req.StartedAt,
		Interval:   req.Interval,
		ReportedAt: timestamppb.Now(),
		Version:    req.Version,
		Token:      t,
	}

	_, err = c.repo.Component().Create(ctx, component)
	if err != nil {
		return nil, err
	}
	return &infrav2.ComponentServicePingResponse{}, nil
}
