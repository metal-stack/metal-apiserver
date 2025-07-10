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
func (s *sizeServiceServer) Create(ctx context.Context, rq *connect.Request[adminv2.SizeServiceCreateRequest]) (*connect.Response[adminv2.SizeServiceCreateResponse], error) {
	size, err := s.repo.Size().Create(ctx, rq.Msg)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	converted, err := s.repo.Size().ConvertToProto(size)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return connect.NewResponse(&adminv2.SizeServiceCreateResponse{Size: converted}), nil
}

// Update implements adminv2connect.SizeServiceHandler.
func (s *sizeServiceServer) Update(ctx context.Context, rq *connect.Request[adminv2.SizeServiceUpdateRequest]) (*connect.Response[adminv2.SizeServiceUpdateResponse], error) {
	size, err := s.repo.Size().Update(ctx, rq.Msg.Id, rq.Msg)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	converted, err := s.repo.Size().ConvertToProto(size)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return connect.NewResponse(&adminv2.SizeServiceUpdateResponse{Size: converted}), nil
}

// Delete implements adminv2connect.SizeServiceHandler.
func (s *sizeServiceServer) Delete(ctx context.Context, rq *connect.Request[adminv2.SizeServiceDeleteRequest]) (*connect.Response[adminv2.SizeServiceDeleteResponse], error) {
	size, err := s.repo.Size().Delete(ctx, rq.Msg.Id)
	if err != nil {
		return nil, errorutil.Convert(err)
	}
	converted, err := s.repo.Size().ConvertToProto(size)
	if err != nil {
		return nil, errorutil.Convert(err)
	}
	return connect.NewResponse(&adminv2.SizeServiceDeleteResponse{Size: converted}), nil
}
