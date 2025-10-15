package machine

import (
	"context"
	"log/slog"

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
func (m *machineServiceServer) Create(context.Context, *apiv2.MachineServiceCreateRequest) (*apiv2.MachineServiceCreateResponse, error) {
	panic("unimplemented")
}

// Get implements apiv2connect.MachineServiceHandler.
func (m *machineServiceServer) Get(ctx context.Context, rq *apiv2.MachineServiceGetRequest) (*apiv2.MachineServiceGetResponse, error) {
	req := rq

	resp, err := m.repo.Machine(req.Project).Get(ctx, req.Uuid)
	if err != nil {
		return nil, errorutil.Convert(err)
	}
	converted, err := m.repo.Machine(req.Project).ConvertToProto(ctx, resp)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return &apiv2.MachineServiceGetResponse{
		Machine: converted,
	}, nil
}

// List implements apiv2connect.MachineServiceHandler.
func (m *machineServiceServer) List(ctx context.Context, rq *apiv2.MachineServiceListRequest) (*apiv2.MachineServiceListResponse, error) {
	machines, err := m.repo.Machine(rq.Project).List(ctx, rq.Query)
	if err != nil {
		return nil, errorutil.Convert(err)
	}
	var result []*apiv2.Machine

	for _, machine := range machines {
		converted, err := m.repo.UnscopedMachine().ConvertToProto(ctx, machine)
		if err != nil {
			return nil, errorutil.Convert(err)
		}
		result = append(result, converted)
	}

	return &apiv2.MachineServiceListResponse{Machines: result}, nil
}

// Update implements apiv2connect.MachineServiceHandler.
func (m *machineServiceServer) Update(ctx context.Context, rq *apiv2.MachineServiceUpdateRequest) (*apiv2.MachineServiceUpdateResponse, error) {
	req := rq

	ms, err := m.repo.Machine(req.Project).Update(ctx, req.Uuid, req)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	converted, err := m.repo.UnscopedMachine().ConvertToProto(ctx, ms)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return &apiv2.MachineServiceUpdateResponse{Machine: converted}, nil
}

// Delete implements apiv2connect.MachineServiceHandler.
func (m *machineServiceServer) Delete(context.Context, *apiv2.MachineServiceDeleteRequest) (*apiv2.MachineServiceDeleteResponse, error) {
	panic("unimplemented")
}
