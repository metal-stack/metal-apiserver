package infra

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/go-cmp/cmp"
	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	infrav2 "github.com/metal-stack/api/go/metalstack/infra/v2"
	"github.com/metal-stack/api/go/metalstack/infra/v2/infrav2connect"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	"github.com/metal-stack/metal-apiserver/pkg/repository"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type Config struct {
	Log  *slog.Logger
	Repo *repository.Store
}

type switchServiceServer struct {
	log  *slog.Logger
	repo *repository.Store
}

func New(c Config) infrav2connect.SwitchServiceHandler {
	return &switchServiceServer{
		log:  c.Log.WithGroup("switchService"),
		repo: c.Repo,
	}
}

func (s *switchServiceServer) Get(ctx context.Context, rq *infrav2.SwitchServiceGetRequest) (*infrav2.SwitchServiceGetResponse, error) {
	sw, err := s.repo.Switch().Get(ctx, rq.Id)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return &infrav2.SwitchServiceGetResponse{Switch: sw}, nil
}

func (s *switchServiceServer) Register(ctx context.Context, rq *infrav2.SwitchServiceRegisterRequest) (*infrav2.SwitchServiceRegisterResponse, error) {
	sw, err := s.repo.Switch().AdditionalMethods().Register(ctx, rq)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return &infrav2.SwitchServiceRegisterResponse{Switch: sw}, nil
}

func (s *switchServiceServer) Heartbeat(ctx context.Context, rq *infrav2.SwitchServiceHeartbeatRequest) (*infrav2.SwitchServiceHeartbeatResponse, error) {
	status, err := s.repo.Switch().AdditionalMethods().GetSwitchStatus(ctx, rq.Id)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	lastSync := &apiv2.SwitchSync{
		Time:     timestamppb.New(time.Now()),
		Duration: rq.Duration,
		Error:    rq.Error,
	}

	if rq.Error == nil {
		status.LastSync = lastSync
	} else {
		status.LastSyncError = lastSync
	}

	err = s.repo.Switch().AdditionalMethods().SetSwitchStatus(ctx, status)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	sw, err := s.repo.Switch().Get(ctx, rq.Id)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	var updated bool
	if rq.PortStates != nil {
		for i, nic := range sw.Nics {
			reportedState, ok := rq.PortStates[nic.Name]
			if !ok {
				return nil, errorutil.Internal("failed to determine switch port state because port %s was not found on switch", nic.Name)
			}

			newState, changed := repository.GetNewNicState(nic.State, reportedState)
			if changed {
				sw.Nics[i].State = newState
				updated = true
			}
		}
	}

	if rq.BgpPortStates != nil {
		for i, nic := range sw.Nics {
			reportedState, ok := rq.BgpPortStates[nic.Name]

			switch {
			case !ok && nic.BgpPortState == nil, cmp.Diff(reportedState, nic.BgpPortState, protocmp.Transform()) == "":
				continue
			case !ok:
				sw.Nics[i].BgpPortState = nil
			default:
				sw.Nics[i].BgpPortState = reportedState
			}
			updated = true
		}
	}

	if updated {
		updateReq := &adminv2.SwitchServiceUpdateRequest{
			Id: sw.Id,
			UpdateMeta: &apiv2.UpdateMeta{
				UpdatedAt:       timestamppb.New(time.Now()),
				LockingStrategy: apiv2.OptimisticLockingStrategy_OPTIMISTIC_LOCKING_STRATEGY_SERVER,
			},
			Nics: sw.Nics,
		}
		_, err := s.repo.Switch().Update(ctx, rq.Id, updateReq)
		if err != nil {
			return nil, err
		}
	}

	res := &infrav2.SwitchServiceHeartbeatResponse{
		Id:            status.ID,
		LastSync:      status.LastSync,
		LastSyncError: status.LastSyncError,
	}

	return res, nil
}
