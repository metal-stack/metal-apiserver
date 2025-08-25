package e2e

import (
	"log/slog"
	"os"
	"testing"

	"connectrpc.com/connect"
	"github.com/metal-stack/api/go/client"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/stretchr/testify/require"
)

func TestUnauthenticated(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	baseURL, adminToken, closer := StartApiserver(t, log)
	defer closer()
	require.NotNil(t, baseURL, adminToken)

	apiClient, err := client.New(&client.DialConfig{
		BaseURL:   baseURL,
		Debug:     true,
		UserAgent: "integration test",
	})
	require.NoError(t, err)

	ctx := t.Context()

	images, err := apiClient.Apiv2().Image().List(ctx, connect.NewRequest(&apiv2.ImageServiceListRequest{}))
	require.Nil(t, images)
	require.EqualError(t, err, "permission_denied: not allowed to call: /metalstack.api.v2.ImageService/List")
}

func TestAuthenticated(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	baseURL, adminToken, closer := StartApiserver(t, log)
	defer closer()
	require.NotNil(t, baseURL, adminToken)

	apiClient, err := client.New(&client.DialConfig{
		BaseURL:   baseURL,
		Token:     adminToken,
		Debug:     true,
		UserAgent: "integration test",
	})
	require.NoError(t, err)

	ctx := t.Context()

	v, err := apiClient.Apiv2().Version().Get(ctx, connect.NewRequest(&apiv2.VersionServiceGetRequest{}))
	require.NotNil(t, v)
	require.NoError(t, err)

}
