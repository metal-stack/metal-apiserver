package network

import (
	"context"
	"log/slog"

	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/api/go/metalstack/api/v2/apiv2connect"
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
func (n *networkServiceServer) Create(ctx context.Context, rq *apiv2.NetworkServiceCreateRequest) (*apiv2.NetworkServiceCreateResponse, error) {
	r := rq

	req := &adminv2.NetworkServiceCreateRequest{
		Project:       &r.Project,
		Name:          r.Name,
		Description:   r.Description,
		Partition:     r.Partition,
		ParentNetwork: r.ParentNetwork,
		Labels:        r.Labels,
		Length:        r.Length,
		AddressFamily: r.AddressFamily,
		Type:          apiv2.NetworkType_NETWORK_TYPE_CHILD, // Non Admins can only create Child Networks
	}

	nw, err := n.repo.Network(r.Project).Create(ctx, req)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return &apiv2.NetworkServiceCreateResponse{Network: nw}, nil
}

// Delete implements apiv2connect.NetworkServiceHandler.
func (n *networkServiceServer) Delete(ctx context.Context, rq *apiv2.NetworkServiceDeleteRequest) (*apiv2.NetworkServiceDeleteResponse, error) {
	req := rq

	nw, err := n.repo.Network(req.Project).Delete(ctx, req.Id)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return &apiv2.NetworkServiceDeleteResponse{Network: nw}, nil
}

// Get implements apiv2connect.NetworkServiceHandler.
func (n *networkServiceServer) Get(ctx context.Context, rq *apiv2.NetworkServiceGetRequest) (*apiv2.NetworkServiceGetResponse, error) {
	req := rq

	// Project is already checked in the tenant-interceptor, ipam must not be consulted
	nw, err := n.repo.Network(req.Project).Get(ctx, req.Id)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return &apiv2.NetworkServiceGetResponse{
		Network: nw,
	}, nil
}

// List implements apiv2connect.NetworkServiceHandler.
func (n *networkServiceServer) List(ctx context.Context, req *apiv2.NetworkServiceListRequest) (*apiv2.NetworkServiceListResponse, error) {
	nw, err := n.repo.Network(req.Project).List(ctx, req.Query)
	if err != nil {
		return nil, err
	}
	return &apiv2.NetworkServiceListResponse{
		Networks: nw,
	}, nil
}

// ListBaseNetworks implements apiv2connect.NetworkServiceHandler.
func (n *networkServiceServer) ListBaseNetworks(ctx context.Context, rq *apiv2.NetworkServiceListBaseNetworksRequest) (*apiv2.NetworkServiceListBaseNetworksResponse, error) {
	req := rq

	var networks []*apiv2.Network

	if req.Project != "" {
		projectNetworks, err := n.repo.Network(req.Project).List(ctx, req.Query)
		if err != nil {
			return nil, err
		}
		networks = append(networks, projectNetworks...)
	}

	if req.Query == nil {
		req.Query = &apiv2.NetworkQuery{}
	}
	baseNetworksQuery := req.Query
	baseNetworksQuery.Project = pointer.Pointer("")

	baseNetworks, err := n.repo.UnscopedNetwork().List(ctx, baseNetworksQuery)
	if err != nil {
		return nil, err
	}
	networks = append(networks, baseNetworks...)

	var res []*apiv2.Network
	for _, nw := range networks {
		// TODO convert to a equivalent reql query
		switch pointer.SafeDeref(nw.Type) {
		case apiv2.NetworkType_NETWORK_TYPE_CHILD_SHARED, apiv2.NetworkType_NETWORK_TYPE_EXTERNAL, apiv2.NetworkType_NETWORK_TYPE_SUPER, apiv2.NetworkType_NETWORK_TYPE_SUPER_NAMESPACED:
			// users should not see usage of global networks, only admins
			if pointer.SafeDeref(nw.Project) == "" {
				nw.Consumption = nil
			}

			res = append(res, nw)
		}
	}

	return &apiv2.NetworkServiceListBaseNetworksResponse{
		Networks: res,
	}, nil
}

// Update implements apiv2connect.NetworkServiceHandler.
func (n *networkServiceServer) Update(ctx context.Context, rq *apiv2.NetworkServiceUpdateRequest) (*apiv2.NetworkServiceUpdateResponse, error) {
	req := rq

	nur := &adminv2.NetworkServiceUpdateRequest{
		Id:          req.Id,
		Name:        req.Name,
		Description: req.Description,
		Labels:      req.Labels,
		UpdateMeta:  req.UpdateMeta,
		// FIXME which fields should be updateable
	}

	nw, err := n.repo.Network(req.Project).Update(ctx, nur.Id, nur)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return &apiv2.NetworkServiceUpdateResponse{Network: nw}, nil
}
