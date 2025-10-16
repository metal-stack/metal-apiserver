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

type networkServiceServer struct {
	log  *slog.Logger
	repo *repository.Store
}

func New(c Config) adminv2connect.NetworkServiceHandler {
	return &networkServiceServer{
		log:  c.Log.WithGroup("adminNetworkService"),
		repo: c.Repo,
	}
}

func (n *networkServiceServer) Get(ctx context.Context, rq *connect.Request[adminv2.NetworkServiceGetRequest]) (*connect.Response[adminv2.NetworkServiceGetResponse], error) {
	req := rq.Msg

	// Project is already checked in the tenant-interceptor, ipam must not be consulted
	nw, err := n.repo.UnscopedNetwork().Get(ctx, req.Id)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return connect.NewResponse(&adminv2.NetworkServiceGetResponse{
		Network: nw,
	}), nil
}

// Create implements adminv2connect.NetworkServiceHandler.
func (n *networkServiceServer) Create(ctx context.Context, rq *connect.Request[adminv2.NetworkServiceCreateRequest]) (*connect.Response[adminv2.NetworkServiceCreateResponse], error) {
	req := rq.Msg

	nw, err := n.repo.UnscopedNetwork().Create(ctx, req)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return connect.NewResponse(&adminv2.NetworkServiceCreateResponse{Network: nw}), nil
}

// Delete implements adminv2connect.NetworkServiceHandler.
func (n *networkServiceServer) Delete(ctx context.Context, rq *connect.Request[adminv2.NetworkServiceDeleteRequest]) (*connect.Response[adminv2.NetworkServiceDeleteResponse], error) {
	req := rq.Msg

	nw, err := n.repo.UnscopedNetwork().Delete(ctx, req.Id)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return connect.NewResponse(&adminv2.NetworkServiceDeleteResponse{Network: nw}), nil
}

// List implements adminv2connect.NetworkServiceHandler.
func (n *networkServiceServer) List(ctx context.Context, rq *connect.Request[adminv2.NetworkServiceListRequest]) (*connect.Response[adminv2.NetworkServiceListResponse], error) {
	req := rq.Msg

	nws, err := n.repo.UnscopedNetwork().List(ctx, req.Query)
	if err != nil {
		return nil, err
	}

	return connect.NewResponse(&adminv2.NetworkServiceListResponse{
		Networks: nws,
	}), nil
}

// Update implements adminv2connect.NetworkServiceHandler.
func (n *networkServiceServer) Update(ctx context.Context, rq *connect.Request[adminv2.NetworkServiceUpdateRequest]) (*connect.Response[adminv2.NetworkServiceUpdateResponse], error) {
	req := rq.Msg

	nw, err := n.repo.UnscopedNetwork().Update(ctx, req.Id, req)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return connect.NewResponse(&adminv2.NetworkServiceUpdateResponse{Network: nw}), nil
}
