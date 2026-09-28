package boot

import (
	"context"
	"log/slog"
	"time"

	"connectrpc.com/connect"
	"github.com/metal-stack/api/go/errorutil"
	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	infrav2 "github.com/metal-stack/api/go/metalstack/infra/v2"
	"github.com/metal-stack/api/go/metalstack/infra/v2/infrav2connect"
	"github.com/metal-stack/metal-apiserver/pkg/repository"
	"github.com/metal-stack/metal-apiserver/pkg/token"
	"google.golang.org/protobuf/types/known/durationpb"
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
	p, err := b.repo.Partition().Get(ctx, req.Partition)
	if err != nil {
		return nil, err
	}

	resp := &infrav2.BootServiceBootResponse{
		Kernel:       p.BootConfiguration.KernelUrl,
		InitRamDisks: []string{p.BootConfiguration.ImageUrl},
		Cmdline:      &p.BootConfiguration.Commandline,
	}

	return resp, nil
}

func (b *bootServiceServer) Dhcp(ctx context.Context, req *infrav2.BootServiceDhcpRequest) (*infrav2.BootServiceDhcpResponse, error) {
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
	resp := &infrav2.BootServiceSuperUserPasswordResponse{
		FeatureDisabled:   b.bmcSuperuserPassword == "",
		SuperUserPassword: b.bmcSuperuserPassword,
	}

	return resp, nil
}

func (b *bootServiceServer) Wait(ctx context.Context, req *infrav2.BootServiceWaitRequest, srv *connect.ServerStream[infrav2.BootServiceWaitResponse]) error {
	return b.repo.UnscopedMachine().AdditionalMethods().Wait(ctx, req, srv)
}

func (b *bootServiceServer) MachineToken(ctx context.Context, req *infrav2.BootServiceMachineTokenRequest) (*infrav2.BootServiceMachineTokenResponse, error) {
	token, ok := token.TokenFromContext(ctx)
	if !ok || token == nil {
		return nil, errorutil.Unauthenticated("no token found in request")
	}

	if req.Expires == nil {
		req.Expires = durationpb.New(3 * 24 * time.Hour)
	}

	res, err := b.repo.Token(token.User).Create(ctx, &adminv2.TokenServiceCreateRequest{
		User: &req.User,
		TokenCreateRequest: &apiv2.TokenServiceCreateRequest{
			Description: "machine token for " + req.Uuid,
			Expires:     req.Expires,
			MachineRoles: map[string]apiv2.MachineRole{
				req.Uuid: apiv2.MachineRole_MACHINE_ROLE_EDITOR,
			},
			Labels: req.Labels,
		},
	})
	if err != nil {
		return nil, err
	}

	return &infrav2.BootServiceMachineTokenResponse{
		Token:  res.Token,
		Secret: res.Secret,
	}, nil
}
