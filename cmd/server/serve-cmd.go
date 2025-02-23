package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/alicebob/miniredis/v2"
	"github.com/avast/retry-go/v4"
	compress "github.com/klauspost/connect-compress/v2"

	"github.com/metal-stack/api-server/pkg/test"
	ipamv1 "github.com/metal-stack/go-ipam/api/v1"
	ipamv1connect "github.com/metal-stack/go-ipam/api/v1/apiv1connect"
	mdm "github.com/metal-stack/masterdata-api/pkg/client"
	"github.com/metal-stack/metal-lib/auditing"
	"github.com/metal-stack/v"
	"github.com/redis/go-redis/v9"
	"github.com/urfave/cli/v2"
	r "gopkg.in/rethinkdb/rethinkdb-go.v6"
)

var serveCmd = &cli.Command{
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
		rethinkdbDBFlag,
		rethinkdbDBNameFlag,
		rethinkdbPasswordFlag,
		rethinkdbUserFlag,
		auditingUrlFlag,
		auditingApiKeyFlag,
		auditingEnabledFlag,
		auditingIndexPrefixFlag,
		auditingIndexIntervalFlag,
		auditingIndexKeepFlag,
		stageFlag,
		redisAddrFlag,
		redisPasswordFlag,
		adminsFlag,
		maxRequestsPerMinuteFlag,
		maxRequestsPerMinuteUnauthenticatedFlag,
		ipamGrpcEndpointFlag,
	},
	Action: func(ctx *cli.Context) error {
		log, level, err := createLoggers(ctx)
		if err != nil {
			return fmt.Errorf("unable to create logger %w", err)
		}
		audit, err := createAuditingClient(ctx, log)
		if err != nil {
			log.Error("unable to create auditing client", "error", err)
			os.Exit(1)
		}

		redisAddr := ctx.String(redisAddrFlag.Name)
		stage := ctx.String(stageFlag.Name)

		rethinkDBSession, err := createRethinkDBClient(ctx, log)
		if err != nil {
			log.Error("unable to create rethinkdb client", "error", err)
			os.Exit(1)
		}

		ipam, err := createIpamClient(ctx, log)
		if err != nil {
			log.Error("unable to create ipam client", "error", err)
			os.Exit(1)
		}

		c := config{
			HttpServerEndpoint:                  ctx.String(httpServerEndpointFlag.Name),
			MetricsServerEndpoint:               ctx.String(metricServerEndpointFlag.Name),
			Log:                                 log,
			MasterClient:                        retryConnectMasterdataClient(ctx, log),
			ServerHttpURL:                       ctx.String(serverHttpUrlFlag.Name),
			Auditing:                            audit,
			Stage:                               stage,
			RedisAddr:                           redisAddr,
			RedisPassword:                       ctx.String(redisPasswordFlag.Name),
			Admins:                              ctx.StringSlice(adminsFlag.Name),
			MaxRequestsPerMinuteToken:           ctx.Int(maxRequestsPerMinuteFlag.Name),
			MaxRequestsPerMinuteUnauthenticated: ctx.Int(maxRequestsPerMinuteUnauthenticatedFlag.Name),
			RethinkDB:                           ctx.String(rethinkdbDBNameFlag.Name),
			RethinkDBSession:                    rethinkDBSession,
			Ipam:                                ipam,
		}

		log.Info("running api-server", "version", v.V, "level", level, "http endpoint", c.HttpServerEndpoint)
		s := newServer(c)
		if err := s.Run(); err != nil {
			log.Error("unable to execute server", "error", err)
			os.Exit(1)
		}
		return nil
	},
}

// retryConnectMasterdataClient creates a client to the masterdata-api
// this is a blocking operation
func retryConnectMasterdataClient(cli *cli.Context, logger *slog.Logger) mdm.Client {
	var err error
	var client mdm.Client
	for {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		client, err = mdm.NewClient(ctx,
			cli.String(masterdataApiHostnameFlag.Name),
			cli.Int(masterdataApiPortFlag.Name),
			cli.String(masterdataApiCertPathFlag.Name),
			cli.String(masterdataApiCertKeyPathFlag.Name),
			cli.String(masterdataApiCAPathFlag.Name),
			cli.String(masterdataApiHmacFlag.Name),
			false, // TLSSkipInsecure
			slog.Default(),
		)
		if err == nil {
			defer cancel()
			break
		}
		cancel()
		logger.Error("unable to initialize masterdata-api client, retrying...", "error", err)
		time.Sleep(3 * time.Second)
	}

	logger.Info("masterdata client initialized")

	return client
}

func createRethinkDBClient(cli *cli.Context, log *slog.Logger) (*r.Session, error) {
	addresses := cli.StringSlice(rethinkdbAddressesFlag.Name)
	dbname := cli.String(rethinkdbDBNameFlag.Name)
	user := cli.String(rethinkdbUserFlag.Name)
	password := cli.String(rethinkdbPasswordFlag.Name)
	log.Info("create rethinkdb client", "addresses", addresses, "dbname", dbname, "user", user, "password", password)
	session, err := r.Connect(r.ConnectOpts{
		Addresses: addresses,
		Database:  dbname,
		Username:  user,
		Password:  password,
		MaxIdle:   10,
		MaxOpen:   20,
	})
	return session, err
}

// createAuditingClient creates a new auditing client
// Can return nil,nil if auditing is disabled!
func createAuditingClient(cli *cli.Context, log *slog.Logger) (auditing.Auditing, error) {
	auditingEnabled := cli.Bool(auditingEnabledFlag.Name)
	if !auditingEnabled {
		return nil, nil
	}
	c := auditing.MeilisearchConfig{
		URL:              cli.String(auditingUrlFlag.Name),
		APIKey:           cli.String(auditingApiKeyFlag.Name),
		IndexPrefix:      cli.String(auditingIndexPrefixFlag.Name),
		RotationInterval: auditing.Interval(cli.String(auditingIndexIntervalFlag.Name)),
		Keep:             cli.Int64(auditingIndexKeepFlag.Name),
	}
	return auditing.NewMeilisearch(auditing.Config{Component: "apiserver", Log: log}, c)
}

type RedisDatabase string

const (
	redisDatabaseTokens       RedisDatabase = "token"
	redisDatabaseRateLimiting RedisDatabase = "rate-limiter"
	redisDatabaseInvites      RedisDatabase = "invite"
	redisDatabaseTx           RedisDatabase = "tx"
)

func createRedisClient(logger *slog.Logger, address, password string, dbName RedisDatabase) (*redis.Client, error) {
	db := 0
	switch dbName {
	case redisDatabaseTokens:
		db = 0
	case redisDatabaseRateLimiting:
		db = 1
	case redisDatabaseInvites:
		db = 2
	case redisDatabaseTx:
		db = 3
	default:
		return nil, fmt.Errorf("invalid db name: %s", dbName)
	}

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
	pong, err := client.Ping(context.Background()).Result()
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
		return test.StartIpam(&testing.T{}), nil
	}

	ipamService := ipamv1connect.NewIpamServiceClient(
		http.DefaultClient,
		ipamgrpcendpoint,
		connect.WithGRPC(),
		compress.WithAll(compress.LevelBalanced),
	)

	err := retry.Do(func() error {
		version, err := ipamService.Version(context.Background(), connect.NewRequest(&ipamv1.VersionRequest{}))
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
