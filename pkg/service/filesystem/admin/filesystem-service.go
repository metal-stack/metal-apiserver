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

type filesystemServiceServer struct {
	log  *slog.Logger
	repo *repository.Store
}

func New(c Config) adminv2connect.FilesystemServiceHandler {
	return &filesystemServiceServer{
		log:  c.Log.WithGroup("adminFilesystemService"),
		repo: c.Repo,
	}
}

// Create implements adminv2connect.FilesystemServiceHandler.
func (f *filesystemServiceServer) Create(ctx context.Context, rq *connect.Request[adminv2.FilesystemServiceCreateRequest]) (*connect.Response[adminv2.FilesystemServiceCreateResponse], error) {
	validated, err := f.repo.FilesystemLayout().ValidateCreate(ctx, rq.Msg)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	fsl, err := f.repo.FilesystemLayout().Create(ctx, validated)
	if err != nil {
		return nil, errorutil.Convert(err)
	}
	converted, err := f.repo.FilesystemLayout().ConvertToProto(fsl)
	if err != nil {
		return nil, errorutil.Convert(err)
	}
	return connect.NewResponse(&adminv2.FilesystemServiceCreateResponse{FilesystemLayout: converted}), nil
}

// Delete implements adminv2connect.FilesystemServiceHandler.
func (f *filesystemServiceServer) Delete(ctx context.Context, rq *connect.Request[adminv2.FilesystemServiceDeleteRequest]) (*connect.Response[adminv2.FilesystemServiceDeleteResponse], error) {
	validated, err := f.repo.FilesystemLayout().ValidateDelete(ctx, &metal.FilesystemLayout{Base: metal.Base{ID: rq.Msg.Id}})
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	fsl, err := f.repo.FilesystemLayout().Delete(ctx, validated)
	if err != nil {
		return nil, errorutil.Convert(err)
	}
	converted, err := f.repo.FilesystemLayout().ConvertToProto(fsl)
	if err != nil {
		return nil, errorutil.Convert(err)
	}
	return connect.NewResponse(&adminv2.FilesystemServiceDeleteResponse{FilesystemLayout: converted}), nil
}

// Update implements adminv2connect.FilesystemServiceHandler.
func (f *filesystemServiceServer) Update(ctx context.Context, rq *connect.Request[adminv2.FilesystemServiceUpdateRequest]) (*connect.Response[adminv2.FilesystemServiceUpdateResponse], error) {
	validated, err := f.repo.FilesystemLayout().ValidateUpdate(ctx, rq.Msg)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	fsl, err := f.repo.FilesystemLayout().Update(ctx, validated)
	if err != nil {
		return nil, errorutil.Convert(err)
	}
	converted, err := f.repo.FilesystemLayout().ConvertToProto(fsl)
	if err != nil {
		return nil, errorutil.Convert(err)
	}
	return connect.NewResponse(&adminv2.FilesystemServiceUpdateResponse{FilesystemLayout: converted}), nil
}
