package admin

import (
	"context"
	"log/slog"

	"connectrpc.com/connect"
	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	"github.com/metal-stack/api/go/metalstack/admin/v2/adminv2connect"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	"github.com/metal-stack/metal-apiserver/pkg/repository"
)

type Config struct {
	Log  *slog.Logger
	Repo *repository.Store
}

type imageServiceServer struct {
	log  *slog.Logger
	repo *repository.Store
}

func New(c Config) adminv2connect.ImageServiceHandler {
	return &imageServiceServer{
		log:  c.Log.WithGroup("adminImageService"),
		repo: c.Repo,
	}
}

// Create implements adminv2connect.ImageServiceHandler.
func (i *imageServiceServer) Create(ctx context.Context, rq *connect.Request[adminv2.ImageServiceCreateRequest]) (*connect.Response[adminv2.ImageServiceCreateResponse], error) {
	validated, err := i.repo.Image().ValidateCreate(ctx, rq.Msg)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	image, err := i.repo.Image().Create(ctx, validated)
	if err != nil {
		return nil, errorutil.Convert(err)
	}
	converted, err := i.repo.Image().ConvertToProto(image)
	if err != nil {
		return nil, errorutil.Convert(err)
	}
	return connect.NewResponse(&adminv2.ImageServiceCreateResponse{Image: converted}), nil
}

// Delete implements adminv2connect.ImageServiceHandler.
func (i *imageServiceServer) Delete(ctx context.Context, rq *connect.Request[adminv2.ImageServiceDeleteRequest]) (*connect.Response[adminv2.ImageServiceDeleteResponse], error) {
	validated, err := i.repo.Image().ValidateDelete(ctx, &metal.Image{Base: metal.Base{ID: rq.Msg.Id}})
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	image, err := i.repo.Image().Delete(ctx, validated)
	if err != nil {
		return nil, errorutil.Convert(err)
	}
	converted, err := i.repo.Image().ConvertToProto(image)
	if err != nil {
		return nil, errorutil.Convert(err)
	}
	return connect.NewResponse(&adminv2.ImageServiceDeleteResponse{Image: converted}), nil
}

// Update implements adminv2connect.ImageServiceHandler.
func (i *imageServiceServer) Update(ctx context.Context, rq *connect.Request[adminv2.ImageServiceUpdateRequest]) (*connect.Response[adminv2.ImageServiceUpdateResponse], error) {
	validated, err := i.repo.Image().ValidateUpdate(ctx, rq.Msg)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	image, err := i.repo.Image().Update(ctx, validated)
	if err != nil {
		return nil, errorutil.Convert(err)
	}
	converted, err := i.repo.Image().ConvertToProto(image)
	if err != nil {
		return nil, errorutil.Convert(err)
	}
	return connect.NewResponse(&adminv2.ImageServiceUpdateResponse{Image: converted}), nil
}

// Usage implements adminv2connect.ImageServiceHandler.
func (i *imageServiceServer) Usage(ctx context.Context, rq *connect.Request[adminv2.ImageServiceUsageRequest]) (*connect.Response[adminv2.ImageServiceUsageResponse], error) {
	panic("unimplemented")
}
