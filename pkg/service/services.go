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
	auditinggrpc "github.com/metal-stack/metal-lib/auditing/grpc"
	"github.com/redis/go-redis/v9"
	"github.com/valkey-io/valkey-go"
	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/sdk/metric"

	mdm "github.com/metal-stack/masterdata-api/pkg/client"
	authpkg "github.com/metal-stack/metal-apiserver/pkg/auth"
	ratelimiter "github.com/metal-stack/metal-apiserver/pkg/rate-limiter"

	"github.com/metal-stack/metal-apiserver/pkg/certs"
	"github.com/metal-stack/metal-apiserver/pkg/db/generic"
	"github.com/metal-stack/metal-apiserver/pkg/invite"
	"github.com/metal-stack/metal-apiserver/pkg/repository"
	repoapi "github.com/metal-stack/metal-apiserver/pkg/repository/api"
	"github.com/metal-stack/metal-apiserver/pkg/service/admin"
	"github.com/metal-stack/metal-apiserver/pkg/service/api"
	"github.com/metal-stack/metal-apiserver/pkg/service/api/tenant"
	"github.com/metal-stack/metal-apiserver/pkg/service/api/token"
	"github.com/metal-stack/metal-apiserver/pkg/service/infra"

	authservice "github.com/metal-stack/metal-apiserver/pkg/service/auth"

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
	})
	if err != nil {
		return nil, fmt.Errorf("unable to initialize authz interceptor: %w", err)
	}

	var (
		// We fetch projects and tenants on every request, if this hurts performance we can
		// put the result into the context, and reuse the result in subsequent queries
		// or we introduce a cache with a short timeout.
		authorizeInterceptor = authpkg.NewAuthorizeInterceptor(log, func(ctx context.Context, userId string) (*repoapi.ProjectsAndTenants, error) {
			return c.Repository.UnscopedProject().AdditionalMethods().GetProjectsAndTenants(ctx, userId)
		})
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
			auditInterceptor, err := auditinggrpc.NewConnectInterceptor(backend, log, shouldAudit)
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

	mux := http.NewServeMux()

	tokenService, err := api.ApiServices(api.Config{
		Log:                log,
		Repository:         c.Repository,
		Datastore:          c.Datastore,
		IpamClient:         c.IpamClient,
		MasterClient:       c.MasterClient,
		Mux:                mux,
		Interceptors:       interceptors,
		ProjectInviteStore: projectInviteStore,
		TenantInviteStore:  tenantInviteStore,
		TokenStore:         tokenStore,
		CertStore:          certStore,
		AuditSearchBackend: c.AuditSearchBackend,
		Redis:              c.RedisConfig.ComponentClient,
		ServerHttpURL:      c.ServerHttpURL,
		Admins:             c.Admins,
		AuditBackends:      c.AuditBackends,
		HeadscaleClient:    c.HeadscaleClient,
	})
	if err != nil {
		return nil, err
	}

	admin.AdminServices(admin.Config{
		Log:                log,
		Repository:         c.Repository,
		Mux:                mux,
		Interceptors:       adminInterceptors,
		InviteStore:        tenantInviteStore,
		TokenStore:         tokenStore,
		TokenService:       tokenService,
		CertStore:          certStore,
		AuditSearchBackend: c.AuditSearchBackend,
	})

	infra.InfraServices(infra.Config{
		Log:                  log,
		Repository:           c.Repository,
		Mux:                  mux,
		Interceptors:         infraInterceptors,
		ComponentExpiration:  c.ComponentExpiration,
		BMCSuperuserPassword: c.BMCSuperuserPassword,
	})

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
