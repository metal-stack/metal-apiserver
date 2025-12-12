package main

import (
	"log"
	"log/slog"
	"os"

	"github.com/urfave/cli/v2"
)

const (
	stageDEV  = "DEV"
	stagePROD = "PROD"
)

var (
	httpServerEndpointFlag = &cli.StringFlag{
		Name:  "http-server-endpoint",
		Value: "localhost:8081",
		Usage: "http server bind address",
	}
	metricServerEndpointFlag = &cli.StringFlag{
		Name:  "metric-server-endpoint",
		Value: "localhost:2112",
		Usage: "metric server endpoint",
	}
	serverHttpUrlFlag = &cli.StringFlag{
		Name:  "server-http-url",
		Value: "http://localhost:8081",
		Usage: "the url on which the http server is reachable from the outside",
	}
	sessionSecretFlag = &cli.StringFlag{
		Name:     "session-secret",
		Value:    "geheim",
		Usage:    "session secret encrypts the cookie, used by goth cookiestore",
		Required: true,
		EnvVars:  []string{"SESSION_SECRET"},
	}
	frontEndUrlFlag = &cli.StringFlag{
		Name:    "front-end-url",
		Value:   "https://metal-stack.io",
		Usage:   "URL of the frontend (metalctl)",
		EnvVars: []string{"FRONT_END_URL"},
	}
	oidcClientIdFlag = &cli.StringFlag{
		Name:     "oidc-client-id",
		Value:    "",
		Usage:    "id of the oauth app in oidc",
		Required: true,
		EnvVars:  []string{"OIDC_CLIENT_ID"},
	}
	oidcClientSecretFlag = &cli.StringFlag{
		Name:     "oidc-client-secret",
		Value:    "",
		Usage:    "client secret of the oauth app in oidc",
		Required: true,
		EnvVars:  []string{"OIDC_CLIENT_SECRET"},
	}
	oidcDiscoveryUrlFlag = &cli.StringFlag{
		Name:     "oidc-discovery-url",
		Value:    "",
		Usage:    "discovery url of the oauth app in oidc",
		Required: true,
		EnvVars:  []string{"OIDC_DISCOVERY_URL"},
	}
	oidcEndSessionUrlFlag = &cli.StringFlag{
		Name:     "oidc-end-session-url",
		Value:    "",
		Required: true,
		EnvVars:  []string{"OIDC_END_SESSION_URL"},
	}
	oidcUniqueUserKeyFlag = &cli.StringFlag{
		Name:    "oidc-unique-user-key",
		Value:   "email", // make sure the oidc provider has unique email addresses, otherwise use "sub"
		Usage:   "used to extract the unique user id from the oidc provider response raw data",
		EnvVars: []string{"OIDC_UNIQUE_USER_KEY"},
	}
	oidcTLSSkipVerifyFlag = &cli.BoolFlag{
		Name:    "oidc-tls-skip-verify",
		Value:   true,
		Usage:   "skip tls verification when talking to the oidc provider, set this to false in real production environments",
		EnvVars: []string{"OIDC_TLS_SKIP_VERIFY"},
	}
	logLevelFlag = &cli.StringFlag{
		Name:  "log-level",
		Value: "info",
		Usage: "log-level can be one of error|warn|info|debug",
	}
	masterdataApiHostnameFlag = &cli.StringFlag{
		Name:  "masterdata-api-hostname",
		Value: "localhost",
		Usage: "masterdata-api hostname",
	}
	masterdataApiPortFlag = &cli.UintFlag{
		Name:  "masterdata-api-port",
		Value: 50051,
		Usage: "masterdata-api port",
	}
	masterdataApiHmacFlag = &cli.StringFlag{
		Name:    "masterdata-api-hmac",
		Value:   "",
		Usage:   "masterdata-api-hmac",
		EnvVars: []string{"MASTERDATA_API_HMAC"},
	}
	masterdataApiCAPathFlag = &cli.StringFlag{
		Name:  "masterdata-api-ca-path",
		Value: "certs/ca.pem",
		Usage: "masterdata-api CA path",
	}
	masterdataApiCertPathFlag = &cli.StringFlag{
		Name:  "masterdata-api-cert-path",
		Value: "certs/client.pem",
		Usage: "masterdata-api certificate path",
	}
	masterdataApiCertKeyPathFlag = &cli.StringFlag{
		Name:  "masterdata-api-cert-key-path",
		Value: "certs/client-key.pem",
		Usage: "masterdata-api certificate key path",
	}
	rethinkdbAddressesFlag = &cli.StringSliceFlag{
		Name:     "rethinkdb-addresses",
		Value:    &cli.StringSlice{},
		Required: true,
		Usage:    "rethinkdb addresses without http prefix",
	}
	rethinkdbDBNameFlag = &cli.StringFlag{
		Name:  "rethinkdb-dbname",
		Value: "metalapi",
		Usage: "rethinkdb database name",
	}
	rethinkdbUserFlag = &cli.StringFlag{
		Name:  "rethinkdb-user",
		Value: "admin",
		Usage: "rethinkdb username to connect",
	}
	rethinkdbPasswordFlag = &cli.StringFlag{
		Name:     "rethinkdb-password",
		Value:    "",
		Required: true,
		Usage:    "rethinkdb password to connect",
	}
	auditingTimescaleEnabledFlag = &cli.BoolFlag{
		Name:  "auditing-timescaledb-enabled",
		Value: false,
		Usage: "enable timescaledb auditing",
	}
	auditingTimescaleHostFlag = &cli.StringFlag{
		Name:  "auditing-timescaledb-host",
		Value: "",
		Usage: "timescaledb auditing database host",
	}
	auditingTimescalePortFlag = &cli.StringFlag{
		Name:  "auditing-timescaledb-port",
		Value: "5432",
		Usage: "timescaledb auditing database port",
	}
	auditingTimescaleDbFlag = &cli.StringFlag{
		Name:  "auditing-timescaledb-db",
		Value: "auditing",
		Usage: "timescaledb auditing database",
	}
	auditingTimescaleUserFlag = &cli.StringFlag{
		Name:  "auditing-timescaledb-user",
		Value: "postgres",
		Usage: "timescaledb auditing database user",
	}
	auditingTimescalePasswordFlag = &cli.StringFlag{
		Name:  "auditing-timescaledb-password",
		Value: "",
		Usage: "timescaledb auditing database password",
	}
	auditingTimescaleRetentionFlag = &cli.StringFlag{
		Name:  "auditing-timescaledb-retention",
		Value: "14 days",
		Usage: "timescaledb auditing database retention",
	}
	stageFlag = &cli.StringFlag{
		Name:    "stage",
		Value:   stagePROD,
		Usage:   "deployment stage of application",
		EnvVars: []string{"STAGE"},
	}
	redisAddrFlag = &cli.StringFlag{
		Name:    "redis-addr",
		Value:   "",
		Usage:   "the address to a redis key value store",
		EnvVars: []string{"REDIS_ADDR"},
	}
	redisPasswordFlag = &cli.StringFlag{
		Name:    "redis-password",
		Value:   "",
		Usage:   "the password to the redis key value store",
		EnvVars: []string{"REDIS_PASSWORD"},
	}
	adminsFlag = &cli.StringSliceFlag{
		Name:  "admin-subjects",
		Value: cli.NewStringSlice("metal-stack-ops@github"),
		Usage: "the user subjects that are considered as administrators when creating api tokens to gain extended api access permissions",
	}
	maxRequestsPerMinuteFlag = &cli.IntFlag{
		Name:  "max-requests-per-minute",
		Value: 100,
		Usage: "the maximum requests per minute per api token",
	}
	maxRequestsPerMinuteUnauthenticatedFlag = &cli.IntFlag{
		Name:  "max-unauthenticated-requests-per-minute",
		Value: 20,
		Usage: "the maximum requests per minute for unauthenticated api access",
	}
	ipamGrpcEndpointFlag = &cli.StringFlag{
		Name:  "ipam-grpc-endpoind",
		Value: "http://ipam:9090",
		Usage: "the ipam grpc server endpoint",
	}
	ensureProviderTenantFlag = &cli.StringFlag{
		Name:  "ensure-provider-tenant",
		Value: "metal-stack",
		Usage: "ensures a provider tenant on startup (used for bootstrapping and technical tokens). can be disabled by setting to empty string.",
	}
)

func main() {
	app := &cli.App{
		Name:  "metal-apiserver",
		Usage: "apiserver for metal-stack.io",
		Commands: []*cli.Command{
			newServeCmd(),
			newTokenCmd(),
			newDatastoreCmd(),
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Fatalf("error in cli: %v", err)
	}
}

func createLogger(ctx *cli.Context) (*slog.Logger, error) {
	var lvlvar slog.LevelVar

	err := lvlvar.UnmarshalText([]byte(ctx.String(logLevelFlag.Name)))
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
