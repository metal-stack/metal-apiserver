package admin

import (
	"context"
	"log/slog"

	"connectrpc.com/connect"
	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	"github.com/metal-stack/api/go/metalstack/admin/v2/adminv2connect"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
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
	resp, err := n.repo.UnscopedNetwork().Get(ctx, req.Id)
	if err != nil {
		return nil, errorutil.Convert(err)
	}
	converted, err := n.repo.UnscopedNetwork().ConvertToProto(ctx, resp)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return connect.NewResponse(&adminv2.NetworkServiceGetResponse{
		Network: converted,
	}), nil
}

// Create implements adminv2connect.NetworkServiceHandler.
func (n *networkServiceServer) Create(ctx context.Context, rq *connect.Request[adminv2.NetworkServiceCreateRequest]) (*connect.Response[adminv2.NetworkServiceCreateResponse], error) {
	req := rq.Msg

	created, err := n.repo.UnscopedNetwork().Create(ctx, req)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	converted, err := n.repo.UnscopedNetwork().ConvertToProto(ctx, created)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return connect.NewResponse(&adminv2.NetworkServiceCreateResponse{Network: converted}), nil
}

// Delete implements adminv2connect.NetworkServiceHandler.
func (n *networkServiceServer) Delete(ctx context.Context, rq *connect.Request[adminv2.NetworkServiceDeleteRequest]) (*connect.Response[adminv2.NetworkServiceDeleteResponse], error) {
	req := rq.Msg

	nw, err := n.repo.UnscopedNetwork().Delete(ctx, req.Id)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	converted, err := n.repo.UnscopedNetwork().ConvertToProto(ctx, nw)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return connect.NewResponse(&adminv2.NetworkServiceDeleteResponse{Network: converted}), nil
}

// List implements adminv2connect.NetworkServiceHandler.
func (n *networkServiceServer) List(ctx context.Context, rq *connect.Request[adminv2.NetworkServiceListRequest]) (*connect.Response[adminv2.NetworkServiceListResponse], error) {
	req := rq.Msg

	resp, err := n.repo.UnscopedNetwork().List(ctx, req.Query)
	if err != nil {
		return nil, err
	}

	var res []*apiv2.Network
	for _, nw := range resp {
		converted, err := n.repo.UnscopedNetwork().ConvertToProto(ctx, nw)
		if err != nil {
			return nil, errorutil.Convert(err)
		}
		res = append(res, converted)
	}

	return connect.NewResponse(&adminv2.NetworkServiceListResponse{
		Networks: res,
	}), nil
}

// Update implements adminv2connect.NetworkServiceHandler.
func (n *networkServiceServer) Update(ctx context.Context, rq *connect.Request[adminv2.NetworkServiceUpdateRequest]) (*connect.Response[adminv2.NetworkServiceUpdateResponse], error) {
	req := rq.Msg

	nw, err := n.repo.UnscopedNetwork().Update(ctx, req.Id, req)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	converted, err := n.repo.UnscopedNetwork().ConvertToProto(ctx, nw)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return connect.NewResponse(&adminv2.NetworkServiceUpdateResponse{Network: converted}), nil
}
