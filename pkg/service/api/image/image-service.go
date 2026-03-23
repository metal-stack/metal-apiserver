package image

import (
	"context"
	"log/slog"

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
func (i *imageServiceServer) Get(ctx context.Context, rq *apiv2.ImageServiceGetRequest) (*apiv2.ImageServiceGetResponse, error) {
	image, err := i.repo.Image().Get(ctx, rq.Id)
	if err != nil {
		return nil, errorutil.Convert(err)
	}
	return &apiv2.ImageServiceGetResponse{Image: image}, nil
}

// List implements apiv2connect.ImageServiceHandler.
func (i *imageServiceServer) List(ctx context.Context, rq *apiv2.ImageServiceListRequest) (*apiv2.ImageServiceListResponse, error) {
	images, err := i.repo.Image().List(ctx, rq.Query)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return &apiv2.ImageServiceListResponse{Images: images}, nil
}

// Fixme, call if Get was called with "Latest:true"
func (i *imageServiceServer) Latest(ctx context.Context, rq *apiv2.ImageServiceLatestRequest) (*apiv2.ImageServiceLatestResponse, error) {
	images, err := i.repo.Image().List(ctx, &apiv2.ImageQuery{})
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	latest, err := i.repo.Image().AdditionalMethods().GetMostRecentImageFor(ctx, rq.Os, images)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return &apiv2.ImageServiceLatestResponse{Image: latest}, nil
}
