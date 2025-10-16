package ip

import (
	"context"
	"log/slog"

	"connectrpc.com/connect"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/api/go/metalstack/api/v2/apiv2connect"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	"github.com/metal-stack/metal-apiserver/pkg/repository"
)

type Config struct {
	Log  *slog.Logger
	Repo *repository.Store
}

type ipServiceServer struct {
	log  *slog.Logger
	repo *repository.Store
}

func New(c Config) apiv2connect.IPServiceHandler {
	return &ipServiceServer{
		log:  c.Log.WithGroup("ipService"),
		repo: c.Repo,
	}
}

func (i *ipServiceServer) Get(ctx context.Context, rq *connect.Request[apiv2.IPServiceGetRequest]) (*connect.Response[apiv2.IPServiceGetResponse], error) {
	var (
		req          = rq.Msg
		namespacedIP = metal.CreateNamespacedIPAddress(req.Namespace, req.Ip)
	)

	ip, err := i.repo.IP(req.Project).Get(ctx, namespacedIP)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return connect.NewResponse(&apiv2.IPServiceGetResponse{
		Ip: ip,
	}), nil
}

// List implements v1.IPServiceServer
func (i *ipServiceServer) List(ctx context.Context, rq *connect.Request[apiv2.IPServiceListRequest]) (*connect.Response[apiv2.IPServiceListResponse], error) {
	req := rq.Msg

	ips, err := i.repo.IP(req.Project).List(ctx, req.Query)
	if err != nil {
		return nil, err
	}

	return connect.NewResponse(&apiv2.IPServiceListResponse{
		Ips: ips,
	}), nil
}

// Delete implements v1.IPServiceServer
func (i *ipServiceServer) Delete(ctx context.Context, rq *connect.Request[apiv2.IPServiceDeleteRequest]) (*connect.Response[apiv2.IPServiceDeleteResponse], error) {
	req := rq.Msg

	ip, err := i.repo.IP(req.Project).Delete(ctx, req.Ip)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return connect.NewResponse(&apiv2.IPServiceDeleteResponse{Ip: ip}), nil
}

func (i *ipServiceServer) Create(ctx context.Context, rq *connect.Request[apiv2.IPServiceCreateRequest]) (*connect.Response[apiv2.IPServiceCreateResponse], error) {
	req := rq.Msg

	ip, err := i.repo.IP(req.Project).Create(ctx, req)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return connect.NewResponse(&apiv2.IPServiceCreateResponse{Ip: ip}), nil
}

// Static implements v1.IPServiceServer
func (i *ipServiceServer) Update(ctx context.Context, rq *connect.Request[apiv2.IPServiceUpdateRequest]) (*connect.Response[apiv2.IPServiceUpdateResponse], error) {
	req := rq.Msg

	ip, err := i.repo.IP(req.Project).Update(ctx, req.Ip, rq.Msg)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return connect.NewResponse(&apiv2.IPServiceUpdateResponse{Ip: ip}), nil
}
