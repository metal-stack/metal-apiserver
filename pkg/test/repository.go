package test

import (
	"context"
	"log/slog"
	"testing"

	"github.com/alicebob/miniredis/v2"
	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	mdc "github.com/metal-stack/masterdata-api/pkg/client"
	apiv1 "github.com/metal-stack/masterdata-api/api/v1"
	"github.com/metal-stack/metal-apiserver/pkg/db/generic"
	"github.com/metal-stack/metal-apiserver/pkg/repository"
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

		validated, err := repo.Network(nil).ValidateCreate(ctx, nw)
		require.NoError(t, err)
		_, err = repo.Network(nil).Create(ctx, validated)
		require.NoError(t, err)
	}
}

// FIXME refactor to use the repo client once project and tenant repository implementation is ready
func CreateProjects(t *testing.T, ctx context.Context, client mdc.Client, projects []*apiv1.Project) {
	for _, p := range projects {
		_, err := client.Project().Create(ctx, &apiv1.ProjectCreateRequest{Project: p})
		require.NoError(t, err)
	}
}
func CreateTenants(t *testing.T, ctx context.Context, client mdc.Client, tenants []*apiv1.Tenant) {
	for _, tenant := range tenants {
		_, err := client.Tenant().Create(ctx, &apiv1.TenantCreateRequest{Tenant: tenant})
		require.NoError(t, err)
	}
}
