package sizeimageconstraint

import (
	"context"
	"log/slog"

	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/api/go/metalstack/api/v2/apiv2connect"
	"github.com/metal-stack/metal-apiserver/pkg/repository"
)

type Config struct {
	Log  *slog.Logger
	Repo *repository.Store
}

type sizeImageConstraintServiceServer struct {
	log  *slog.Logger
	repo *repository.Store
}

func New(c Config) apiv2connect.SizeImageConstraintServiceHandler {
	return &sizeImageConstraintServiceServer{
		log:  c.Log.WithGroup("sizeImageConstraintService"),
		repo: c.Repo,
	}
}

func (s *sizeImageConstraintServiceServer) Try(ctx context.Context, req *apiv2.SizeImageConstraintServiceTryRequest) (*apiv2.SizeImageConstraintServiceTryResponse, error) {
	err := s.repo.SizeImageConstraint().AdditionalMethods().Try(ctx, req)
	if err != nil {
		return nil, err
	}

	return &apiv2.SizeImageConstraintServiceTryResponse{}, nil
}
