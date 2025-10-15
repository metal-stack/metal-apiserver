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

type partitionServiceServer struct {
	log  *slog.Logger
	repo *repository.Store
}

func New(c Config) adminv2connect.PartitionServiceHandler {
	return &partitionServiceServer{
		log:  c.Log.WithGroup("adminPartitionService"),
		repo: c.Repo,
	}
}

// Create implements adminv2connect.PartitionServiceHandler.
func (p *partitionServiceServer) Create(ctx context.Context, rq *adminv2.PartitionServiceCreateRequest) (*adminv2.PartitionServiceCreateResponse, error) {
	image, err := p.repo.Partition().Create(ctx, rq)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	converted, err := p.repo.Partition().ConvertToProto(ctx, image)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return &adminv2.PartitionServiceCreateResponse{Partition: converted}, nil
}

// Delete implements adminv2connect.PartitionServiceHandler.
func (p *partitionServiceServer) Delete(ctx context.Context, rq *adminv2.PartitionServiceDeleteRequest) (*adminv2.PartitionServiceDeleteResponse, error) {
	partition, err := p.repo.Partition().Delete(ctx, rq.Id)
	if err != nil {
		return nil, errorutil.Convert(err)
	}
	converted, err := p.repo.Partition().ConvertToProto(ctx, partition)
	if err != nil {
		return nil, errorutil.Convert(err)
	}
	return &adminv2.PartitionServiceDeleteResponse{Partition: converted}, nil
}

// Update implements adminv2connect.PartitionServiceHandler.
func (p *partitionServiceServer) Update(ctx context.Context, rq *adminv2.PartitionServiceUpdateRequest) (*adminv2.PartitionServiceUpdateResponse, error) {
	partition, err := p.repo.Partition().Update(ctx, rq.Id, rq)
	if err != nil {
		return nil, errorutil.Convert(err)
	}
	converted, err := p.repo.Partition().ConvertToProto(ctx, partition)
	if err != nil {
		return nil, errorutil.Convert(err)
	}
	return &adminv2.PartitionServiceUpdateResponse{Partition: converted}, nil
}

// Capacity implements adminv2connect.PartitionServiceHandler.
func (p *partitionServiceServer) Capacity(ctx context.Context, rq *adminv2.PartitionServiceCapacityRequest) (*adminv2.PartitionServiceCapacityResponse, error) {
	// FIXME size reservations must be implemented to be able to calculate this
	panic("unimplemented")
}
