package main

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"github.com/alicebob/miniredis/v2"
	"github.com/avast/retry-go/v4"
	compress "github.com/klauspost/connect-compress/v2"
	"gopkg.in/rethinkdb/rethinkdb-go.v6"

	ipamv1 "github.com/metal-stack/go-ipam/api/v1"
	ipamv1connect "github.com/metal-stack/go-ipam/api/v1/apiv1connect"
	mdm "github.com/metal-stack/masterdata-api/pkg/client"
	"github.com/metal-stack/metal-apiserver/pkg/db/generic"
	"github.com/metal-stack/metal-apiserver/pkg/repository"
	"github.com/metal-stack/metal-apiserver/pkg/service"
	"github.com/metal-stack/metal-apiserver/pkg/test"

	"github.com/metal-stack/metal-lib/auditing"
	"github.com/metal-stack/v"
	"github.com/redis/go-redis/v9"
	"github.com/urfave/cli/v2"
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
			masterdataApiHostnameFlag,
			masterdataApiPortFlag,
			masterdataApiHmacFlag,
			masterdataApiCAPathFlag,
			masterdataApiCertPathFlag,
			masterdataApiCertKeyPathFlag,
			rethinkdbAddressesFlag,
			rethinkdbDBNameFlag,
			rethinkdbPasswordFlag,
			rethinkdbUserFlag,
			auditingTimescaleEnabledFlag,
			auditingTimescaleHostFlag,
			auditingTimescalePortFlag,
			auditingTimescaleDbFlag,
			auditingTimescaleUserFlag,
			auditingTimescalePasswordFlag,
			auditingTimescaleRetentionFlag,
			stageFlag,
			redisAddrFlag,
			redisPasswordFlag,
			adminsFlag,
			maxRequestsPerMinuteFlag,
			maxRequestsPerMinuteUnauthenticatedFlag,
			ipamGrpcEndpointFlag,
			ensureProviderTenantFlag,
			frontEndUrlFlag,
			oidcClientIdFlag,
			oidcClientSecretFlag,
			oidcDiscoveryUrlFlag,
			oidcEndSessionUrlFlag,
		},
		Action: func(ctx *cli.Context) error {
			log, err := createLogger(ctx)
			if err != nil {
				return fmt.Errorf("unable to create logger %w", err)
			}

			audit, err := createAuditingClient(ctx, log)
			if err != nil {
				return fmt.Errorf("unable to create auditing client: %w", err)
			}

			ipam, err := createIpamClient(ctx, log)
			if err != nil {
				return fmt.Errorf("unable to create ipam client: %w", err)
			}

			redisConfig, err := createRedisClients(ctx, log)
			if err != nil {
				return fmt.Errorf("unable to create redis clients: %w", err)
			}

			mc, err := createMasterdataClient(ctx, log)
			if err != nil {
				return fmt.Errorf("unable to create masterdata.client: %w", err)
			}

			connectOpts := rethinkdb.ConnectOpts{
				Addresses: ctx.StringSlice(rethinkdbAddressesFlag.Name),
				Database:  ctx.String(rethinkdbDBNameFlag.Name),
				Username:  ctx.String(rethinkdbUserFlag.Name),
				Password:  ctx.String(rethinkdbPasswordFlag.Name),
				MaxIdle:   10,
				MaxOpen:   20,
			}

			ds, err := generic.New(log.WithGroup("datastore"), connectOpts)
			if err != nil {
				return fmt.Errorf("unable to create datastore: %w", err)
			}

			repo, err := repository.New(log, mc, ds, ipam, redisConfig.AsyncClient)
			if err != nil {
				return fmt.Errorf("unable to create repository: %w", err)
			}

			stage := ctx.String(stageFlag.Name)
			c := service.Config{
				HttpServerEndpoint:                  ctx.String(httpServerEndpointFlag.Name),
				MetricsServerEndpoint:               ctx.String(metricServerEndpointFlag.Name),
				Log:                                 log,
				Repository:                          repo,
				MasterClient:                        mc,
				Datastore:                           ds,
				IpamClient:                          ipam,
				ServerHttpURL:                       ctx.String(serverHttpUrlFlag.Name),
				FrontEndUrl:                         ctx.String(frontEndUrlFlag.Name),
				Auditing:                            audit,
				Stage:                               stage,
				RedisConfig:                         redisConfig,
				Admins:                              ctx.StringSlice(adminsFlag.Name),
				MaxRequestsPerMinuteToken:           ctx.Int(maxRequestsPerMinuteFlag.Name),
				MaxRequestsPerMinuteUnauthenticated: ctx.Int(maxRequestsPerMinuteUnauthenticatedFlag.Name),
				OIDCClientID:                        ctx.String(oidcClientIdFlag.Name),
				OIDCClientSecret:                    ctx.String(oidcClientSecretFlag.Name),
				OIDCDiscoveryURL:                    ctx.String(oidcDiscoveryUrlFlag.Name),
				OIDCEndSessionURL:                   ctx.String(oidcEndSessionUrlFlag.Name),
				IsStageDev:                          strings.EqualFold(stage, stageDEV),
			}

			if providerTenant := ctx.String(ensureProviderTenantFlag.Name); providerTenant != "" {
				err := repo.Tenant().AdditionalMethods().EnsureProviderTenant(ctx.Context, providerTenant)
				if err != nil {
					return err
				}

				err = repo.UnscopedProject().AdditionalMethods().EnsureProviderProject(ctx.Context, providerTenant)
				if err != nil {
					return err
				}

				log.Info("ensured provider tenant", "id", providerTenant)

				c.Admins = append(c.Admins, providerTenant)
			}

			log.Info("running api-server", "version", v.V, "http endpoint", c.HttpServerEndpoint)

			s := newServer(c)
			if err := s.Run(ctx.Context); err != nil {
				return fmt.Errorf("unable to execute server: %w", err)
			}

			return nil
		},
	}
}

// createMasterdataClient creates a client to the masterdata-api
func createMasterdataClient(cli *cli.Context, log *slog.Logger) (mdm.Client, error) {
	const masterdataNamespace = "metal-stack.io"

	client, err := mdm.NewClient(&mdm.Config{
		Logger:    log.WithGroup("masterdata-client"),
		Hostname:  cli.String(masterdataApiHostnameFlag.Name),
		Port:      cli.Uint(masterdataApiPortFlag.Name),
		CertFile:  cli.String(masterdataApiCertPathFlag.Name),
		KeyFile:   cli.String(masterdataApiCertKeyPathFlag.Name),
		CaFile:    cli.String(masterdataApiCAPathFlag.Name),
		Insecure:  false,
		HmacKey:   cli.String(masterdataApiHmacFlag.Name),
		Namespace: masterdataNamespace,
	})
	if err != nil {
		return nil, err
	}

	log.Info("masterdata client initialized")

	return client, nil
}

// createAuditingClient creates a new auditing client
// Can return nil,nil if auditing is disabled!
func createAuditingClient(cli *cli.Context, log *slog.Logger) (auditing.Auditing, error) {
	const auditingComponent = "metal-stack.io"

	auditingEnabled := cli.Bool(auditingTimescaleEnabledFlag.Name)
	if !auditingEnabled {
		return nil, nil
	}

	auditingCfg := auditing.Config{
		Log:       log,
		Component: auditingComponent,
	}
	return auditing.NewTimescaleDB(auditingCfg, auditing.TimescaleDbConfig{
		Host:      cli.String(auditingTimescaleHostFlag.Name),
		Port:      cli.String(auditingTimescalePortFlag.Name),
		DB:        cli.String(auditingTimescaleDbFlag.Name),
		User:      cli.String(auditingTimescaleUserFlag.Name),
		Password:  cli.String(auditingTimescalePasswordFlag.Name),
		Retention: cli.String(auditingTimescaleRetentionFlag.Name),
	})
}

type RedisDatabase string

const (
	redisDatabaseTokens       RedisDatabase = "token"
	redisDatabaseRateLimiting RedisDatabase = "rate-limiter"
	redisDatabaseInvites      RedisDatabase = "invite"
	redisDatabaseAsync        RedisDatabase = "async"
)

func createRedisClients(cli *cli.Context, logger *slog.Logger) (*service.RedisConfig, error) {

	token, err := createRedisClient(cli, logger, redisDatabaseTokens)
	if err != nil {
		return nil, err
	}
	rate, err := createRedisClient(cli, logger, redisDatabaseRateLimiting)
	if err != nil {
		return nil, err
	}
	invite, err := createRedisClient(cli, logger, redisDatabaseInvites)
	if err != nil {
		return nil, err
	}
	async, err := createRedisClient(cli, logger, redisDatabaseAsync)
	if err != nil {
		return nil, err
	}
	return &service.RedisConfig{
		TokenClient:     token,
		RateLimitClient: rate,
		InviteClient:    invite,
		AsyncClient:     async,
	}, nil
}

func createRedisClient(cli *cli.Context, logger *slog.Logger, dbName RedisDatabase) (*redis.Client, error) {
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
	default:
		return nil, fmt.Errorf("invalid db name: %s", dbName)
	}

	address := cli.String(redisAddrFlag.Name)
	password := cli.String(redisPasswordFlag.Name)

	// If we see performance Issues we can try this client
	// client, err := rueidis.NewClient(rueidis.ClientOption{InitAddress: c.RedisAddresses})
	// if err != nil {
	// 	return nil, err
	// }

	if address == "" {
		logger.Warn("no redis address given, start in-memory redis database")
		mr, _ := miniredis.Run()
		address = mr.Addr()
	}

	client := redis.NewClient(&redis.Options{
		Addr:       address,
		Password:   password,
		DB:         db,
		ClientName: "metal-apiserver",
	})
	pong, err := client.Ping(cli.Context).Result()
	if err != nil {
		return nil, fmt.Errorf("unable to create redis client: %w", err)
	}

	if strings.ToLower(pong) != "pong" {
		return nil, fmt.Errorf("unable to create redis client, did not get PONG result: %q", pong)
	}

	return client, nil
}

func createIpamClient(cli *cli.Context, log *slog.Logger) (ipamv1connect.IpamServiceClient, error) {
	ipamgrpcendpoint := cli.String(ipamGrpcEndpointFlag.Name)
	log.Info("create ipam client", "stage", cli.String(stageFlag.Name))
	if cli.String(stageFlag.Name) == stageDEV {
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
		version, err := ipamService.Version(cli.Context, connect.NewRequest(&ipamv1.VersionRequest{}))
		if err != nil {
			return err
		}
		log.Info("connected to ipam service", "version", version.Msg)
		return nil
	})

	if err != nil {
		log.Error("unable to connect to ipam service", "error", err)
		return nil, err
	}

	log.Info("ipam initialized")
	return ipamService, nil
}
