package admin

import (
	"context"
	"log/slog"

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
func (m *machineServiceServer) Get(ctx context.Context, rq *connect.Request[adminv2.MachineServiceGetRequest]) (*connect.Response[adminv2.MachineServiceGetResponse], error) {
	req := rq.Msg

	resp, err := m.repo.UnscopedMachine().Get(ctx, req.Uuid)
	if err != nil {
		return nil, errorutil.Convert(err)
	}
	converted, err := m.repo.UnscopedMachine().ConvertToProto(ctx, resp)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return connect.NewResponse(&adminv2.MachineServiceGetResponse{
		Machine: converted,
	}), nil
}

// List implements apiv2connect.MachineServiceHandler.
func (m *machineServiceServer) List(ctx context.Context, rq *connect.Request[adminv2.MachineServiceListRequest]) (*connect.Response[adminv2.MachineServiceListResponse], error) {
	partitions, err := m.repo.Partition().List(ctx, &apiv2.PartitionQuery{})
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	partition := rq.Msg.Partition
	if partition == "" {
		if len(partitions) > 1 {
			return nil, errorutil.InvalidArgument("no partition specified, but %d partitions available", len(partitions))
		}
		if len(partitions) == 1 {
			partition = partitions[0].ID
		}
	}

	q := rq.Msg.Query
	q.Partition = &partition

	machines, err := m.repo.UnscopedMachine().List(ctx, q)
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

	return connect.NewResponse(&adminv2.MachineServiceListResponse{Machines: result}), nil
}
