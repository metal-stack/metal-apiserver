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
	"github.com/metal-stack/metal-apiserver/pkg/repository"
	"github.com/metal-stack/metal-apiserver/pkg/service"
	tokencommon "github.com/metal-stack/metal-apiserver/pkg/token"
	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/metal-stack/metal-apiserver/pkg/test"

	"github.com/stretchr/testify/require"
)

func StartApiserver(t *testing.T, log *slog.Logger) (baseURL, adminToken string, closer func()) {
	ctx := t.Context()

	testStore, repocloser := test.StartRepositoryWithCleanup(t, log, test.WithCockroach(true), test.WithValkey(true))
	repo := testStore.Store

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintln(w, "{}")
	}))
	discoveryURL := ts.URL
	defer ts.Close()

	subject := "e2e-tests"
	providerTenant := "metal-stack"

	c := service.Config{
		Log:           log,
		Repository:    repo,
		ServerHttpURL: "https://test.io",
		RedisConfig: &service.RedisConfig{
			// Take the same redis db for all
			TokenClient:     testStore.GetRedisClient(),
			RateLimitClient: testStore.GetRedisClient(),
			InviteClient:    testStore.GetRedisClient(),
			AsyncClient:     testStore.GetRedisClient(),
		},
		MasterClient:     testStore.GetMasterdataClient(),
		Datastore:        nil, // TODO: for healthcheck e2e tests this needs to be wired up
		OIDCClientID:     "oidc-client-id",
		OIDCClientSecret: "oidc-client-secret",
		OIDCDiscoveryURL: discoveryURL,
		Admins:           []string{providerTenant, subject},
	}

	mux, err := service.New(log, c)
	require.NoError(t, err)

	// TODO start asynq server mux

	server := httptest.NewUnstartedServer(mux)
	server.Start()

	tok, err := testStore.GetTokenService().CreateApiTokenWithoutPermissionCheck(t.Context(), subject, connect.NewRequest(&apiv2.TokenServiceCreateRequest{
		Expires:   durationpb.New(time.Minute),
		AdminRole: apiv2.AdminRole_ADMIN_ROLE_EDITOR.Enum(),
	}))
	require.NoError(t, err)

	reqCtx := tokencommon.ContextWithToken(t.Context(), tok.Msg.Token)

	err = repo.Tenant().AdditionalMethods().EnsureProviderTenant(reqCtx, providerTenant)
	require.NoError(t, err)

	err = repo.UnscopedProject().AdditionalMethods().EnsureProviderProject(ctx, providerTenant)
	require.NoError(t, err)

	tenant, err := repo.Tenant().AdditionalMethods().CreateWithID(reqCtx, &apiv2.TenantServiceCreateRequest{
		Name: subject,
	}, subject)
	require.NoError(t, err)

	_, err = repo.Tenant().AdditionalMethods().Member(tenant.Meta.Id).Create(reqCtx, &repository.TenantMemberCreateRequest{
		MemberID: subject,
		Role:     apiv2.TenantRole_TENANT_ROLE_OWNER,
	})
	require.NoError(t, err)

	_, err = repo.Tenant().AdditionalMethods().Member(providerTenant).Create(ctx, &repository.TenantMemberCreateRequest{
		MemberID: tenant.Meta.Id,
		Role:     apiv2.TenantRole_TENANT_ROLE_OWNER,
	})

	require.NoError(t, err)

	resp, err := testStore.GetTokenService().CreateApiTokenWithoutPermissionCheck(ctx, subject, connect.NewRequest(&apiv2.TokenServiceCreateRequest{
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
		server.Close()
	}

	return server.URL, resp.Msg.Secret, closer
}
