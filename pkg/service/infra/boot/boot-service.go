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
	Log                  *slog.Logger
	Repo                 *repository.Store
	BMCSuperuserPassword string
}

type bootServiceServer struct {
	log                  *slog.Logger
	repo                 *repository.Store
	bmcSuperuserPassword string
}

func New(c Config) infrav2connect.BootServiceHandler {
	return &bootServiceServer{
		log:                  c.Log.WithGroup("bootService"),
		repo:                 c.Repo,
		bmcSuperuserPassword: c.BMCSuperuserPassword,
	}
}

func (b *bootServiceServer) Boot(ctx context.Context, req *infrav2.BootServiceBootRequest) (*infrav2.BootServiceBootResponse, error) {
	b.log.Info("boot", "req", req)

	p, err := b.repo.Partition().Get(ctx, req.Partition)
	if err != nil {
		return nil, err
	}

	resp := &infrav2.BootServiceBootResponse{
		Kernel:       p.BootConfiguration.KernelUrl,
		InitRamDisks: []string{p.BootConfiguration.ImageUrl},
		Cmdline:      &p.BootConfiguration.Commandline,
	}
	b.log.Info("boot", "resp", resp)
	return resp, nil
}

func (b *bootServiceServer) Dhcp(ctx context.Context, req *infrav2.BootServiceDhcpRequest) (*infrav2.BootServiceDhcpResponse, error) {
	b.log.Info("dhcp", "req", req)
	return b.repo.UnscopedMachine().AdditionalMethods().Dhcp(ctx, req)
}

func (b *bootServiceServer) Register(ctx context.Context, req *infrav2.BootServiceRegisterRequest) (*infrav2.BootServiceRegisterResponse, error) {
	m, err := b.repo.UnscopedMachine().AdditionalMethods().Register(ctx, req)
	if err != nil {
		return nil, err
	}

	return &infrav2.BootServiceRegisterResponse{
		Uuid:      m.ID,
		Size:      m.SizeID,
		Partition: m.PartitionID,
	}, nil
}

func (b *bootServiceServer) InstallationSucceeded(ctx context.Context, req *infrav2.BootServiceInstallationSucceededRequest) (*infrav2.BootServiceInstallationSucceededResponse, error) {
	_, err := b.repo.UnscopedMachine().AdditionalMethods().InstallationSucceeded(ctx, req)
	if err != nil {
		return nil, err
	}
	return &infrav2.BootServiceInstallationSucceededResponse{}, nil
}

func (b *bootServiceServer) SuperUserPassword(ctx context.Context, req *infrav2.BootServiceSuperUserPasswordRequest) (*infrav2.BootServiceSuperUserPasswordResponse, error) {
	b.log.Info("superuserpassword", "req", req)

	resp := &infrav2.BootServiceSuperUserPasswordResponse{
		FeatureDisabled:   b.bmcSuperuserPassword == "",
		SuperUserPassword: b.bmcSuperuserPassword,
	}

	return resp, nil
}

func (b *bootServiceServer) Wait(ctx context.Context, req *infrav2.BootServiceWaitRequest, srv *connect.ServerStream[infrav2.BootServiceWaitResponse]) error {
	return b.repo.UnscopedMachine().AdditionalMethods().Wait(ctx, req, srv)
}
