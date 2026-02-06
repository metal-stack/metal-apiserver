package admin

import (
	"context"
	"log/slog"

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
func (m *machineServiceServer) Get(ctx context.Context, req *adminv2.MachineServiceGetRequest) (*adminv2.MachineServiceGetResponse, error) {
	machine, err := m.repo.UnscopedMachine().Get(ctx, req.Uuid)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return &adminv2.MachineServiceGetResponse{
		Machine: machine,
	}, nil
}

// List implements apiv2connect.MachineServiceHandler.
func (m *machineServiceServer) List(ctx context.Context, rq *adminv2.MachineServiceListRequest) (*adminv2.MachineServiceListResponse, error) {
	partitions, err := m.repo.Partition().List(ctx, &apiv2.PartitionQuery{})
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	partition := rq.Partition
	if partition == nil {
		if len(partitions) > 1 {
			return nil, errorutil.InvalidArgument("no partition specified, but %d partitions available", len(partitions))
		}
		if len(partitions) == 1 {
			partition = &partitions[0].Id
		}
	}

	if rq.Query == nil {
		rq.Query = &apiv2.MachineQuery{}
	}
	q := rq.Query
	q.Partition = partition

	machines, err := m.repo.UnscopedMachine().List(ctx, q)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return &adminv2.MachineServiceListResponse{Machines: machines}, nil
}

func (m *machineServiceServer) BMCCommand(ctx context.Context, req *adminv2.MachineServiceBMCCommandRequest) (*adminv2.MachineServiceBMCCommandResponse, error) {
	machine, err := m.repo.UnscopedMachine().Get(ctx, req.Uuid)
	if err != nil {
		return nil, errorutil.Convert(err)
	}
	resp, err := m.repo.UnscopedMachine().AdditionalMethods().GetBMC(ctx, &adminv2.MachineServiceGetBMCRequest{Uuid: req.Uuid})
	if err != nil {
		return nil, err
	}
	if resp.Bmc == nil || resp.Bmc.Bmc == nil {
		return nil, errorutil.FailedPrecondition("machine %s does not have bmc details yet", req.Uuid)
	}
	if resp.Bmc.Bmc.Address == "" || resp.Bmc.Bmc.Password == "" || resp.Bmc.Bmc.User == "" {
		return nil, errorutil.FailedPrecondition("machine %s does not have bmc connections details yet", req.Uuid)
	}

	err = m.repo.UnscopedMachine().AdditionalMethods().MachineBMCCommand(ctx, machine.Uuid, machine.Partition.Id, req.Command)
	if err != nil {
		return nil, err
	}
	return &adminv2.MachineServiceBMCCommandResponse{}, nil
}

func (m *machineServiceServer) GetBMC(ctx context.Context, req *adminv2.MachineServiceGetBMCRequest) (*adminv2.MachineServiceGetBMCResponse, error) {
	return m.repo.UnscopedMachine().AdditionalMethods().GetBMC(ctx, req)
}

func (m *machineServiceServer) ListBMC(ctx context.Context, req *adminv2.MachineServiceListBMCRequest) (*adminv2.MachineServiceListBMCResponse, error) {
	return m.repo.UnscopedMachine().AdditionalMethods().ListBMC(ctx, req)
}

func (m *machineServiceServer) ConsolePassword(ctx context.Context, req *adminv2.MachineServiceConsolePasswordRequest) (*adminv2.MachineServiceConsolePasswordResponse, error) {
	password, err := m.repo.UnscopedMachine().AdditionalMethods().GetConsolePassword(ctx, req.Uuid)
	if err != nil {
		return nil, err
	}

	return &adminv2.MachineServiceConsolePasswordResponse{
		Uuid:     req.Uuid,
		Password: password,
	}, nil
}
