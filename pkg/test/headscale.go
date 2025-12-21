package test

import (
	"context"
	_ "embed"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	headscalev1 "github.com/juanfont/headscale/gen/go/headscale/v1"
	"github.com/metal-stack/metal-apiserver/pkg/headscale"
	"tailscale.com/tsnet"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/exec"
	tlog "github.com/testcontainers/testcontainers-go/log"
	"github.com/testcontainers/testcontainers-go/wait"
)

//go:embed headscale-config.yaml
var headscaleConfig string

func StartHeadscale(t testing.TB) (headscalev1.HeadscaleServiceClient, string, string, func()) {
	ctx := t.Context()

	headscaleContainer, err := testcontainers.Run(
		ctx,
		"ghcr.io/juanfont/headscale:v0.27.1",
		testcontainers.WithFiles(testcontainers.ContainerFile{
			Reader:            strings.NewReader(headscaleConfig),
			ContainerFilePath: "/config.yaml",
			FileMode:          0o644,
		}),
		testcontainers.WithTmpfs(map[string]string{
			"/tmp":                "rw",
			"/var/lib/headscsale": "rw",
		}),
		testcontainers.WithExposedPorts("8080/tcp", "50443/tcp"),
		testcontainers.WithWaitStrategy(wait.ForListeningPort("50443/tcp").WithStartupTimeout(time.Second*5)),
		testcontainers.WithCmd("serve", "-c", "/config.yaml"),
		testcontainers.WithLogger(tlog.TestLogger(t)),
	)
	require.NoError(t, err)

	c, reader, err := headscaleContainer.Exec(ctx, []string{"headscale", "apikeys", "create"}, exec.Multiplexed())
	require.NoError(t, err)
	assert.Zerof(t, c, "apikeys should have been created, expected return code 0, got %d", c)

	output, err := io.ReadAll(reader)
	require.NoError(t, err)
	apikey := strings.TrimSpace(string(output))
	t.Logf("apikey:%q\n", apikey)
	require.NoError(t, err)

	endpoint, err = headscaleContainer.PortEndpoint(ctx, "50443/tcp", "")
	require.NoError(t, err)
	t.Log(endpoint)
	controllerURL, err := headscaleContainer.PortEndpoint(ctx, "8080/tcp", "http")
	require.NoError(t, err)
	t.Log(controllerURL)

	client, err := headscale.NewClient(headscale.Config{
		Log:      slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{})),
		Apikey:   apikey,
		Endpoint: endpoint,
	})
	require.NoError(t, err)

	_, err = client.Health(ctx, &headscalev1.HealthRequest{})
	require.NoError(t, err)

	closer := func() {
		_ = headscaleContainer.Terminate(ctx)
	}

	return client, endpoint, controllerURL, closer
}

func ConnectVPNClient(t testing.TB, hostname, controllerURL, authkey string) {
	s := &tsnet.Server{
		Hostname:   hostname,
		ControlURL: controllerURL,
		AuthKey:    authkey,
	}
	lc, err := s.LocalClient()
	require.NoError(t, err)
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		status, err := lc.Status(t.Context())
		require.NoError(c, err)
		require.True(c, status.Self.Online)
	}, 10*time.Second, 50*time.Millisecond)
}

type tokenAuth struct {
	token string
}

func (t tokenAuth) GetRequestMetadata(
	ctx context.Context,
	_ ...string,
) (map[string]string, error) {
	return map[string]string{
		"authorization": "Bearer " + t.token,
	}, nil
}

func (tokenAuth) RequireTransportSecurity() bool {
	return false
}
