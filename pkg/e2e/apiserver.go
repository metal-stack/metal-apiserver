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
	v1 "github.com/metal-stack/masterdata-api/api/v1"
	"github.com/metal-stack/metal-apiserver/pkg/certs"
	"github.com/metal-stack/metal-apiserver/pkg/service"
	"github.com/metal-stack/metal-apiserver/pkg/service/token"
	tokencommon "github.com/metal-stack/metal-apiserver/pkg/token"
	"github.com/metal-stack/metal-lib/pkg/pointer"
	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/metal-stack/metal-apiserver/pkg/test"

	putil "github.com/metal-stack/metal-apiserver/pkg/project"
	tutil "github.com/metal-stack/metal-apiserver/pkg/tenant"

	"github.com/stretchr/testify/require"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
)

func StartApiserver(t *testing.T, log *slog.Logger) (baseURL, adminToken string, closer func()) {
	ctx := t.Context()

	repo, masterdataClient, repocloser := test.StartRepositoryWithCockroach(t, log)
	redis, valkeycloser := test.StartValkey(t)

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
		ServerHttpURL: subject,
		RedisConfig: &service.RedisConfig{
			// Take the same redis db for all
			TokenClient:     redis,
			RateLimitClient: redis,
			InviteClient:    redis,
			AsyncClient:     redis,
		},
		MasterClient:     masterdataClient,
		Datastore:        nil, // TODO: for healthcheck e2e tests this needs to be wired up
		OIDCClientID:     "oidc-client-id",
		OIDCClientSecret: "oidc-client-secret",
		OIDCDiscoveryURL: discoveryURL,
		Admins:           []string{providerTenant, subject},
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

	err = tutil.EnsureProviderTenant(ctx, c.MasterClient, providerTenant)
	require.NoError(t, err)

	err = putil.EnsureProviderProject(ctx, c.MasterClient, providerTenant)
	require.NoError(t, err)

	_, err = masterdataClient.Tenant().Create(ctx, &v1.TenantCreateRequest{Tenant: &v1.Tenant{Meta: &v1.Meta{Id: subject}, Name: subject}})
	require.NoError(t, err)

	_, err = masterdataClient.TenantMember().Create(ctx, &v1.TenantMemberCreateRequest{
		TenantMember: &v1.TenantMember{
			Meta: &v1.Meta{
				Annotations: map[string]string{
					tutil.TenantRoleAnnotation: apiv2.TenantRole_TENANT_ROLE_OWNER.String(),
				},
			},
			TenantId: subject,
			MemberId: subject,
		},
	})
	require.NoError(t, err)

	resp, err := tokenService.CreateApiTokenWithoutPermissionCheck(ctx, subject, connect.NewRequest(&apiv2.TokenServiceCreateRequest{
		Description:  "e2e admin token",
		Expires:      durationpb.New(time.Hour),
		ProjectRoles: nil,
		TenantRoles:  nil,
		AdminRole:    apiv2.AdminRole_ADMIN_ROLE_EDITOR.Enum(),
		Permissions: []*apiv2.MethodPermission{
			{
				Subject: pointer.Pointer("*"),
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
