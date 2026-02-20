package main

import (
	"log"
	"log/slog"
	"os"
	"time"

	"github.com/urfave/cli/v2"
)

const (
	stageDEV  = "DEV"
	stagePROD = "PROD"
)

var (
	httpServerEndpointFlag = &cli.StringFlag{
		Name:    "http-server-endpoint",
		Value:   "localhost:8081",
		Usage:   "http server bind address",
		EnvVars: []string{"HTTP_SERVER_ENDPOINT"},
	}
	metricServerEndpointFlag = &cli.StringFlag{
		Name:    "metric-server-endpoint",
		Value:   "localhost:2112",
		Usage:   "metric server endpoint",
		EnvVars: []string{"METRIC_SERVER_ENDPOINT"},
	}
	serverHttpUrlFlag = &cli.StringFlag{
		Name:    "server-http-url",
		Value:   "http://localhost:8081",
		Usage:   "the url on which the http server is reachable from the outside",
		EnvVars: []string{"SERVER_HTTP_URL"},
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
		Name:    "log-level",
		Value:   "info",
		Usage:   "log-level can be one of error|warn|info|debug",
		EnvVars: []string{"LOG_LEVEL"},
	}
	masterdataApiHostnameFlag = &cli.StringFlag{
		Name:    "masterdata-api-hostname",
		Value:   "localhost",
		Usage:   "masterdata-api hostname",
		EnvVars: []string{"MASTERDATA_API_HOSTNAME"},
	}
	masterdataApiPortFlag = &cli.UintFlag{
		Name:    "masterdata-api-port",
		Value:   50051,
		Usage:   "masterdata-api port",
		EnvVars: []string{"MASTERDATA_API_PORT"},
	}
	masterdataApiHmacFlag = &cli.StringFlag{
		Name:    "masterdata-api-hmac",
		Value:   "",
		Usage:   "masterdata-api-hmac",
		EnvVars: []string{"MASTERDATA_API_HMAC"},
	}
	masterdataApiCAPathFlag = &cli.StringFlag{
		Name:    "masterdata-api-ca-path",
		Value:   "certs/ca.pem",
		Usage:   "masterdata-api CA path",
		EnvVars: []string{"MASTERDATA_API_CA_PATH"},
	}
	masterdataApiCertPathFlag = &cli.StringFlag{
		Name:    "masterdata-api-cert-path",
		Value:   "certs/client.pem",
		Usage:   "masterdata-api certificate path",
		EnvVars: []string{"MASTERDATA_API_CERT_PATH"},
	}
	masterdataApiCertKeyPathFlag = &cli.StringFlag{
		Name:    "masterdata-api-cert-key-path",
		Value:   "certs/client-key.pem",
		Usage:   "masterdata-api certificate key path",
		EnvVars: []string{"MASTERDATA_API_CERT_KEY_PATH"},
	}
	rethinkdbAddressesFlag = &cli.StringSliceFlag{
		Name:     "rethinkdb-addresses",
		Value:    &cli.StringSlice{},
		Required: true,
		Usage:    "rethinkdb addresses without http prefix",
		EnvVars:  []string{"RETHINKDB_ADDRESSES"},
	}
	rethinkdbDBNameFlag = &cli.StringFlag{
		Name:    "rethinkdb-dbname",
		Value:   "metalapi",
		Usage:   "rethinkdb database name",
		EnvVars: []string{"RETHINKDB_DBNAME"},
	}
	rethinkdbUserFlag = &cli.StringFlag{
		Name:    "rethinkdb-user",
		Value:   "admin",
		Usage:   "rethinkdb username to connect",
		EnvVars: []string{"RETHINKDB_USER"},
	}
	rethinkdbPasswordFlag = &cli.StringFlag{
		Name:     "rethinkdb-password",
		Value:    "",
		Required: true,
		Usage:    "rethinkdb password to connect",
		EnvVars:  []string{"RETHINKDB_PASSWORD"},
	}
	asnPoolRangeMinFlag = &cli.UintFlag{
		Name:    "asnpool-range-min",
		Value:   1,
		Usage:   "asn pool range min, this can not be changed after initial deployment",
		EnvVars: []string{"ASN_POOL_RANGE_MIN"},
	}
	asnPoolRangeMaxFlag = &cli.UintFlag{
		Name:    "asnpool-range-max",
		Value:   131072,
		Usage:   "asn pool range max, this can not be changed after initial deployment",
		EnvVars: []string{"ASN_POOL_RANGE_MAX"},
	}
	vrfPoolRangeMinFlag = &cli.UintFlag{
		Name:    "vrfpool-range-min",
		Value:   1,
		Usage:   "vrf pool range min, this can not be changed after initial deployment",
		EnvVars: []string{"VRF_POOL_RANGE_MIN"},
	}
	vrfPoolRangeMaxFlag = &cli.UintFlag{
		Name:    "vrfpool-range-max",
		Value:   131072,
		Usage:   "vrf pool range max, this can not be changed after initial deployment",
		EnvVars: []string{"VRF_POOL_RANGE_MAX"},
	}
	auditingTimescaleEnabledFlag = &cli.BoolFlag{
		Name:    "auditing-timescaledb-enabled",
		Value:   false,
		Usage:   "enable timescaledb auditing",
		EnvVars: []string{"AUDITING_TIMESCALEDB_ENABLED"},
	}
	auditingTimescaleHostFlag = &cli.StringFlag{
		Name:    "auditing-timescaledb-host",
		Value:   "",
		Usage:   "timescaledb auditing database host",
		EnvVars: []string{"AUDITING_TIMESCALEDB_HOST"},
	}
	auditingTimescalePortFlag = &cli.StringFlag{
		Name:    "auditing-timescaledb-port",
		Value:   "5432",
		Usage:   "timescaledb auditing database port",
		EnvVars: []string{"AUDITING_TIMESCALEDB_PORT"},
	}
	auditingTimescaleDbFlag = &cli.StringFlag{
		Name:    "auditing-timescaledb-db",
		Value:   "auditing",
		Usage:   "timescaledb auditing database",
		EnvVars: []string{"AUDITING_TIMESCALEDB_DB"},
	}
	auditingTimescaleUserFlag = &cli.StringFlag{
		Name:    "auditing-timescaledb-user",
		Value:   "postgres",
		Usage:   "timescaledb auditing database user",
		EnvVars: []string{"AUDITING_TIMESCALEDB_USER"},
	}
	auditingTimescalePasswordFlag = &cli.StringFlag{
		Name:    "auditing-timescaledb-password",
		Value:   "",
		Usage:   "timescaledb auditing database password",
		EnvVars: []string{"AUDITING_TIMESCALEDB_PASSWORD"},
	}
	auditingTimescaleRetentionFlag = &cli.StringFlag{
		Name:    "auditing-timescaledb-retention",
		Value:   "14 days",
		Usage:   "timescaledb auditing database retention",
		EnvVars: []string{"AUDITING_TIMESCALEDB_RETENTION"},
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
		Name:    "admin-subjects",
		Value:   cli.NewStringSlice("metal-stack-ops@github"),
		Usage:   "the user subjects that are considered as administrators when creating api tokens to gain extended api access permissions",
		EnvVars: []string{"ADMIN_SUBJECTS"},
	}
	maxRequestsPerMinuteFlag = &cli.IntFlag{
		Name:    "max-requests-per-minute",
		Value:   100,
		Usage:   "the maximum requests per minute per api token",
		EnvVars: []string{"MAX_REQUESTS_PER_MINUTE"},
	}
	maxRequestsPerMinuteUnauthenticatedFlag = &cli.IntFlag{
		Name:    "max-unauthenticated-requests-per-minute",
		Value:   20,
		Usage:   "the maximum requests per minute for unauthenticated api access",
		EnvVars: []string{"MAX_UNAUTHENTICATED_PER_MINUTE"},
	}
	ipamGrpcEndpointFlag = &cli.StringFlag{
		Name:    "ipam-grpc-endpoint",
		Value:   "http://ipam:9090",
		Usage:   "the ipam grpc server endpoint",
		EnvVars: []string{"IPAM_GRPC_ENDPOINT"},
	}
	ensureProviderTenantFlag = &cli.StringFlag{
		Name:    "ensure-provider-tenant",
		Value:   "metal-stack",
		Usage:   "ensures a provider tenant on startup (used for bootstrapping and technical tokens). can be disabled by setting to empty string.",
		EnvVars: []string{"ENSURE_PROVIDER_TENANT"},
	}
	bmcSuperuserPasswordFlag = &cli.StringFlag{
		Name:    "bmc-superuser-pwd",
		Value:   "",
		Usage:   "the BMC superuser password",
		EnvVars: []string{"BMC_SUPER_USER_PASSWORD"},
	}
	// Headscale
	headscaleAddressFlag = &cli.StringFlag{
		Name:    "headscale-addr",
		Value:   "headscale:50443",
		Usage:   "address of headscale grpc server endpoint",
		EnvVars: []string{"HEADSCALE_ADDRESS"},
	}
	headscaleControlplaneAddressFlag = &cli.StringFlag{
		Name:    "headscale-cp-addr",
		Value:   "",
		Usage:   "controlplane address of headscale server reachable from the nodes to join",
		EnvVars: []string{"HEADSCALE_CONTROLPLANE_ADDRESS"},
	}
	headscaleApikeyFlag = &cli.StringFlag{
		Name:    "headscale-api-key",
		Value:   "",
		Usage:   "initial api key to connect to the headscale grpc server",
		EnvVars: []string{"HEADSCALE_API_KEY"},
	}
	headscaleEnabledFlag = &cli.BoolFlag{
		Name:    "headscale-enabled",
		Value:   false,
		Usage:   "toggle if headscale should be enabled",
		EnvVars: []string{"HEADSCALE_ENABLED"},
	}
	// End Headscale
	componentExpirationFlag = &cli.DurationFlag{
		Name:    "component-expiration",
		Value:   24 * time.Hour,
		Usage:   "duration after which inactive component entries are removed",
		EnvVars: []string{"COMPONENT_EXPIRATION"},
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
			newVPNCmd(),
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
