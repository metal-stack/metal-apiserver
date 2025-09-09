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

type ipServiceServer struct {
	log  *slog.Logger
	repo *repository.Store
}

func New(c Config) adminv2connect.IPServiceHandler {
	return &ipServiceServer{
		log:  c.Log.WithGroup("adminIpService"),
		repo: c.Repo,
	}
}

func (i *ipServiceServer) List(ctx context.Context, rq *connect.Request[adminv2.IPServiceListRequest]) (*connect.Response[adminv2.IPServiceListResponse], error) {
	req := rq.Msg

	resp, err := i.repo.UnscopedIP().List(ctx, req.Query)
	if err != nil {
		return nil, err
	}

	var res []*apiv2.IP
	for _, ip := range resp {
		converted, err := i.repo.UnscopedIP().ConvertToProto(ctx, ip)
		if err != nil {
			return nil, errorutil.Convert(err)
		}
		res = append(res, converted)
	}

	return connect.NewResponse(&adminv2.IPServiceListResponse{
		Ips: res,
	}), nil
}

func (i *ipServiceServer) Issues(ctx context.Context, rq *connect.Request[adminv2.IPServiceIssuesRequest]) (*connect.Response[adminv2.IPServiceIssuesResponse], error) {
	panic("unimplemented")
}
