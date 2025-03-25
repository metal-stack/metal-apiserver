package test

import (
	"testing"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go/modules/valkey"
)

func StartValkey(t *testing.T) (*redis.Client, func()) {
	ctx := t.Context()
	valkeyContainer, err := valkey.Run(ctx,
		"valkey/valkey:8-alpine",
		valkey.WithSnapshotting(10, 1),
		valkey.WithLogLevel(valkey.LogLevelVerbose),
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
