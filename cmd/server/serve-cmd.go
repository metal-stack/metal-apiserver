package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

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

	putil "github.com/metal-stack/metal-apiserver/pkg/project"
	tutil "github.com/metal-stack/metal-apiserver/pkg/tenant"

	"github.com/metal-stack/metal-lib/auditing"
	"github.com/metal-stack/v"
	"github.com/redis/go-redis/v9"
	"github.com/urfave/cli/v3"
)

func newServeCmd(ctx context.Context) *cli.Command {
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
		Action: func(ctx context.Context, cmd *cli.Command) error {
			log, err := createLogger(cmd)
			if err != nil {
				return fmt.Errorf("unable to create logger %w", err)
			}

			audit, err := createAuditingClient(cmd, log)
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

			mc, err := createMasterdataClient(ctx, cmd, log)
			if err != nil {
				return fmt.Errorf("unable to create masterdata.client: %w", err)
			}

			connectOpts := rethinkdb.ConnectOpts{
				Addresses: cmd.StringSlice(rethinkdbAddressesFlag.Name),
				Database:  cmd.String(rethinkdbDBNameFlag.Name),
				Username:  cmd.String(rethinkdbUserFlag.Name),
				Password:  cmd.String(rethinkdbPasswordFlag.Name),
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

			stage := cmd.String(stageFlag.Name)
			c := service.Config{
				HttpServerEndpoint:                  cmd.String(httpServerEndpointFlag.Name),
				MetricsServerEndpoint:               cmd.String(metricServerEndpointFlag.Name),
				Log:                                 log,
				Repository:                          repo,
				MasterClient:                        mc,
				Datastore:                           ds,
				IpamClient:                          ipam,
				ServerHttpURL:                       cmd.String(serverHttpUrlFlag.Name),
				FrontEndUrl:                         cmd.String(frontEndUrlFlag.Name),
				Auditing:                            audit,
				Stage:                               stage,
				RedisConfig:                         redisConfig,
				Admins:                              cmd.StringSlice(adminsFlag.Name),
				MaxRequestsPerMinuteToken:           cmd.Int(maxRequestsPerMinuteFlag.Name),
				MaxRequestsPerMinuteUnauthenticated: cmd.Int(maxRequestsPerMinuteUnauthenticatedFlag.Name),
				OIDCClientID:                        cmd.String(oidcClientIdFlag.Name),
				OIDCClientSecret:                    cmd.String(oidcClientSecretFlag.Name),
				OIDCDiscoveryURL:                    cmd.String(oidcDiscoveryUrlFlag.Name),
				OIDCEndSessionURL:                   cmd.String(oidcEndSessionUrlFlag.Name),
				IsStageDev:                          strings.EqualFold(stage, stageDEV),
			}

			if providerTenant := cmd.String(ensureProviderTenantFlag.Name); providerTenant != "" {
				err := tutil.EnsureProviderTenant(ctx, c.MasterClient, providerTenant)
				if err != nil {
					return err
				}

				err = putil.EnsureProviderProject(ctx, c.MasterClient, providerTenant)
				if err != nil {
					return err
				}

				log.Info("ensured provider tenant", "id", providerTenant)

				c.Admins = append(c.Admins, providerTenant)
			}

			log.Info("running api-server", "version", v.V, "http endpoint", c.HttpServerEndpoint)

			s := newServer(c)
			if err := s.Run(ctx); err != nil {
				return fmt.Errorf("unable to execute server: %w", err)
			}

			return nil
		},
	}
}

// createMasterdataClient creates a client to the masterdata-api
func createMasterdataClient(ctx context.Context, cmd *cli.Command, log *slog.Logger) (mdm.Client, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	client, err := mdm.NewClient(ctx,
		cmd.String(masterdataApiHostnameFlag.Name),
		cmd.Int(masterdataApiPortFlag.Name),
		cmd.String(masterdataApiCertPathFlag.Name),
		cmd.String(masterdataApiCertKeyPathFlag.Name),
		cmd.String(masterdataApiCAPathFlag.Name),
		cmd.String(masterdataApiHmacFlag.Name),
		false, // TLSSkipInsecure
		log.WithGroup("masterdata-client"),
	)
	if err != nil {
		return nil, err
	}

	log.Info("masterdata client initialized")

	return client, nil
}

// createAuditingClient creates a new auditing client
// Can return nil,nil if auditing is disabled!
func createAuditingClient(cmd *cli.Command, log *slog.Logger) (auditing.Auditing, error) {
	const auditingComponent = "metal-stack.io"

	auditingEnabled := cmd.Bool(auditingTimescaleEnabledFlag.Name)
	if !auditingEnabled {
		return nil, nil
	}

	auditingCfg := auditing.Config{
		Log:       log,
		Component: auditingComponent,
	}
	return auditing.NewTimescaleDB(auditingCfg, auditing.TimescaleDbConfig{
		Host:      cmd.String(auditingTimescaleHostFlag.Name),
		Port:      cmd.String(auditingTimescalePortFlag.Name),
		DB:        cmd.String(auditingTimescaleDbFlag.Name),
		User:      cmd.String(auditingTimescaleUserFlag.Name),
		Password:  cmd.String(auditingTimescalePasswordFlag.Name),
		Retention: cmd.String(auditingTimescaleRetentionFlag.Name),
	})
}

type RedisDatabase string

const (
	redisDatabaseTokens       RedisDatabase = "token"
	redisDatabaseRateLimiting RedisDatabase = "rate-limiter"
	redisDatabaseInvites      RedisDatabase = "invite"
	redisDatabaseAsync        RedisDatabase = "async"
)

func createRedisClients(ctx context.Context, cmd *cli.Command, logger *slog.Logger) (*service.RedisConfig, error) {

	token, err := createRedisClient(ctx, cmd, logger, redisDatabaseTokens)
	if err != nil {
		return nil, err
	}
	rate, err := createRedisClient(ctx, cmd, logger, redisDatabaseRateLimiting)
	if err != nil {
		return nil, err
	}
	invite, err := createRedisClient(ctx, cmd, logger, redisDatabaseInvites)
	if err != nil {
		return nil, err
	}
	async, err := createRedisClient(ctx, cmd, logger, redisDatabaseAsync)
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

func createRedisClient(ctx context.Context, cmd *cli.Command, logger *slog.Logger, dbName RedisDatabase) (*redis.Client, error) {
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

	address := cmd.String(redisAddrFlag.Name)
	password := cmd.String(redisPasswordFlag.Name)

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
	pong, err := client.Ping(ctx).Result()
	if err != nil {
		return nil, fmt.Errorf("unable to create redis client: %w", err)
	}

	if strings.ToLower(pong) != "pong" {
		return nil, fmt.Errorf("unable to create redis client, did not get PONG result: %q", pong)
	}

	return client, nil
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
		version, err := ipamService.Version(ctx, connect.NewRequest(&ipamv1.VersionRequest{}))
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
