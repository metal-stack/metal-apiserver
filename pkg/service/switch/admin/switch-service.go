package admin

import (
	"context"
	"log/slog"
	"slices"
	"strings"

	"connectrpc.com/connect"
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

func (s *switchServiceServer) Get(ctx context.Context, rq *connect.Request[adminv2.SwitchServiceGetRequest]) (*connect.Response[adminv2.SwitchServiceGetResponse], error) {
	sw, err := s.repo.Switch().Get(ctx, rq.Msg.Id)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	converted, err := s.repo.Switch().ConvertToProto(ctx, sw)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return connect.NewResponse(&adminv2.SwitchServiceGetResponse{Switch: converted}), nil
}

func (s *switchServiceServer) List(ctx context.Context, rq *connect.Request[adminv2.SwitchServiceListRequest]) (*connect.Response[adminv2.SwitchServiceListResponse], error) {
	switches, err := s.repo.Switch().List(ctx, rq.Msg.Query)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	var res []*apiv2.Switch
	for _, sw := range switches {
		converted, err := s.repo.Switch().ConvertToProto(ctx, sw)
		if err != nil {
			return nil, errorutil.Convert(err)
		}
		res = append(res, converted)
	}

	slices.SortFunc(res, func(s1, s2 *apiv2.Switch) int {
		return strings.Compare(s1.Id, s2.Id)
	})

	return connect.NewResponse(&adminv2.SwitchServiceListResponse{Switches: res}), nil
}

func (s *switchServiceServer) Update(ctx context.Context, rq *connect.Request[adminv2.SwitchServiceUpdateRequest]) (*connect.Response[adminv2.SwitchServiceUpdateResponse], error) {
	sw, err := s.repo.Switch().Update(ctx, rq.Msg.Id, rq.Msg)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	converted, err := s.repo.Switch().ConvertToProto(ctx, sw)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return connect.NewResponse(&adminv2.SwitchServiceUpdateResponse{Switch: converted}), nil
}

func (s *switchServiceServer) Delete(ctx context.Context, rq *connect.Request[adminv2.SwitchServiceDeleteRequest]) (*connect.Response[adminv2.SwitchServiceDeleteResponse], error) {
	sw, err := s.repo.Switch().Delete(ctx, rq.Msg.Id)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	converted, err := s.repo.Switch().ConvertToProto(ctx, sw)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return connect.NewResponse(&adminv2.SwitchServiceDeleteResponse{Switch: converted}), nil
}

func (s *switchServiceServer) Migrate(ctx context.Context, rq *connect.Request[adminv2.SwitchServiceMigrateRequest]) (*connect.Response[adminv2.SwitchServiceMigrateResponse], error) {
	sw, err := s.repo.Switch().AdditionalMethods().Migrate(ctx, rq.Msg.OldSwitch, rq.Msg.NewSwitch)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	converted, err := s.repo.Switch().ConvertToProto(ctx, sw)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return connect.NewResponse(&adminv2.SwitchServiceMigrateResponse{Switch: converted}), nil
}

func (s *switchServiceServer) Port(ctx context.Context, rq *connect.Request[adminv2.SwitchServicePortRequest]) (*connect.Response[adminv2.SwitchServicePortResponse], error) {
	sw, err := s.repo.Switch().AdditionalMethods().Port(ctx, rq.Msg.Id, rq.Msg.NicName, rq.Msg.Status)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	converted, err := s.repo.Switch().ConvertToProto(ctx, sw)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return connect.NewResponse(&adminv2.SwitchServicePortResponse{Switch: converted}), nil
}
