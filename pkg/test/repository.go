package test

import (
	"context"
	"log/slog"
	"testing"

	"github.com/alicebob/miniredis/v2"
	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/generic"
	"github.com/metal-stack/metal-apiserver/pkg/repository"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func StartRepository(t *testing.T, log *slog.Logger) (*repository.Store, func()) {
	container, c, err := StartRethink(t, log)
	require.NoError(t, err)

	r := miniredis.RunT(t)
	rc := redis.NewClient(&redis.Options{Addr: r.Addr()})

	ipam := StartIpam(t)

	ds, err := generic.New(log, c)
	require.NoError(t, err)

	mdc, connection := StartMasterdataInMemory(t, log)

	closer := func() {
		_ = connection.Close()
		_ = container.Terminate(context.Background())
	}

	repo, err := repository.New(log, mdc, ds, ipam, rc)
	require.NoError(t, err)
	return repo, closer
}

func CreateImages(t *testing.T, ctx context.Context, repo *repository.Store, images []*adminv2.ImageServiceCreateRequest) {
	for _, img := range images {
		validated, err := repo.Image().ValidateCreate(ctx, img)
		require.NoError(t, err)
		_, err = repo.Image().Create(ctx, validated)
		require.NoError(t, err)
	}
}

func CreateIPs(t *testing.T, ctx context.Context, repo *repository.Store, ips []*apiv2.IPServiceCreateRequest) {
	for _, ip := range ips {
		validated, err := repo.UnscopedIP().ValidateCreate(ctx, ip)
		require.NoError(t, err)

		_, err = repo.UnscopedIP().Create(ctx, validated)
		require.NoError(t, err)
	}
}

func CreateNetworks(t *testing.T, ctx context.Context, repo *repository.Store, nws []*apiv2.NetworkServiceCreateRequest) {
	for _, nw := range nws {
		// TODO do not care about project here

		validated, err := repo.UnscopedNetwork().ValidateCreate(ctx, nw)
		require.NoError(t, err)
		_, err = repo.UnscopedNetwork().Create(ctx, validated)
		require.NoError(t, err)
	}
}

func CreateProjects(t *testing.T, ctx context.Context, repo *repository.Store, projects []*apiv2.ProjectServiceCreateRequest) {
	for _, p := range projects {
		validated, err := repo.UnscopedProject().ValidateCreate(ctx, p)
		require.NoError(t, err)
		_, err = repo.UnscopedProject().Create(ctx, validated)
		require.NoError(t, err)
	}
}
func CreateTenants(t *testing.T, ctx context.Context, repo *repository.Store, tenants []*apiv2.TenantServiceCreateRequest) {
	for _, tenant := range tenants {
		validated, err := repo.Tenant().ValidateCreate(ctx, tenant)
		require.NoError(t, err)
		_, err = repo.Tenant().Create(ctx, validated)
		require.NoError(t, err)
	}
}
