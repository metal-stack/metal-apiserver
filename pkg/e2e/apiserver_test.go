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

	apiClient := client.New(client.DialConfig{
		BaseURL:   baseURL,
		Token:     "a-token",
		Debug:     true,
		UserAgent: "integration test",
	})

	ctx := t.Context()

	v, err := apiClient.Apiv2().Version().Get(ctx, connect.NewRequest(&apiv2.VersionServiceGetRequest{}))
	require.Nil(t, v)
	require.EqualError(t, err, "unauthenticated: invalid token")
}
func TestAuthenticated(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	baseURL, adminToken, closer := StartApiserver(t, log)
	defer closer()
	require.NotNil(t, baseURL, adminToken)

	apiClient := client.New(client.DialConfig{
		BaseURL:   baseURL,
		Token:     adminToken,
		Debug:     true,
		UserAgent: "integration test",
	})

	ctx := t.Context()

	v, err := apiClient.Apiv2().Version().Get(ctx, connect.NewRequest(&apiv2.VersionServiceGetRequest{}))
	require.NotNil(t, v)
	require.NoError(t, err)

}
