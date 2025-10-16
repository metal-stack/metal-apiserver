package admin

import (
	"context"
	"log/slog"

	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	"github.com/metal-stack/api/go/metalstack/admin/v2/adminv2connect"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	"github.com/metal-stack/metal-apiserver/pkg/repository"
)

type Config struct {
	Log  *slog.Logger
	Repo *repository.Store
}

type sizeServiceServer struct {
	log  *slog.Logger
	repo *repository.Store
}

func New(c Config) adminv2connect.SizeServiceHandler {
	return &sizeServiceServer{
		log:  c.Log.WithGroup("adminSizeService"),
		repo: c.Repo,
	}
}

// Create implements adminv2connect.SizeServiceHandler.
func (s *sizeServiceServer) Create(ctx context.Context, rq *adminv2.SizeServiceCreateRequest) (*adminv2.SizeServiceCreateResponse, error) {
	size, err := s.repo.Size().Create(ctx, rq)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return &adminv2.SizeServiceCreateResponse{Size: size}, nil
}

// Update implements adminv2connect.SizeServiceHandler.
func (s *sizeServiceServer) Update(ctx context.Context, rq *adminv2.SizeServiceUpdateRequest) (*adminv2.SizeServiceUpdateResponse, error) {
	size, err := s.repo.Size().Update(ctx, rq.Id, rq)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return &adminv2.SizeServiceUpdateResponse{Size: size}, nil
}

// Delete implements adminv2connect.SizeServiceHandler.
func (s *sizeServiceServer) Delete(ctx context.Context, rq *adminv2.SizeServiceDeleteRequest) (*adminv2.SizeServiceDeleteResponse, error) {
	size, err := s.repo.Size().Delete(ctx, rq.Id)
	if err != nil {
		return nil, errorutil.Convert(err)
	}
	return &adminv2.SizeServiceDeleteResponse{Size: size}, nil
}
