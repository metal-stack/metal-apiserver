package infra

import (
	"context"
	"log/slog"
	"time"

	"connectrpc.com/connect"
	"github.com/google/go-cmp/cmp"
	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	infrav2 "github.com/metal-stack/api/go/metalstack/infra/v2"
	"github.com/metal-stack/api/go/metalstack/infra/v2/infrav2connect"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	"github.com/metal-stack/metal-apiserver/pkg/repository"
	"google.golang.org/protobuf/types/known/durationpb"
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

func (s *switchServiceServer) Get(ctx context.Context, rq *connect.Request[infrav2.SwitchServiceGetRequest]) (*connect.Response[infrav2.SwitchServiceGetResponse], error) {
	sw, err := s.repo.Switch().Get(ctx, rq.Msg.Id)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	converted, err := s.repo.Switch().ConvertToProto(ctx, sw)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return connect.NewResponse(&infrav2.SwitchServiceGetResponse{Switch: converted}), nil
}

func (s *switchServiceServer) Register(ctx context.Context, rq *connect.Request[infrav2.SwitchServiceRegisterRequest]) (*connect.Response[infrav2.SwitchServiceRegisterResponse], error) {
	sw, err := s.repo.Switch().AdditionalMethods().Register(ctx, rq.Msg)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	converted, err := s.repo.Switch().ConvertToProto(ctx, sw)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return connect.NewResponse(&infrav2.SwitchServiceRegisterResponse{Switch: converted}), nil
}

func (s *switchServiceServer) Heartbeat(ctx context.Context, rq *connect.Request[infrav2.SwitchServiceHeartbeatRequest]) (*connect.Response[infrav2.SwitchServiceHeartbeatResponse], error) {
	req := rq.Msg

	status, err := s.repo.Switch().AdditionalMethods().GetSwitchStatus(ctx, req.Id)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	lastSync := &metal.SwitchSync{
		Time:     time.Now(),
		Duration: req.Duration.AsDuration(),
		Error:    req.Error,
	}

	if req.Error == nil {
		status.LastSync = lastSync
	} else {
		status.LastSyncError = lastSync
	}

	err = s.repo.Switch().AdditionalMethods().SetSwitchStatus(ctx, status)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	sw, err := s.repo.Switch().Get(ctx, req.Id)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	var updated bool
	if req.PortStates != nil {
		for i, nic := range sw.Nics {
			reported, ok := req.PortStates[nic.Name]
			if !ok {
				return nil, errorutil.Internal("failed to determine switch port state because port %s was not found on switch", nic.Name)
			}

			state, err := metal.ToSwitchPortStatus(reported)
			if err != nil {
				return nil, errorutil.InvalidArgument("failed to interpret switch port state: %w", err)
			}

			// FIXME: check nil
			newState, changed := nic.State.SetState(state)
			if changed {
				sw.Nics[i].State = newState
				updated = true
			}
		}
	}

	if req.BgpPortStates != nil {
		for i, nic := range sw.Nics {
			reported, ok := req.BgpPortStates[nic.Name]
			state, err := repository.ToSwitchBGPPortState(reported)
			if err != nil {
				return nil, errorutil.InvalidArgument("failed to convert bgp port state: %w", err)
			}

			switch {
			case !ok && nic.BGPPortState == nil, cmp.Diff(state, nic.BGPPortState) == "":
				continue
			case !ok:
				sw.Nics[i].BGPPortState = nil
			default:
				sw.Nics[i].BGPPortState = state
			}
			updated = true
		}
	}

	if updated {
		// FIXME: this is very expensive
		// try to find a solution for updating only port states and bgp states without contructing the entire switch
		newSwitch, err := s.repo.Switch().ConvertToProto(ctx, sw)
		if err != nil {
			return nil, err
		}

		updateReq := &adminv2.SwitchServiceUpdateRequest{
			Id:   newSwitch.Id,
			Nics: newSwitch.Nics,
		}
		_, err = s.repo.Switch().Update(ctx, req.Id, updateReq)
		if err != nil {
			return nil, err
		}
	}

	res := &infrav2.SwitchServiceHeartbeatResponse{
		Id: status.ID,
		LastSync: &infrav2.SwitchSync{
			Time:     timestamppb.New(status.LastSync.Time),
			Duration: durationpb.New(status.LastSync.Duration),
			Error:    status.LastSync.Error,
		},
		LastSyncError: &infrav2.SwitchSync{
			Time:     timestamppb.New(status.LastSyncError.Time),
			Duration: durationpb.New(status.LastSyncError.Duration),
			Error:    status.LastSyncError.Error,
		},
	}

	return connect.NewResponse(res), nil
}
