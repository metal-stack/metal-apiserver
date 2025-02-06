package repository_test

import (
	"context"
	"fmt"
	"log/slog"
	"testing"

	"github.com/metal-stack/api-server/pkg/db/generic"
	"github.com/metal-stack/api-server/pkg/repository"
	"github.com/metal-stack/api-server/pkg/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGet(t *testing.T) {
	ctx := context.Background()
	log := slog.Default()

	container, c, err := test.StartRethink(t)
	require.NoError(t, err)
	defer func() {
		_ = container.Terminate(context.Background())
	}()

	ipam := test.StartIpam(t)

	ds, err := generic.New(log, "metal", c)
	require.NoError(t, err)

	repo := repository.New(log, nil, ds, ipam)

	ip, err := repo.IP("project1").Get(ctx, "asdf")
	require.Error(t, err)
	nw, err := repo.Network().Get(ctx, "asdf")
	require.Error(t, err)

	fmt.Printf("%s %s", ip, nw)
}

func TestIpUnscopedList(t *testing.T) {
	ctx := context.Background()
	log := slog.Default()

	container, c, err := test.StartRethink(t)
	require.NoError(t, err)
	defer func() {
		_ = container.Terminate(context.Background())
	}()

	ipam := test.StartIpam(t)

	ds, err := generic.New(log, "metal", c)
	require.NoError(t, err)

	repo := repository.New(log, nil, ds, ipam)

	ips, err := repo.UnscopedIP().List(ctx)
	require.NoError(t, err)

	assert.Empty(t, ips)
}
