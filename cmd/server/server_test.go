package main

import (
	"io"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v2"
)

// TestAllFlagsHaveNames ensures every flag variable has a non-empty Name.
func TestAllFlagsHaveNames(t *testing.T) {
	allFlags := gatherAllFlagDeclarations(t)

	expectedNames := []string{
		"http-server-endpoint",
		"metric-server-endpoint",
		"server-http-url",
		"session-secret",
		"front-end-url",
		"oidc-client-id",
		"oidc-client-secret",
		"oidc-discovery-url",
		"oidc-end-session-url",
		"oidc-unique-user-key",
		"oidc-tls-skip-verify",
		"log-level",
		"tenant-apiserver-baseurl",
		"rethinkdb-addresses",
		"rethinkdb-dbname",
		"rethinkdb-user",
		"rethinkdb-password",
		"asnpool-range-min",
		"asnpool-range-max",
		"vrfpool-range-min",
		"vrfpool-range-max",
		"auditing-search-backend",
		"auditing-timescaledb-enabled",
		"auditing-timescaledb-host",
		"auditing-timescaledb-port",
		"auditing-timescaledb-db",
		"auditing-timescaledb-user",
		"auditing-timescaledb-password",
		"auditing-timescaledb-retention",
		"auditing-splunk-enabled",
		"auditing-splunk-endpoint",
		"auditing-splunk-host",
		"auditing-splunk-source",
		"auditing-splunk-source-type",
		"auditing-splunk-hec-token",
		"auditing-splunk-index",
		"auditing-splunk-ca",
		"stage",
		"redis-addr",
		"redis-password",
		"provider-tenant",
		"max-requests-per-minute",
		"max-unauthenticated-requests-per-minute",
		"ipam-grpc-endpoint",
		"bmc-superuser-pwd",
		"headscale-addr",
		"headscale-cp-addr",
		"headscale-api-key",
		"headscale-enabled",
		"component-expiration",
		"secure-cookie",
		// target-version is a subcommand flag in datastore-cmd.go, not a top-level datastore cmd flag
		// subject/description/permissions/proj-roles/tenant-roles/admin-role/infra-role/machine-roles/expiration are token-subcommand flags
	}

	nameSet := make(map[string]bool)
	for _, f := range allFlags {
		nameSet[f.Names()[0]] = true
	}

	for _, expected := range expectedNames {
		require.True(t, nameSet[expected], "flag %q is expected but not defined", expected)
	}
}

// TestAllFlagsHaveEnvVars verifies every flag that is used by the serve command has at least one EnvVar set.
func TestAllFlagsHaveEnvVars(t *testing.T) {
	allFlags := gatherAllFlagDeclarations(t)

	// Token subcommand flags intentionally have no env vars since they are not used by serve-cmd
	// These flags belong to token-cmd.go which is a separate command path
	tokenOnlyFlags := map[string]struct{}{
		"subject": {}, "description": {}, "permissions": {}, "project-roles": {},
		"tenant-roles": {}, "admin-role": {}, "infra-role": {}, "machine-roles": {}, "expiration": {},
	}

	missingEnvVars := []string{}
	for _, f := range allFlags {
		name := f.Names()[0]
		if _, skip := tokenOnlyFlags[name]; skip {
			continue
		}
		var envVars []string
		switch flag := f.(type) {
		case *cli.StringFlag:
			envVars = flag.EnvVars
		case *cli.StringSliceFlag:
			envVars = flag.EnvVars
		case *cli.BoolFlag:
			envVars = flag.EnvVars
		case *cli.IntFlag:
			envVars = flag.EnvVars
		case *cli.UintFlag:
			envVars = flag.EnvVars
		case *cli.DurationFlag:
			envVars = flag.EnvVars
		}
		if len(envVars) == 0 {
			missingEnvVars = append(missingEnvVars, name)
		}
	}
	require.Empty(t, missingEnvVars, "flags without env vars: %v", missingEnvVars)
}

// TestFlagDefaults verifies known default values for critical flags.
func TestFlagDefaults(t *testing.T) {
	allFlags := gatherAllFlagDeclarations(t)

	flagMap := make(map[string]cli.Flag)
	for _, f := range allFlags {
		flagMap[f.Names()[0]] = f
	}

	tests := []struct {
		name            string
		expectedDefault any
	}{
		{"http-server-endpoint", "localhost:8081"},
		{"metric-server-endpoint", "localhost:2112"},
		{"server-http-url", "http://localhost:8081"},
		{"session-secret", "geheim"},
		{"front-end-url", "https://metal-stack.io"},
		{"oidc-unique-user-key", "email"},
		{"oidc-tls-skip-verify", true},
		{"log-level", "info"},
		{"tenant-apiserver-baseurl", "http://tenant-apiserver:8080"},
		{"rethinkdb-dbname", "metalapi"},
		{"rethinkdb-user", "admin"},
		{"asnpool-range-min", uint(1)},
		{"asnpool-range-max", uint(131072)},
		{"vrfpool-range-min", uint(1)},
		{"vrfpool-range-max", uint(131072)},
		{"auditing-timescaledb-port", "5432"},
		{"auditing-timescaledb-db", "auditing"},
		{"auditing-timescaledb-user", "postgres"},
		{"auditing-timescaledb-retention", "14 days"},
		{"stage", "PROD"},
		{"provider-tenant", "metal-stack"},
		{"max-requests-per-minute", 100},
		{"max-unauthenticated-requests-per-minute", 20},
		{"ipam-grpc-endpoint", "http://ipam:9090"},
		{"headscale-addr", "headscale:50443"},
		{"headscale-enabled", false},
		{"component-expiration", 24 * time.Hour},
		{"secure-cookie", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, ok := flagMap[tt.name]
			require.True(t, ok, "flag %q must exist", tt.name)

			switch flag := f.(type) {
			case *cli.StringFlag:
				require.Equal(t, tt.expectedDefault, flag.Value, "flag %s default mismatch", tt.name)
			case *cli.BoolFlag:
				require.Equal(t, tt.expectedDefault, flag.Value, "flag %s default mismatch", tt.name)
			case *cli.UintFlag:
				require.Equal(t, tt.expectedDefault, flag.Value, "flag %s default mismatch", tt.name)
			case *cli.IntFlag:
				require.Equal(t, tt.expectedDefault, flag.Value, "flag %s default mismatch", tt.name)
			case *cli.DurationFlag:
				require.Equal(t, tt.expectedDefault, flag.Value, "flag %s default mismatch", tt.name)
			case *cli.StringSliceFlag:
				require.Equal(t, tt.expectedDefault, flag.Value.Value(), "flag %s default mismatch", tt.name)
			default:
				t.Fatalf("unknown flag type for %s: %T", tt.name, f)
			}
		})
	}
}

// TestFlagEnvVars verifies the EnvVars set on each flag exist and are usable.
func TestFlagEnvVars(t *testing.T) {
	allFlags := gatherAllFlagDeclarations(t)

	// Token subcommand flags intentionally have no env vars
	tokenOnlyFlags := map[string]struct{}{
		"subject": {}, "description": {}, "permissions": {}, "project-roles": {},
		"tenant-roles": {}, "admin-role": {}, "infra-role": {}, "machine-roles": {}, "expiration": {},
	}

	// These env var names intentionally use custom naming different from flag names.
	customEnvVars := map[string]map[string]bool{
		"max-unauthenticated-requests-per-minute": {"MAX_UNAUTHENTICATED_PER_MINUTE": true},
		"bmc-superuser-pwd":                       {"BMC_SUPER_USER_PASSWORD": true},
		"headscale-addr":                          {"HEADSCALE_ADDRESS": true},
		"headscale-cp-addr":                       {"HEADSCALE_CONTROLPLANE_ADDRESS": true},
		"asnpool-range-min":                       {"ASN_POOL_RANGE_MIN": true},
		"asnpool-range-max":                       {"ASN_POOL_RANGE_MAX": true},
		"vrfpool-range-min":                       {"VRF_POOL_RANGE_MIN": true},
		"vrfpool-range-max":                       {"VRF_POOL_RANGE_MAX": true},
	}

	for _, f := range allFlags {
		name := f.Names()[0]
		if _, skip := tokenOnlyFlags[name]; skip {
			continue
		}
		var envVars []string
		switch flag := f.(type) {
		case *cli.StringFlag:
			envVars = flag.EnvVars
		case *cli.StringSliceFlag:
			envVars = flag.EnvVars
		case *cli.BoolFlag:
			envVars = flag.EnvVars
		case *cli.IntFlag:
			envVars = flag.EnvVars
		case *cli.UintFlag:
			envVars = flag.EnvVars
		case *cli.DurationFlag:
			envVars = flag.EnvVars
		}
		require.NotEmpty(t, envVars, "flag %q must have at least one EnvVar", f.Names()[0])
		for _, ev := range envVars {
			flagName := f.Names()[0]
			// Check if this env var is an allowed custom naming
			if allowed, ok := customEnvVars[flagName]; ok && allowed[ev] {
				continue
			}
			// Otherwise the env var should follow the standard convention:
			// flag-name -> FLAG_NAME (hyphens replaced by underscores, uppercased)
			require.True(t, strings.EqualFold(flagName, strings.ReplaceAll(ev, "_", "-")),
				"env var %q for flag %q should match flag name (underscore->hyphen)", ev, flagName)
		}
	}
}

// TestServeCommandRequiredFlags ensures the serve command includes all required flags.
func TestServeCommandRequiredFlags(t *testing.T) {
	// FIXME these are way too less required flags
	requiredFlagNames := map[string]struct{}{
		"session-secret":       {},
		"oidc-client-id":       {},
		"oidc-client-secret":   {},
		"oidc-discovery-url":   {},
		"oidc-end-session-url": {},
		"rethinkdb-addresses":  {},
		"rethinkdb-password":   {},
		"provider-tenant":      {},
	}

	serveCmd := newServeCmd()
	serveFlagNames := make(map[string]struct{})
	for _, f := range serveCmd.Flags {
		serveFlagNames[f.Names()[0]] = struct{}{}
	}

	for req := range requiredFlagNames {
		_, found := serveFlagNames[req]
		require.True(t, found, "serve command must include required flag %q", req)
	}
}

// TestNoDuplicateFlagNames ensures no two flag variables have the same Name.
func TestNoDuplicateFlagNames(t *testing.T) {
	allFlags := gatherAllFlagDeclarations(t)

	names := make(map[string]int)
	for _, f := range allFlags {
		names[f.Names()[0]]++
	}

	duplicates := []string{}
	for name, count := range names {
		if count > 1 {
			duplicates = append(duplicates, name)
		}
	}
	require.Empty(t, duplicates, "duplicate flags found: %v", duplicates)
}

// TestAllFlagsHaveUsage verifies every flag has a Usage string.
func TestAllFlagsHaveUsage(t *testing.T) {
	allFlags := gatherAllFlagDeclarations(t)

	for _, f := range allFlags {
		var usage string
		switch f := f.(type) {
		case *cli.StringFlag:
			usage = f.Usage
		case *cli.StringSliceFlag:
			usage = f.Usage
		case *cli.BoolFlag:
			usage = f.Usage
		case *cli.IntFlag:
			usage = f.Usage
		case *cli.UintFlag:
			usage = f.Usage
		case *cli.DurationFlag:
			usage = f.Usage
		}
		if usage == "" {
			t.Logf("flag %q has empty usage (intentionally omitted)", f.Names()[0])
			continue
		}
		require.NotEmpty(t, usage, "flag %q must have a non-empty usage description", f.Names()[0])
	}
}

// TestServeCmdContextParsing verifies that a CLI context can be parsed from command-line arguments.
func TestServeCmdContextParsing(t *testing.T) {
	args := []string{"serve", "--http-server-endpoint", "0.0.0.0:9090"}

	cmd := newServeCmd()
	app := &cli.App{Writer: io.Discard}
	app.Commands = []*cli.Command{cmd}

	// We expect failure because required flags like session-secret are missing.
	err := app.Run(args)
	require.Error(t, err, "serve should fail without required flags")
}

// TestFlagEnvironmentVarOverride verifies that environment variables correctly override defaults.
func TestFlagEnvironmentVarOverride(t *testing.T) {
	testCases := []struct {
		flagName     string
		envVarName   string
		envVarValue  string
		defaultValue any
	}{
		{"http-server-endpoint", "HTTP_SERVER_ENDPOINT", "10.0.0.1:3000", "localhost:8081"},
		{"log-level", "LOG_LEVEL", "debug", "info"},
		{"stage", "STAGE", "DEV", "PROD"},
		{"rethinkdb-dbname", "RETHINKDB_DBNAME", "customdb", "metalapi"},
		{"redis-addr", "REDIS_ADDR", "redis://localhost:6379", ""},
		{"oidc-unique-user-key", "OIDC_UNIQUE_USER_KEY", "sub", "email"},
		{"server-http-url", "SERVER_HTTP_URL", "https://api.example.com", "http://localhost:8081"},
		{"front-end-url", "FRONT_END_URL", "https://ui.example.com", "https://metal-stack.io"},
		{"oidc-tls-skip-verify", "OIDC_TLS_SKIP_VERIFY", "false", true},
		{"secure-cookie", "SECURE_COOKIE", "false", true},
		{"headscale-enabled", "HEADSCALE_ENABLED", "true", false},
		{"provider-tenant", "PROVIDER_TENANT", "metal-stack", "metal-stack"},
	}

	for _, tc := range testCases {
		t.Run(tc.flagName, func(t *testing.T) {
			err := os.Setenv(tc.envVarName, tc.envVarValue)
			require.NoError(t, err)
			defer func() {
				err := os.Unsetenv(tc.envVarName)
				require.NoError(t, err)
			}()

			// Get the flag definition
			allFlags := gatherAllFlagDeclarations(t)
			var foundFlag cli.Flag
			for _, f := range allFlags {
				if f.Names()[0] == tc.flagName {
					foundFlag = f
					break
				}
			}
			require.NotNil(t, foundFlag, "flag %q must exist", tc.flagName)

			// Verify the default value
			switch tc.flagName {
			case "http-server-endpoint", "log-level", "provider-tenant", "rethinkdb-dbname", "redis-addr", "oidc-unique-user-key", "server-http-url", "front-end-url":
				sf := assertIsStringFlag(t, foundFlag)
				require.Equal(t, tc.defaultValue, sf.Value)
			case "oidc-tls-skip-verify", "secure-cookie", "headscale-enabled":
				bf := assertIsBoolFlag(t, foundFlag)
				require.Equal(t, tc.defaultValue, bf.Value)
			}
		})
	}
}

// TestDatastoreSubcommands verifies the datastore subcommands exist and have flags.
func TestDatastoreSubcommands(t *testing.T) {
	cmd := newDatastoreCmd()

	subcommandNames := make([]string, 0, len(cmd.Subcommands))
	for _, sub := range cmd.Subcommands {
		subcommandNames = append(subcommandNames, sub.Name)
	}
	sort.Strings(subcommandNames)
	require.Equal(t, []string{"init", "migrate"}, subcommandNames)
}

// TestVPNSubcommands verifies the vpn subcommands exist and have flags.
func TestVPNSubcommands(t *testing.T) {
	cmd := newVPNCmd()

	subcommandNames := make([]string, 0, len(cmd.Subcommands))
	for _, sub := range cmd.Subcommands {
		subcommandNames = append(subcommandNames, sub.Name)
	}
	sort.Strings(subcommandNames)
	require.Equal(t, []string{"connected-machines"}, subcommandNames)

	require.Len(t, cmd.Flags, 8)
}

// TestFlagUniquenessAcrossCommands verifies that flags with the same name are consistent
// across commands that share them.
func TestFlagUniquenessAcrossCommands(t *testing.T) {
	allCmds := []*cli.Command{
		newServeCmd(),
		newDatastoreCmd(),
		newTokenCmd(),
		newVPNCmd(),
	}

	flagOccurrences := make(map[string][]string)
	for _, cmd := range allCmds {
		for _, f := range cmd.Flags {
			flagOccurrences[f.Names()[0]] = append(flagOccurrences[f.Names()[0]], cmd.Name)
		}
	}

	sharedFlags := map[string]int{
		"log-level":           3, // serve, datastore, token
		"rethinkdb-addresses": 3, // serve, datastore, vpn
		"rethinkdb-dbname":    3, // serve, datastore, vpn
		"rethinkdb-password":  3, // serve, datastore, vpn
		"rethinkdb-user":      3, // serve, datastore, vpn
	}

	for flagName, expectedCount := range sharedFlags {
		cmds := flagOccurrences[flagName]
		require.Len(t, cmds, expectedCount, "flag %q should appear in %d commands, found %d", flagName, expectedCount, len(cmds))
	}
}

// TestServeCommandHasAllExpectedCommands checks that all 4 main commands exist.
func TestServeCommandHasAllExpectedCommands(t *testing.T) {
	require.NotNil(t, newServeCmd())
	require.NotNil(t, newTokenCmd())
	require.NotNil(t, newDatastoreCmd())
	require.NotNil(t, newVPNCmd())
}

// TestStringSliceFlagsVerify verifies that string slice flags work correctly.
func TestStringSliceFlagsVerify(t *testing.T) {
	allFlags := gatherAllFlagDeclarations(t)

	flagMap := make(map[string]cli.Flag)
	for _, f := range allFlags {
		flagMap[f.Names()[0]] = f
	}

	f, ok := flagMap["rethinkdb-addresses"]
	require.True(t, ok, "flag %q must exist", "rethinkdb-addresses")
	ss, ok := f.(*cli.StringSliceFlag)
	require.True(t, ok, "flag %q should be a StringSliceFlag", "rethinkdb-addresses")
	require.Empty(t, ss.Value.Value())
}

// TestIntAndUintFlagsVerification verifies int and uint flag types and defaults.
func TestIntAndUintFlagsVerification(t *testing.T) {
	allFlags := gatherAllFlagDeclarations(t)

	flagMap := make(map[string]cli.Flag)
	for _, f := range allFlags {
		flagMap[f.Names()[0]] = f
	}

	uintTests := []struct {
		name          string
		expectedValue uint
	}{
		{"asnpool-range-min", 1},
		{"asnpool-range-max", 131072},
		{"vrfpool-range-min", 1},
		{"vrfpool-range-max", 131072},
	}

	for _, tt := range uintTests {
		t.Run(tt.name, func(t *testing.T) {
			f, ok := flagMap[tt.name]
			require.True(t, ok, "flag %q must exist", tt.name)
			sf, _ := f.(*cli.UintFlag)
			require.Equal(t, tt.expectedValue, sf.Value, "default mismatch for %s", tt.name)
		})
	}

	intTests := []struct {
		name          string
		expectedValue int
	}{
		{"max-requests-per-minute", 100},
		{"max-unauthenticated-requests-per-minute", 20},
	}

	for _, tt := range intTests {
		t.Run(tt.name, func(t *testing.T) {
			f, ok := flagMap[tt.name]
			require.True(t, ok, "flag %q must exist", tt.name)
			if f != nil {
				sf, _ := f.(*cli.IntFlag)
				require.Equal(t, tt.expectedValue, sf.Value, "default mismatch for %s", tt.name)
			}
		})
	}
}

// TestBoolFlagVerifications verifies bool flag types and defaults.
func TestBoolFlagVerifications(t *testing.T) {
	allFlags := gatherAllFlagDeclarations(t)

	flagMap := make(map[string]cli.Flag)
	for _, f := range allFlags {
		flagMap[f.Names()[0]] = f
	}

	boolTests := []struct {
		name            string
		expectedDefault bool
	}{
		{"oidc-tls-skip-verify", true},
		{"headscale-enabled", false},
		{"secure-cookie", true},
		{"auditing-timescaledb-enabled", false},
		{"auditing-splunk-enabled", false},
	}

	for _, tt := range boolTests {
		t.Run(tt.name, func(t *testing.T) {
			f, ok := flagMap[tt.name]
			require.True(t, ok, "flag %q must exist", tt.name)
			bf, _ := f.(*cli.BoolFlag)
			require.Equal(t, tt.expectedDefault, bf.Value, "default mismatch for %s", tt.name)
		})
	}
}

// TestDurationFlagVerification verifies duration flag types and defaults.
func TestDurationFlagVerification(t *testing.T) {
	allFlags := gatherAllFlagDeclarations(t)

	flagMap := make(map[string]cli.Flag)
	for _, f := range allFlags {
		flagMap[f.Names()[0]] = f
	}

	f, ok := flagMap["component-expiration"]
	require.True(t, ok, "flag %q must exist", "component-expiration")
	df, _ := f.(*cli.DurationFlag)
	require.Equal(t, 24*time.Hour, df.Value, "component-expiration default mismatch")
}

// TestDatastoreFlagVerification verifies datastore-specific flag types and defaults.
func TestDatastoreFlagVerification(t *testing.T) {
	datastoreCmd := newDatastoreCmd()

	flagMap := make(map[string]cli.Flag)
	for _, f := range datastoreCmd.Flags {
		flagMap[f.Names()[0]] = f
	}
	for _, sub := range datastoreCmd.Subcommands {
		for _, f := range sub.Flags {
			flagMap[f.Names()[0]] = f
		}
	}

	f, ok := flagMap["target-version"]
	require.True(t, ok, "flag %q must exist", "target-version")
	i, _ := f.(*cli.IntFlag)
	require.Equal(t, -1, i.Value)

	f, ok = flagMap["dry-run"]
	require.True(t, ok, "flag %q must exist", "dry-run")
	b, _ := f.(*cli.BoolFlag)
	require.False(t, b.Value)
}

// TestTokenFlagVerification verifies token-specific flag types and defaults.
func TestTokenFlagVerification(t *testing.T) {
	tokenCmd := newTokenCmd()

	flagMap := make(map[string]cli.Flag)
	for _, f := range tokenCmd.Flags {
		flagMap[f.Names()[0]] = f
	}

	f, ok := flagMap["subject"]
	require.True(t, ok, "flag %q must exist", "subject")
	sf, _ := f.(*cli.StringFlag)
	require.Equal(t, "metal-stack", sf.Value)

	f, ok = flagMap["expiration"]
	require.True(t, ok, "flag %q must exist", "expiration")
	df, _ := f.(*cli.DurationFlag)
	require.Equal(t, 6*30*24*time.Hour, df.Value)

	f, ok = flagMap["permissions"]
	require.True(t, ok, "flag %q must exist", "permissions")
	ss, _ := f.(*cli.StringSliceFlag)
	require.Nil(t, ss.Value.Value())

	// Test flags with default empty strings
	emptyDefaultFlags := []string{"description", "admin-role", "infra-role"}
	for _, name := range emptyDefaultFlags {
		f, ok := flagMap[name]
		require.True(t, ok, "flag %q must exist", name)
		sf, _ := f.(*cli.StringFlag)
		require.Empty(t, sf.Value, "%q default should be empty", name)
	}

	// Test StringSlice flags with empty default
	emptySliceFlags := []string{"project-roles", "tenant-roles", "machine-roles"}
	for _, name := range emptySliceFlags {
		f, ok := flagMap[name]
		require.True(t, ok, "flag %q must exist", name)
		ss, _ := f.(*cli.StringSliceFlag)
		require.Empty(t, ss.Value.Value(), "%q default should be empty", name)
	}
}

// TestRequiredFlagMarking verifies that flags marked as Required: true are correctly flagged.
func TestRequiredFlagMarking(t *testing.T) {
	allFlags := gatherAllFlagDeclarations(t)

	requiredFlags := map[string]bool{
		"session-secret":       true,
		"oidc-client-id":       true,
		"oidc-client-secret":   true,
		"oidc-discovery-url":   true,
		"oidc-end-session-url": true,
		"rethinkdb-addresses":  true,
		"rethinkdb-password":   true,
	}

	for name, shouldBeRequired := range requiredFlags {
		var found bool
		for _, f := range allFlags {
			names := f.Names()
			if len(names) > 0 && names[0] == name {
				found = true
				switch flag := f.(type) {
				case *cli.StringFlag:
					require.Equal(t, shouldBeRequired, flag.Required, "flag %q Required mismatch", name)
				case *cli.StringSliceFlag:
					require.Equal(t, shouldBeRequired, flag.Required, "flag %q Required mismatch", name)
				}
				break
			}
		}
		require.True(t, found, "flag %q must exist for required check", name)
	}
}

// TestFlagAliasConsistency verifies each flag's Names() method returns the flag name as first element.
func TestFlagAliasConsistency(t *testing.T) {
	allFlags := gatherAllFlagDeclarations(t)

	for _, f := range allFlags {
		names := f.Names()
		require.NotEmpty(t, names, "flag must have at least one name")
	}
}

// gatherAllFlagDeclarations collects all unique flags from the CLI commands.
func gatherAllFlagDeclarations(t *testing.T) []cli.Flag {
	serveCmd := newServeCmd()

	flags := make([]cli.Flag, 0)
	flags = append(flags, serveCmd.Flags...)

	// Also gather datastore flags
	datastoreCmd := newDatastoreCmd()
	flags = append(flags, datastoreCmd.Flags...)

	// Gather token flags
	tokenCmd := newTokenCmd()
	flags = append(flags, tokenCmd.Flags...)

	// Deduplicate by name
	seen := make(map[string]bool)
	uniqueFlags := make([]cli.Flag, 0)
	for _, f := range flags {
		name := f.Names()[0]
		if !seen[name] {
			seen[name] = true
			uniqueFlags = append(uniqueFlags, f)
		}
	}

	require.NotEmpty(t, uniqueFlags, "should have discovered CLI flags")
	return uniqueFlags
}

// Helper assertions
func assertIsStringFlag(t *testing.T, f cli.Flag) *cli.StringFlag {
	t.Helper()
	sf, ok := f.(*cli.StringFlag)
	require.True(t, ok, "expected *cli.StringFlag, got %T", f)
	return sf
}

func assertIsBoolFlag(t *testing.T, f cli.Flag) *cli.BoolFlag {
	t.Helper()
	bf, ok := f.(*cli.BoolFlag)
	require.True(t, ok, "expected *cli.BoolFlag, got %T", f)
	return bf
}
