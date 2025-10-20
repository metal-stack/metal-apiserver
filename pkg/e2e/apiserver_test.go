package e2e

import (
	"fmt"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/metal-stack/api/go/client"
	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-lib/pkg/pointer"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/durationpb"
)

func TestUnauthenticated(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	baseURL, adminToken, _, closer := StartApiserver(t, log)
	defer closer()
	require.NotNil(t, baseURL, adminToken)

	apiClient, err := client.New(&client.DialConfig{
		BaseURL:   baseURL,
		UserAgent: "integration test",
		Log:       log,
	})
	require.NoError(t, err)

	ctx := t.Context()

	images, err := apiClient.Apiv2().Image().List(ctx, &apiv2.ImageServiceListRequest{})
	require.Nil(t, images)
	require.EqualError(t, err, "permission_denied: not allowed to call: /metalstack.api.v2.ImageService/List")
}

func TestAuthenticated(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	baseURL, adminToken, _, closer := StartApiserver(t, log)
	defer closer()
	require.NotNil(t, baseURL, adminToken)

	apiClient, err := client.New(&client.DialConfig{
		BaseURL:   baseURL,
		Token:     adminToken,
		UserAgent: "integration test",
		Log:       log,
	})
	require.NoError(t, err)

	ctx := t.Context()

	v, err := apiClient.Apiv2().Version().Get(ctx, &apiv2.VersionServiceGetRequest{})
	require.NoError(t, err)
	require.NotNil(t, v)
}

func TestListBaseNetworks(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	baseURL, adminToken, tenantTokenSecrets, closer := StartApiserver(t, log, "user-a")
	defer closer()
	require.NotNil(t, baseURL, adminToken)
	log.Info("token", "secret", tenantTokenSecrets["user-a"])

	adminClient, err := client.New(&client.DialConfig{
		BaseURL:   baseURL,
		Token:     adminToken,
		UserAgent: "integration test admin",
		Log:       log,
	})
	require.NoError(t, err)

	ctx := t.Context()
	internet, err := adminClient.Adminv2().Network().Create(ctx, &adminv2.NetworkServiceCreateRequest{
		Id:       pointer.Pointer("internet"),
		Name:     pointer.Pointer("internet"),
		Type:     apiv2.NetworkType_NETWORK_TYPE_EXTERNAL,
		Prefixes: []string{"10.0.0.0/16"},
		Vrf:      pointer.Pointer(uint32(42)),
	})
	require.NoError(t, err)
	require.NotNil(t, internet)

	userClient, err := client.New(&client.DialConfig{
		BaseURL:   baseURL,
		Token:     tenantTokenSecrets["user-a"],
		UserAgent: "integration test user",
		Log:       log,
	})
	require.NoError(t, err)

	p1, err := userClient.Apiv2().Project().Create(ctx, &apiv2.ProjectServiceCreateRequest{
		Name:  "testproject-1",
		Login: "user-a",
	})
	require.NoError(t, err)
	require.NotNil(t, p1)

	pslr, err := userClient.Apiv2().Project().List(ctx, &apiv2.ProjectServiceListRequest{})
	require.NoError(t, err)
	require.NotNil(t, pslr)

	nslr, err := userClient.Apiv2().Network().ListBaseNetworks(ctx, &apiv2.NetworkServiceListBaseNetworksRequest{
		Project: p1.Project.Uuid,
	})
	require.NoError(t, err)
	require.NotNil(t, nslr)
	require.Len(t, nslr.Networks, 1)
}

func TestImageCacheServiceToken(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	baseURL, adminToken, tenantTokenSecrets, closer := StartApiserver(t, log, "metal-image-cache-sync")
	defer closer()
	require.NotNil(t, baseURL, adminToken)
	log.Info("token", "secret", tenantTokenSecrets["metal-image-cache-sync"])

	adminClient, err := client.New(&client.DialConfig{
		BaseURL:   baseURL,
		Token:     adminToken,
		UserAgent: "integration test admin",
		Log:       log,
	})
	require.NoError(t, err)

	tokenResp, err := adminClient.Apiv2().Token().Create(t.Context(), &apiv2.TokenServiceCreateRequest{
		Description: "metal-image-cache-sync token",
		Permissions: []*apiv2.MethodPermission{
			{
				Subject: "metal-image-cache-sync",
				Methods: []string{
					//	"/metalstack.api.v2.ImageService/List",
					"/metalstack.api.v2.PartitionService/List",
					"/metalstack.api.v2.TokenService/Refresh",
				},
			},
		},
		Expires: durationpb.New(10 * time.Minute),
	})
	require.NoError(t, err)

	userClient, err := client.New(&client.DialConfig{
		BaseURL:   baseURL,
		Token:     tokenResp.Secret,
		UserAgent: "metal-image-cache-sync user",
		Log:       log,
	})
	require.NoError(t, err)

	fmt.Println(tokenResp.Secret)

	_, err = userClient.Apiv2().Image().Get(t.Context(), &apiv2.ImageServiceGetRequest{})
	require.NoError(t, err)

	_, err = userClient.Apiv2().Partition().List(t.Context(), &apiv2.PartitionServiceListRequest{})
	require.NoError(t, err)
}
