package image

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

type imageServiceServer struct {
	log  *slog.Logger
	repo *repository.Store
}

func New(c Config) apiv2connect.ImageServiceHandler {
	return &imageServiceServer{
		log:  c.Log.WithGroup("imageService"),
		repo: c.Repo,
	}
}

// Get implements apiv2connect.ImageServiceHandler.
func (i *imageServiceServer) Get(ctx context.Context, rq *connect.Request[apiv2.ImageServiceGetRequest]) (*connect.Response[apiv2.ImageServiceGetResponse], error) {
	image, err := i.repo.Image().Get(ctx, rq.Msg.Id)
	if err != nil {
		return nil, errorutil.Convert(err)
	}
	converted, err := i.repo.Image().ConvertToProto(image)
	if err != nil {
		return nil, errorutil.Convert(err)
	}
	return connect.NewResponse(&apiv2.ImageServiceGetResponse{Image: converted}), nil
}

// List implements apiv2connect.ImageServiceHandler.
func (i *imageServiceServer) List(ctx context.Context, rq *connect.Request[apiv2.ImageServiceListRequest]) (*connect.Response[apiv2.ImageServiceListResponse], error) {
	images, err := i.repo.Image().List(ctx, rq.Msg.Query)
	if err != nil {
		return nil, errorutil.Convert(err)
	}
	var result []*apiv2.Image

	sortedImages := i.repo.Image().AdditionalMethods().SortImages(images)
	for _, image := range sortedImages {
		converted, err := i.repo.Image().ConvertToProto(image)
		if err != nil {
			return nil, errorutil.Convert(err)
		}
		result = append(result, converted)
	}

	return connect.NewResponse(&apiv2.ImageServiceListResponse{Images: result}), nil
}

// Fixme, call if Get was called with "Latest:true"
func (i *imageServiceServer) Latest(ctx context.Context, rq *connect.Request[apiv2.ImageServiceLatestRequest]) (*connect.Response[apiv2.ImageServiceLatestResponse], error) {
	images, err := i.repo.Image().List(ctx, &apiv2.ImageQuery{})
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	latest, err := i.repo.Image().AdditionalMethods().GetMostRecentImageFor(rq.Msg.Os, images)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	converted, err := i.repo.Image().ConvertToProto(latest)
	if err != nil {
		return nil, errorutil.Convert(err)
	}
	return connect.NewResponse(&apiv2.ImageServiceLatestResponse{Image: converted}), nil
}
