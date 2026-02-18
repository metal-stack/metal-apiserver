package admin

import (
	"context"
	"log/slog"

	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	"github.com/metal-stack/api/go/metalstack/admin/v2/adminv2connect"
	"github.com/metal-stack/metal-apiserver/pkg/repository"
)

type Config struct {
	Log  *slog.Logger
	Repo *repository.Store
}

type componentServiceServer struct {
	log  *slog.Logger
	repo *repository.Store
}

func New(config Config) adminv2connect.ComponentServiceHandler {
	return &componentServiceServer{
		log:  config.Log,
		repo: config.Repo,
	}
}

func (c *componentServiceServer) Get(ctx context.Context, req *adminv2.ComponentServiceGetRequest) (*adminv2.ComponentServiceGetResponse, error) {
	resp, err := c.repo.Component().Get(ctx, req.Uuid)
	if err != nil {
		return nil, err
	}

	return &adminv2.ComponentServiceGetResponse{
		Component: resp,
	}, nil
}

func (c *componentServiceServer) List(ctx context.Context, req *adminv2.ComponentServiceListRequest) (*adminv2.ComponentServiceListResponse, error) {
	resp, err := c.repo.Component().List(ctx, req.Query)
	if err != nil {
		return nil, err
	}

	return &adminv2.ComponentServiceListResponse{
		Components: resp,
	}, nil
}

func (c *componentServiceServer) Delete(ctx context.Context, req *adminv2.ComponentServiceDeleteRequest) (*adminv2.ComponentServiceDeleteResponse, error) {
	resp, err := c.repo.Component().Delete(ctx, req.Uuid)
	if err != nil {
		return nil, err
	}

	return &adminv2.ComponentServiceDeleteResponse{
		Component: resp,
	}, nil
}
