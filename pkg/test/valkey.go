package test

import (
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	valkeygo "github.com/valkey-io/valkey-go"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/valkey"
)
type testOptMiniRedis struct {
	with bool
}

func WithMiniRedis(with bool) *testOptMiniRedis {
	return &testOptMiniRedis{
		with: with,
	}
}

func StartValkey(t testing.TB, testOpts ...testOpt) (*redis.Client, valkeygo.Client, func()) {
	ctx := t.Context()
	var (
		withMiniRedis = false
	)

	for _, opt := range testOpts {
		switch o := opt.(type) {
		case *testOptMiniRedis:
			withMiniRedis = o.with
		default:
			t.Errorf("unsupported test option: %T", o)
		}
	}

	if withMiniRedis {
		mr := miniredis.RunT(t)
		rc := redis.NewClient(&redis.Options{Addr: mr.Addr()})
		vc, err := valkeygo.NewClient(valkeygo.ClientOption{
			InitAddress: []string{mr.Addr()},
			// This is required because otherwise we get:
			// unknown subcommand 'TRACKING'. Try CLIENT HELP.: [CLIENT TRACKING ON OPTIN]
			// ClientOption.DisableCache must be true for valkey not supporting client-side caching or not supporting RESP3
			DisableCache: true,
		})
		require.NoError(t, err)
		return rc, vc, nil
	}

	valkeyContainer, err := valkey.Run(ctx,
		"valkey/valkey:9-alpine",
		valkey.WithSnapshotting(1000, 1000),
		valkey.WithLogLevel(valkey.LogLevelVerbose),
		testcontainers.WithTmpfs(map[string]string{"/data": "rw"}),
		testcontainers.WithName(containerName(t)),
	)
	require.NoError(t, err)

	uri, err := valkeyContainer.ConnectionString(ctx)
	require.NoError(t, err)

	options, err := redis.ParseURL(uri)
	require.NoError(t, err)

	client := redis.NewClient(options)

	valkeyUri := strings.TrimPrefix(uri, "redis://")

	valkeygoclient, err := valkeygo.NewClient(valkeygo.ClientOption{
		InitAddress: []string{valkeyUri},
	})
	require.NoError(t, err)

	closer := func() {
		valkeygoclient.Close()
		_ = valkeyContainer.Terminate(ctx)
	}

	return client, valkeygoclient, closer
}
