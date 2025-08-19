package admin

import (
	"context"
	"log/slog"

	"connectrpc.com/connect"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	"github.com/metal-stack/api/go/metalstack/admin/v2/adminv2connect"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	"github.com/metal-stack/metal-apiserver/pkg/repository"
)

type Config struct {
	Log  *slog.Logger
	Repo *repository.Store
}

type machineServiceServer struct {
	log  *slog.Logger
	repo *repository.Store
}

func New(c Config) adminv2connect.MachineServiceHandler {
	return &machineServiceServer{
		log:  c.Log.WithGroup("adminMachineService"),
		repo: c.Repo,
	}
}


// Get implements apiv2connect.MachineServiceHandler.
func (m *machineServiceServer) Get(ctx context.Context, rq *connect.Request[adminv2.MachineServiceGetRequest]) (*connect.Response[adminv2.MachineServiceGetResponse], error) {
	req := rq.Msg

	resp, err := m.repo.UnscopedMachine().Get(ctx, req.Uuid)
	if err != nil {
		return nil, errorutil.Convert(err)
	}
	converted, err := m.repo.UnscopedMachine().ConvertToProto(resp)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return connect.NewResponse(&adminv2.MachineServiceGetResponse{
		Machine: converted,
	}), nil
}

// List implements apiv2connect.MachineServiceHandler.
func (m *machineServiceServer) List(ctx context.Context, rq *connect.Request[adminv2.MachineServiceListRequest]) (*connect.Response[adminv2.MachineServiceListResponse], error) {
	machines, err := m.repo.UnscopedMachine().List(ctx, rq.Msg.Query)
	if err != nil {
		return nil, errorutil.Convert(err)
	}
	var result []*apiv2.Machine

	for _, machine := range machines {
		converted, err := m.repo.UnscopedMachine().ConvertToProto(machine)
		if err != nil {
			return nil, errorutil.Convert(err)
		}
		result = append(result, converted)
	}

	return connect.NewResponse(&adminv2.MachineServiceListResponse{Machines: result}), nil
}

