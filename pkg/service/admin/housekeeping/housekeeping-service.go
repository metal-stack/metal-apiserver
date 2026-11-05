package housekeeping

import (
	"context"
	"log/slog"

	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	"github.com/metal-stack/api/go/metalstack/admin/v2/adminv2connect"
	"github.com/metal-stack/metal-apiserver/pkg/repository"
	"github.com/metal-stack/metal-apiserver/pkg/vpn"
)

type Config struct {
	Log  *slog.Logger
	Repo *repository.Store
}

type housekeepingServiceServer struct {
	log  *slog.Logger
	repo *repository.Store
}

func New(c Config) adminv2connect.HousekeepingServiceHandler {
	return &housekeepingServiceServer{
		log:  c.Log.WithGroup("housekeepingServiceServer"),
		repo: c.Repo,
	}
}

func (m *housekeepingServiceServer) MachineLiveliness(ctx context.Context, _ *adminv2.HousekeepingServiceMachineLivelinessRequest) (*adminv2.HousekeepingServiceMachineLivelinessResponse, error) {
	modified, err := m.repo.UnscopedMachine().AdditionalMethods().MachineLiveliness(ctx)
	if err != nil {
		return nil, err
	}
	return &adminv2.HousekeepingServiceMachineLivelinessResponse{
		ModifiedMachines: modified,
	}, nil
}

func (m *housekeepingServiceServer) MachineResurrect(ctx context.Context, _ *adminv2.HousekeepingServiceMachineResurrectRequest) (*adminv2.HousekeepingServiceMachineResurrectResponse, error) {
	modified, err := m.repo.UnscopedMachine().AdditionalMethods().Resurrect(ctx)
	if err != nil {
		return nil, err
	}

	return &adminv2.HousekeepingServiceMachineResurrectResponse{
		ModifiedMachines: modified,
	}, nil
}

func (m *housekeepingServiceServer) MachineVPNConnected(ctx context.Context, _ *adminv2.HousekeepingServiceMachineVPNConnectedRequest) (*adminv2.HousekeepingServiceMachineVPNConnectedResponse, error) {
	modified, err := vpn.EvaluateVPNConnected(ctx, m.log, m.repo)
	if err != nil {
		return nil, err
	}
	return &adminv2.HousekeepingServiceMachineVPNConnectedResponse{
		ModifiedMachines: modified,
	}, nil
}
