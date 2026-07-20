package e2e

import (
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/metal-stack/api/go/client"
	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	infrav2 "github.com/metal-stack/api/go/metalstack/infra/v2"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestImageCacheServiceToken(t *testing.T) {
	t.Parallel()
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

	tcr := &adminv2.TokenServiceCreateRequest{
		User: new("metal-image-cache-sync"),
		TokenCreateRequest: &apiv2.TokenServiceCreateRequest{
			Description: "metal-image-cache-sync token",
			Permissions: []*apiv2.MethodPermission{
				{
					Subject: "",
					Methods: []string{
						"/metalstack.api.v2.ImageService/List",
						"/metalstack.api.v2.PartitionService/List",
						"/metalstack.api.v2.TokenService/Refresh",
						"/metalstack.infra.v2.ComponentService/Ping",
					},
				},
			},
			Expires: durationpb.New(10 * time.Minute),
		},
	}
	tokenResp, err := adminClient.Adminv2().Token().Create(t.Context(), tcr)
	require.NoError(t, err)

	userClient, err := client.New(&client.DialConfig{
		BaseURL:   baseURL,
		Token:     tokenResp.Secret,
		UserAgent: "metal-image-cache-sync user",
		Log:       log,
	})
	require.NoError(t, err)
	_, err = userClient.Infrav2().Component().Ping(t.Context(), &infrav2.ComponentServicePingRequest{
		Type:       apiv2.ComponentType_COMPONENT_TYPE_METAL_IMAGE_CACHE_SYNC,
		Identifier: "image-cache-e2e",
		StartedAt:  timestamppb.Now(),
		Interval:   durationpb.New(5 * time.Minute),
		Version:    &apiv2.Version{Version: "v0.0.0", Revision: "abc"},
	})
	require.NoError(t, err)

	_, err = userClient.Apiv2().Image().List(t.Context(), &apiv2.ImageServiceListRequest{})
	require.NoError(t, err)

	_, err = userClient.Apiv2().Partition().List(t.Context(), &apiv2.PartitionServiceListRequest{})
	require.NoError(t, err)
	_, err = userClient.Apiv2().Token().Refresh(t.Context(), &apiv2.TokenServiceRefreshRequest{})
	require.NoError(t, err)
}
