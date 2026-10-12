package infra

import (
	"log/slog"
	"time"

	"github.com/metal-stack/api/go/metalstack/infra/v2/infrav2connect"
	"github.com/metal-stack/metal-apiserver/pkg/repository"
	"github.com/metal-stack/metal-apiserver/pkg/service/infra/bmc"
	"github.com/metal-stack/metal-apiserver/pkg/service/infra/boot"
	componentinfra "github.com/metal-stack/metal-apiserver/pkg/service/infra/component"
	eventinfra "github.com/metal-stack/metal-apiserver/pkg/service/infra/event"
	switchinfra "github.com/metal-stack/metal-apiserver/pkg/service/infra/switch"

	"connectrpc.com/connect/v2"
)

type Config struct {
	Log                  *slog.Logger
	Repository           *repository.Store
	Server               *connect.Server
	ComponentExpiration  time.Duration
	BMCSuperuserPassword string
}

func InfraServices(cfg Config) {

	var (
		bmcService            = bmc.New(bmc.Config{Log: cfg.Log, Repo: cfg.Repository})
		bootService           = boot.New(boot.Config{Log: cfg.Log, Repo: cfg.Repository, BMCSuperuserPassword: cfg.BMCSuperuserPassword})
		infraComponentService = componentinfra.New(componentinfra.Config{Log: cfg.Log, Repo: cfg.Repository, Expiration: cfg.ComponentExpiration})
		infraEventService     = eventinfra.New(eventinfra.Config{Log: cfg.Log, Repo: cfg.Repository})
		infraSwitchService    = switchinfra.New(switchinfra.Config{Log: cfg.Log, Repo: cfg.Repository})
	)

	infrav2connect.RegisterBMCServiceHandler(cfg.Server, bmcService)
	infrav2connect.RegisterBootServiceHandler(cfg.Server, bootService)
	infrav2connect.RegisterComponentServiceHandler(cfg.Server, infraComponentService)
	infrav2connect.RegisterEventServiceHandler(cfg.Server, infraEventService)
	infrav2connect.RegisterSwitchServiceHandler(cfg.Server, infraSwitchService)
}
