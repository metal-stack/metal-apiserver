package partition

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

type partitionServiceServer struct {
	log  *slog.Logger
	repo *repository.Store
}

func New(c Config) apiv2connect.PartitionServiceHandler {
	return &partitionServiceServer{
		log:  c.Log.WithGroup("partitionService"),
		repo: c.Repo,
	}
}

// Get implements apiv2connect.PartitionServiceHandler.
func (p *partitionServiceServer) Get(ctx context.Context, rq *connect.Request[apiv2.PartitionServiceGetRequest]) (*connect.Response[apiv2.PartitionServiceGetResponse], error) {
	partition, err := p.repo.Partition().Get(ctx, rq.Msg.Id)
	if err != nil {
		return nil, errorutil.Convert(err)
	}
	converted, err := p.repo.Partition().ConvertToProto(ctx, partition)
	if err != nil {
		return nil, errorutil.Convert(err)
	}
	return connect.NewResponse(&apiv2.PartitionServiceGetResponse{Partition: converted}), nil
}

// List implements apiv2connect.PartitionServiceHandler.
func (p *partitionServiceServer) List(ctx context.Context, rq *connect.Request[apiv2.PartitionServiceListRequest]) (*connect.Response[apiv2.PartitionServiceListResponse], error) {
	partitions, err := p.repo.Partition().List(ctx, rq.Msg.Query)
	if err != nil {
		return nil, errorutil.Convert(err)
	}
	var result []*apiv2.Partition

	for _, partition := range partitions {
		converted, err := p.repo.Partition().ConvertToProto(ctx, partition)
		if err != nil {
			return nil, errorutil.Convert(err)
		}
		result = append(result, converted)
	}

	return connect.NewResponse(&apiv2.PartitionServiceListResponse{Partitions: result}), nil

}
