package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"runtime"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"github.com/alicebob/miniredis/v2"
	"github.com/avast/retry-go/v4"
	compress "github.com/klauspost/connect-compress/v2"
	"github.com/valkey-io/valkey-go"
	"gopkg.in/rethinkdb/rethinkdb-go.v6"

	ipamv1 "github.com/metal-stack/go-ipam/api/v1"
	ipamv1connect "github.com/metal-stack/go-ipam/api/v1/apiv1connect"
	"github.com/metal-stack/metal-apiserver/pkg/async/queue"
	"github.com/metal-stack/metal-apiserver/pkg/async/task"
	"github.com/metal-stack/metal-apiserver/pkg/certs"
	"github.com/metal-stack/metal-apiserver/pkg/db/generic"
	"github.com/metal-stack/metal-apiserver/pkg/headscale"
	"github.com/metal-stack/metal-apiserver/pkg/repository"
	"github.com/metal-stack/metal-apiserver/pkg/service"
	"github.com/metal-stack/metal-apiserver/pkg/test"
	"github.com/metal-stack/metal-apiserver/pkg/token"
	tenant "github.com/metal-stack/tenant-api/go/client"

	"github.com/metal-stack/v"
	"github.com/redis/go-redis/v9"
	"github.com/urfave/cli/v3"
)

func newServeCmd() *cli.Command {
	return &cli.Command{
		Name:  "serve",
		Usage: "start the api server",
		Flags: []cli.Flag{
			logLevelFlag,
			httpServerEndpointFlag,
			metricServerEndpointFlag,
			sessionSecretFlag,
			serverHttpUrlFlag,
			tenantApiserverBaseURLFlag,
			rethinkdbAddressesFlag,
			rethinkdbDBNameFlag,
			rethinkdbPasswordFlag,
			rethinkdbUserFlag,
			auditingSearchBackendFlag,
			auditingTimescaleEnabledFlag,
			auditingTimescaleHostFlag,
			auditingTimescalePortFlag,
			auditingTimescaleDbFlag,
			auditingTimescaleUserFlag,
			auditingTimescalePasswordFlag,
			auditingTimescaleRetentionFlag,
			auditingSplunkEnabledFlag,
			auditingSplunkEndpointFlag,
			auditingSplunkHecTokenFlag,
			auditingSplunkHostFlag,
			auditingSplunkIndexFlag,
			auditingSplunkSourceFlag,
			auditingSplunkSourceTypeFlag,
			auditingSplunkCaFlag,
			stageFlag,
			redisAddrFlag,
			redisPasswordFlag,
			providerTenantFlag,
			maxRequestsPerMinuteFlag,
			maxRequestsPerMinuteUnauthenticatedFlag,
			ipamGrpcEndpointFlag,
			frontEndUrlFlag,
			oidcClientIdFlag,
			oidcClientSecretFlag,
			oidcDiscoveryUrlFlag,
			oidcEndSessionUrlFlag,
			oidcUniqueUserKeyFlag,
			oidcTLSSkipVerifyFlag,
			bmcSuperuserPasswordFlag,
			headscaleAddressFlag,
			headscaleControlplaneAddressFlag,
			headscaleApikeyFlag,
			headscaleEnabledFlag,
			componentExpirationFlag,
			secureCookieFlag,
			redirectUrlsFlag,
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			log, err := createLogger(cmd)
			if err != nil {
				return fmt.Errorf("unable to create logger %w", err)
			}

			auditSearchBackend, auditBackends, err := createAuditingClient(cmd, log)
			if err != nil {
				return fmt.Errorf("unable to create auditing client: %w", err)
			}

			ipam, err := createIpamClient(ctx, cmd, log)
			if err != nil {
				return fmt.Errorf("unable to create ipam client: %w", err)
			}

			redisConfig, err := createRedisClients(ctx, cmd, log)
			if err != nil {
				return fmt.Errorf("unable to create redis clients: %w", err)
			}

			tc, err := createTenantApiserverClient(cmd, log)
			if err != nil {
				return fmt.Errorf("unable to create tenant-apiserver client: %w", err)
			}

			var (
				hc *headscale.Client
			)

			if cmd.Bool(headscaleEnabledFlag.Name) {
				hc, err = headscale.NewClient(headscale.Config{
					Log:           log,
					Apikey:        cmd.String(headscaleApikeyFlag.Name),
					Endpoint:      cmd.String(headscaleAddressFlag.Name),
					ControllerURL: cmd.String(headscaleControlplaneAddressFlag.Name),
				})
				if err != nil {
					return err
				}

				log.Info("headscale enabled")
			} else {
				log.Info("headscale is not enabled, not configuring vpn services")
			}

			connectOpts := rethinkdb.ConnectOpts{
				Addresses:  cmd.StringSlice(rethinkdbAddressesFlag.Name),
				Database:   cmd.String(rethinkdbDBNameFlag.Name),
				Username:   cmd.String(rethinkdbUserFlag.Name),
				Password:   cmd.String(rethinkdbPasswordFlag.Name),
				InitialCap: 20,
				MaxOpen:    50,
			}

			ds, err := generic.New(log.WithGroup("datastore"), connectOpts)
			if err != nil {
				return fmt.Errorf("unable to create datastore: %w", err)
			}

			var (
				task  = task.NewClient(log, redisConfig.AsyncClient)
				queue = queue.New(log, redisConfig.QueueClient)
				repo  = repository.New(repository.Config{
					Log:                   log,
					TenantApiserverClient: tc,
					Datastore:             ds,
					Ipam:                  ipam,
					Task:                  task,
					Queue:                 queue,
					Component:             redisConfig.ComponentClient,
					Auditing:              auditSearchBackend,
					HeadscaleClient:       hc,
					TokenConfig: repository.TokenConfig{
						TokenStore: token.NewRedisStore(redisConfig.TokenClient),
						CertStore: certs.NewRedisStore(&certs.Config{
							RedisClient: redisConfig.TokenClient,
						}),
						ProviderTenant: cmd.String(providerTenantFlag.Name),
						Issuer:         cmd.String(serverHttpUrlFlag.Name),
					},
				})
				stage = cmd.String(stageFlag.Name)
			)

			if cmd.Bool(headscaleEnabledFlag.Name) {
				err = repo.UnscopedVPN().SetDefaultPolicy(ctx)
				if err != nil {
					return fmt.Errorf("unable to ensure headscale default policy: %w", err)
				}
			}

			c := service.Config{
				HttpServerEndpoint:                  cmd.String(httpServerEndpointFlag.Name),
				MetricsServerEndpoint:               cmd.String(metricServerEndpointFlag.Name),
				Log:                                 log,
				Repository:                          repo,
				TenantClient:                        tc,
				Datastore:                           ds,
				IpamClient:                          ipam,
				ServerHttpURL:                       cmd.String(serverHttpUrlFlag.Name),
				FrontEndUrl:                         cmd.String(frontEndUrlFlag.Name),
				RedirectURLs:                        cmd.StringSlice(redirectUrlsFlag.Name),
				AuditSearchBackend:                  auditSearchBackend,
				AuditBackends:                       auditBackends,
				Stage:                               stage,
				RedisConfig:                         redisConfig,
				ProviderTenant:                      cmd.String(providerTenantFlag.Name),
				MaxRequestsPerMinuteToken:           cmd.Int(maxRequestsPerMinuteFlag.Name),
				MaxRequestsPerMinuteUnauthenticated: cmd.Int(maxRequestsPerMinuteUnauthenticatedFlag.Name),
				OIDCClientID:                        cmd.String(oidcClientIdFlag.Name),
				OIDCClientSecret:                    cmd.String(oidcClientSecretFlag.Name),
				OIDCDiscoveryURL:                    cmd.String(oidcDiscoveryUrlFlag.Name),
				OIDCEndSessionURL:                   cmd.String(oidcEndSessionUrlFlag.Name),
				OIDCUniqueUserKey:                   cmd.String(oidcUniqueUserKeyFlag.Name),
				OIDCTLSSkipVerify:                   cmd.Bool(oidcTLSSkipVerifyFlag.Name),
				IsStageDev:                          strings.EqualFold(stage, stageDEV),
				SecureCookie:                        cmd.Bool(secureCookieFlag.Name),
				BMCSuperuserPassword:                cmd.String(bmcSuperuserPasswordFlag.Name),
				HeadscaleClient:                     hc,
				ComponentExpiration:                 cmd.Duration(componentExpirationFlag.Name),
			}

			err = repo.Tenant().AdditionalMethods().EnsureProviderTenant(ctx, c.ProviderTenant)
			if err != nil {
				return err
			}

			log.Info("ensured provider tenant", "id", c.ProviderTenant)

			log.Info("running api-server", "version", v.V.String(), "go-runtime", runtime.Version(), "http-endpoint", c.HttpServerEndpoint)

			s := newServer(c)
			if err := s.Run(ctx); err != nil {
				return fmt.Errorf("unable to execute server: %w", err)
			}

			return nil
		},
	}
}

// createTenantApiserverClient creates a client to the tenant-apiserver
func createTenantApiserverClient(cmd *cli.Command, log *slog.Logger) (tenant.Client, error) {
	const tenantApiserverNamespace = "metal-stack.io"

	client, err := tenant.New(&tenant.DialConfig{
		Log:       log.WithGroup("tenant-apiserver-client"),
		BaseURL:   cmd.String(tenantApiserverBaseURLFlag.Name),
		Namespace: tenantApiserverNamespace,
	})
	if err != nil {
		return nil, err
	}

	log.Info("tenant-apiserver client initialized")

	return client, nil
}

type RedisDatabase string

const (
	redisDatabaseTokens       RedisDatabase = "token"
	redisDatabaseRateLimiting RedisDatabase = "rate-limiter"
	redisDatabaseInvites      RedisDatabase = "invite"
	redisDatabaseAsync        RedisDatabase = "async"
	redisDatabaseComponent    RedisDatabase = "component"
)

func createRedisClients(ctx context.Context, cmd *cli.Command, logger *slog.Logger) (*service.RedisConfig, error) {

	token, _, err := createRedisClient(ctx, cmd, logger, redisDatabaseTokens)
	if err != nil {
		return nil, err
	}
	rate, _, err := createRedisClient(ctx, cmd, logger, redisDatabaseRateLimiting)
	if err != nil {
		return nil, err
	}
	invite, _, err := createRedisClient(ctx, cmd, logger, redisDatabaseInvites)
	if err != nil {
		return nil, err
	}
	async, queue, err := createRedisClient(ctx, cmd, logger, redisDatabaseAsync)
	if err != nil {
		return nil, err
	}
	_, component, err := createRedisClient(ctx, cmd, logger, redisDatabaseComponent)
	if err != nil {
		return nil, err
	}
	return &service.RedisConfig{
		TokenClient:     token,
		RateLimitClient: rate,
		InviteClient:    invite,
		AsyncClient:     async,
		QueueClient:     queue,
		ComponentClient: component,
	}, nil
}

func createRedisClient(ctx context.Context, cmd *cli.Command, logger *slog.Logger, dbName RedisDatabase) (*redis.Client, valkey.Client, error) {
	db := 0
	switch dbName {
	case redisDatabaseTokens:
		db = 0
	case redisDatabaseRateLimiting:
		db = 1
	case redisDatabaseInvites:
		db = 2
	case redisDatabaseAsync:
		db = 3
	case redisDatabaseComponent:
		db = 4
	default:
		return nil, nil, fmt.Errorf("invalid db name: %s", dbName)
	}

	address := cmd.String(redisAddrFlag.Name)
	password := cmd.String(redisPasswordFlag.Name)

	if address == "" {
		logger.Warn("no redis address given, start in-memory redis database")
		mr, _ := miniredis.Run()
		address = mr.Addr()
	}

	client := redis.NewClient(&redis.Options{
		Addr:       address,
		Password:   password,
		DB:         db,
		ClientName: applicationName,
	})
	pong, err := client.Ping(ctx).Result()
	if err != nil {
		return nil, nil, fmt.Errorf("unable to create redis client: %w", err)
	}

	if strings.ToLower(pong) != "pong" {
		return nil, nil, fmt.Errorf("unable to create redis client, did not get PONG result: %q", pong)
	}

	valkeyClient, err := valkey.NewClient(valkey.ClientOption{
		InitAddress: []string{address},
		AuthCredentialsFn: func(acc valkey.AuthCredentialsContext) (valkey.AuthCredentials, error) {
			return valkey.AuthCredentials{
				Password: password,
			}, nil
		},
		SelectDB:   db,
		ClientName: applicationName,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("unable to create valkey client: %w", err)
	}

	return client, valkeyClient, nil
}

func createIpamClient(ctx context.Context, cmd *cli.Command, log *slog.Logger) (ipamv1connect.IpamServiceClient, error) {
	ipamgrpcendpoint := cmd.String(ipamGrpcEndpointFlag.Name)
	log.Info("create ipam client", "stage", cmd.String(stageFlag.Name))
	if cmd.String(stageFlag.Name) == stageDEV {
		log.Warn("ipam grpc endpoint not configured, starting in memory ipam service")
		ipam, _ := test.StartIpam(&testing.T{})
		return ipam, nil
	}

	ipamService := ipamv1connect.NewIpamServiceClient(
		http.DefaultClient,
		ipamgrpcendpoint,
		connect.WithGRPC(),
		compress.WithAll(compress.LevelBalanced),
	)

	err := retry.Do(func() error {
		version, err := ipamService.Version(ctx, &ipamv1.VersionRequest{})
		if err != nil {
			return err
		}
		log.Info("connected to ipam service", "version", version)
		return nil
	})

	if err != nil {
		log.Error("unable to connect to ipam service", "error", err)
		return nil, err
	}

	log.Info("ipam initialized")
	return ipamService, nil
}
