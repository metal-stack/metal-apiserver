package test

import (
	"context"
	"testing"

	"github.com/redis/go-redis/v9"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/valkey"
)

func StartValkey(t *testing.T, ctx context.Context) (testcontainers.Container, *redis.Client, error) {
	valkeyContainer, err := valkey.Run(ctx,
		"valkey/valkey:8-alpine",
		valkey.WithSnapshotting(10, 1),
		valkey.WithLogLevel(valkey.LogLevelVerbose),
	)

	if err != nil {
		return nil, nil, err
	}

	uri, err := valkeyContainer.ConnectionString(ctx)
	if err != nil {
		return nil, nil, err
	}
	options, err := redis.ParseURL(uri)
	if err != nil {
		return nil, nil, err
	}

	client := redis.NewClient(options)

	return valkeyContainer, client, nil
}
