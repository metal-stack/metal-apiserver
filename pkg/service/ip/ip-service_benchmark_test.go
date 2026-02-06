package ip

import (
	"log/slog"
	"os"
	"testing"

	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/test"
	"github.com/metal-stack/metal-lib/pkg/pointer"
	"github.com/stretchr/testify/require"
)

func Benchmark_ipServiceServer_Create(b *testing.B) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	testStore, closer := test.StartRepositoryWithCleanup(b, log)
	defer closer()

	ctx := b.Context()

	test.CreateTenants(b, testStore, []*apiv2.TenantServiceCreateRequest{{Name: "t1"}})
	test.CreateProjects(b, testStore, []*apiv2.ProjectServiceCreateRequest{{Name: "p1", Login: "t1"}, {Name: "p2", Login: "t1"}})
	test.CreateNetworks(b, testStore, []*adminv2.NetworkServiceCreateRequest{
		{Id: pointer.Pointer("internet"), Prefixes: []string{"1.2.3.0/24"}, Type: apiv2.NetworkType_NETWORK_TYPE_EXTERNAL, Vrf: pointer.Pointer(uint32(11))},
		{
			Id:                       pointer.Pointer("tenant-super-namespaced"),
			Prefixes:                 []string{"12.100.0.0/16"},
			DestinationPrefixes:      []string{"1.2.3.0/24"},
			DefaultChildPrefixLength: &apiv2.ChildPrefixLength{Ipv4: pointer.Pointer(uint32(22))},
			Type:                     apiv2.NetworkType_NETWORK_TYPE_SUPER_NAMESPACED,
		},
	})

	i := &ipServiceServer{
		log:  log,
		repo: testStore.Store,
	}
	for b.Loop() {
		got, err := i.Create(ctx, &apiv2.IPServiceCreateRequest{
			Network: "internet",
			Project: "p1",
		})
		require.NoError(b, err)
		require.NotNil(b, got)
		require.NotEmpty(b, got.Ip)
	}
}
