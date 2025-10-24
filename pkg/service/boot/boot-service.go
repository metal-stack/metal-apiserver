package boot

import (
	"context"
	"log/slog"

	"connectrpc.com/connect"
	infrav2 "github.com/metal-stack/api/go/metalstack/infra/v2"
	"github.com/metal-stack/api/go/metalstack/infra/v2/infrav2connect"
	"github.com/metal-stack/metal-apiserver/pkg/repository"
)

type Config struct {
	Log  *slog.Logger
	Repo *repository.Store
}

type bootServiceServer struct {
	log  *slog.Logger
	repo *repository.Store
}

func New(c Config) infrav2connect.BootServiceHandler {
	return &bootServiceServer{
		log:  c.Log.WithGroup("bootService"),
		repo: c.Repo,
	}
}

// Boot implements infrav2connect.BootServiceHandler.
func (b *bootServiceServer) Boot(context.Context, *infrav2.BootServiceBootRequest) (*infrav2.BootServiceBootResponse, error) {
	panic("unimplemented")
}

// Dhcp implements infrav2connect.BootServiceHandler.
func (b *bootServiceServer) Dhcp(context.Context, *infrav2.BootServiceDhcpRequest) (*infrav2.BootServiceDhcpResponse, error) {
	panic("unimplemented")
}

// Register implements infrav2connect.BootServiceHandler.
func (b *bootServiceServer) Register(ctx context.Context, rq *infrav2.BootServiceRegisterRequest) (*infrav2.BootServiceRegisterResponse, error) {
	m, err := b.repo.UnscopedMachine().AdditionalMethods().Register(ctx, rq)
	if err != nil {
		return nil, err
	}

	return &infrav2.BootServiceRegisterResponse{
		Uuid:      m.ID,
		Size:      m.SizeID,
		Partition: m.PartitionID,
	}, nil
}

// Report implements infrav2connect.BootServiceHandler.
func (b *bootServiceServer) Report(context.Context, *infrav2.BootServiceReportRequest) (*infrav2.BootServiceReportResponse, error) {
	panic("unimplemented")
}

// SuperUserPassword implements infrav2connect.BootServiceHandler.
func (b *bootServiceServer) SuperUserPassword(context.Context, *infrav2.BootServiceSuperUserPasswordRequest) (*infrav2.BootServiceSuperUserPasswordResponse, error) {
	panic("unimplemented")
}

// Wait implements infrav2connect.BootServiceHandler.
func (b *bootServiceServer) Wait(context.Context, *infrav2.BootServiceWaitRequest, *connect.ServerStream[infrav2.BootServiceWaitResponse]) error {
	panic("unimplemented")
}
