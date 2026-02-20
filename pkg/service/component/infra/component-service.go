package infra

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	infrav2 "github.com/metal-stack/api/go/metalstack/infra/v2"
	"github.com/metal-stack/api/go/metalstack/infra/v2/infrav2connect"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	"github.com/metal-stack/metal-apiserver/pkg/repository"
	"github.com/metal-stack/metal-apiserver/pkg/token"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type Config struct {
	Log        *slog.Logger
	Repo       *repository.Store
	Expiration time.Duration
}

type componentServiceServer struct {
	log        *slog.Logger
	repo       *repository.Store
	expiration time.Duration
}

func New(config Config) infrav2connect.ComponentServiceHandler {
	return &componentServiceServer{
		log:        config.Log,
		repo:       config.Repo,
		expiration: config.Expiration,
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

	component := &apiv2.Component{
		Uuid:       uid.String(),
		Type:       req.Type,
		Identifier: req.Identifier,
		StartedAt:  req.StartedAt,
		Interval:   req.Interval,
		ReportedAt: timestamppb.Now(),
		Version:    req.Version,
		Token:      t,
	}

	_, err = c.repo.Component().Create(ctx, &repository.ComponentServiceCreateRequest{Component: component, Expiration: c.expiration})
	if err != nil {
		return nil, err
	}
	return &infrav2.ComponentServicePingResponse{}, nil
}
