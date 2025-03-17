package e2e

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"connectrpc.com/connect"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/certs"
	"github.com/metal-stack/metal-apiserver/pkg/service"
	"github.com/metal-stack/metal-apiserver/pkg/service/token"
	tokencommon "github.com/metal-stack/metal-apiserver/pkg/token"
	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/metal-stack/metal-apiserver/pkg/test"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
)

func StartApiserver(t *testing.T, log *slog.Logger) (baseURL, adminToken string, closer func()) {
	repo, repocloser := test.StartRepository(t, log)
	redis, valkeycloser := test.StartValkey(t, t.Context())

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "{}")
	}))
	discoveryURL := ts.URL
	defer ts.Close()

	subject := "e2e-tests"
	c := service.Config{
		Log:           log,
		Repository:    repo,
		ServerHttpURL: subject,
		RedisConfig: &service.RedisConfig{
			// Take the same redis db for all
			TokenClient:     redis,
			RateLimitClient: redis,
			InviteClient:    redis,
			AsyncClient:     redis,
		},
		OIDCClientID:     "oidc-client-id",
		OIDCClientSecret: "oidc-client-secret",
		OIDCDiscoveryURL: discoveryURL,
	}

	mux, err := service.New(log, c)
	require.NoError(t, err)

	// TODO start asynq server mux

	server := httptest.NewUnstartedServer(h2c.NewHandler(mux, &http2.Server{}))
	server.Start()

	tokenStore := tokencommon.NewRedisStore(redis)
	certStore := certs.NewRedisStore(&certs.Config{
		RedisClient: redis,
	})

	tokenService := token.New(token.Config{
		Log:        log,
		TokenStore: tokenStore,
		CertStore:  certStore,
		Issuer:     subject,
	})

	resp, err := tokenService.CreateApiTokenWithoutPermissionCheck(t.Context(), subject, connect.NewRequest(&apiv2.TokenServiceCreateRequest{
		Description:  "e2e admin token",
		Expires:      durationpb.New(time.Hour),
		ProjectRoles: nil,
		TenantRoles:  nil,
		AdminRole:    apiv2.AdminRole_ADMIN_ROLE_EDITOR.Enum(),
		Permissions: []*apiv2.MethodPermission{
			{
				Subject: "*",
			},
		},
	}))
	require.NoError(t, err)

	closer = func() {
		repocloser()
		valkeycloser()
		server.Close()
	}

	return server.URL, resp.Msg.Secret, closer
}
