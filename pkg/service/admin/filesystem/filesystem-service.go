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
func (f *filesystemServiceServer) Create(ctx context.Context, rq *adminv2.FilesystemServiceCreateRequest) (*adminv2.FilesystemServiceCreateResponse, error) {
	fsl, err := f.repo.FilesystemLayout().Create(ctx, rq)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return &adminv2.FilesystemServiceCreateResponse{FilesystemLayout: fsl}, nil
}

// Delete implements adminv2connect.FilesystemServiceHandler.
func (f *filesystemServiceServer) Delete(ctx context.Context, rq *adminv2.FilesystemServiceDeleteRequest) (*adminv2.FilesystemServiceDeleteResponse, error) {
	fsl, err := f.repo.FilesystemLayout().Delete(ctx, rq.Id)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return &adminv2.FilesystemServiceDeleteResponse{FilesystemLayout: fsl}, nil
}

// Update implements adminv2connect.FilesystemServiceHandler.
func (f *filesystemServiceServer) Update(ctx context.Context, rq *adminv2.FilesystemServiceUpdateRequest) (*adminv2.FilesystemServiceUpdateResponse, error) {
	fsl, err := f.repo.FilesystemLayout().Update(ctx, rq.Id, rq)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return &adminv2.FilesystemServiceUpdateResponse{FilesystemLayout: fsl}, nil
}
