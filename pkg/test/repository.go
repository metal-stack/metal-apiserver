package test

import (
	"log/slog"
	"testing"

	"github.com/alicebob/miniredis/v2"
	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/masterdata-api/pkg/client"
	"github.com/metal-stack/metal-apiserver/pkg/repository"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func StartRepositoryWithCockroach(t *testing.T, log *slog.Logger) (*repository.Store, client.Client, func()) {
	ds, _, rethinkCloser := StartRethink(t, log)

	r := miniredis.RunT(t)
	rc := redis.NewClient(&redis.Options{Addr: r.Addr()})

	ipam, ipamCloser := StartIpam(t)

	mdc, connection, masterdataCloser := StartMasterdataWithCochroach(t, log)

	closer := func() {
		_ = connection.Close()
		rethinkCloser()
		ipamCloser()
		masterdataCloser()
	}

	repo, err := repository.New(log, mdc, ds, ipam, rc)
	require.NoError(t, err)
	return repo, mdc, closer
}

func StartRepository(t *testing.T, log *slog.Logger) (*repository.Store, func()) {
	ds, _, rethinkCloser := StartRethink(t, log)

	r := miniredis.RunT(t)
	rc := redis.NewClient(&redis.Options{Addr: r.Addr()})

	ipam, ipamCloser := StartIpam(t)

	mdc, connection, masterdataCloser := StartMasterdataInMemory(t, log)

	closer := func() {
		_ = connection.Close()
		rethinkCloser()
		ipamCloser()
		masterdataCloser()
	}

	repo, err := repository.New(log, mdc, ds, ipam, rc)
	require.NoError(t, err)
	return repo, closer
}

func CreateImages(t *testing.T, repo *repository.Store, images []*adminv2.ImageServiceCreateRequest) {
	for _, img := range images {
		validated, err := repo.Image().ValidateCreate(t.Context(), img)
		require.NoError(t, err)
		_, err = repo.Image().Create(t.Context(), validated)
		require.NoError(t, err)
	}
}

func CreateIPs(t *testing.T, repo *repository.Store, ips []*apiv2.IPServiceCreateRequest) {
	for _, ip := range ips {
		validated, err := repo.UnscopedIP().ValidateCreate(t.Context(), ip)
		require.NoError(t, err)

		_, err = repo.UnscopedIP().Create(t.Context(), validated)
		require.NoError(t, err)
	}
}

func CreateNetworks(t *testing.T, repo *repository.Store, nws []*apiv2.NetworkServiceCreateRequest) {
	for _, nw := range nws {
		// TODO do not care about project here

		validated, err := repo.UnscopedNetwork().ValidateCreate(t.Context(), nw)
		require.NoError(t, err)
		_, err = repo.UnscopedNetwork().Create(t.Context(), validated)
		require.NoError(t, err)
	}
}

func CreatePartitions(t *testing.T, repo *repository.Store, partitions []*adminv2.PartitionServiceCreateRequest) {
	for _, partition := range partitions {
		validated, err := repo.Partition().ValidateCreate(t.Context(), partition)
		require.NoError(t, err)
		_, err = repo.Partition().Create(t.Context(), validated)
		require.NoError(t, err)
	}
}

func CreateProjects(t *testing.T, repo *repository.Store, projects []*apiv2.ProjectServiceCreateRequest) {
	for _, p := range projects {
		validated, err := repo.UnscopedProject().ValidateCreate(t.Context(), p)
		require.NoError(t, err)
		_, err = repo.UnscopedProject().Create(t.Context(), validated)
		require.NoError(t, err)
	}
}
func CreateTenants(t *testing.T, repo *repository.Store, tenants []*apiv2.TenantServiceCreateRequest) {
	for _, tenant := range tenants {
		validated, err := repo.Tenant().ValidateCreate(t.Context(), tenant)
		require.NoError(t, err)
		_, err = repo.Tenant().Create(t.Context(), validated)
		require.NoError(t, err)
	}
}
