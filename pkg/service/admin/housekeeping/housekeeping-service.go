package housekeeping

import (
	"context"
	"log/slog"

	"github.com/metal-stack/api/go/errorutil"
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

func (m *housekeepingServiceServer) Machine(ctx context.Context, req *adminv2.HousekeepingServiceMachineRequest) (*adminv2.HousekeepingServiceMachineResponse, error) {
	var (
		machines []string
		err      error
	)
	switch req.Action {
	case adminv2.MachineHousekeepingAction_MACHINE_HOUSEKEEPING_ACTION_LIVELINESS:
		machines, err = m.repo.UnscopedMachine().AdditionalMethods().MachineLiveliness(ctx)
		if err != nil {
			return nil, err
		}
	case adminv2.MachineHousekeepingAction_MACHINE_HOUSEKEEPING_ACTION_RESURRECT:
		machines, err = m.repo.UnscopedMachine().AdditionalMethods().Resurrect(ctx)
		if err != nil {
			return nil, err
		}
	case adminv2.MachineHousekeepingAction_MACHINE_HOUSEKEEPING_ACTION_VPN_CONNECTED:
		ms, err := vpn.EvaluateVPNConnected(ctx, m.log, m.repo)
		if err != nil {
			return nil, err
		}
		for _, m := range ms {
			machines = append(machines, m.Uuid)
		}
	default:
		return nil, errorutil.InvalidArgument("given action is not supported %q", req.Action)
	}

	return &adminv2.HousekeepingServiceMachineResponse{Action: req.Action, ModifiedMachineIds: machines}, nil
}
