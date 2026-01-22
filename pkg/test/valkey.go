package test

import (
	"testing"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/valkey"
)

func StartValkey(t testing.TB) (*redis.Client, func()) {
	ctx := t.Context()
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

	closer := func() {
		_ = valkeyContainer.Terminate(ctx)
	}

	return client, closer
}
