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
	"github.com/metal-stack/api/go/metalstack/admin/v2/adminv2connect"
	"github.com/metal-stack/api/go/metalstack/api/v2/apiv2connect"
	"github.com/metal-stack/api/go/permissions"
	ipamv1connect "github.com/metal-stack/go-ipam/api/v1/apiv1connect"
	mdm "github.com/metal-stack/masterdata-api/pkg/client"
	authpkg "github.com/metal-stack/metal-apiserver/pkg/auth"
	"github.com/metal-stack/metal-apiserver/pkg/certs"
	"github.com/metal-stack/metal-apiserver/pkg/db/generic"
	"github.com/metal-stack/metal-apiserver/pkg/invite"
	ratelimiter "github.com/metal-stack/metal-apiserver/pkg/rate-limiter"
	"github.com/metal-stack/metal-apiserver/pkg/repository"
	authservice "github.com/metal-stack/metal-apiserver/pkg/service/auth"
	"github.com/metal-stack/metal-apiserver/pkg/service/filesystem"
	filesystemadmin "github.com/metal-stack/metal-apiserver/pkg/service/filesystem/admin"
	"github.com/metal-stack/metal-apiserver/pkg/service/health"
	"github.com/metal-stack/metal-apiserver/pkg/service/image"
	imageadmin "github.com/metal-stack/metal-apiserver/pkg/service/image/admin"
	"github.com/metal-stack/metal-apiserver/pkg/service/ip"
	ipadmin "github.com/metal-stack/metal-apiserver/pkg/service/ip/admin"
	"github.com/metal-stack/metal-apiserver/pkg/service/method"
	"github.com/metal-stack/metal-apiserver/pkg/service/partition"
	partitionadmin "github.com/metal-stack/metal-apiserver/pkg/service/partition/admin"
	"github.com/metal-stack/metal-apiserver/pkg/service/project"
	"github.com/metal-stack/metal-apiserver/pkg/service/tenant"
	tenantadmin "github.com/metal-stack/metal-apiserver/pkg/service/tenant/admin"
	"github.com/metal-stack/metal-apiserver/pkg/service/token"
	"github.com/metal-stack/metal-apiserver/pkg/service/version"
	tokencommon "github.com/metal-stack/metal-apiserver/pkg/token"
	"github.com/metal-stack/metal-lib/auditing"
	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/sdk/metric"
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
	Datastore                           generic.Datastore
	Repository                          *repository.Store
	MasterClient                        mdm.Client
	IpamClient                          ipamv1connect.IpamServiceClient
	Auditing                            auditing.Auditing
	Stage                               string
	RedisConfig                         *RedisConfig
	Admins                              []string
	MaxRequestsPerMinuteToken           int
	MaxRequestsPerMinuteUnauthenticated int
	IsStageDev                          bool
}

type RedisConfig struct {
	TokenClient     *redis.Client
	RateLimitClient *redis.Client
	InviteClient    *redis.Client
	AsyncClient     *redis.Client
}

func New(log *slog.Logger, c Config) (*http.ServeMux, error) {

	tokenStore := tokencommon.NewRedisStore(c.RedisConfig.TokenClient)
	certStore := certs.NewRedisStore(&certs.Config{
		RedisClient: c.RedisConfig.TokenClient,
	})
	projectInviteStore := invite.NewProjectRedisStore(c.RedisConfig.InviteClient)
	tenantInviteStore := invite.NewTenantRedisStore(c.RedisConfig.InviteClient)

	authcfg := authpkg.Config{
		Log:            log,
		CertStore:      certStore,
		AllowedIssuers: []string{c.ServerHttpURL},
		AdminSubjects:  c.Admins,
		TokenStore:     tokenStore,
		MasterClient:   c.MasterClient,
	}
	authz, err := authpkg.New(authcfg)
	if err != nil {
		return nil, fmt.Errorf("unable to initialize authz interceptor: %w", err)
	}

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
	validationInterceptor, err := validate.NewInterceptor()
	if err != nil {
		return nil, err
	}

	var (
		logInteceptor        = newLogRequestInterceptor(log)
		tenantInterceptor    = tenant.NewInterceptor(log, c.MasterClient)
		ratelimitInterceptor = ratelimiter.NewInterceptor(&ratelimiter.Config{
			Log:                                 log,
			RedisClient:                         c.RedisConfig.RateLimitClient,
			MaxRequestsPerMinuteToken:           c.MaxRequestsPerMinuteToken,
			MaxRequestsPerMinuteUnauthenticated: c.MaxRequestsPerMinuteUnauthenticated,
		})
	)

	allInterceptors := []connect.Interceptor{metricsInterceptor, logInteceptor, authz, ratelimitInterceptor, validationInterceptor, tenantInterceptor}
	allAdminInterceptors := []connect.Interceptor{metricsInterceptor, logInteceptor, authz, validationInterceptor, tenantInterceptor}
	if c.Auditing != nil {
		servicePermissions := permissions.GetServicePermissions()
		shouldAudit := func(fullMethod string) bool {
			shouldAudit, ok := servicePermissions.Auditable[fullMethod]
			if !ok {
				log.Warn("method not found in permissions, audit implicitly", "method", fullMethod)
				return true
			}
			return shouldAudit
		}
		auditInterceptor, err := auditing.NewConnectInterceptor(c.Auditing, log, shouldAudit)
		if err != nil {
			return nil, fmt.Errorf("unable to create auditing interceptor: %w", err)
		}
		allInterceptors = append(allInterceptors, auditInterceptor)
		allAdminInterceptors = append(allAdminInterceptors, auditInterceptor)
	}
	interceptors := connect.WithInterceptors(allInterceptors...)
	adminInterceptors := connect.WithInterceptors(allAdminInterceptors...)

	methodService := method.New()
	tenantService := tenant.New(tenant.Config{
		Log:          log,
		MasterClient: c.MasterClient,
		InviteStore:  tenantInviteStore,
		TokenStore:   tokenStore,
	})

	adminTenantService := tenantadmin.New(tenantadmin.Config{
		Log:          log,
		MasterClient: c.MasterClient,
		InviteStore:  tenantInviteStore,
		TokenStore:   tokenStore,
	})
	projectService := project.New(project.Config{
		Log:          log,
		MasterClient: c.MasterClient,
		InviteStore:  projectInviteStore,
		Repo:         c.Repository,
		TokenStore:   tokenStore,
	})

	ipService := ip.New(ip.Config{Log: log, Repo: c.Repository})
	filesystemService := filesystem.New(filesystem.Config{Log: log, Repo: c.Repository})
	partitionService := partition.New(partition.Config{Log: log, Repo: c.Repository})
	imageService := image.New(image.Config{Log: log, Repo: c.Repository})
	tokenService := token.New(token.Config{
		Log:           log,
		CertStore:     certStore,
		TokenStore:    tokenStore,
		MasterClient:  c.MasterClient,
		Issuer:        c.ServerHttpURL,
		AdminSubjects: c.Admins,
	})
	versionService := version.New(version.Config{Log: log})
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
	mux.Handle(apiv2connect.NewTokenServiceHandler(tokenService, interceptors))
	mux.Handle(apiv2connect.NewTenantServiceHandler(tenantService, interceptors))
	mux.Handle(apiv2connect.NewProjectServiceHandler(projectService, interceptors))
	mux.Handle(apiv2connect.NewFilesystemServiceHandler(filesystemService, interceptors))
	mux.Handle(apiv2connect.NewPartitionServiceHandler(partitionService, interceptors))
	mux.Handle(apiv2connect.NewImageServiceHandler(imageService, interceptors))
	mux.Handle(apiv2connect.NewIPServiceHandler(ipService, interceptors))
	mux.Handle(apiv2connect.NewMethodServiceHandler(methodService, interceptors))
	mux.Handle(apiv2connect.NewVersionServiceHandler(versionService, interceptors))
	mux.Handle(apiv2connect.NewHealthServiceHandler(healthService, interceptors))

	// Admin services
	adminIpService := ipadmin.New(ipadmin.Config{Log: log, Repo: c.Repository})
	adminImageService := imageadmin.New(imageadmin.Config{Log: log, Repo: c.Repository})
	adminFilesystemService := filesystemadmin.New(filesystemadmin.Config{Log: log, Repo: c.Repository})
	adminPartitionService := partitionadmin.New(partitionadmin.Config{Log: log, Repo: c.Repository})
	mux.Handle(adminv2connect.NewIPServiceHandler(adminIpService, adminInterceptors))
	mux.Handle(adminv2connect.NewImageServiceHandler(adminImageService, adminInterceptors))
	mux.Handle(adminv2connect.NewFilesystemServiceHandler(adminFilesystemService, adminInterceptors))
	mux.Handle(adminv2connect.NewPartitionServiceHandler(adminPartitionService, adminInterceptors))
	mux.Handle(adminv2connect.NewTenantServiceHandler(adminTenantService, adminInterceptors))

	allServiceNames := permissions.GetServices()
	// Static HealthCheckers
	checker := grpchealth.NewStaticChecker(allServiceNames...)
	mux.Handle(grpchealth.NewHandler(checker))

	// enable remote service listing by enabling reflection
	reflector := grpcreflect.NewStaticReflector(allServiceNames...)
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
		Log:          log,
		TokenService: tokenService,
		MasterClient: c.MasterClient,
		Auditing:     c.Auditing,
		FrontEndUrl:  frontendURL,
		CallbackUrl:  c.ServerHttpURL + "/auth/{provider}/callback",
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
		}),
	)
	if err != nil {
		return "", nil, err
	}

	return auth.NewHandler(c.IsStageDev)

}
