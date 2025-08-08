package main

import (
	"context"
	"log"
	"log/slog"
	"os"

	"github.com/urfave/cli/v3"
)

const (
	stageDEV  = "DEV"
	stagePROD = "PROD"
)

var (
	// FIXME add cli.EnvVar to all flags
	httpServerEndpointFlag = &cli.StringFlag{
		Name:    "http-server-endpoint",
		Value:   "localhost:8081",
		Usage:   "http server bind address",
		Sources: cli.EnvVars("HTTP_SERVER_ENDPOINT"),
		Local:   true,
	}
	metricServerEndpointFlag = &cli.StringFlag{
		Name:    "metric-server-endpoint",
		Value:   "localhost:2112",
		Usage:   "metric server endpoint",
		Sources: cli.EnvVars("METRICS_SERVER_ENDPOINT"),
		Local:   true,
	}
	serverHttpUrlFlag = &cli.StringFlag{
		Name:    "server-http-url",
		Value:   "http://localhost:8081",
		Usage:   "the url on which the http server is reachable from the outside",
		Sources: cli.EnvVars("SERVER_HTTP_URL"),
		Local:   true,
	}
	sessionSecretFlag = &cli.StringFlag{
		Name:     "session-secret",
		Value:    "geheim",
		Usage:    "session secret encrypts the cookie, used by goth cookiestore",
		Required: true,
		Sources:  cli.EnvVars("SESSION_SECRET"),
		Local:    true,
	}
	frontEndUrlFlag = &cli.StringFlag{
		Name:    "front-end-url",
		Value:   "https://metal-stack.io",
		Usage:   "URL of the frontend (metalctl)",
		Sources: cli.EnvVars("FRONT_END_URL"),
		Local:   true,
	}
	oidcClientIdFlag = &cli.StringFlag{
		Name:     "oidc-client-id",
		Value:    "",
		Usage:    "id of the oauth app in oidc",
		Required: true,
		Sources:  cli.EnvVars("OIDC_CLIENT_ID"),
		Local:    true,
	}
	oidcClientSecretFlag = &cli.StringFlag{
		Name:     "oidc-client-secret",
		Value:    "",
		Usage:    "client secret of the oauth app in oidc",
		Required: true,
		Sources:  cli.EnvVars("OIDC_CLIENT_SECRET"),
		Local:    true,
	}
	oidcDiscoveryUrlFlag = &cli.StringFlag{
		Name:     "oidc-discovery-url",
		Value:    "",
		Usage:    "discovery url of the oauth app in oidc",
		Required: true,
		Sources:  cli.EnvVars("OIDC_DISCOVERY_URL"),
		Local:    true,
	}
	oidcEndSessionUrlFlag = &cli.StringFlag{
		Name:     "oidc-end-session-url",
		Value:    "",
		Required: false,
		Sources:  cli.EnvVars("OIDC_END_SESSION_URL"),
		Local:    true,
	}
	logLevelFlag = &cli.StringFlag{
		Name:    "log-level",
		Value:   "info",
		Usage:   "log-level can be one of error|warn|info|debug",
		Sources: cli.EnvVars("LOG_LEVEL"),
		Local:   true,
	}
	masterdataApiHostnameFlag = &cli.StringFlag{
		Name:    "masterdata-api-hostname",
		Value:   "localhost",
		Usage:   "masterdata-api hostname",
		Sources: cli.EnvVars("MASTERDATA_API_HOSTNAME"),
		Local:   true,
	}
	masterdataApiPortFlag = &cli.IntFlag{
		Name:    "masterdata-api-port",
		Value:   50051,
		Usage:   "masterdata-api port",
		Sources: cli.EnvVars("MASTERDATA_API_PORT"),
		Local:   true,
	}
	masterdataApiHmacFlag = &cli.StringFlag{
		Name:    "masterdata-api-hmac",
		Value:   "",
		Usage:   "masterdata-api-hmac",
		Sources: cli.EnvVars("MASTERDATA_API_HMAC"),
		Local:   true,
	}
	masterdataApiCAPathFlag = &cli.StringFlag{
		Name:    "masterdata-api-ca-path",
		Value:   "certs/ca.pem",
		Usage:   "masterdata-api CA path",
		Sources: cli.EnvVars("MASTERDATA_API_CA_PATH"),
		Local:   true,
	}
	masterdataApiCertPathFlag = &cli.StringFlag{
		Name:    "masterdata-api-cert-path",
		Value:   "certs/client.pem",
		Usage:   "masterdata-api certificate path",
		Sources: cli.EnvVars("MASTERDATA_API_CERT_PATH"),
		Local:   true,
	}
	masterdataApiCertKeyPathFlag = &cli.StringFlag{
		Name:    "masterdata-api-cert-key-path",
		Value:   "certs/client-key.pem",
		Usage:   "masterdata-api certificate key path",
		Sources: cli.EnvVars("MASTERDATA_API_CERT_KEY_PATH"),
		Local:   true,
	}
	rethinkdbAddressesFlag = &cli.StringSliceFlag{
		Name:     "rethinkdb-addresses",
		Value:    []string{},
		Required: true,
		Usage:    "rethinkdb addresses without http prefix",
		Sources:  cli.EnvVars("RETHINKDB_ADDRESSES"),
		Local:    true,
	}
	rethinkdbDBNameFlag = &cli.StringFlag{
		Name:    "rethinkdb-dbname",
		Value:   "metalapi",
		Usage:   "rethinkdb database name",
		Sources: cli.EnvVars("RETHINKDB_DBNAME"),
		Local:   true,
	}
	rethinkdbUserFlag = &cli.StringFlag{
		Name:    "rethinkdb-user",
		Value:   "admin",
		Usage:   "rethinkdb username to connect",
		Sources: cli.EnvVars("RETHINKDB_USER"),
		Local:   true,
	}
	rethinkdbPasswordFlag = &cli.StringFlag{
		Name:     "rethinkdb-password",
		Value:    "",
		Required: true,
		Usage:    "rethinkdb password to connect",
		Sources:  cli.EnvVars("RETHINKDB_PASSWORD"),
		Local:    true,
	}
	auditingTimescaleEnabledFlag = &cli.BoolFlag{
		Name:    "auditing-timescaledb-enabled",
		Value:   false,
		Usage:   "enable timescaledb auditing",
		Sources: cli.EnvVars("AUDITING_TIMESCALEDB_ENABLED"),
		Local:   true,
	}
	auditingTimescaleHostFlag = &cli.StringFlag{
		Name:    "auditing-timescaledb-host",
		Value:   "",
		Usage:   "timescaledb auditing database host",
		Sources: cli.EnvVars("AUDITING_TIMESCALEDB_HOST"),
		Local:   true,
	}
	auditingTimescalePortFlag = &cli.StringFlag{
		Name:    "auditing-timescaledb-port",
		Value:   "5432",
		Usage:   "timescaledb auditing database port",
		Sources: cli.EnvVars("AUDITING_TIMESCALEDB_PORT"),
		Local:   true,
	}
	auditingTimescaleDbFlag = &cli.StringFlag{
		Name:    "auditing-timescaledb-db",
		Value:   "auditing",
		Usage:   "timescaledb auditing database",
		Sources: cli.EnvVars("AUDITING_TIMESCALEDB_DB"),
		Local:   true,
	}
	auditingTimescaleUserFlag = &cli.StringFlag{
		Name:    "auditing-timescaledb-user",
		Value:   "postgres",
		Usage:   "timescaledb auditing database user",
		Sources: cli.EnvVars("AUDITING_TIMESCALEDB_USER"),
		Local:   true,
	}
	auditingTimescalePasswordFlag = &cli.StringFlag{
		Name:    "auditing-timescaledb-password",
		Value:   "",
		Usage:   "timescaledb auditing database password",
		Sources: cli.EnvVars("AUDITING_TIMESCALEDB_PASSWORD"),
		Local:   true,
	}
	auditingTimescaleRetentionFlag = &cli.StringFlag{
		Name:    "auditing-timescaledb-retention",
		Value:   "14 days",
		Usage:   "timescaledb auditing database retention",
		Sources: cli.EnvVars("AUDITING_TIMESCALEDB_RETENTION"),
		Local:   true,
	}
	stageFlag = &cli.StringFlag{
		Name:    "stage",
		Value:   stagePROD,
		Usage:   "deployment stage of application",
		Sources: cli.EnvVars("STAGE"),
		Local:   true,
	}
	redisAddrFlag = &cli.StringFlag{
		Name:    "redis-addr",
		Value:   "",
		Usage:   "the address to a redis key value store",
		Sources: cli.EnvVars("REDIS_ADDR"),
		Local:   true,
	}
	redisPasswordFlag = &cli.StringFlag{
		Name:    "redis-password",
		Value:   "",
		Usage:   "the password to the redis key value store",
		Sources: cli.EnvVars("REDIS_PASSWORD"),
		Local:   true,
	}
	adminsFlag = &cli.StringSliceFlag{
		Name:    "admin-subjects",
		Value:   []string{"metal-stack-ops@github"},
		Usage:   "the user subjects that are considered as administrators when creating api tokens to gain extended api access permissions",
		Sources: cli.EnvVars("ADMIN_SUBJECTS"),
		Local:   true,
	}
	maxRequestsPerMinuteFlag = &cli.IntFlag{
		Name:    "max-requests-per-minute",
		Value:   100,
		Usage:   "the maximum requests per minute per api token",
		Sources: cli.EnvVars("MAX_REQUESTS_PER_MINUTE"),
		Local:   true,
	}
	maxRequestsPerMinuteUnauthenticatedFlag = &cli.IntFlag{
		Name:    "max-unauthenticated-requests-per-minute",
		Value:   20,
		Usage:   "the maximum requests per minute for unauthenticated api access",
		Sources: cli.EnvVars("MAX_UNAUTHENTICATED_REQUESTS_PER_MINUTE"),
		Local:   true,
	}
	ipamGrpcEndpointFlag = &cli.StringFlag{
		Name:    "ipam-grpc-endpoint",
		Value:   "http://ipam:9090",
		Usage:   "the ipam grpc server endpoint",
		Sources: cli.EnvVars("IPAM_GRPC_ENDPOINT"),
		Local:   true,
	}
	ensureProviderTenantFlag = &cli.StringFlag{
		Name:    "ensure-provider-tenant",
		Value:   "metal-stack",
		Usage:   "ensures a provider tenant on startup (used for bootstrapping and technical tokens). can be disabled by setting to empty string.",
		Sources: cli.EnvVars("ENSURE_PROVIDER_TENANT"),
		Local:   true,
	}
)

func main() {
	ctx := context.Background()
	app := &cli.Command{
		Name:  "metal-apiserver",
		Usage: "apiserver for metal-stack.io",
		Commands: []*cli.Command{
			newServeCmd(ctx),
			newTokenCmd(ctx),
			newDatastoreCmd(ctx),
		},
	}

	err := app.Run(ctx, os.Args)
	if err != nil {
		log.Fatalf("error in cli: %v", err)
	}
}

func createLogger(cmd *cli.Command) (*slog.Logger, error) {
	var lvlvar slog.LevelVar

	err := lvlvar.UnmarshalText([]byte(cmd.String(logLevelFlag.Name)))
	if err != nil {
		return nil, err
	}

	log := slog.New(
		slog.NewJSONHandler(
			os.Stdout,
			&slog.HandlerOptions{
				Level: lvlvar.Level(),
			},
		),
	)

	log.Info("created slog logger", "level", lvlvar.String())

	return log, nil
}
