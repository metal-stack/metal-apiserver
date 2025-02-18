package ip

import (
	"context"
	"errors"
	"log/slog"

	"connectrpc.com/connect"
	"github.com/metal-stack/api-server/pkg/db/generic"
	"github.com/metal-stack/api-server/pkg/db/metal"
	"github.com/metal-stack/api-server/pkg/db/repository"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/api/go/metalstack/api/v2/apiv2connect"

	"github.com/metal-stack/metal-lib/pkg/tag"
)

type Config struct {
	Log  *slog.Logger
	Repo *repository.Repostore
}

type ipServiceServer struct {
	log  *slog.Logger
	repo *repository.Repostore
}

func New(c Config) apiv2connect.IPServiceHandler {
	return &ipServiceServer{
		log:  c.Log.WithGroup("ipService"),
		repo: c.Repo,
	}
}

func (i *ipServiceServer) Get(ctx context.Context, rq *connect.Request[apiv2.IPServiceGetRequest]) (*connect.Response[apiv2.IPServiceGetResponse], error) {
	i.log.Debug("get", "ip", rq)
	req := rq.Msg

	// Project is already checked in the tenant-interceptor, ipam must not be consulted
	resp, err := i.repo.IP(&req.Project).Get(ctx, req.Ip)
	if err != nil {
		if generic.IsNotFound(err) {
			return nil, connect.NewError(connect.CodeNotFound, err)
		}
		return nil, err
	}
	converted, err := i.repo.IP(&req.Project).ConvertToProto(resp)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(&apiv2.IPServiceGetResponse{
		Ip: converted,
	}), nil
}

// List implements v1.IPServiceServer
func (i *ipServiceServer) List(ctx context.Context, rq *connect.Request[apiv2.IPServiceListRequest]) (*connect.Response[apiv2.IPServiceListResponse], error) {
	i.log.Debug("list", "ip", rq)
	req := rq.Msg

	resp, err := i.repo.IP(&req.Project).List(ctx, req)
	if err != nil {
		return nil, err
	}

	var res []*apiv2.IP
	for _, ip := range resp {

		m := tag.NewTagMap(ip.Tags)
		if _, ok := m.Value(tag.MachineID); ok {
			// we do not want to show machine ips (e.g. firewall public ips)
			continue
		}

		converted, err := i.repo.IP(&req.Project).ConvertToProto(ip)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
		res = append(res, converted)
	}

	return connect.NewResponse(&apiv2.IPServiceListResponse{
		Ips: res,
	}), nil
}

// Delete implements v1.IPServiceServer
func (i *ipServiceServer) Delete(ctx context.Context, rq *connect.Request[apiv2.IPServiceDeleteRequest]) (*connect.Response[apiv2.IPServiceDeleteResponse], error) {
	i.log.Debug("delete", "ip", rq)
	req := rq.Msg

	// TODO also delete in go-ipam in one transaction
	// maybe reuse asyncActor from metal-api
	// Ensure that only this ip with the same uuid gets deleted
	ip, err := i.repo.IP(&req.Project).Delete(ctx, &metal.IP{IPAddress: req.Ip})
	if err != nil {
		if generic.IsNotFound(err) {
			return nil, connect.NewError(connect.CodeNotFound, err)
		}
		return nil, err
	}
	converted, err := i.repo.IP(&req.Project).ConvertToProto(ip)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(&apiv2.IPServiceDeleteResponse{Ip: converted}), nil
}

func (i *ipServiceServer) Create(ctx context.Context, rq *connect.Request[apiv2.IPServiceCreateRequest]) (*connect.Response[apiv2.IPServiceCreateResponse], error) {
	i.log.Debug("create", "ip", rq)
	req := rq.Msg

	if req.Network == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("network should not be empty"))
	}
	if req.Project == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("project should not be empty"))
	}

	created, err := i.repo.IP(&req.Project).Create(ctx, req)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	converted, err := i.repo.IP(&req.Project).ConvertToProto(created)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(&apiv2.IPServiceCreateResponse{Ip: converted}), nil
}

// Static implements v1.IPServiceServer
func (i *ipServiceServer) Update(ctx context.Context, rq *connect.Request[apiv2.IPServiceUpdateRequest]) (*connect.Response[apiv2.IPServiceUpdateResponse], error) {
	i.log.Debug("update", "ip", rq)

	req := rq.Msg

	ip, err := i.repo.IP(&req.Project).Update(ctx, req)
	if err != nil {
		if generic.IsNotFound(err) {
			return nil, connect.NewError(connect.CodeNotFound, err)
		}
		return nil, err
	}
	converted, err := i.repo.IP(&req.Project).ConvertToProto(ip)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&apiv2.IPServiceUpdateResponse{Ip: converted}), nil
}
