package filesystem

import (
	"context"
	"log/slog"

	"connectrpc.com/connect"
	"github.com/metal-stack/api-server/pkg/db/generic"
	"github.com/metal-stack/api-server/pkg/db/repository"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/api/go/metalstack/api/v2/apiv2connect"
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

func (f *filesystemServiceServer) Get(ctx context.Context, rq *connect.Request[apiv2.FilesystemServiceGetRequest]) (*connect.Response[apiv2.FilesystemServiceGetResponse], error) {
	req := rq.Msg
	resp, err := f.repo.FilesystemLayout().Get(ctx, req.Id)
	if err != nil {
		if generic.IsNotFound(err) {
			return nil, connect.NewError(connect.CodeNotFound, err)
		}
		return nil, err
	}

	fsl, err := f.repo.FilesystemLayout().ConvertToProto(resp)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&apiv2.FilesystemServiceGetResponse{
		FilesystemLayout: fsl,
	}), nil
}

func (f *filesystemServiceServer) List(ctx context.Context, rq *connect.Request[apiv2.FilesystemServiceListRequest]) (*connect.Response[apiv2.FilesystemServiceListResponse], error) {
	req := rq.Msg
	resp, err := f.repo.FilesystemLayout().List(ctx, req)
	if err != nil {
		if generic.IsNotFound(err) {
			return nil, connect.NewError(connect.CodeNotFound, err)
		}
		return nil, err
	}
	var fsls []*apiv2.FilesystemLayout
	for _, r := range resp {
		fsl, err := f.repo.FilesystemLayout().ConvertToProto(r)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
		fsls = append(fsls, fsl)
	}
	return connect.NewResponse(&apiv2.FilesystemServiceListResponse{
		FilesystemLayouts: fsls,
	}), nil
}

func (f *filesystemServiceServer) Match(ctx context.Context, rq *connect.Request[apiv2.FilesystemServiceMatchRequest]) (*connect.Response[apiv2.FilesystemServiceMatchResponse], error) {
	panic("unimplemented")
}

func (f *filesystemServiceServer) Try(ctx context.Context, rq *connect.Request[apiv2.FilesystemServiceTryRequest]) (*connect.Response[apiv2.FilesystemServiceTryResponse], error) {
	panic("unimplemented")
}
