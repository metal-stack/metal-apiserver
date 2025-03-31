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

// Create implements adminv2connect.NetworkServiceHandler.
func (n *networkServiceServer) Create(ctx context.Context, rq *connect.Request[adminv2.NetworkServiceCreateRequest]) (*connect.Response[adminv2.NetworkServiceCreateResponse], error) {
	n.log.Debug("create", "req", rq)
	req := rq.Msg

	validated, err := n.repo.UnscopedNetwork().ValidateCreate(ctx, req)
	if err != nil {
		return nil, err
	}

	created, err := n.repo.UnscopedNetwork().Create(ctx, validated)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	converted, err := n.repo.UnscopedNetwork().ConvertToProto(created)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return connect.NewResponse(&adminv2.NetworkServiceCreateResponse{Network: converted}), nil
}

// Delete implements adminv2connect.NetworkServiceHandler.
func (n *networkServiceServer) Delete(ctx context.Context, rq *connect.Request[adminv2.NetworkServiceDeleteRequest]) (*connect.Response[adminv2.NetworkServiceDeleteResponse], error) {
	n.log.Debug("delete", "req", rq)
	req := rq.Msg

	nw, err := n.repo.UnscopedNetwork().Get(ctx, req.Id)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	validated, err := n.repo.UnscopedNetwork().ValidateDelete(ctx, nw)
	if err != nil {
		return nil, err
	}

	nw, err = n.repo.UnscopedNetwork().Delete(ctx, validated)
	if err != nil {
		return nil, errorutil.Convert(err)
	}
	converted, err := n.repo.UnscopedNetwork().ConvertToProto(nw)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return connect.NewResponse(&adminv2.NetworkServiceDeleteResponse{Network: converted}), nil
}

// List implements adminv2connect.NetworkServiceHandler.
func (n *networkServiceServer) List(ctx context.Context, rq *connect.Request[adminv2.NetworkServiceListRequest]) (*connect.Response[adminv2.NetworkServiceListResponse], error) {
	n.log.Debug("list", "req", rq)
	req := rq.Msg

	resp, err := n.repo.UnscopedNetwork().List(ctx, req.Query)
	if err != nil {
		return nil, err
	}

	var res []*apiv2.Network
	for _, nw := range resp {
		converted, err := n.repo.UnscopedNetwork().ConvertToProto(nw)
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
	n.log.Debug("update", "req", rq)
	req := rq.Msg

	validated, err := n.repo.UnscopedNetwork().ValidateUpdate(ctx, req)
	if err != nil {
		return nil, err
	}

	nw, err := n.repo.UnscopedNetwork().Update(ctx, validated)
	if err != nil {
		return nil, errorutil.Convert(err)
	}
	converted, err := n.repo.UnscopedNetwork().ConvertToProto(nw)
	if err != nil {
		return nil, errorutil.Convert(err)
	}
	return connect.NewResponse(&adminv2.NetworkServiceUpdateResponse{Network: converted}), nil
}
