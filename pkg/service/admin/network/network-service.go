package admin

import (
	"context"
	"log/slog"

	"github.com/metal-stack/api/go/errorutil"
	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	"github.com/metal-stack/api/go/metalstack/admin/v2/adminv2connect"
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

// AddExternalMember implements [adminv2connect.NetworkServiceHandler].
func (n *networkServiceServer) AddExternalMember(context.Context, *adminv2.NetworkServiceAddExternalMemberRequest) (*adminv2.NetworkServiceAddExternalMemberResponse, error) {
	return nil, errorutil.Unimplemented("")
}

// ListExternalMembers implements [adminv2connect.NetworkServiceHandler].
func (n *networkServiceServer) ListExternalMembers(context.Context, *adminv2.NetworkServiceListExternalMembersRequest) (*adminv2.NetworkServiceListExternalMembersResponse, error) {
	return nil, errorutil.Unimplemented("")
}

// RemoveExternalMember implements [adminv2connect.NetworkServiceHandler].
func (n *networkServiceServer) RemoveExternalMember(context.Context, *adminv2.NetworkServiceRemoveExternalMemberRequest) (*adminv2.NetworkServiceRemoveExternalMemberResponse, error) {
	return nil, errorutil.Unimplemented("")
}

func New(c Config) adminv2connect.NetworkServiceHandler {
	return &networkServiceServer{
		log:  c.Log.WithGroup("adminNetworkService"),
		repo: c.Repo,
	}
}

func (n *networkServiceServer) Get(ctx context.Context, req *adminv2.NetworkServiceGetRequest) (*adminv2.NetworkServiceGetResponse, error) {
	// Project is already checked in the tenant-interceptor, ipam must not be consulted
	nw, err := n.repo.UnscopedNetwork().Get(ctx, req.Id)
	if err != nil {
		return nil, err
	}

	return &adminv2.NetworkServiceGetResponse{
		Network: nw,
	}, nil
}

func (n *networkServiceServer) Create(ctx context.Context, req *adminv2.NetworkServiceCreateRequest) (*adminv2.NetworkServiceCreateResponse, error) {
	nw, err := n.repo.UnscopedNetwork().Create(ctx, req)
	if err != nil {
		return nil, err
	}

	return &adminv2.NetworkServiceCreateResponse{Network: nw}, nil
}

func (n *networkServiceServer) Delete(ctx context.Context, req *adminv2.NetworkServiceDeleteRequest) (*adminv2.NetworkServiceDeleteResponse, error) {
	nw, err := n.repo.UnscopedNetwork().Delete(ctx, req.Id)
	if err != nil {
		return nil, err
	}

	return &adminv2.NetworkServiceDeleteResponse{Network: nw}, nil
}

func (n *networkServiceServer) List(ctx context.Context, req *adminv2.NetworkServiceListRequest) (*adminv2.NetworkServiceListResponse, error) {
	nws, err := n.repo.UnscopedNetwork().List(ctx, req.Query)
	if err != nil {
		return nil, err
	}

	return &adminv2.NetworkServiceListResponse{
		Networks: nws,
	}, nil
}

func (n *networkServiceServer) Update(ctx context.Context, req *adminv2.NetworkServiceUpdateRequest) (*adminv2.NetworkServiceUpdateResponse, error) {
	nw, err := n.repo.UnscopedNetwork().Update(ctx, req.Id, req)
	if err != nil {
		return nil, err
	}

	return &adminv2.NetworkServiceUpdateResponse{Network: nw}, nil
}

func (n *networkServiceServer) ListExternalMembers(ctx context.Context, req *adminv2.NetworkServiceListExternalMembersRequest) (*adminv2.NetworkServiceListExternalMembersResponse, error) {
	members, err := n.repo.UnscopedNetwork().AdditionalMethods().ListExternalMembers(ctx, req)
	if err != nil {
		return nil, err
	}

	return &adminv2.NetworkServiceListExternalMembersResponse{Members: members}, nil
}

func (n *networkServiceServer) AddExternalMember(ctx context.Context, req *adminv2.NetworkServiceAddExternalMemberRequest) (*adminv2.NetworkServiceAddExternalMemberResponse, error) {
	sw, err := n.repo.UnscopedNetwork().AdditionalMethods().AddExternalMember(ctx, req)
	if err != nil {
		return nil, err
	}

	return &adminv2.NetworkServiceAddExternalMemberResponse{Switch: sw}, nil
}

func (n *networkServiceServer) RemoveExternalMember(ctx context.Context, req *adminv2.NetworkServiceRemoveExternalMemberRequest) (*adminv2.NetworkServiceRemoveExternalMemberResponse, error) {
	sw, err := n.repo.UnscopedNetwork().AdditionalMethods().RemoveExternalMember(ctx, req)
	if err != nil {
		return nil, err
	}

	return &adminv2.NetworkServiceRemoveExternalMemberResponse{Switch: sw}, nil
}
