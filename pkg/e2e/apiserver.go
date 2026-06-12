package e2e

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/repository"
	"github.com/metal-stack/metal-apiserver/pkg/repository/api"
	"github.com/metal-stack/metal-apiserver/pkg/service"
	"github.com/metal-stack/metal-apiserver/pkg/test"
	tokencommon "github.com/metal-stack/metal-apiserver/pkg/token"
	"github.com/metal-stack/metal-apiserver/pkg/vpn"
	"github.com/metal-stack/metal-lib/auditing"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/durationpb"
)

func StartApiserver(t testing.TB, log *slog.Logger, additionalTenants ...string) (baseURL, adminToken string, tenantTokens map[string]string, closer func()) {
	ctx := t.Context()

	subject := "e2e-tests"
	providerTenant := "metal-stack"

	testStore, repocloser := test.StartRepositoryWithCleanup(t, log, test.WithPostgres(true), test.WithValkey(true), test.WithHeadscale(true), test.WithAdminSubjects(providerTenant, subject))

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintln(w, "{}")
	}))
	discoveryURL := ts.URL
	defer ts.Close()

	hc := testStore.GetHeadscaleClient()

	c := service.Config{
		Log:           log,
		Repository:    testStore.Store,
		ServerHttpURL: "https://test.io",
		RedisConfig: &service.RedisConfig{
			// Take the same redis db for all
			TokenClient:     testStore.GetRedisClient(),
			RateLimitClient: testStore.GetRedisClient(),
			InviteClient:    testStore.GetRedisClient(),
			AsyncClient:     testStore.GetRedisClient(),
			QueueClient:     testStore.GetValkeyClient(),
			ComponentClient: testStore.GetValkeyClient(),
		},

		AuditBackends:                       []auditing.Auditing{testStore.GetAuditBackend()},
		AuditSearchBackend:                  testStore.GetAuditBackend(),
		TenantClient:                        testStore.GetTenantApiserverClient(),
		Datastore:                           testStore.GetDatastore(),
		IpamClient:                          testStore.GetIpamClient(),
		OIDCClientID:                        "oidc-client-id",
		OIDCClientSecret:                    "oidc-client-secret",
		OIDCDiscoveryURL:                    discoveryURL,
		MaxRequestsPerMinuteToken:           100,
		MaxRequestsPerMinuteUnauthenticated: 100,
		HeadscaleClient:                     hc,
	}

	mux, err := service.New(ctx, log, c)
	require.NoError(t, err)

	server := httptest.NewUnstartedServer(mux)
	server.Start()

	tok, err := testStore.UnscopedToken().AdditionalMethods().CreateApiTokenWithoutPermissionCheck(t.Context(), subject, &apiv2.TokenServiceCreateRequest{
		Expires:   durationpb.New(time.Minute),
		AdminRole: apiv2.AdminRole_ADMIN_ROLE_EDITOR.Enum(),
	})
	require.NoError(t, err)

	reqCtx := tokencommon.ContextWithToken(t.Context(), tok.Token)

	err = testStore.Tenant().AdditionalMethods().EnsureProviderTenant(reqCtx, providerTenant)
	require.NoError(t, err)

	err = testStore.UnscopedProject().AdditionalMethods().EnsureProviderProject(ctx, providerTenant)
	require.NoError(t, err)

	tenant, err := testStore.Tenant().AdditionalMethods().CreateWithID(reqCtx, &apiv2.TenantServiceCreateRequest{
		Name: subject,
	}, subject)
	require.NoError(t, err)

	_, err = testStore.Tenant().AdditionalMethods().Member(tenant.Login).Create(reqCtx, &api.TenantMemberCreateRequest{
		MemberID: subject,
		Role:     apiv2.TenantRole_TENANT_ROLE_OWNER,
	})
	require.NoError(t, err)

	_, err = testStore.Tenant().AdditionalMethods().Member(providerTenant).Create(ctx, &api.TenantMemberCreateRequest{
		MemberID: tenant.Login,
		Role:     apiv2.TenantRole_TENANT_ROLE_OWNER,
	})

	require.NoError(t, err)

	resp, err := testStore.UnscopedToken().AdditionalMethods().CreateApiTokenWithoutPermissionCheck(ctx, subject, &apiv2.TokenServiceCreateRequest{
		Description:  "e2e admin token",
		Expires:      durationpb.New(time.Hour),
		ProjectRoles: nil,
		TenantRoles:  nil,
		AdminRole:    apiv2.AdminRole_ADMIN_ROLE_EDITOR.Enum(),
	})
	require.NoError(t, err)

	vpnEvalCtx, vpnEvalCancel := context.WithCancel(ctx)

	go func() {
		ticker := time.NewTicker(1 * time.Second)

		for {
			select {
			case <-ticker.C:
				if _, err = vpn.EvaluateVPNConnected(vpnEvalCtx, log, testStore.Store); err != nil {
					log.Error("unable to evaluate vpn connected", "error", err)
				}
			case <-vpnEvalCtx.Done():
				return
			}
		}
	}()

	closer = func() {
		repocloser()
		server.Close()
		vpnEvalCancel()
	}

	tenantTokenSecrets := createTenantTokens(t, testStore.Store, additionalTenants...)

	return server.URL, resp.Secret, tenantTokenSecrets, closer
}

func createTenantTokens(t testing.TB, repo *repository.Store, tenants ...string) map[string]string {
	ctx := t.Context()
	tenantTokens := make(map[string]string)

	for _, tenant := range tenants {
		_, err := repo.Tenant().AdditionalMethods().CreateWithID(ctx, &apiv2.TenantServiceCreateRequest{
			Name: tenant,
		}, tenant, repository.NewTenantCreateOptWithCreator(tenant))
		require.NoError(t, err)

		_, err = repo.Tenant().AdditionalMethods().Member(tenant).Create(ctx, &api.TenantMemberCreateRequest{
			Role:     apiv2.TenantRole_TENANT_ROLE_OWNER,
			MemberID: tenant,
		})
		require.NoError(t, err)

		tcr, err := repo.UnscopedToken().AdditionalMethods().CreateUserTokenWithoutPermissionCheck(ctx, tenant, nil)
		require.NoError(t, err)

		tenantTokens[tenant] = tcr.Secret
	}

	return tenantTokens
}
