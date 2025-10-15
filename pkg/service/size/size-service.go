package size

import (
	"context"
	"log/slog"

	"connectrpc.com/connect"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/api/go/metalstack/api/v2/apiv2connect"
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

func New(c Config) apiv2connect.SizeServiceHandler {
	return &sizeServiceServer{
		log:  c.Log.WithGroup("sizeService"),
		repo: c.Repo,
	}
}

// Get implements apiv2connect.SizeServiceHandler.
func (s *sizeServiceServer) Get(ctx context.Context, rq *connect.Request[apiv2.SizeServiceGetRequest]) (*connect.Response[apiv2.SizeServiceGetResponse], error) {
	size, err := s.repo.Size().Get(ctx, rq.Msg.Id)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	converted, err := s.repo.Size().ConvertToProto(ctx, size)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return connect.NewResponse(&apiv2.SizeServiceGetResponse{Size: converted}), nil
}

// List implements apiv2connect.SizeServiceHandler.
func (s *sizeServiceServer) List(ctx context.Context, rq *connect.Request[apiv2.SizeServiceListRequest]) (*connect.Response[apiv2.SizeServiceListResponse], error) {
	sizes, err := s.repo.Size().List(ctx, rq.Msg.Query)
	if err != nil {
		return nil, errorutil.Convert(err)
	}
	var result []*apiv2.Size
	for _, size := range sizes {
		converted, err := s.repo.Size().ConvertToProto(ctx, size)
		if err != nil {
			return nil, errorutil.Convert(err)
		}
		result = append(result, converted)
	}

	return connect.NewResponse(&apiv2.SizeServiceListResponse{Sizes: result}), nil
}
