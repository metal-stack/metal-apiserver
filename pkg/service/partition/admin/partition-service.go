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
func (p *partitionServiceServer) Create(ctx context.Context, rq *connect.Request[adminv2.PartitionServiceCreateRequest]) (*connect.Response[adminv2.PartitionServiceCreateResponse], error) {
	partition, err := p.repo.Partition().Create(ctx, rq.Msg)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return connect.NewResponse(&adminv2.PartitionServiceCreateResponse{Partition: partition}), nil
}

// Delete implements adminv2connect.PartitionServiceHandler.
func (p *partitionServiceServer) Delete(ctx context.Context, rq *connect.Request[adminv2.PartitionServiceDeleteRequest]) (*connect.Response[adminv2.PartitionServiceDeleteResponse], error) {
	partition, err := p.repo.Partition().Delete(ctx, rq.Msg.Id)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return connect.NewResponse(&adminv2.PartitionServiceDeleteResponse{Partition: partition}), nil
}

// Update implements adminv2connect.PartitionServiceHandler.
func (p *partitionServiceServer) Update(ctx context.Context, rq *connect.Request[adminv2.PartitionServiceUpdateRequest]) (*connect.Response[adminv2.PartitionServiceUpdateResponse], error) {
	partition, err := p.repo.Partition().Update(ctx, rq.Msg.Id, rq.Msg)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return connect.NewResponse(&adminv2.PartitionServiceUpdateResponse{Partition: partition}), nil
}

// Capacity implements adminv2connect.PartitionServiceHandler.
func (p *partitionServiceServer) Capacity(ctx context.Context, rq *connect.Request[adminv2.PartitionServiceCapacityRequest]) (*connect.Response[adminv2.PartitionServiceCapacityResponse], error) {
	// FIXME size reservations must be implemented to be able to calculate this
	panic("unimplemented")
}
