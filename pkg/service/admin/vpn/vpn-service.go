package admin

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"connectrpc.com/connect"
	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	"github.com/metal-stack/api/go/metalstack/admin/v2/adminv2connect"
	"github.com/metal-stack/metal-apiserver/pkg/repository"
)

const defaultExpiration = time.Hour

type Config struct {
	Log  *slog.Logger
	Repo *repository.Store
}

type vpnService struct {
	log      *slog.Logger
	repo     *repository.Store
	disabled bool
}

func New(c Config) adminv2connect.VPNServiceHandler {
	return &vpnService{
		log:      c.Log.WithGroup("vpnService"),
		repo:     c.Repo,
		disabled: c.Repo.UnscopedVPN().Enabled(),
	}
}

func (v *vpnService) AuthKey(ctx context.Context, req *adminv2.VPNServiceAuthKeyRequest) (*adminv2.VPNServiceAuthKeyResponse, error) {
	if v.disabled {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("vpn is currently disabled"))
	}

	key, err := v.repo.VPN(req.Project).CreateAuthKey(ctx, req)
	if err != nil {
		return nil, err
	}

	return key, nil
}

func (v *vpnService) ListNodes(ctx context.Context, req *adminv2.VPNServiceListNodesRequest) (*adminv2.VPNServiceListNodesResponse, error) {
	if v.disabled {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("vpn is currently disabled"))
	}

	nodes, err := v.repo.UnscopedVPN().ListNodes(ctx, req)
	if err != nil {
		return nil, err
	}

	return nodes, nil
}
