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
	req := rq.Msg

	var (
		metalIP *metal.IP
		err     error
	)

	ip := metal.CreateNamespacedIPAddress(req.Namespace, req.Ip)
	metalIP, err = i.repo.IP(req.Project).Get(ctx, ip)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	converted, err := i.repo.IP(req.Project).ConvertToProto(ctx, metalIP)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return connect.NewResponse(&apiv2.IPServiceGetResponse{
		Ip: converted,
	}), nil
}

// List implements v1.IPServiceServer
func (i *ipServiceServer) List(ctx context.Context, rq *connect.Request[apiv2.IPServiceListRequest]) (*connect.Response[apiv2.IPServiceListResponse], error) {
	req := rq.Msg

	resp, err := i.repo.IP(req.Project).List(ctx, req.Query)
	if err != nil {
		return nil, err
	}

	var res []*apiv2.IP
	for _, ip := range resp {
		converted, err := i.repo.IP(req.Project).ConvertToProto(ctx, ip)
		if err != nil {
			return nil, errorutil.Convert(err)
		}
		res = append(res, converted)
	}

	return connect.NewResponse(&apiv2.IPServiceListResponse{
		Ips: res,
	}), nil
}

// Delete implements v1.IPServiceServer
func (i *ipServiceServer) Delete(ctx context.Context, rq *connect.Request[apiv2.IPServiceDeleteRequest]) (*connect.Response[apiv2.IPServiceDeleteResponse], error) {
	req := rq.Msg

	ip, err := i.repo.IP(req.Project).Delete(ctx, req.Ip)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	converted, err := i.repo.IP(req.Project).ConvertToProto(ctx, ip)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return connect.NewResponse(&apiv2.IPServiceDeleteResponse{Ip: converted}), nil
}

func (i *ipServiceServer) Create(ctx context.Context, rq *connect.Request[apiv2.IPServiceCreateRequest]) (*connect.Response[apiv2.IPServiceCreateResponse], error) {
	req := rq.Msg

	created, err := i.repo.IP(req.Project).Create(ctx, req)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	converted, err := i.repo.IP(req.Project).ConvertToProto(ctx, created)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return connect.NewResponse(&apiv2.IPServiceCreateResponse{Ip: converted}), nil
}

// Static implements v1.IPServiceServer
func (i *ipServiceServer) Update(ctx context.Context, rq *connect.Request[apiv2.IPServiceUpdateRequest]) (*connect.Response[apiv2.IPServiceUpdateResponse], error) {
	req := rq.Msg

	ip, err := i.repo.IP(req.Project).Update(ctx, req.Ip, rq.Msg)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	converted, err := i.repo.IP(req.Project).ConvertToProto(ctx, ip)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return connect.NewResponse(&apiv2.IPServiceUpdateResponse{Ip: converted}), nil
}
