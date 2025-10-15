package partition

import (
	"context"
	"log/slog"

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
func (p *partitionServiceServer) Get(ctx context.Context, rq *apiv2.PartitionServiceGetRequest) (*apiv2.PartitionServiceGetResponse, error) {
	partition, err := p.repo.Partition().Get(ctx, rq.Id)
	if err != nil {
		return nil, errorutil.Convert(err)
	}
	converted, err := p.repo.Partition().ConvertToProto(ctx, partition)
	if err != nil {
		return nil, errorutil.Convert(err)
	}
	return &apiv2.PartitionServiceGetResponse{Partition: converted}, nil
}

// List implements apiv2connect.PartitionServiceHandler.
func (p *partitionServiceServer) List(ctx context.Context, rq *apiv2.PartitionServiceListRequest) (*apiv2.PartitionServiceListResponse, error) {
	partitions, err := p.repo.Partition().List(ctx, rq.Query)
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

	return &apiv2.PartitionServiceListResponse{Partitions: result}, nil

}
