package bench

import (
	"log/slog"
	"os"
	"testing"

	"connectrpc.com/connect"
	"github.com/metal-stack/api/go/client"
	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"

	"github.com/metal-stack/metal-apiserver/pkg/e2e"
	"github.com/metal-stack/metal-lib/pkg/pointer"
	"github.com/stretchr/testify/require"
)

func Benchmark_e2e_ipService_Create(b *testing.B) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	baseURL, adminToken, _, closer := e2e.StartApiserver(b, log)
	defer closer()
	require.NotNil(b, baseURL, adminToken)

	apiClient, err := client.New(&client.DialConfig{
		BaseURL:   baseURL,
		Token:     adminToken,
		UserAgent: "integration test",
		Log:       log,
	})
	require.NoError(b, err)

	ctx := b.Context()

	tcr, err := apiClient.Adminv2().Tenant().Create(ctx, connect.NewRequest(&adminv2.TenantServiceCreateRequest{
		Name: "benchmark",
	}))
	require.NoError(b, err)

	pcr, err := apiClient.Apiv2().Project().Create(ctx, connect.NewRequest(&apiv2.ProjectServiceCreateRequest{
		Login: tcr.Msg.Tenant.Login,
		Name:  "Benchmark",
	}))
	require.NoError(b, err)

	_, err = apiClient.Adminv2().Network().Create(ctx, connect.NewRequest(&adminv2.NetworkServiceCreateRequest{
		Id:                  pointer.Pointer("internet"),
		Name:                pointer.Pointer("internet"),
		Prefixes:            []string{"10.1.0.0/16"},
		DestinationPrefixes: []string{"0.0.0.0/0"},
		Type:                apiv2.NetworkType_NETWORK_TYPE_EXTERNAL,
		Vrf:                 pointer.Pointer(uint32(42)),
	}))
	require.NoError(b, err)

	for b.Loop() {
		got, err := apiClient.Apiv2().IP().Create(ctx, connect.NewRequest(&apiv2.IPServiceCreateRequest{
			Network: "internet",
			Project: pcr.Msg.Project.Uuid,
		}))
		require.NoError(b, err)
		require.NotNil(b, got)
		require.NotEmpty(b, got.Msg.Ip)
	}
}
