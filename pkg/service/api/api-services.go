package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"connectrpc.com/connect"
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

	mdm "github.com/metal-stack/masterdata-api/pkg/client"
	tokencommon "github.com/metal-stack/metal-apiserver/pkg/token"
)

type Config struct {
	Log                *slog.Logger
	Datastore          generic.Datastore
	Repository         *repository.Store
	MasterClient       mdm.Client
	IpamClient         ipamv1connect.IpamServiceClient
	Mux                *http.ServeMux
	Interceptors       connect.Option
	ProjectInviteStore invite.ProjectInviteStore
	TenantInviteStore  invite.TenantInviteStore
	TokenStore         tokencommon.TokenStore
	CertStore          certs.CertStore
	AuditSearchBackend auditing.Auditing
	Redis              valkey.Client
	AuditBackends      []auditing.Auditing
	HeadscaleClient    *headscale.Client

	ServerHttpURL string
	Admins        []string
}

func ApiServices(cfg Config) (token.TokenService, error) {
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
			Log:           cfg.Log,
			CertStore:     cfg.CertStore,
			TokenStore:    cfg.TokenStore,
			Repo:          cfg.Repository,
			Issuer:        cfg.ServerHttpURL,
			AdminSubjects: cfg.Admins,
		})
		userService = user.New(&user.Config{
			Log:  cfg.Log,
			Repo: cfg.Repository,
		})
		versionService = version.New(version.Config{Log: cfg.Log})
	)

	healthService, err := health.New(health.Config{
		Ctx:                 context.Background(),
		Log:                 cfg.Log,
		HealthcheckInterval: 1 * time.Minute,
		Ipam:                cfg.IpamClient,
		Masterdata:          cfg.MasterClient,
		Datastore:           cfg.Datastore,
		Redis:               cfg.Redis,
		Headscale:           cfg.HeadscaleClient,
		AuditBackends:       cfg.AuditBackends,
		TaskClient:          cfg.Repository.Task(),
	})
	if err != nil {
		return nil, fmt.Errorf("unable to initialize health service %w", err)
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

	return tokenService, nil
}
