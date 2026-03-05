package admin

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"strings"

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

type switchServiceServer struct {
	log  *slog.Logger
	repo *repository.Store
}

func New(c Config) adminv2connect.SwitchServiceHandler {
	return &switchServiceServer{
		log:  c.Log.WithGroup("switchService"),
		repo: c.Repo,
	}
}

func (s *switchServiceServer) Get(ctx context.Context, rq *adminv2.SwitchServiceGetRequest) (*adminv2.SwitchServiceGetResponse, error) {
	sw, err := s.repo.Switch().Get(ctx, rq.Id)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return &adminv2.SwitchServiceGetResponse{Switch: sw}, nil
}

func (s *switchServiceServer) List(ctx context.Context, rq *adminv2.SwitchServiceListRequest) (*adminv2.SwitchServiceListResponse, error) {
	switches, err := s.repo.Switch().List(ctx, rq.Query)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	slices.SortFunc(switches, func(s1, s2 *apiv2.Switch) int {
		return strings.Compare(s1.Id, s2.Id)
	})

	return &adminv2.SwitchServiceListResponse{Switches: switches}, nil
}

func (s *switchServiceServer) Update(ctx context.Context, rq *adminv2.SwitchServiceUpdateRequest) (*adminv2.SwitchServiceUpdateResponse, error) {
	sw, err := s.repo.Switch().Update(ctx, rq.Id, rq)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return &adminv2.SwitchServiceUpdateResponse{Switch: sw}, nil
}

func (s *switchServiceServer) Delete(ctx context.Context, rq *adminv2.SwitchServiceDeleteRequest) (*adminv2.SwitchServiceDeleteResponse, error) {
	if rq.Force {
		return s.forceDelete(ctx, rq.Id)
	}

	sw, err := s.repo.Switch().Delete(ctx, rq.Id)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return &adminv2.SwitchServiceDeleteResponse{Switch: sw}, nil
}

func (s *switchServiceServer) Migrate(ctx context.Context, rq *adminv2.SwitchServiceMigrateRequest) (*adminv2.SwitchServiceMigrateResponse, error) {
	sw, err := s.repo.Switch().AdditionalMethods().Migrate(ctx, rq.OldSwitch, rq.NewSwitch)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return &adminv2.SwitchServiceMigrateResponse{Switch: sw}, nil
}

func (s *switchServiceServer) Port(ctx context.Context, rq *adminv2.SwitchServicePortRequest) (*adminv2.SwitchServicePortResponse, error) {
	sw, err := s.repo.Switch().AdditionalMethods().Port(ctx, rq.Id, rq.NicName, rq.Status)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return &adminv2.SwitchServicePortResponse{Switch: sw}, nil
}

func (s *switchServiceServer) ConnectedMachines(ctx context.Context, rq *adminv2.SwitchServiceConnectedMachinesRequest) (*adminv2.SwitchServiceConnectedMachinesResponse, error) {
	machines, err := s.repo.UnscopedMachine().List(ctx, rq.MachineQuery)
	if err != nil {
		return nil, errorutil.Convert(fmt.Errorf("failed to list machines: %w", err))
	}

	switches, err := s.repo.Switch().List(ctx, rq.Query)
	if err != nil {
		return nil, errorutil.Convert(fmt.Errorf("failed to list switches: %w", err))
	}

	switchesWithConnectedMachines, err := s.repo.Switch().AdditionalMethods().GetSwitchesWithConnectedMachines(ctx, rq.MachineQuery, switches, machines)
	if err != nil {
		return nil, errorutil.Convert(fmt.Errorf("failed to get switches with connected machines: %w", err))
	}

	return &adminv2.SwitchServiceConnectedMachinesResponse{SwitchesWithMachines: switchesWithConnectedMachines}, nil
}

func (s *switchServiceServer) forceDelete(ctx context.Context, id string) (*adminv2.SwitchServiceDeleteResponse, error) {
	sw, err := s.repo.Switch().AdditionalMethods().ForceDelete(ctx, id)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return &adminv2.SwitchServiceDeleteResponse{Switch: sw}, nil
}
