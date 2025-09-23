package filesystem

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

	fsl, err := f.repo.FilesystemLayout().Get(ctx, req.Id)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return connect.NewResponse(&apiv2.FilesystemServiceGetResponse{
		FilesystemLayout: fsl,
	}), nil
}

func (f *filesystemServiceServer) List(ctx context.Context, rq *connect.Request[apiv2.FilesystemServiceListRequest]) (*connect.Response[apiv2.FilesystemServiceListResponse], error) {
	req := rq.Msg

	fsls, err := f.repo.FilesystemLayout().List(ctx, req)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return connect.NewResponse(&apiv2.FilesystemServiceListResponse{
		FilesystemLayouts: fsls,
	}), nil
}

func (f *filesystemServiceServer) Match(ctx context.Context, rq *connect.Request[apiv2.FilesystemServiceMatchRequest]) (*connect.Response[apiv2.FilesystemServiceMatchResponse], error) {
	req := rq.Msg

	switch match := req.Match.(type) {
	case *apiv2.FilesystemServiceMatchRequest_SizeAndImage:
		// call old school fsl try
	case *apiv2.FilesystemServiceMatchRequest_MachineAndFilesystemlayout:
		// call old school fsl match
	default:
		return nil, errorutil.InvalidArgument("given matchtype %T is unsupported", match)
	}

	panic("unimplemented")
}
