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
	p.log.Debug("create", "msg", rq.Msg)

	image, err := p.repo.Partition().Create(ctx, rq.Msg)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	converted, err := p.repo.Partition().ConvertToProto(image)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return connect.NewResponse(&adminv2.PartitionServiceCreateResponse{Partition: converted}), nil
}

// Delete implements adminv2connect.PartitionServiceHandler.
func (p *partitionServiceServer) Delete(ctx context.Context, rq *connect.Request[adminv2.PartitionServiceDeleteRequest]) (*connect.Response[adminv2.PartitionServiceDeleteResponse], error) {
	p.log.Debug("delete", "msg", rq.Msg)

	partition, err := p.repo.Partition().Delete(ctx, rq.Msg.Id)
	if err != nil {
		return nil, errorutil.Convert(err)
	}
	converted, err := p.repo.Partition().ConvertToProto(partition)
	if err != nil {
		return nil, errorutil.Convert(err)
	}
	return connect.NewResponse(&adminv2.PartitionServiceDeleteResponse{Partition: converted}), nil
}

// Update implements adminv2connect.PartitionServiceHandler.
func (p *partitionServiceServer) Update(ctx context.Context, rq *connect.Request[adminv2.PartitionServiceUpdateRequest]) (*connect.Response[adminv2.PartitionServiceUpdateResponse], error) {
	p.log.Debug("update", "msg", rq.Msg)

	partition, err := p.repo.Partition().Update(ctx, rq.Msg.Partition.Id, rq.Msg)
	if err != nil {
		return nil, errorutil.Convert(err)
	}
	converted, err := p.repo.Partition().ConvertToProto(partition)
	if err != nil {
		return nil, errorutil.Convert(err)
	}
	return connect.NewResponse(&adminv2.PartitionServiceUpdateResponse{Partition: converted}), nil
}

// Capacity implements adminv2connect.PartitionServiceHandler.
func (p *partitionServiceServer) Capacity(ctx context.Context, rq *connect.Request[adminv2.PartitionServiceCapacityRequest]) (*connect.Response[adminv2.PartitionServiceCapacityResponse], error) {
	panic("unimplemented")
}
