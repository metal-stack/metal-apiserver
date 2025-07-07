package size

import (
	"context"
	"log/slog"

	"connectrpc.com/connect"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/api/go/metalstack/api/v2/apiv2connect"
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

func New(c Config) apiv2connect.SizeServiceHandler {
	return &sizeServiceServer{
		log:  c.Log.WithGroup("sizeService"),
		repo: c.Repo,
	}
}

// Get implements apiv2connect.SizeServiceHandler.
func (s *sizeServiceServer) Get(context.Context, *connect.Request[apiv2.SizeServiceGetRequest]) (*connect.Response[apiv2.SizeServiceGetResponse], error) {
	panic("unimplemented")
}

// List implements apiv2connect.SizeServiceHandler.
func (s *sizeServiceServer) List(context.Context, *connect.Request[apiv2.SizeServiceListRequest]) (*connect.Response[apiv2.SizeServiceListResponse], error) {
	panic("unimplemented")
}
