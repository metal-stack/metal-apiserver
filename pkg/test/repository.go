package test

import (
	"log/slog"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/metal-stack/api-server/pkg/db/generic"
	"github.com/metal-stack/api-server/pkg/db/repository"
	mdc "github.com/metal-stack/masterdata-api/pkg/client"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
)

func StartRepository(t *testing.T, log *slog.Logger, masterdataMockClient mdc.Client) (*repository.Store, testcontainers.Container) {
	container, c, err := StartRethink(t, log)
	require.NoError(t, err)

	r := miniredis.RunT(t)
	rc := redis.NewClient(&redis.Options{Addr: r.Addr()})

	ipam := StartIpam(t)

	ds, err := generic.New(log, c)
	require.NoError(t, err)

	repo, err := repository.New(log, masterdataMockClient, ds, ipam, rc)
	require.NoError(t, err)
	return repo, container
}
