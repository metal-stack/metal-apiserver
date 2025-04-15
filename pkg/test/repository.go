package test

import (
	"log/slog"
	"testing"

	"connectrpc.com/connect"
	"github.com/alicebob/miniredis/v2"
	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	ipamv1 "github.com/metal-stack/go-ipam/api/v1"
	"github.com/metal-stack/go-ipam/api/v1/apiv1connect"
	"github.com/metal-stack/masterdata-api/pkg/client"
	"github.com/metal-stack/metal-apiserver/pkg/repository"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	r "gopkg.in/rethinkdb/rethinkdb-go.v6"
)

func StartRepositoryWithCockroach(t *testing.T, log *slog.Logger) (*repository.Store, client.Client, func()) {
	ds, _, rethinkCloser := StartRethink(t, log)

	r := miniredis.RunT(t)
	rc := redis.NewClient(&redis.Options{Addr: r.Addr()})

	ipam, ipamCloser := StartIpam(t)

	mdc, connection, masterdataCloser := StartMasterdataWithCochroach(t, log)

	repo, err := repository.New(log, mdc, ds, ipam, rc)
	require.NoError(t, err)

	asyncCloser := StartAsynqServer(t, log.WithGroup("asynq"), repo, rc)

	closer := func() {
		_ = connection.Close()
		rethinkCloser()
		ipamCloser()
		masterdataCloser()
		asyncCloser()
	}
	return repo, mdc, closer
}

type testStore struct {
	*repository.Store
	queryExecutor *r.Session
	ipam          apiv1connect.IpamServiceClient
}

func (s *testStore) CleanNetworkTable(t *testing.T) {
	_, err := r.DB("metal").Table("network").Delete().RunWrite(s.queryExecutor)
	require.NoError(t, err)
}

func StartRepository(t *testing.T, log *slog.Logger) (*repository.Store, func()) {
	s, close := StartRepositoryWithCleanup(t, log)
	return s.Store, close
}

func StartRepositoryWithCleanup(t *testing.T, log *slog.Logger) (*testStore, func()) {
	ds, opts, rethinkCloser := StartRethink(t, log)

	mr := miniredis.RunT(t)
	rc := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	ipam, ipamCloser := StartIpam(t)

	mdc, connection, masterdataCloser := StartMasterdataInMemory(t, log)

	repo, err := repository.New(log, mdc, ds, ipam, rc)
	require.NoError(t, err)

	asyncCloser := StartAsynqServer(t, log.WithGroup("asynq"), repo, rc)

	closer := func() {
		_ = connection.Close()
		rethinkCloser()
		ipamCloser()
		masterdataCloser()
		asyncCloser()
	}

	session, err := r.Connect(opts)
	require.NoError(t, err)

	return &testStore{
		Store:         repo,
		queryExecutor: session,
		ipam:          ipam,
	}, closer
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

func CreateNetworks(t *testing.T, repo *repository.Store, nws []*adminv2.NetworkServiceCreateRequest) {
	for _, nw := range nws {
		validated, err := repo.UnscopedNetwork().ValidateCreate(t.Context(), nw)
		require.NoError(t, err)
		_, err = repo.UnscopedNetwork().Create(t.Context(), validated)
		require.NoError(t, err)
	}
}

func DeleteNetworks(t *testing.T, testStore *testStore) {
	_, err := r.DB("metal").Table("network").Delete().RunWrite(testStore.queryExecutor)
	require.NoError(t, err)

	resp, err := testStore.ipam.ListPrefixes(t.Context(), connect.NewRequest(&ipamv1.ListPrefixesRequest{}))
	require.NoError(t, err)
	for _, prefix := range resp.Msg.Prefixes {
		_, err := testStore.ipam.DeletePrefix(t.Context(), connect.NewRequest(&ipamv1.DeletePrefixRequest{Cidr: prefix.Cidr}))
		require.NoError(t, err)
	}
}

// NetworkMap maps network.Name to network.Id
type NetworkMap map[string]string

func AllocateNetworks(t *testing.T, repo *repository.Store, nws []*apiv2.NetworkServiceCreateRequest) NetworkMap {
	var networkMap = NetworkMap{}
	for _, nw := range nws {
		validated, err := repo.UnscopedNetwork().ValidateAllocateNetwork(t.Context(), nw)
		require.NoError(t, err)
		resp, err := repo.UnscopedNetwork().AllocateNetwork(t.Context(), validated)
		require.NoError(t, err)
		networkMap[resp.Name] = resp.ID
	}
	return networkMap
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
