package network

import (
	"context"
	"log/slog"

	"connectrpc.com/connect"
	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/api/go/metalstack/api/v2/apiv2connect"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	"github.com/metal-stack/metal-apiserver/pkg/repository"
	"github.com/metal-stack/metal-lib/pkg/pointer"
)

type Config struct {
	Log  *slog.Logger
	Repo *repository.Store
}

type networkServiceServer struct {
	log  *slog.Logger
	repo *repository.Store
}

func New(c Config) apiv2connect.NetworkServiceHandler {
	return &networkServiceServer{
		log:  c.Log.WithGroup("networkService"),
		repo: c.Repo,
	}
}

// Create implements apiv2connect.NetworkServiceHandler.
func (n *networkServiceServer) Create(ctx context.Context, rq *connect.Request[apiv2.NetworkServiceCreateRequest]) (*connect.Response[apiv2.NetworkServiceCreateResponse], error) {
	r := rq.Msg

	req := &adminv2.NetworkServiceCreateRequest{
		Project:         &r.Project,
		Name:            r.Name,
		Description:     r.Description,
		Partition:       r.Partition,
		ParentNetworkId: r.ParentNetworkId,
		Labels:          r.Labels,
		Length:          r.Length,
		AddressFamily:   r.AddressFamily,
		Type:            apiv2.NetworkType_NETWORK_TYPE_CHILD, // Non Admins can only create Child Networks
	}

	validated, err := n.repo.Network(r.Project).ValidateCreate(ctx, req)
	if err != nil {
		return nil, err
	}

	created, err := n.repo.Network(r.Project).Create(ctx, validated)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	converted, err := n.repo.Network(r.Project).ConvertToProto(created)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return connect.NewResponse(&apiv2.NetworkServiceCreateResponse{Network: converted}), nil
}

// Delete implements apiv2connect.NetworkServiceHandler.
func (n *networkServiceServer) Delete(ctx context.Context, rq *connect.Request[apiv2.NetworkServiceDeleteRequest]) (*connect.Response[apiv2.NetworkServiceDeleteResponse], error) {
	req := rq.Msg

	nw, err := n.repo.Network(req.Project).Get(ctx, req.Id)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	validated, err := n.repo.Network(req.Project).ValidateDelete(ctx, nw)
	if err != nil {
		return nil, err
	}

	nw, err = n.repo.Network(req.Project).Delete(ctx, validated)
	if err != nil {
		return nil, errorutil.Convert(err)
	}
	converted, err := n.repo.Network(req.Project).ConvertToProto(nw)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return connect.NewResponse(&apiv2.NetworkServiceDeleteResponse{Network: converted}), nil
}

// Get implements apiv2connect.NetworkServiceHandler.
func (n *networkServiceServer) Get(ctx context.Context, rq *connect.Request[apiv2.NetworkServiceGetRequest]) (*connect.Response[apiv2.NetworkServiceGetResponse], error) {
	req := rq.Msg

	// Project is already checked in the tenant-interceptor, ipam must not be consulted
	resp, err := n.repo.Network(req.Project).Get(ctx, req.Id)
	if err != nil {
		return nil, errorutil.Convert(err)
	}
	converted, err := n.repo.Network(req.Project).ConvertToProto(resp)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return connect.NewResponse(&apiv2.NetworkServiceGetResponse{
		Network: converted,
	}), nil
}

// List implements apiv2connect.NetworkServiceHandler.
func (n *networkServiceServer) List(ctx context.Context, rq *connect.Request[apiv2.NetworkServiceListRequest]) (*connect.Response[apiv2.NetworkServiceListResponse], error) {

	req := rq.Msg
	resp, err := n.repo.Network(req.Project).List(ctx, req.Query)
	if err != nil {
		return nil, err
	}

	var res []*apiv2.Network
	for _, nw := range resp {
		converted, err := n.repo.Network(req.Project).ConvertToProto(nw)
		if err != nil {
			return nil, errorutil.Convert(err)
		}
		res = append(res, converted)
	}

	return connect.NewResponse(&apiv2.NetworkServiceListResponse{
		Networks: res,
	}), nil
}

// ListBaseNetworks implements apiv2connect.NetworkServiceHandler.
func (n *networkServiceServer) ListBaseNetworks(ctx context.Context, rq *connect.Request[apiv2.NetworkServiceListBaseNetworksRequest]) (*connect.Response[apiv2.NetworkServiceListBaseNetworksResponse], error) {
	req := rq.Msg

	projectNetworks, err := n.repo.Network(req.Project).List(ctx, req.Query)
	if err != nil {
		return nil, err
	}

	baseNetworksQuery := req.Query
	baseNetworksQuery.Project = pointer.Pointer("")

	baseNetworks, err := n.repo.UnscopedNetwork().List(ctx, baseNetworksQuery)
	if err != nil {
		return nil, err
	}

	var res []*apiv2.Network
	for _, nw := range append(baseNetworks, projectNetworks...) {
		// TODO convert to a equivalent reql query
		switch pointer.SafeDeref(nw.NetworkType) {
		case metal.ChildSharedNetworkType, metal.ExternalNetworkType, metal.SuperNetworkType, metal.SuperNamespacedNetworkType:
			converted, err := n.repo.UnscopedNetwork().ConvertToProto(nw)
			if err != nil {
				return nil, errorutil.Convert(err)
			}

			// users should not see usage of global networks, only admins
			if nw.ProjectID == "" {
				converted.Consumption = nil
			}

			res = append(res, converted)
		}
	}

	return connect.NewResponse(&apiv2.NetworkServiceListBaseNetworksResponse{
		Networks: res,
	}), nil
}

// Update implements apiv2connect.NetworkServiceHandler.
func (n *networkServiceServer) Update(ctx context.Context, rq *connect.Request[apiv2.NetworkServiceUpdateRequest]) (*connect.Response[apiv2.NetworkServiceUpdateResponse], error) {
	req := rq.Msg

	nur := &adminv2.NetworkServiceUpdateRequest{
		Id:          req.Id,
		Name:        req.Name,
		Description: req.Description,
		Labels:      req.Labels,
		// FIXME which fields should be updateable
	}

	validated, err := n.repo.Network(req.Project).ValidateUpdate(ctx, nur)
	if err != nil {
		return nil, err
	}

	nw, err := n.repo.Network(req.Project).Update(ctx, validated)
	if err != nil {
		return nil, errorutil.Convert(err)
	}
	converted, err := n.repo.Network(req.Project).ConvertToProto(nw)
	if err != nil {
		return nil, errorutil.Convert(err)
	}
	return connect.NewResponse(&apiv2.NetworkServiceUpdateResponse{Network: converted}), nil
}
