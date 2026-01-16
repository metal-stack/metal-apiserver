package machine

import (
	"context"
	"log/slog"

	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
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
func (m *machineServiceServer) Get(ctx context.Context, req *apiv2.MachineServiceGetRequest) (*apiv2.MachineServiceGetResponse, error) {
	machine, err := m.repo.Machine(req.Project).Get(ctx, req.Uuid)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return &apiv2.MachineServiceGetResponse{
		Machine: machine,
	}, nil
}

// List implements apiv2connect.MachineServiceHandler.
func (m *machineServiceServer) List(ctx context.Context, rq *apiv2.MachineServiceListRequest) (*apiv2.MachineServiceListResponse, error) {
	machines, err := m.repo.Machine(rq.Project).List(ctx, rq.Query)
	if err != nil {
		return nil, errorutil.Convert(err)
	}
	return &apiv2.MachineServiceListResponse{Machines: machines}, nil
}

// Update implements apiv2connect.MachineServiceHandler.
func (m *machineServiceServer) Update(ctx context.Context, req *apiv2.MachineServiceUpdateRequest) (*apiv2.MachineServiceUpdateResponse, error) {
	machine, err := m.repo.Machine(req.Project).Update(ctx, req.Uuid, req)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return &apiv2.MachineServiceUpdateResponse{Machine: machine}, nil
}

// Delete implements apiv2connect.MachineServiceHandler.
func (m *machineServiceServer) Delete(context.Context, *apiv2.MachineServiceDeleteRequest) (*apiv2.MachineServiceDeleteResponse, error) {
	panic("unimplemented")
}

func (m *machineServiceServer) BMCCommand(ctx context.Context, req *apiv2.MachineServiceBMCCommandRequest) (*apiv2.MachineServiceBMCCommandResponse, error) {
	machine, err := m.repo.Machine(req.Project).Get(ctx, req.Uuid)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	bmc, err := m.repo.Machine(req.Project).AdditionalMethods().GetBMC(ctx, &adminv2.MachineServiceGetBMCRequest{Uuid: req.Uuid})
	if err != nil {
		return nil, err
	}
	if bmc.Bmc == nil || bmc.Bmc.Bmc == nil {
		return nil, errorutil.FailedPrecondition("machine %s does not have bmc details yet", req.Uuid)
	}
	if bmc.Bmc.Bmc.Address == "" || bmc.Bmc.Bmc.Password == "" || bmc.Bmc.Bmc.User == "" {
		return nil, errorutil.FailedPrecondition("machine %s does not have bmc connections details yet", req.Uuid)
	}

	err = m.repo.Machine(req.Project).AdditionalMethods().MachineBMCCommand(ctx, machine.Uuid, machine.Partition.Id, req.Command)
	if err != nil {
		return nil, err
	}
	return &apiv2.MachineServiceBMCCommandResponse{}, nil
}

func (m *machineServiceServer) GetBMC(ctx context.Context, req *apiv2.MachineServiceGetBMCRequest) (*apiv2.MachineServiceGetBMCResponse, error) {
	resp, err := m.repo.Machine(req.Project).AdditionalMethods().GetBMC(ctx, &adminv2.MachineServiceGetBMCRequest{Uuid: req.Uuid})
	if err != nil {
		return nil, err
	}
	return &apiv2.MachineServiceGetBMCResponse{
		Uuid: req.Uuid,
		Bmc:  resp.Bmc,
	}, nil
}
