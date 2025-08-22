package tenant

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"connectrpc.com/connect"
	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-lib/pkg/pointer"

	"github.com/metal-stack/api/go/client"
	"github.com/metal-stack/metal-apiserver/pkg/test"
	"github.com/metal-stack/metal-apiserver/pkg/token"

	"github.com/metal-stack/security"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type interceptorTestFn func(string, []connect.Interceptor, func(context.Context)) *connect.Handler

func Test_tenantInterceptor_AuditingCtx(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	testStore, closer := test.StartRepositoryWithCleanup(t, log)
	defer closer()

	tests := []struct {
		name             string
		method           string
		reqFn            func(ctx context.Context, c client.Client) error
		handler          interceptorTestFn
		token            *apiv2.Token
		existingProjects []*apiv2.ProjectServiceCreateRequest
		existingTenants  []*apiv2.TenantServiceCreateRequest
		wantUser         *security.User
		wantErr          string
	}{
		{
			name: "anonymous request against public endpoint",
			reqFn: func(ctx context.Context, c client.Client) error {
				_, err := c.Apiv2().Health().Get(ctx, connect.NewRequest(&apiv2.HealthServiceGetRequest{}))
				return err
			},
			method:  "/metalstack.api.v2.HealthService/Get",
			handler: handler[apiv2.HealthServiceGetRequest, apiv2.HealthServiceGetResponse](),
			wantUser: &security.User{
				EMail:   "",
				Name:    "",
				Groups:  []security.ResourceAccess{},
				Tenant:  "",
				Issuer:  "",
				Subject: "",
			},
		},
		{
			name: "request against public endpoint",
			reqFn: func(ctx context.Context, c client.Client) error {
				_, err := c.Apiv2().Health().Get(ctx, connect.NewRequest(&apiv2.HealthServiceGetRequest{}))
				return err
			},
			token: &apiv2.Token{
				User:      "john@github",
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
				AdminRole: nil,
			},
			method:  "/metalstack.api.v2.HealthService/Get",
			handler: handler[apiv2.HealthServiceGetRequest, apiv2.HealthServiceGetResponse](),
			existingTenants: []*apiv2.TenantServiceCreateRequest{
				{
					Name:  "john@github",
					Email: pointer.Pointer("mail@john"),
				},
			},
			wantUser: &security.User{
				EMail:   "mail@john",
				Name:    "",
				Groups:  []security.ResourceAccess{},
				Tenant:  "",
				Project: "",
				Issuer:  "",
				Subject: "john@github",
			},
		},
		{
			name: "request without token against non-public endpoint",
			reqFn: func(ctx context.Context, c client.Client) error {
				_, err := c.Apiv2().IP().Create(ctx, connect.NewRequest(&apiv2.IPServiceCreateRequest{}))
				return err
			},
			method:  "/metalstack.api.v2.IPService/Create",
			handler: handler[apiv2.IPServiceCreateRequest, apiv2.IPServiceCreateResponse](),
			wantErr: "unauthenticated: token must be present when requesting non-public scope method",
		},
		{
			name: "request against self scope method",
			reqFn: func(ctx context.Context, c client.Client) error {
				_, err := c.Apiv2().Project().List(ctx, connect.NewRequest(&apiv2.ProjectServiceListRequest{}))
				return err
			},
			token: &apiv2.Token{
				Uuid: "a-uuid",
				User: "user@github",
			},
			method:  "/metalstack.api.v2.ProjectService/List",
			handler: handler[apiv2.ProjectServiceListRequest, apiv2.ProjectServiceListResponse](),
			existingTenants: []*apiv2.TenantServiceCreateRequest{
				{
					Name:  "user@github",
					Email: pointer.Pointer("mail@user"),
				},
			},
			wantUser: &security.User{
				EMail:   "mail@user",
				Name:    "",
				Groups:  []security.ResourceAccess{},
				Tenant:  "user@github",
				Issuer:  "",
				Subject: "user@github",
			},
		},
		{
			name: "project request",
			reqFn: func(ctx context.Context, c client.Client) error {
				_, err := c.Apiv2().IP().Get(ctx, connect.NewRequest(&apiv2.IPServiceGetRequest{
					Project: "a-project",
				}))
				return err
			},
			token: &apiv2.Token{
				User: "user@github",
			},
			method:  "/metalstack.api.v2.IPService/Get",
			handler: handler[apiv2.IPServiceGetRequest, apiv2.IPServiceGetResponse](),
			existingTenants: []*apiv2.TenantServiceCreateRequest{
				{
					Name:  "t1",
					Email: pointer.Pointer("mail@t1"),
				},
			},
			existingProjects: []*apiv2.ProjectServiceCreateRequest{
				{
					Name:  "a-project",
					Login: "t1",
				},
			},
			wantUser: &security.User{
				EMail:   "mail@t1",
				Name:    "",
				Groups:  []security.ResourceAccess{},
				Tenant:  "t1",
				Project: "a-project",
				Issuer:  "",
				Subject: "user@github",
			},
		},
		{
			name: "tenant request",
			reqFn: func(ctx context.Context, c client.Client) error {
				_, err := c.Apiv2().Tenant().Get(ctx, connect.NewRequest(&apiv2.TenantServiceGetRequest{
					Login: "a-tenant",
				}))
				return err
			},
			token: &apiv2.Token{
				User: "user@github",
			},
			method:  "/metalstack.api.v2.TenantService/Get",
			handler: handler[apiv2.TenantServiceGetRequest, apiv2.TenantServiceGetResponse](),
			existingTenants: []*apiv2.TenantServiceCreateRequest{
				{
					Name:  "a-tenant",
					Email: pointer.Pointer("mail@tenant-a"),
				},
			},
			wantUser: &security.User{
				EMail:   "mail@tenant-a",
				Name:    "",
				Groups:  []security.ResourceAccess{},
				Tenant:  "a-tenant",
				Issuer:  "",
				Subject: "user@github",
			},
		},
		{
			name: "admin list tenant request",
			reqFn: func(ctx context.Context, c client.Client) error {
				_, err := c.Adminv2().Tenant().List(ctx, connect.NewRequest(&adminv2.TenantServiceListRequest{}))
				return err
			},
			token: &apiv2.Token{
				User: "user@github",
			},
			method:  "/metalstack.admin.v2.TenantService/List",
			handler: handler[adminv2.TenantServiceListRequest, adminv2.TenantServiceListResponse](),
			existingTenants: []*apiv2.TenantServiceCreateRequest{
				{
					Name:  "user@github",
					Email: pointer.Pointer("mail@github"),
				},
			},
			wantUser: &security.User{
				EMail:   "mail@github",
				Name:    "",
				Groups:  []security.ResourceAccess{},
				Tenant:  "",
				Issuer:  "",
				Subject: "user@github",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				tenantInterceptor = NewInterceptor(slog.Default(), testStore.GetMasterdataClient())
				called            = false

				interceptors = []connect.Interceptor{
					&tokenInjector{token: tt.token},
					tenantInterceptor,
				}
			)

			test.CreateTenants(t, testStore, tt.existingTenants)
			test.CreateProjects(t, testStore.Store, tt.existingProjects)

			defer func() {
				testStore.DeleteProjects()
				testStore.DeleteTenants()
			}()

			mux := http.NewServeMux()
			mux.Handle(tt.method, tt.handler(tt.method, interceptors, func(ctx context.Context) {
				called = true

				user := security.GetUserFromContext(ctx)
				assert.Equal(t, tt.wantUser, user)
			}))

			server := httptest.NewServer(mux)
			defer server.Close()

			c, err := client.New(&client.DialConfig{
				BaseURL: server.URL,
			})
			require.NoError(t, err)

			require.NotNil(t, tt.reqFn)
			err = tt.reqFn(t.Context(), c)
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)
			assert.True(t, called, "request was not forwarded to next")
		})
	}
}

func handler[Req, Resp any]() interceptorTestFn {
	return func(procedure string, interceptors []connect.Interceptor, test func(context.Context)) *connect.Handler {
		return connect.NewUnaryHandler(
			procedure,
			func(ctx context.Context, r *connect.Request[Req]) (*connect.Response[Resp], error) {
				test(ctx)
				var zero Resp
				return connect.NewResponse(&zero), nil
			},
			connect.WithInterceptors(interceptors...),
		)
	}
}

type tokenInjector struct {
	token *apiv2.Token
}

// WrapStreamingClient implements connect.Interceptor.
func (t *tokenInjector) WrapStreamingClient(connect.StreamingClientFunc) connect.StreamingClientFunc {
	panic("unimplemented")
}

// WrapStreamingHandler implements connect.Interceptor.
func (t *tokenInjector) WrapStreamingHandler(connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	panic("unimplemented")
}

// WrapUnary implements connect.Interceptor.
func (t *tokenInjector) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, ar connect.AnyRequest) (connect.AnyResponse, error) {
		if t.token != nil {
			ctx = token.ContextWithToken(ctx, t.token)
		}
		return next(ctx, ar)
	}
}
