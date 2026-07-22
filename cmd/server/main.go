package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"os"
	"time"

	"github.com/urfave/cli/v3"
)

const (
	stageDEV        = "DEV"
	stagePROD       = "PROD"
	applicationName = "metal-apiserver"
)

var (
	httpServerEndpointFlag = &cli.StringFlag{
		Name:    "http-server-endpoint",
		Value:   "localhost:8081",
		Usage:   "http server bind address",
		Sources: cli.EnvVars("HTTP_SERVER_ENDPOINT"),
	}
	metricServerEndpointFlag = &cli.StringFlag{
		Name:    "metric-server-endpoint",
		Value:   "localhost:2112",
		Usage:   "metric server endpoint",
		Sources: cli.EnvVars("METRIC_SERVER_ENDPOINT"),
	}
	serverHttpUrlFlag = &cli.StringFlag{
		Name:    "server-http-url",
		Value:   "http://localhost:8081",
		Usage:   "the url on which the http server is reachable from the outside",
		Sources: cli.EnvVars("SERVER_HTTP_URL"),
	}
	redirectUrlsFlag = &cli.StringSliceFlag{
		Name:    "redirect-urls",
		Value:   []string{"http://localhost", "https://localhost", "http://127.0.0.1"},
		Usage:   "allowed redirect urls after login",
		Sources: cli.EnvVars("REDIRECT_URLS"),
	}
	sessionSecretFlag = &cli.StringFlag{
		Name:     "session-secret",
		Value:    "",
		Usage:    "session secret encrypts the cookie, used by goth cookiestore",
		Required: true,
		Sources:  cli.EnvVars("SESSION_SECRET"),
		Action: func(ctx context.Context, cmd *cli.Command, s string) error {
			if len(s) < 10 {
				return fmt.Errorf("session-secret must be at least 10 characters long")
			}
			return nil
		},
	}
	frontEndUrlFlag = &cli.StringFlag{
		Name:    "front-end-url",
		Value:   "https://metal-stack.io",
		Usage:   "URL of the frontend (metalctl)",
		Sources: cli.EnvVars("FRONT_END_URL"),
	}
	oidcClientIdFlag = &cli.StringFlag{
		Name:     "oidc-client-id",
		Value:    "",
		Usage:    "id of the oauth app in oidc",
		Required: true,
		Sources:  cli.EnvVars("OIDC_CLIENT_ID"),
		Action: func(ctx context.Context, cmd *cli.Command, s string) error {
			if len(s) < 2 {
				return fmt.Errorf("oidc-client-id must be at least 2 characters long")
			}
			return nil
		},
	}
	oidcClientSecretFlag = &cli.StringFlag{
		Name:     "oidc-client-secret",
		Value:    "",
		Usage:    "client secret of the oauth app in oidc",
		Required: true,
		Sources:  cli.EnvVars("OIDC_CLIENT_SECRET"),
		Action: func(ctx context.Context, cmd *cli.Command, s string) error {
			if len(s) < 2 {
				return fmt.Errorf("oidc-client-secret must be at least 2 characters long")
			}
			return nil
		},
	}
	oidcDiscoveryUrlFlag = &cli.StringFlag{
		Name:     "oidc-discovery-url",
		Value:    "",
		Usage:    "discovery url of the oauth app in oidc",
		Required: true,
		Sources:  cli.EnvVars("OIDC_DISCOVERY_URL"),
	}
	oidcEndSessionUrlFlag = &cli.StringFlag{
		Name:     "oidc-end-session-url",
		Value:    "",
		Required: true,
		Sources:  cli.EnvVars("OIDC_END_SESSION_URL"),
	}
	oidcUniqueUserKeyFlag = &cli.StringFlag{
		Name:    "oidc-unique-user-key",
		Value:   "email", // make sure the oidc provider has unique email addresses, otherwise use "sub"
		Usage:   "used to extract the unique user id from the oidc provider response raw data",
		Sources: cli.EnvVars("OIDC_UNIQUE_USER_KEY"),
	}
	oidcTLSSkipVerifyFlag = &cli.BoolFlag{
		Name:    "oidc-tls-skip-verify",
		Value:   true,
		Usage:   "skip tls verification when talking to the oidc provider, set this to false in real production environments",
		Sources: cli.EnvVars("OIDC_TLS_SKIP_VERIFY"),
	}
	logLevelFlag = &cli.StringFlag{
		Name:    "log-level",
		Value:   "info",
		Usage:   "log-level can be one of error|warn|info|debug",
		Sources: cli.EnvVars("LOG_LEVEL"),
	}
	tenantApiserverBaseURLFlag = &cli.StringFlag{
		Name:    "tenant-apiserver-baseurl",
		Value:   "http://tenant-apiserver:8080",
		Usage:   "tenant-apiserver base url",
		Sources: cli.EnvVars("TENANT_APISERVER_BASEURL"),
	}
	rethinkdbAddressesFlag = &cli.StringSliceFlag{
		Name:     "rethinkdb-addresses",
		Value:    []string{},
		Required: true,
		Usage:    "rethinkdb addresses without http prefix",
		Sources:  cli.EnvVars("RETHINKDB_ADDRESSES"),
	}
	rethinkdbDBNameFlag = &cli.StringFlag{
		Name:    "rethinkdb-dbname",
		Value:   "metalapi",
		Usage:   "rethinkdb database name",
		Sources: cli.EnvVars("RETHINKDB_DBNAME"),
	}
	rethinkdbUserFlag = &cli.StringFlag{
		Name:    "rethinkdb-user",
		Value:   "admin",
		Usage:   "rethinkdb username to connect",
		Sources: cli.EnvVars("RETHINKDB_USER"),
	}
	rethinkdbPasswordFlag = &cli.StringFlag{
		Name:     "rethinkdb-password",
		Value:    "",
		Required: true,
		Usage:    "rethinkdb password to connect",
		Sources:  cli.EnvVars("RETHINKDB_PASSWORD"),
	}
	asnPoolRangeMinFlag = &cli.UintFlag{
		Name:    "asnpool-range-min",
		Value:   1,
		Usage:   "asn pool range min, this can not be changed after initial deployment",
		Sources: cli.EnvVars("ASN_POOL_RANGE_MIN"),
	}
	asnPoolRangeMaxFlag = &cli.UintFlag{
		Name:    "asnpool-range-max",
		Value:   131072,
		Usage:   "asn pool range max, this can not be changed after initial deployment",
		Sources: cli.EnvVars("ASN_POOL_RANGE_MAX"),
	}
	vrfPoolRangeMinFlag = &cli.UintFlag{
		Name:    "vrfpool-range-min",
		Value:   1,
		Usage:   "vrf pool range min, this can not be changed after initial deployment",
		Sources: cli.EnvVars("VRF_POOL_RANGE_MIN"),
	}
	vrfPoolRangeMaxFlag = &cli.UintFlag{
		Name:    "vrfpool-range-max",
		Value:   131072,
		Usage:   "vrf pool range max, this can not be changed after initial deployment",
		Sources: cli.EnvVars("VRF_POOL_RANGE_MAX"),
	}

	auditingSearchBackendFlag = &cli.StringFlag{
		Name:    "auditing-search-backend",
		Value:   "",
		Usage:   "the audit backend used for searches, defaults to timescaledb",
		Sources: cli.EnvVars("AUDITING_SEARCH_BACKEND"),
	}

	auditingTimescaleEnabledFlag = &cli.BoolFlag{
		Name:    "auditing-timescaledb-enabled",
		Value:   false,
		Usage:   "enable timescaledb auditing",
		Sources: cli.EnvVars("AUDITING_TIMESCALEDB_ENABLED"),
	}
	auditingTimescaleHostFlag = &cli.StringFlag{
		Name:    "auditing-timescaledb-host",
		Value:   "",
		Usage:   "timescaledb auditing database host",
		Sources: cli.EnvVars("AUDITING_TIMESCALEDB_HOST"),
	}
	auditingTimescalePortFlag = &cli.StringFlag{
		Name:    "auditing-timescaledb-port",
		Value:   "5432",
		Usage:   "timescaledb auditing database port",
		Sources: cli.EnvVars("AUDITING_TIMESCALEDB_PORT"),
	}
	auditingTimescaleDbFlag = &cli.StringFlag{
		Name:    "auditing-timescaledb-db",
		Value:   "auditing",
		Usage:   "timescaledb auditing database",
		Sources: cli.EnvVars("AUDITING_TIMESCALEDB_DB"),
	}
	auditingTimescaleUserFlag = &cli.StringFlag{
		Name:    "auditing-timescaledb-user",
		Value:   "postgres",
		Usage:   "timescaledb auditing database user",
		Sources: cli.EnvVars("AUDITING_TIMESCALEDB_USER"),
	}
	auditingTimescalePasswordFlag = &cli.StringFlag{
		Name:    "auditing-timescaledb-password",
		Value:   "",
		Usage:   "timescaledb auditing database password",
		Sources: cli.EnvVars("AUDITING_TIMESCALEDB_PASSWORD"),
	}
	auditingTimescaleRetentionFlag = &cli.StringFlag{
		Name:    "auditing-timescaledb-retention",
		Value:   "14 days",
		Usage:   "timescaledb auditing database retention",
		Sources: cli.EnvVars("AUDITING_TIMESCALEDB_RETENTION"),
	}

	auditingSplunkEnabledFlag = &cli.BoolFlag{
		Name:    "auditing-splunk-enabled",
		Value:   false,
		Usage:   "enable splunk auditing",
		Sources: cli.EnvVars("AUDITING_SPLUNK_ENABLED"),
	}
	auditingSplunkEndpointFlag = &cli.StringFlag{
		Name:    "auditing-splunk-endpoint",
		Value:   "",
		Usage:   "splunk auditing endpoint",
		Sources: cli.EnvVars("AUDITING_SPLUNK_ENDPOINT"),
	}
	auditingSplunkHostFlag = &cli.StringFlag{
		Name:    "auditing-splunk-host",
		Value:   "",
		Usage:   "splunk auditing host",
		Sources: cli.EnvVars("AUDITING_SPLUNK_HOST"),
	}
	auditingSplunkSourceFlag = &cli.StringFlag{
		Name:    "auditing-splunk-source",
		Value:   "",
		Usage:   "splunk auditing source",
		Sources: cli.EnvVars("AUDITING_SPLUNK_SOURCE"),
	}
	auditingSplunkSourceTypeFlag = &cli.StringFlag{
		Name:    "auditing-splunk-source-type",
		Value:   "",
		Usage:   "splunk auditing source type",
		Sources: cli.EnvVars("AUDITING_SPLUNK_SOURCE_TYPE"),
	}
	auditingSplunkHecTokenFlag = &cli.StringFlag{
		Name:    "auditing-splunk-hec-token",
		Value:   "",
		Usage:   "splunk auditing hec token",
		Sources: cli.EnvVars("AUDITING_SPLUNK_HEC_TOKEN"),
	}
	auditingSplunkIndexFlag = &cli.StringFlag{
		Name:    "auditing-splunk-index",
		Value:   "",
		Usage:   "splunk auditing index",
		Sources: cli.EnvVars("AUDITING_SPLUNK_INDEX"),
	}
	auditingSplunkCaFlag = &cli.StringFlag{
		Name:    "auditing-splunk-ca",
		Value:   "",
		Usage:   "splunk auditing ca path",
		Sources: cli.EnvVars("AUDITING_SPLUNK_CA"),
	}

	stageFlag = &cli.StringFlag{
		Name:    "stage",
		Value:   stagePROD,
		Usage:   "deployment stage of application",
		Sources: cli.EnvVars("STAGE"),
	}
	redisAddrFlag = &cli.StringFlag{
		Name:    "redis-addr",
		Value:   "",
		Usage:   "the address to a redis key value store",
		Sources: cli.EnvVars("REDIS_ADDR"),
	}
	redisPasswordFlag = &cli.StringFlag{
		Name:    "redis-password",
		Value:   "",
		Usage:   "the password to the redis key value store",
		Sources: cli.EnvVars("REDIS_PASSWORD"),
	}
	providerTenantFlag = &cli.StringFlag{
		Name:  "provider-tenant",
		Value: "metal-stack",
		Usage: `provider tenant, other tenants which are made member with owner rights of this tenant can request admin-role-editor,
if they have editor or viewer rights, they can request admin-role-viewer.
Can not be changed after initial creation.
`,
		Sources: cli.EnvVars("PROVIDER_TENANT"),
		Action: func(ctx context.Context, cmd *cli.Command, s string) error {
			if len(s) < 2 {
				return fmt.Errorf("provider-tenant must be longer than 2 characters")
			}
			return nil
		},
	}
	maxRequestsPerMinuteFlag = &cli.IntFlag{
		Name:    "max-requests-per-minute",
		Value:   100,
		Usage:   "the maximum requests per minute per api token",
		Sources: cli.EnvVars("MAX_REQUESTS_PER_MINUTE"),
	}
	maxRequestsPerMinuteUnauthenticatedFlag = &cli.IntFlag{
		Name:    "max-unauthenticated-requests-per-minute",
		Value:   20,
		Usage:   "the maximum requests per minute for unauthenticated api access",
		Sources: cli.EnvVars("MAX_UNAUTHENTICATED_PER_MINUTE"),
	}
	ipamGrpcEndpointFlag = &cli.StringFlag{
		Name:    "ipam-grpc-endpoint",
		Value:   "http://ipam:9090",
		Usage:   "the ipam grpc server endpoint",
		Sources: cli.EnvVars("IPAM_GRPC_ENDPOINT"),
	}
	bmcSuperuserPasswordFlag = &cli.StringFlag{
		Name:    "bmc-superuser-pwd",
		Value:   "",
		Usage:   "the BMC superuser password",
		Sources: cli.EnvVars("BMC_SUPER_USER_PASSWORD"),
		Action: func(ctx context.Context, cmd *cli.Command, s string) error {
			if len(s) < 8 {
				return fmt.Errorf("bmc superuser password must be longer than 2 characters")
			}
			return nil
		},
	}
	// Headscale
	headscaleAddressFlag = &cli.StringFlag{
		Name:    "headscale-addr",
		Value:   "headscale:50443",
		Usage:   "address of headscale grpc server endpoint",
		Sources: cli.EnvVars("HEADSCALE_ADDRESS"),
	}
	headscaleControlplaneAddressFlag = &cli.StringFlag{
		Name:    "headscale-cp-addr",
		Value:   "",
		Usage:   "controlplane address of headscale server reachable from the nodes to join",
		Sources: cli.EnvVars("HEADSCALE_CONTROLPLANE_ADDRESS"),
	}
	headscaleApikeyFlag = &cli.StringFlag{
		Name:    "headscale-api-key",
		Value:   "",
		Usage:   "initial api key to connect to the headscale grpc server",
		Sources: cli.EnvVars("HEADSCALE_API_KEY"),
	}
	headscaleEnabledFlag = &cli.BoolFlag{
		Name:    "headscale-enabled",
		Value:   false,
		Usage:   "toggle if headscale should be enabled",
		Sources: cli.EnvVars("HEADSCALE_ENABLED"),
	}
	// End Headscale
	componentExpirationFlag = &cli.DurationFlag{
		Name:    "component-expiration",
		Value:   24 * time.Hour,
		Usage:   "duration after which inactive component entries are removed",
		Sources: cli.EnvVars("COMPONENT_EXPIRATION"),
	}
	secureCookieFlag = &cli.BoolFlag{
		Name:    "secure-cookie",
		Value:   true,
		Usage:   "enable secure cookie transport, should be disabled in mini-lab or test and dev environments where authentication endpoint is not https terminated",
		Sources: cli.EnvVars("SECURE_COOKIE"),
	}
)

func main() {
	app := &cli.Command{
		Name:  applicationName,
		Usage: "apiserver for metal-stack.io",
		Commands: []*cli.Command{
			newServeCmd(),
			newTokenCmd(),
			newDatastoreCmd(),
			newVPNCmd(),
		},
	}

	err := app.Run(context.Background(), os.Args)
	if err != nil {
		log.Fatalf("error in cli: %v", err)
	}
}

func createLogger(ctx *cli.Command) (*slog.Logger, error) {
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
	return log, nil
}
