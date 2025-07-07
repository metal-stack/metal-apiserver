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
func (s *sizeServiceServer) Create(context.Context, *connect.Request[adminv2.SizeServiceCreateRequest]) (*connect.Response[adminv2.SizeServiceCreateResponse], error) {
	panic("unimplemented")
}

// Delete implements adminv2connect.SizeServiceHandler.
func (s *sizeServiceServer) Delete(context.Context, *connect.Request[adminv2.SizeServiceDeleteRequest]) (*connect.Response[adminv2.SizeServiceDeleteResponse], error) {
	panic("unimplemented")
}
