package repository_test

import (
	"context"
	"fmt"
	"log/slog"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/generic"
	"github.com/metal-stack/metal-apiserver/pkg/repository"
	"github.com/metal-stack/metal-apiserver/pkg/test"
	"github.com/metal-stack/metal-lib/pkg/pointer"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGet(t *testing.T) {
	ctx := context.Background()
	log := slog.Default()
	r := miniredis.RunT(t)
	rc := redis.NewClient(&redis.Options{Addr: r.Addr()})

	container, c, err := test.StartRethink(t, log)
	require.NoError(t, err)
	defer func() {
		_ = container.Terminate(context.Background())
	}()

	ipam := test.StartIpam(t)

	ds, err := generic.New(log, c)
	require.NoError(t, err)

	repo, err := repository.New(log, nil, ds, ipam, rc)
	require.NoError(t, err)

	ip, err := repo.IP("project1").Get(ctx, "asdf")
	require.Error(t, err)
	nw, err := repo.Network(pointer.Pointer("project1")).Get(ctx, "asdf")
	require.Error(t, err)

	fmt.Printf("%v %v", ip, nw)
}

func TestIpUnscopedList(t *testing.T) {
	ctx := context.Background()
	log := slog.Default()
	r := miniredis.RunT(t)
	rc := redis.NewClient(&redis.Options{Addr: r.Addr()})

	container, c, err := test.StartRethink(t, log)
	require.NoError(t, err)
	defer func() {
		_ = container.Terminate(context.Background())
	}()

	ipam := test.StartIpam(t)

	ds, err := generic.New(log, c)
	require.NoError(t, err)

	repo, err := repository.New(log, nil, ds, ipam, rc)
	require.NoError(t, err)

	ips, err := repo.UnscopedIP().List(ctx, nil)
	require.NoError(t, err)

	assert.Empty(t, ips)
}
