package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/alicebob/miniredis/v2"
	"github.com/avast/retry-go/v4"
	compress "github.com/klauspost/connect-compress/v2"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"gopkg.in/rethinkdb/rethinkdb-go.v6"

	"github.com/metal-stack/api-server/pkg/db/generic"
	"github.com/metal-stack/api-server/pkg/test"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	ipamv1 "github.com/metal-stack/go-ipam/api/v1"
	ipamv1connect "github.com/metal-stack/go-ipam/api/v1/apiv1connect"
	mdm "github.com/metal-stack/masterdata-api/pkg/client"

	putil "github.com/metal-stack/api-server/pkg/project"
	tutil "github.com/metal-stack/api-server/pkg/tenant"
	mdmv1 "github.com/metal-stack/masterdata-api/api/v1"

	"github.com/metal-stack/metal-lib/auditing"
	"github.com/metal-stack/v"
	"github.com/redis/go-redis/v9"
	"github.com/urfave/cli/v2"
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

		mc, err := createMasterdataClient(ctx, log)
		if err != nil {
			return fmt.Errorf("unable to create masterdata.client: %w", err)
		}

		ds, err := generic.New(log.WithGroup("datastore"), rethinkdb.ConnectOpts{
			Addresses: ctx.StringSlice(rethinkdbAddressesFlag.Name),
			Database:  ctx.String(rethinkdbDBNameFlag.Name),
			Username:  ctx.String(rethinkdbUserFlag.Name),
			Password:  ctx.String(rethinkdbPasswordFlag.Name),
			MaxIdle:   10,
			MaxOpen:   20,
		})
		if err != nil {
			return fmt.Errorf("unable to create datastore: %w", err)
		}

		c := config{
			HttpServerEndpoint:                  ctx.String(httpServerEndpointFlag.Name),
			MetricsServerEndpoint:               ctx.String(metricServerEndpointFlag.Name),
			Log:                                 log,
			MasterClient:                        mc,
			ServerHttpURL:                       ctx.String(serverHttpUrlFlag.Name),
			FrontEndUrl:                         ctx.String(frontEndUrlFlag.Name),
			Auditing:                            audit,
			Stage:                               ctx.String(stageFlag.Name),
			RedisAddr:                           ctx.String(redisAddrFlag.Name),
			RedisPassword:                       ctx.String(redisPasswordFlag.Name),
			Admins:                              ctx.StringSlice(adminsFlag.Name),
			MaxRequestsPerMinuteToken:           ctx.Int(maxRequestsPerMinuteFlag.Name),
			MaxRequestsPerMinuteUnauthenticated: ctx.Int(maxRequestsPerMinuteUnauthenticatedFlag.Name),
			Datastore:                           ds,
			Ipam:                                ipam,
			OIDCClientID:                        ctx.String(oidcClientIdFlag.Name),
			OIDCClientSecret:                    ctx.String(oidcClientSecretFlag.Name),
			OIDCDiscoveryURL:                    ctx.String(oidcDiscoveryUrlFlag.Name),
			OIDCEndSessionURL:                   ctx.String(oidcEndSessionUrlFlag.Name),
		}

		if providerTenant := ctx.String(ensureProviderTenantFlag.Name); providerTenant != "" {
			err := ensureProviderTenant(ctx.Context, &c, providerTenant)
			if err != nil {
				return err
			}

			err = ensureProviderProject(ctx.Context, &c, providerTenant)
			if err != nil {
				return err
			}

			log.Info("ensured provider tenant", "id", providerTenant)

			c.Admins = append(c.Admins, providerTenant)
		}

		log.Info("running api-server", "version", v.V, "http endpoint", c.HttpServerEndpoint)

		s := newServer(c)
		if err := s.Run(); err != nil {
			return fmt.Errorf("unable to execute server: %w", err)
		}

		return nil
	},
}

// createMasterdataClient creates a client to the masterdata-api
func createMasterdataClient(cli *cli.Context, log *slog.Logger) (mdm.Client, error) {
	ctx, cancel := context.WithTimeout(cli.Context, 3*time.Second)
	defer cancel()

	client, err := mdm.NewClient(ctx,
		cli.String(masterdataApiHostnameFlag.Name),
		cli.Int(masterdataApiPortFlag.Name),
		cli.String(masterdataApiCertPathFlag.Name),
		cli.String(masterdataApiCertKeyPathFlag.Name),
		cli.String(masterdataApiCAPathFlag.Name),
		cli.String(masterdataApiHmacFlag.Name),
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

func ensureProviderTenant(ctx context.Context, c *config, providerTenantID string) error {
	_, err := c.MasterClient.Tenant().Get(ctx, &mdmv1.TenantGetRequest{
		Id: providerTenantID,
	})
	if err != nil && !mdmv1.IsNotFound(err) {
		return fmt.Errorf("unable to get tenant %q: %w", providerTenantID, err)
	}

	if err != nil && mdmv1.IsNotFound(err) {
		_, err := c.MasterClient.Tenant().Create(ctx, &mdmv1.TenantCreateRequest{
			Tenant: &mdmv1.Tenant{
				Meta: &mdmv1.Meta{
					Id: providerTenantID,
					Annotations: map[string]string{
						tutil.TagCreator: providerTenantID,
					},
				},
				Name:        providerTenantID,
				Description: "initial provider tenant for metal-stack",
			},
		})
		if err != nil {
			return fmt.Errorf("unable to create tenant:%s %w", providerTenantID, err)
		}
	}

	_, err = tutil.GetTenantMember(ctx, c.MasterClient, providerTenantID, providerTenantID)
	if err == nil {
		return nil
	}

	if connect.CodeOf(err) != connect.CodeNotFound {
		return err
	}

	_, err = c.MasterClient.TenantMember().Create(ctx, &mdmv1.TenantMemberCreateRequest{
		TenantMember: &mdmv1.TenantMember{
			Meta: &mdmv1.Meta{
				Annotations: map[string]string{
					tutil.TenantRoleAnnotation: apiv2.TenantRole_TENANT_ROLE_OWNER.String(),
				},
			},
			TenantId: providerTenantID,
			MemberId: providerTenantID,
		},
	})
	if err != nil {
		return err
	}

	return nil
}

func ensureProviderProject(ctx context.Context, c *config, providerTenantID string) error {
	ensureMembership := func(projectId string) error {
		_, _, err := putil.GetProjectMember(ctx, c.MasterClient, projectId, providerTenantID)
		if err == nil {
			return nil
		}
		if connect.CodeOf(err) != connect.CodeNotFound {
			return err
		}

		_, err = c.MasterClient.ProjectMember().Create(ctx, &mdmv1.ProjectMemberCreateRequest{
			ProjectMember: &mdmv1.ProjectMember{
				Meta: &mdmv1.Meta{
					Annotations: map[string]string{
						putil.ProjectRoleAnnotation: apiv2.ProjectRole_PROJECT_ROLE_OWNER.String(),
					},
				},
				ProjectId: projectId,
				TenantId:  providerTenantID,
			},
		})

		return err
	}

	resp, err := c.MasterClient.Project().Find(ctx, &mdmv1.ProjectFindRequest{
		TenantId: wrapperspb.String(providerTenantID),
		Annotations: map[string]string{
			putil.DefaultProjectAnnotation: strconv.FormatBool(true),
		},
	})
	if err != nil {
		return fmt.Errorf("unable to get find project %q: %w", providerTenantID, err)
	}

	if len(resp.Projects) > 0 {
		return ensureMembership(resp.Projects[0].Meta.Id)
	}

	project, err := c.MasterClient.Project().Create(ctx, &mdmv1.ProjectCreateRequest{
		Project: &mdmv1.Project{
			Meta: &mdmv1.Meta{
				Annotations: map[string]string{
					putil.DefaultProjectAnnotation: strconv.FormatBool(true),
				},
			},
			Name:        "Default Project",
			TenantId:    providerTenantID,
			Description: "Default project of " + providerTenantID,
		},
	})
	if err != nil {
		return fmt.Errorf("unable to create project: %w", err)
	}

	return ensureMembership(project.Project.Meta.Id)
}
