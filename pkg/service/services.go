package service

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"connectrpc.com/connect"
	"connectrpc.com/grpchealth"
	"connectrpc.com/grpcreflect"
	"connectrpc.com/otelconnect"
	"connectrpc.com/validate"

	headscalev1 "github.com/juanfont/headscale/gen/go/headscale/v1"
	"github.com/metal-stack/api/go/permissions"
	ipamv1connect "github.com/metal-stack/go-ipam/api/v1/apiv1connect"
	"github.com/metal-stack/metal-lib/auditing"
	"github.com/redis/go-redis/v9"
	"github.com/valkey-io/valkey-go"
	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/sdk/metric"

	"github.com/metal-stack/api/go/metalstack/admin/v2/adminv2connect"
	"github.com/metal-stack/api/go/metalstack/api/v2/apiv2connect"
	"github.com/metal-stack/api/go/metalstack/infra/v2/infrav2connect"

	mdm "github.com/metal-stack/masterdata-api/pkg/client"
	authpkg "github.com/metal-stack/metal-apiserver/pkg/auth"
	ratelimiter "github.com/metal-stack/metal-apiserver/pkg/rate-limiter"

	"github.com/metal-stack/metal-apiserver/pkg/certs"
	"github.com/metal-stack/metal-apiserver/pkg/db/generic"
	"github.com/metal-stack/metal-apiserver/pkg/invite"
	"github.com/metal-stack/metal-apiserver/pkg/repository"

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

	auditadmin "github.com/metal-stack/metal-apiserver/pkg/service/admin/audit"
	componentadmin "github.com/metal-stack/metal-apiserver/pkg/service/admin/component"
	filesystemadmin "github.com/metal-stack/metal-apiserver/pkg/service/admin/filesystem"
	imageadmin "github.com/metal-stack/metal-apiserver/pkg/service/admin/image"
	ipadmin "github.com/metal-stack/metal-apiserver/pkg/service/admin/ip"
	machineadmin "github.com/metal-stack/metal-apiserver/pkg/service/admin/machine"
	networkadmin "github.com/metal-stack/metal-apiserver/pkg/service/admin/network"
	partitionadmin "github.com/metal-stack/metal-apiserver/pkg/service/admin/partition"
	projectadmin "github.com/metal-stack/metal-apiserver/pkg/service/admin/project"
	sizeadmin "github.com/metal-stack/metal-apiserver/pkg/service/admin/size"
	sizeimageconstraintadmin "github.com/metal-stack/metal-apiserver/pkg/service/admin/size-image-constraint"
	sizereservationadmin "github.com/metal-stack/metal-apiserver/pkg/service/admin/size-reservation"
	switchadmin "github.com/metal-stack/metal-apiserver/pkg/service/admin/switch"
	taskadmin "github.com/metal-stack/metal-apiserver/pkg/service/admin/task"
	tenantadmin "github.com/metal-stack/metal-apiserver/pkg/service/admin/tenant"
	tokenadmin "github.com/metal-stack/metal-apiserver/pkg/service/admin/token"
	vpnadmin "github.com/metal-stack/metal-apiserver/pkg/service/admin/vpn"

	authservice "github.com/metal-stack/metal-apiserver/pkg/service/auth"

	"github.com/metal-stack/metal-apiserver/pkg/service/infra/bmc"
	"github.com/metal-stack/metal-apiserver/pkg/service/infra/boot"
	componentinfra "github.com/metal-stack/metal-apiserver/pkg/service/infra/component"
	eventinfra "github.com/metal-stack/metal-apiserver/pkg/service/infra/event"
	switchinfra "github.com/metal-stack/metal-apiserver/pkg/service/infra/switch"

	tokencommon "github.com/metal-stack/metal-apiserver/pkg/token"
)

type Config struct {
	Log                                 *slog.Logger
	HttpServerEndpoint                  string
	MetricsServerEndpoint               string
	FrontEndUrl                         string
	ServerHttpURL                       string
	OIDCClientID                        string
	OIDCClientSecret                    string
	OIDCDiscoveryURL                    string
	OIDCEndSessionURL                   string
	OIDCUniqueUserKey                   string
	OIDCTLSSkipVerify                   bool
	Datastore                           generic.Datastore
	Repository                          *repository.Store
	MasterClient                        mdm.Client
	IpamClient                          ipamv1connect.IpamServiceClient
	AuditSearchBackend                  auditing.Auditing
	AuditBackends                       []auditing.Auditing
	Stage                               string
	RedisConfig                         *RedisConfig
	Admins                              []string
	MaxRequestsPerMinuteToken           int
	MaxRequestsPerMinuteUnauthenticated int
	IsStageDev                          bool
	BMCSuperuserPassword                string
	HeadscaleControlplaneAddress        string
	HeadscaleClient                     headscalev1.HeadscaleServiceClient
	ComponentExpiration                 time.Duration
}

type RedisConfig struct {
	TokenClient     *redis.Client
	RateLimitClient *redis.Client
	InviteClient    *redis.Client
	AsyncClient     *redis.Client
	QueueClient     valkey.Client
	ComponentClient valkey.Client
}

func New(log *slog.Logger, c Config) (*http.ServeMux, error) {
	var (
		tokenStore = tokencommon.NewRedisStore(c.RedisConfig.TokenClient)
		certStore  = certs.NewRedisStore(&certs.Config{
			RedisClient: c.RedisConfig.TokenClient,
		})
		projectInviteStore = invite.NewProjectRedisStore(c.RedisConfig.InviteClient)
		tenantInviteStore  = invite.NewTenantRedisStore(c.RedisConfig.InviteClient)
	)

	authz, err := authpkg.NewAuthenticatorInterceptor(authpkg.Config{
		Log:            log,
		CertStore:      certStore,
		AllowedIssuers: []string{c.ServerHttpURL},
		TokenStore:     tokenStore,
		Repo:           c.Repository,
	})
	if err != nil {
		return nil, fmt.Errorf("unable to initialize authz interceptor: %w", err)
	}

	var (
		authorizeInterceptor  = authpkg.NewAuthorizeInterceptor(log, c.Repository)
		validationInterceptor = validate.NewInterceptor()
	)

	// metrics interceptor
	exporter, err := prometheus.New()
	if err != nil {
		return nil, err
	}

	provider := metric.NewMeterProvider(metric.WithReader(exporter))
	metricsInterceptor, err := otelconnect.NewInterceptor(otelconnect.WithMeterProvider(provider))
	if err != nil {
		return nil, err
	}

	var (
		logInterceptor       = newLogRequestInterceptor(log)
		tenantInterceptor    = tenant.NewInterceptor(log, c.MasterClient)
		ratelimitInterceptor = ratelimiter.NewInterceptor(&ratelimiter.Config{
			Log:                                 log,
			RedisClient:                         c.RedisConfig.RateLimitClient,
			MaxRequestsPerMinuteToken:           c.MaxRequestsPerMinuteToken,
			MaxRequestsPerMinuteUnauthenticated: c.MaxRequestsPerMinuteUnauthenticated,
		})

		allInterceptors      = []connect.Interceptor{metricsInterceptor, logInterceptor, authz, authorizeInterceptor, ratelimitInterceptor, validationInterceptor, tenantInterceptor}
		allAdminInterceptors = []connect.Interceptor{metricsInterceptor, logInterceptor, authz, authorizeInterceptor, validationInterceptor, tenantInterceptor}
		allInfraInterceptors = []connect.Interceptor{metricsInterceptor, logInterceptor, authz, authorizeInterceptor, validationInterceptor}
	)

	if len(c.AuditBackends) > 0 {
		servicePermissions := permissions.GetServicePermissions()
		shouldAudit := func(fullMethod string) bool {
			shouldAudit, ok := servicePermissions.Auditable[fullMethod]
			if !ok {
				log.Warn("method not found in permissions, audit implicitly", "method", fullMethod)
				return true
			}
			return shouldAudit
		}

		for _, backend := range c.AuditBackends {
			auditInterceptor, err := auditing.NewConnectInterceptor(backend, log, shouldAudit)
			if err != nil {
				return nil, fmt.Errorf("unable to create auditing interceptor: %w", err)
			}

			allInterceptors = append(allInterceptors, auditInterceptor)
			allAdminInterceptors = append(allAdminInterceptors, auditInterceptor)
		}
	}

	var (
		interceptors      = connect.WithInterceptors(allInterceptors...)
		adminInterceptors = connect.WithInterceptors(allAdminInterceptors...)
		infraInterceptors = connect.WithInterceptors(allInfraInterceptors...)
	)

	var (
		auditService      = audit.New(audit.Config{Log: log, Repo: c.Repository, AuditClient: c.AuditSearchBackend})
		filesystemService = filesystem.New(filesystem.Config{Log: log, Repo: c.Repository})
		imageService      = image.New(image.Config{Log: log, Repo: c.Repository})
		ipService         = ip.New(ip.Config{Log: log, Repo: c.Repository})
		machineService    = machine.New(machine.Config{Log: log, Repo: c.Repository})
		methodService     = method.New(log, c.Repository)
		networkService    = network.New(network.Config{Log: log, Repo: c.Repository})
		partitionService  = partition.New(partition.Config{Log: log, Repo: c.Repository})
		projectService    = project.New(project.Config{
			Log:         log,
			InviteStore: projectInviteStore,
			Repo:        c.Repository,
			TokenStore:  tokenStore,
		})
		sizeImageConstraintService = sizeimageconstraint.New(sizeimageconstraint.Config{Log: log, Repo: c.Repository})
		sizeReservationService     = sizereservation.New(sizereservation.Config{Log: log, Repo: c.Repository})
		sizeService                = size.New(size.Config{Log: log, Repo: c.Repository})
		tenantService              = tenant.New(tenant.Config{
			Log:         log,
			Repo:        c.Repository,
			InviteStore: tenantInviteStore,
			TokenStore:  tokenStore,
		})
		tokenService = token.New(token.Config{
			Log:           log,
			CertStore:     certStore,
			TokenStore:    tokenStore,
			Repo:          c.Repository,
			Issuer:        c.ServerHttpURL,
			AdminSubjects: c.Admins,
		})
		userService = user.New(&user.Config{
			Log:  log,
			Repo: c.Repository,
		})
		versionService = version.New(version.Config{Log: log})
	)

	healthService, err := health.New(health.Config{
		Ctx:                 context.Background(),
		Log:                 log,
		HealthcheckInterval: 1 * time.Minute,
		Ipam:                c.IpamClient,
		Masterdata:          c.MasterClient,
		Datastore:           c.Datastore,
	})
	if err != nil {
		return nil, fmt.Errorf("unable to initialize health service %w", err)
	}

	mux := http.NewServeMux()

	// Register the services
	mux.Handle(apiv2connect.NewAuditServiceHandler(auditService, interceptors))
	mux.Handle(apiv2connect.NewFilesystemServiceHandler(filesystemService, interceptors))
	mux.Handle(apiv2connect.NewHealthServiceHandler(healthService, interceptors))
	mux.Handle(apiv2connect.NewImageServiceHandler(imageService, interceptors))
	mux.Handle(apiv2connect.NewIPServiceHandler(ipService, interceptors))
	mux.Handle(apiv2connect.NewMachineServiceHandler(machineService, interceptors))
	mux.Handle(apiv2connect.NewMethodServiceHandler(methodService, interceptors))
	mux.Handle(apiv2connect.NewNetworkServiceHandler(networkService, interceptors))
	mux.Handle(apiv2connect.NewPartitionServiceHandler(partitionService, interceptors))
	mux.Handle(apiv2connect.NewProjectServiceHandler(projectService, interceptors))
	mux.Handle(apiv2connect.NewSizeImageConstraintServiceHandler(sizeImageConstraintService, interceptors))
	mux.Handle(apiv2connect.NewSizeReservationServiceHandler(sizeReservationService, interceptors))
	mux.Handle(apiv2connect.NewSizeServiceHandler(sizeService, interceptors))
	mux.Handle(apiv2connect.NewTenantServiceHandler(tenantService, interceptors))
	mux.Handle(apiv2connect.NewTokenServiceHandler(tokenService, interceptors))
	mux.Handle(apiv2connect.NewUserServiceHandler(userService, interceptors))
	mux.Handle(apiv2connect.NewVersionServiceHandler(versionService, interceptors))

	// Admin services
	var (
		adminAuditService               = auditadmin.New(auditadmin.Config{Log: log, Repo: c.Repository, AuditClient: c.AuditSearchBackend})
		adminComponentService           = componentadmin.New(componentadmin.Config{Log: log, Repo: c.Repository})
		adminFilesystemService          = filesystemadmin.New(filesystemadmin.Config{Log: log, Repo: c.Repository})
		adminImageService               = imageadmin.New(imageadmin.Config{Log: log, Repo: c.Repository})
		adminIpService                  = ipadmin.New(ipadmin.Config{Log: log, Repo: c.Repository})
		adminMachineService             = machineadmin.New(machineadmin.Config{Log: log, Repo: c.Repository})
		adminNetworkService             = networkadmin.New(networkadmin.Config{Log: log, Repo: c.Repository})
		adminPartitionService           = partitionadmin.New(partitionadmin.Config{Log: log, Repo: c.Repository})
		adminProjectService             = projectadmin.New(projectadmin.Config{Log: log, Repo: c.Repository})
		adminSizeImageConstraintService = sizeimageconstraintadmin.New(sizeimageconstraintadmin.Config{Log: log, Repo: c.Repository})
		adminSizeReservationService     = sizereservationadmin.New(sizereservationadmin.Config{Log: log, Repo: c.Repository})
		adminSizeService                = sizeadmin.New(sizeadmin.Config{Log: log, Repo: c.Repository})
		adminSwitchService              = switchadmin.New(switchadmin.Config{Log: log, Repo: c.Repository})
		adminTaskService                = taskadmin.New(taskadmin.Config{Log: log, Repo: c.Repository})
		adminTenantService              = tenantadmin.New(tenantadmin.Config{
			Log:         log,
			Repo:        c.Repository,
			InviteStore: tenantInviteStore,
			TokenStore:  tokenStore,
		})
		adminTokenService = tokenadmin.New(tokenadmin.Config{Log: log, CertStore: certStore, TokenStore: tokenStore, TokenService: tokenService})
	)

	mux.Handle(adminv2connect.NewAuditServiceHandler(adminAuditService, adminInterceptors))
	mux.Handle(adminv2connect.NewComponentServiceHandler(adminComponentService, adminInterceptors))
	mux.Handle(adminv2connect.NewFilesystemServiceHandler(adminFilesystemService, adminInterceptors))
	mux.Handle(adminv2connect.NewImageServiceHandler(adminImageService, adminInterceptors))
	mux.Handle(adminv2connect.NewIPServiceHandler(adminIpService, adminInterceptors))
	mux.Handle(adminv2connect.NewMachineServiceHandler(adminMachineService, adminInterceptors))
	mux.Handle(adminv2connect.NewNetworkServiceHandler(adminNetworkService, adminInterceptors))
	mux.Handle(adminv2connect.NewPartitionServiceHandler(adminPartitionService, adminInterceptors))
	mux.Handle(adminv2connect.NewProjectServiceHandler(adminProjectService, adminInterceptors))
	mux.Handle(adminv2connect.NewSizeImageConstraintServiceHandler(adminSizeImageConstraintService, adminInterceptors))
	mux.Handle(adminv2connect.NewSizeReservationServiceHandler(adminSizeReservationService, adminInterceptors))
	mux.Handle(adminv2connect.NewSizeServiceHandler(adminSizeService, adminInterceptors))
	mux.Handle(adminv2connect.NewSwitchServiceHandler(adminSwitchService, adminInterceptors))
	mux.Handle(adminv2connect.NewTaskServiceHandler(adminTaskService, adminInterceptors))
	mux.Handle(adminv2connect.NewTenantServiceHandler(adminTenantService, adminInterceptors))
	mux.Handle(adminv2connect.NewTokenServiceHandler(adminTokenService, adminInterceptors))

	if c.HeadscaleClient != nil {
		adminVPNService := vpnadmin.New(vpnadmin.Config{
			Log:                          log,
			Repo:                         c.Repository,
			HeadscaleClient:              c.HeadscaleClient,
			HeadscaleControlplaneAddress: c.HeadscaleControlplaneAddress,
		})
		mux.Handle(adminv2connect.NewVPNServiceHandler(adminVPNService))
	}

	// Infra services, we use adminInterceptors to prevent rate limiting
	var (
		bmcService            = bmc.New(bmc.Config{Log: log, Repo: c.Repository})
		bootService           = boot.New(boot.Config{Log: log, Repo: c.Repository, BMCSuperuserPassword: c.BMCSuperuserPassword})
		infraComponentService = componentinfra.New(componentinfra.Config{Log: log, Repo: c.Repository, Expiration: c.ComponentExpiration})
		infraEventService     = eventinfra.New(eventinfra.Config{Log: log, Repo: c.Repository})
		infraSwitchService    = switchinfra.New(switchinfra.Config{Log: log, Repo: c.Repository})
	)

	mux.Handle(infrav2connect.NewBMCServiceHandler(bmcService, infraInterceptors))
	mux.Handle(infrav2connect.NewBootServiceHandler(bootService, infraInterceptors))
	mux.Handle(infrav2connect.NewComponentServiceHandler(infraComponentService, infraInterceptors))
	mux.Handle(infrav2connect.NewEventServiceHandler(infraEventService, infraInterceptors))
	mux.Handle(infrav2connect.NewSwitchServiceHandler(infraSwitchService, infraInterceptors))

	var (
		allServiceNames = permissions.GetServices()
		checker         = grpchealth.NewStaticChecker(allServiceNames...)
		reflector       = grpcreflect.NewStaticReflector(allServiceNames...)
	)

	// Static HealthCheckers
	mux.Handle(grpchealth.NewHandler(checker))
	// enable remote service listing by enabling reflection
	mux.Handle(grpcreflect.NewHandlerV1(reflector))
	mux.Handle(grpcreflect.NewHandlerV1Alpha(reflector))

	// Add OIDC Handlers
	authHandlerPath, authHandler, err := oidcAuthHandler(log, tokenService, c)
	if err != nil {
		return nil, err
	}

	// Add all authentication handlers in one go
	mux.Handle(authHandlerPath, authHandler)
	// END OIDC Login Authentication

	return mux, nil
}

func oidcAuthHandler(log *slog.Logger, tokenService token.TokenService, c Config) (string, http.Handler, error) {
	frontendURL, err := url.Parse(c.FrontEndUrl)
	if err != nil {
		return "", nil, fmt.Errorf("failed to parse frontend url %w", err)
	}

	auth, err := authservice.New(authservice.Config{
		Log:           log,
		TokenService:  tokenService,
		Repo:          c.Repository,
		AuditBackends: c.AuditBackends,
		FrontEndUrl:   frontendURL,
		CallbackUrl:   c.ServerHttpURL + "/auth/{provider}/callback",
	})
	if err != nil {
		return "", nil, err
	}

	_, err = auth.With(
		authservice.OIDCHubProvider(authservice.ProviderConfig{
			ClientID:      c.OIDCClientID,
			ClientSecret:  c.OIDCClientSecret,
			DiscoveryURL:  c.OIDCDiscoveryURL,
			EndsessionURL: c.OIDCEndSessionURL,
			UniqueUserKey: &c.OIDCUniqueUserKey,
			TLSSkipVerify: c.OIDCTLSSkipVerify,
		}),
	)
	if err != nil {
		return "", nil, err
	}

	return auth.NewHandler(c.IsStageDev)

}
