package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"connectrpc.com/connect"
	"github.com/metal-stack/api/go/metalstack/api/v2/apiv2connect"
	"github.com/metal-stack/api/go/metalstack/api/v2/apiv2mcp"
	"github.com/metal-stack/metal-apiserver/pkg/certs"
	"github.com/metal-stack/metal-apiserver/pkg/db/generic"
	"github.com/metal-stack/metal-apiserver/pkg/headscale"
	"github.com/metal-stack/metal-apiserver/pkg/invite"
	"github.com/metal-stack/metal-apiserver/pkg/repository"
	"github.com/metal-stack/metal-lib/auditing"
	"github.com/redpanda-data/protoc-gen-go-mcp/pkg/runtime"
	"github.com/valkey-io/valkey-go"

	ipamv1connect "github.com/metal-stack/go-ipam/api/v1/apiv1connect"
	"github.com/metal-stack/metal-apiserver/pkg/service/api/audit"
	"github.com/metal-stack/metal-apiserver/pkg/service/api/filesystem"
	"github.com/metal-stack/metal-apiserver/pkg/service/api/health"
	"github.com/metal-stack/metal-apiserver/pkg/service/api/image"
	"github.com/metal-stack/metal-apiserver/pkg/service/api/ip"
	"github.com/metal-stack/metal-apiserver/pkg/service/api/machine"
	"github.com/metal-stack/metal-apiserver/pkg/service/api/method"
	"github.com/metal-stack/metal-apiserver/pkg/service/api/network"
	"github.com/metal-stack/metal-apiserver/pkg/service/api/partition"
	"github.com/metal-stack/metal-apiserver/pkg/service/api/project"
	"github.com/metal-stack/metal-apiserver/pkg/service/api/size"
	sizeimageconstraint "github.com/metal-stack/metal-apiserver/pkg/service/api/size-image-constraint"
	sizereservation "github.com/metal-stack/metal-apiserver/pkg/service/api/size-reservation"
	"github.com/metal-stack/metal-apiserver/pkg/service/api/tenant"
	"github.com/metal-stack/metal-apiserver/pkg/service/api/token"
	"github.com/metal-stack/metal-apiserver/pkg/service/api/user"
	"github.com/metal-stack/metal-apiserver/pkg/service/api/version"

	tokencommon "github.com/metal-stack/metal-apiserver/pkg/token"
	tenantclient "github.com/metal-stack/tenant-api/go/client"
)

type Config struct {
	Log                *slog.Logger
	Datastore          generic.Datastore
	Repository         *repository.Store
	TenantClient       tenantclient.Client
	IpamClient         ipamv1connect.IpamServiceClient
	Mux                *http.ServeMux
	Interceptors       connect.Option
	ProjectInviteStore invite.ProjectInviteStore
	TenantInviteStore  invite.TenantInviteStore
	TokenStore         tokencommon.TokenStore
	CertStore          certs.CertStore
	AuditSearchBackend auditing.Auditing
	Valkey             valkey.Client
	AuditBackends      []auditing.Auditing
	HeadscaleClient    *headscale.Client
	MCPServer          runtime.MCPServer

	ServerHttpURL  string
	ProviderTenant string
}

func ApiServices(ctx context.Context, cfg Config) error {
	var (
		auditService      = audit.New(audit.Config{Log: cfg.Log, Repo: cfg.Repository, AuditClient: cfg.AuditSearchBackend})
		filesystemService = filesystem.New(filesystem.Config{Log: cfg.Log, Repo: cfg.Repository})
		imageService      = image.New(image.Config{Log: cfg.Log, Repo: cfg.Repository})
		ipService         = ip.New(ip.Config{Log: cfg.Log, Repo: cfg.Repository})
		machineService    = machine.New(machine.Config{Log: cfg.Log, Repo: cfg.Repository})
		methodService     = method.New(cfg.Log, cfg.Repository)
		networkService    = network.New(network.Config{Log: cfg.Log, Repo: cfg.Repository})
		partitionService  = partition.New(partition.Config{Log: cfg.Log, Repo: cfg.Repository})
		projectService    = project.New(project.Config{
			Log:         cfg.Log,
			InviteStore: cfg.ProjectInviteStore,
			Repo:        cfg.Repository,
			TokenStore:  cfg.TokenStore,
		})
		sizeImageConstraintService = sizeimageconstraint.New(sizeimageconstraint.Config{Log: cfg.Log, Repo: cfg.Repository})
		sizeReservationService     = sizereservation.New(sizereservation.Config{Log: cfg.Log, Repo: cfg.Repository})
		sizeService                = size.New(size.Config{Log: cfg.Log, Repo: cfg.Repository})
		tenantService              = tenant.New(tenant.Config{
			Log:         cfg.Log,
			Repo:        cfg.Repository,
			InviteStore: cfg.TenantInviteStore,
			TokenStore:  cfg.TokenStore,
		})
		tokenService = token.New(token.Config{
			Log:  cfg.Log,
			Repo: cfg.Repository,
		})
		userService = user.New(&user.Config{
			Log:  cfg.Log,
			Repo: cfg.Repository,
		})
		versionService = version.New(version.Config{Log: cfg.Log})
	)

	healthService, err := health.New(health.Config{
		Ctx:                 ctx,
		Log:                 cfg.Log,
		HealthcheckInterval: 1 * time.Minute,
		Ipam:                cfg.IpamClient,
		TenantClient:        cfg.TenantClient,
		Datastore:           cfg.Datastore,
		Valkey:              cfg.Valkey,
		Headscale:           cfg.HeadscaleClient,
		AuditBackends:       cfg.AuditBackends,
		TaskClient:          cfg.Repository.Task(),
	})
	if err != nil {
		return fmt.Errorf("unable to initialize health service %w", err)
	}

	// Register the services
	cfg.Mux.Handle(apiv2connect.NewAuditServiceHandler(auditService, cfg.Interceptors))
	cfg.Mux.Handle(apiv2connect.NewFilesystemServiceHandler(filesystemService, cfg.Interceptors))
	cfg.Mux.Handle(apiv2connect.NewHealthServiceHandler(healthService, cfg.Interceptors))
	cfg.Mux.Handle(apiv2connect.NewImageServiceHandler(imageService, cfg.Interceptors))
	cfg.Mux.Handle(apiv2connect.NewIPServiceHandler(ipService, cfg.Interceptors))
	cfg.Mux.Handle(apiv2connect.NewMachineServiceHandler(machineService, cfg.Interceptors))
	cfg.Mux.Handle(apiv2connect.NewMethodServiceHandler(methodService, cfg.Interceptors))
	cfg.Mux.Handle(apiv2connect.NewNetworkServiceHandler(networkService, cfg.Interceptors))
	cfg.Mux.Handle(apiv2connect.NewPartitionServiceHandler(partitionService, cfg.Interceptors))
	cfg.Mux.Handle(apiv2connect.NewProjectServiceHandler(projectService, cfg.Interceptors))
	cfg.Mux.Handle(apiv2connect.NewSizeImageConstraintServiceHandler(sizeImageConstraintService, cfg.Interceptors))
	cfg.Mux.Handle(apiv2connect.NewSizeReservationServiceHandler(sizeReservationService, cfg.Interceptors))
	cfg.Mux.Handle(apiv2connect.NewSizeServiceHandler(sizeService, cfg.Interceptors))
	cfg.Mux.Handle(apiv2connect.NewTenantServiceHandler(tenantService, cfg.Interceptors))
	cfg.Mux.Handle(apiv2connect.NewTokenServiceHandler(tokenService, cfg.Interceptors))
	cfg.Mux.Handle(apiv2connect.NewUserServiceHandler(userService, cfg.Interceptors))
	cfg.Mux.Handle(apiv2connect.NewVersionServiceHandler(versionService, cfg.Interceptors))

	// Register MCP Handlers
	apiv2mcp.RegisterAuditServiceHandler(cfg.MCPServer, auditService)
	apiv2mcp.RegisterFilesystemServiceHandler(cfg.MCPServer, filesystemService)
	apiv2mcp.RegisterHealthServiceHandler(cfg.MCPServer, healthService)
	apiv2mcp.RegisterImageServiceHandler(cfg.MCPServer, imageService)
	apiv2mcp.RegisterIPServiceHandler(cfg.MCPServer, ipService)
	apiv2mcp.RegisterMachineServiceHandler(cfg.MCPServer, machineService)
	apiv2mcp.RegisterMethodServiceHandler(cfg.MCPServer, methodService)
	apiv2mcp.RegisterNetworkServiceHandler(cfg.MCPServer, networkService)
	apiv2mcp.RegisterPartitionServiceHandler(cfg.MCPServer, partitionService)
	apiv2mcp.RegisterProjectServiceHandler(cfg.MCPServer, projectService)
	apiv2mcp.RegisterSizeImageConstraintServiceHandler(cfg.MCPServer, sizeImageConstraintService)
	apiv2mcp.RegisterSizeReservationServiceHandler(cfg.MCPServer, sizeReservationService)
	apiv2mcp.RegisterSizeServiceHandler(cfg.MCPServer, sizeService)
	apiv2mcp.RegisterTenantServiceHandler(cfg.MCPServer, tenantService)
	apiv2mcp.RegisterTokenServiceHandler(cfg.MCPServer, tokenService)
	apiv2mcp.RegisterUserServiceHandler(cfg.MCPServer, userService)
	apiv2mcp.RegisterVersionServiceHandler(cfg.MCPServer, versionService)

	return nil
}
