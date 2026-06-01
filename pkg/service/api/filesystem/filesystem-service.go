package filesystem

import (
	"context"
	"log/slog"

	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/api/go/metalstack/api/v2/apiv2connect"
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

func New(c Config) apiv2connect.FilesystemServiceHandler {
	return &filesystemServiceServer{
		log:  c.Log.WithGroup("filesystemService"),
		repo: c.Repo,
	}
}

func (f *filesystemServiceServer) Get(ctx context.Context, req *apiv2.FilesystemServiceGetRequest) (*apiv2.FilesystemServiceGetResponse, error) {
	fsl, err := f.repo.FilesystemLayout().Get(ctx, req.Id)
	if err != nil {
		return nil, err
	}

	return &apiv2.FilesystemServiceGetResponse{
		FilesystemLayout: fsl,
	}, nil
}

func (f *filesystemServiceServer) List(ctx context.Context, req *apiv2.FilesystemServiceListRequest) (*apiv2.FilesystemServiceListResponse, error) {
	fsls, err := f.repo.FilesystemLayout().List(ctx, req)
	if err != nil {
		return nil, err
	}
	return &apiv2.FilesystemServiceListResponse{
		FilesystemLayouts: fsls,
	}, nil
}
