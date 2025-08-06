package machine

import (
	"context"
	"log/slog"

	"connectrpc.com/connect"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/api/go/metalstack/api/v2/apiv2connect"
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

func New(c Config) apiv2connect.MachineServiceHandler {
	return &machineServiceServer{
		log:  c.Log.WithGroup("machineService"),
		repo: c.Repo,
	}
}

// Create implements apiv2connect.MachineServiceHandler.
func (m *machineServiceServer) Create(context.Context, *connect.Request[apiv2.MachineServiceCreateRequest]) (*connect.Response[apiv2.MachineServiceCreateResponse], error) {
	panic("unimplemented")
}

// Get implements apiv2connect.MachineServiceHandler.
func (m *machineServiceServer) Get(ctx context.Context, rq *connect.Request[apiv2.MachineServiceGetRequest]) (*connect.Response[apiv2.MachineServiceGetResponse], error) {
	req := rq.Msg

	resp, err := m.repo.Machine(req.Project).Get(ctx, req.Uuid)
	if err != nil {
		return nil, errorutil.Convert(err)
	}
	converted, err := m.repo.Machine(req.Project).ConvertToProto(resp)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return connect.NewResponse(&apiv2.MachineServiceGetResponse{
		Machine: converted,
	}), nil
}

// List implements apiv2connect.MachineServiceHandler.
func (m *machineServiceServer) List(ctx context.Context, rq *connect.Request[apiv2.MachineServiceListRequest]) (*connect.Response[apiv2.MachineServiceListResponse], error) {
	machines, err := m.repo.Machine(rq.Msg.Project).List(ctx, rq.Msg.Query)
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

	return connect.NewResponse(&apiv2.MachineServiceListResponse{Machines: result}), nil
}

// Update implements apiv2connect.MachineServiceHandler.
func (m *machineServiceServer) Update(context.Context, *connect.Request[apiv2.MachineServiceUpdateRequest]) (*connect.Response[apiv2.MachineServiceUpdateResponse], error) {
	panic("unimplemented")
}

// Delete implements apiv2connect.MachineServiceHandler.
func (m *machineServiceServer) Delete(context.Context, *connect.Request[apiv2.MachineServiceDeleteRequest]) (*connect.Response[apiv2.MachineServiceDeleteResponse], error) {
	panic("unimplemented")
}
