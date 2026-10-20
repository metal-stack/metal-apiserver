package api

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"connectrpc.com/connect/v2"
	"github.com/metal-stack/api/go/metalstack/api/v2/apiv2connect"
	"github.com/metal-stack/metal-apiserver/pkg/certs"
	"github.com/metal-stack/metal-apiserver/pkg/db/generic"
	"github.com/metal-stack/metal-apiserver/pkg/headscale"
	"github.com/metal-stack/metal-apiserver/pkg/invite"
	"github.com/metal-stack/metal-apiserver/pkg/repository"
	"github.com/metal-stack/metal-lib/auditing"
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
	Server             *connect.Server
	ProjectInviteStore invite.ProjectInviteStore
	TenantInviteStore  invite.TenantInviteStore
	TokenStore         tokencommon.TokenStore
	CertStore          certs.CertStore
	AuditSearchBackend auditing.Auditing
	Valkey             valkey.Client
	AuditBackends      []auditing.Auditing
	HeadscaleClient    *headscale.Client

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
	apiv2connect.RegisterAuditServiceHandler(cfg.Server, auditService)
	apiv2connect.RegisterFilesystemServiceHandler(cfg.Server, filesystemService)
	apiv2connect.RegisterHealthServiceHandler(cfg.Server, healthService)
	apiv2connect.RegisterImageServiceHandler(cfg.Server, imageService)
	apiv2connect.RegisterIPServiceHandler(cfg.Server, ipService)
	apiv2connect.RegisterMachineServiceHandler(cfg.Server, machineService)
	apiv2connect.RegisterMethodServiceHandler(cfg.Server, methodService)
	apiv2connect.RegisterNetworkServiceHandler(cfg.Server, networkService)
	apiv2connect.RegisterPartitionServiceHandler(cfg.Server, partitionService)
	apiv2connect.RegisterProjectServiceHandler(cfg.Server, projectService)
	apiv2connect.RegisterSizeImageConstraintServiceHandler(cfg.Server, sizeImageConstraintService)
	apiv2connect.RegisterSizeReservationServiceHandler(cfg.Server, sizeReservationService)
	apiv2connect.RegisterSizeServiceHandler(cfg.Server, sizeService)
	apiv2connect.RegisterTenantServiceHandler(cfg.Server, tenantService)
	apiv2connect.RegisterTokenServiceHandler(cfg.Server, tokenService)
	apiv2connect.RegisterUserServiceHandler(cfg.Server, userService)
	apiv2connect.RegisterVersionServiceHandler(cfg.Server, versionService)

	return nil
}
