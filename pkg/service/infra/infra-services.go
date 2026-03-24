package infra

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/metal-stack/api/go/metalstack/infra/v2/infrav2connect"
	"github.com/metal-stack/metal-apiserver/pkg/repository"
	"github.com/metal-stack/metal-apiserver/pkg/service/infra/bmc"
	"github.com/metal-stack/metal-apiserver/pkg/service/infra/boot"
	componentinfra "github.com/metal-stack/metal-apiserver/pkg/service/infra/component"
	eventinfra "github.com/metal-stack/metal-apiserver/pkg/service/infra/event"
	switchinfra "github.com/metal-stack/metal-apiserver/pkg/service/infra/switch"

	"connectrpc.com/connect"
)

type Config struct {
	Log                  *slog.Logger
	Repository           *repository.Store
	Mux                  *http.ServeMux
	Interceptors         connect.Option
	ComponentExpiration  time.Duration
	BMCSuperuserPassword string
}

func InfraServices(cfg Config) {

	// Infra services, we use adminInterceptors to prevent rate limiting
	var (
		bmcService            = bmc.New(bmc.Config{Log: cfg.Log, Repo: cfg.Repository})
		bootService           = boot.New(boot.Config{Log: cfg.Log, Repo: cfg.Repository, BMCSuperuserPassword: cfg.BMCSuperuserPassword})
		infraComponentService = componentinfra.New(componentinfra.Config{Log: cfg.Log, Repo: cfg.Repository, Expiration: cfg.ComponentExpiration})
		infraEventService     = eventinfra.New(eventinfra.Config{Log: cfg.Log, Repo: cfg.Repository})
		infraSwitchService    = switchinfra.New(switchinfra.Config{Log: cfg.Log, Repo: cfg.Repository})
	)

	cfg.Mux.Handle(infrav2connect.NewBMCServiceHandler(bmcService, cfg.Interceptors))
	cfg.Mux.Handle(infrav2connect.NewBootServiceHandler(bootService, cfg.Interceptors))
	cfg.Mux.Handle(infrav2connect.NewComponentServiceHandler(infraComponentService, cfg.Interceptors))
	cfg.Mux.Handle(infrav2connect.NewEventServiceHandler(infraEventService, cfg.Interceptors))
	cfg.Mux.Handle(infrav2connect.NewSwitchServiceHandler(infraSwitchService, cfg.Interceptors))
}
