package repository_test

import (
	"fmt"
	"log/slog"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/metal-stack/metal-apiserver/pkg/repository"
	"github.com/metal-stack/metal-apiserver/pkg/test"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGet(t *testing.T) {
	ctx := t.Context()
	log := slog.Default()
	r := miniredis.RunT(t)
	rc := redis.NewClient(&redis.Options{Addr: r.Addr()})

	ds, _, rethinkCloser := test.StartRethink(t, log)
	defer func() {
		rethinkCloser()
	}()

	ipam, closer := test.StartIpam(t)
	defer closer()

	repo, err := repository.New(log, nil, ds, ipam, rc)
	require.NoError(t, err)

	ip, err := repo.IP("project1").Get(ctx, "asdf")
	require.Error(t, err)
	nw, err := repo.Network("project1").Get(ctx, "asdf")
	require.Error(t, err)

	fmt.Printf("%v %v", ip, nw)
}

func TestIpUnscopedList(t *testing.T) {
	ctx := t.Context()
	log := slog.Default()
	r := miniredis.RunT(t)
	rc := redis.NewClient(&redis.Options{Addr: r.Addr()})

	ds, _, rethinkCloser := test.StartRethink(t, log)
	defer func() {
		rethinkCloser()
	}()

	ipam, closer := test.StartIpam(t)
	defer closer()

	repo, err := repository.New(log, nil, ds, ipam, rc)
	require.NoError(t, err)

	ips, err := repo.UnscopedIP().List(ctx, nil)
	require.NoError(t, err)

	assert.Empty(t, ips)
}
