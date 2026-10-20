package tenant

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"connectrpc.com/connect/v2"
	"connectrpc.com/connect/v2/connecthttp"
	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	infrav2 "github.com/metal-stack/api/go/metalstack/infra/v2"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"

	"github.com/metal-stack/api/go/client"
	"github.com/metal-stack/metal-apiserver/pkg/test"
	"github.com/metal-stack/metal-apiserver/pkg/token"

	"github.com/metal-stack/metal-lib/auditing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type interceptorTestFn func(*testing.T, string, func(context.Context)) connect.Method

func Test_tenantInterceptor_AuditingCtx(t *testing.T) {
	t.Parallel()

	testStore, closer := test.StartRepositoryWithCleanup(t)
	log := testStore.GetLogger()
	defer closer()

	tests := []struct {
		name             string
		method           string
		reqFn            func(ctx context.Context, c client.Client) error
		handler          interceptorTestFn
		token            *apiv2.Token
		existingProjects []*apiv2.ProjectServiceCreateRequest
		existingTenants  []*apiv2.TenantServiceCreateRequest
		wantUser         *auditing.User
		wantErr          string
	}{
		{
			name: "anonymous request against public endpoint",
			reqFn: func(ctx context.Context, c client.Client) error {
				_, err := c.Apiv2().Health().Get(ctx, &apiv2.HealthServiceGetRequest{})
				return err
			},
			method:  "/metalstack.api.v2.HealthService/Get",
			handler: handler[apiv2.HealthServiceGetRequest, apiv2.HealthServiceGetResponse](),
			wantUser: &auditing.User{
				EMail:   "",
				Name:    "",
				Tenant:  "",
				Subject: "",
			},
		},
		{
			name: "request against public endpoint",
			reqFn: func(ctx context.Context, c client.Client) error {
				_, err := c.Apiv2().Health().Get(ctx, &apiv2.HealthServiceGetRequest{})
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
					Email: new("mail@john"),
				},
			},
			wantUser: &auditing.User{
				EMail:   "mail@john",
				Name:    "",
				Tenant:  "",
				Project: "",
				Subject: "john@github",
			},
		},
		{
			name: "request without token against non-public endpoint",
			reqFn: func(ctx context.Context, c client.Client) error {
				_, err := c.Apiv2().IP().Create(ctx, &apiv2.IPServiceCreateRequest{})
				return err
			},
			method:  "/metalstack.api.v2.IPService/Create",
			handler: handler[apiv2.IPServiceCreateRequest, apiv2.IPServiceCreateResponse](),
			wantErr: "unauthenticated: token must be present when requesting non-public scope method",
		},
		{
			name: "request against self scope method",
			reqFn: func(ctx context.Context, c client.Client) error {
				_, err := c.Apiv2().Project().List(ctx, &apiv2.ProjectServiceListRequest{})
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
					Email: new("mail@user"),
				},
			},
			wantUser: &auditing.User{
				EMail:   "mail@user",
				Name:    "",
				Tenant:  "user@github",
				Subject: "user@github",
			},
		},
		{
			name: "project request",
			reqFn: func(ctx context.Context, c client.Client) error {
				_, err := c.Apiv2().IP().Get(ctx, &apiv2.IPServiceGetRequest{
					Project: "a-project",
				})
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
					Email: new("mail@t1"),
				},
			},
			existingProjects: []*apiv2.ProjectServiceCreateRequest{
				{
					Name:  "a-project",
					Login: "t1",
				},
			},
			wantUser: &auditing.User{
				EMail:   "mail@t1",
				Name:    "",
				Tenant:  "t1",
				Project: "a-project",
				Subject: "user@github",
			},
		},
		{
			name: "tenant request",
			reqFn: func(ctx context.Context, c client.Client) error {
				_, err := c.Apiv2().Tenant().Get(ctx, &apiv2.TenantServiceGetRequest{
					Login: "a-tenant",
				})
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
					Email: new("mail@tenant-a"),
				},
			},
			wantUser: &auditing.User{
				EMail:   "mail@tenant-a",
				Name:    "",
				Tenant:  "a-tenant",
				Subject: "user@github",
			},
		},
		{
			name: "admin list tenant request",
			reqFn: func(ctx context.Context, c client.Client) error {
				_, err := c.Adminv2().Tenant().List(ctx, &adminv2.TenantServiceListRequest{})
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
					Email: new("mail@github"),
				},
			},
			wantUser: &auditing.User{
				EMail:   "mail@github",
				Name:    "",
				Tenant:  "",
				Subject: "user@github",
			},
		},

		{
			name: "machine register request",
			reqFn: func(ctx context.Context, c client.Client) error {
				_, err := c.Infrav2().Boot().Register(ctx, &infrav2.BootServiceRegisterRequest{
					Uuid: "7c042d8c-2ee1-4c71-b4e9-82953fb9bd94",
				})
				return err
			},
			token: &apiv2.Token{
				User: "7c042d8c-2ee1-4c71-b4e9-82953fb9bd94@metal-stack.dev",
			},
			method:  "/metalstack.infra.v2.BootService/Register",
			handler: handler[infrav2.BootServiceRegisterRequest, infrav2.BootServiceRegisterResponse](),
			existingTenants: []*apiv2.TenantServiceCreateRequest{
				{
					Name:  "user@github",
					Email: new("mail@github"),
				},
			},
			wantUser: &auditing.User{
				EMail:   "",
				Name:    "7c042d8c-2ee1-4c71-b4e9-82953fb9bd94@metal-stack.dev",
				Tenant:  "",
				Subject: "7c042d8c-2ee1-4c71-b4e9-82953fb9bd94",
			},
		},
		{
			name: "pixie sends a machine boot request",
			reqFn: func(ctx context.Context, c client.Client) error {
				_, err := c.Infrav2().Boot().Boot(ctx, &infrav2.BootServiceBootRequest{})
				return err
			},
			token: &apiv2.Token{
				User: "pixiecore@metal-stack.dev",
			},
			method:  "/metalstack.infra.v2.BootService/Boot",
			handler: handler[infrav2.BootServiceBootRequest, infrav2.BootServiceBootResponse](),
			existingTenants: []*apiv2.TenantServiceCreateRequest{
				{
					Name:  "user@github",
					Email: new("mail@github"),
				},
			},
			wantUser: &auditing.User{
				EMail:   "",
				Name:    "pixiecore@metal-stack.dev",
				Tenant:  "",
				Subject: "pixiecore@metal-stack.dev",
			},
		},
		{
			name: "pixie sends a machine dhcp request",
			reqFn: func(ctx context.Context, c client.Client) error {
				_, err := c.Infrav2().Boot().Dhcp(ctx, &infrav2.BootServiceDhcpRequest{
					Uuid:      "7c042d8c-2ee1-4c71-b4e9-82953fb9bd94",
					Partition: "partition-1",
				})
				return err
			},
			token: &apiv2.Token{
				User: "pixiecore@metal-stack.dev",
			},
			method:  "/metalstack.infra.v2.BootService/Dhcp",
			handler: handler[infrav2.BootServiceDhcpRequest, infrav2.BootServiceDhcpResponse](),
			existingTenants: []*apiv2.TenantServiceCreateRequest{
				{
					Name:  "user@github",
					Email: new("mail@github"),
				},
			},
			wantUser: &auditing.User{
				EMail:   "",
				Name:    "pixiecore@metal-stack.dev",
				Tenant:  "",
				Subject: "pixiecore@metal-stack.dev",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				tenantInterceptor = NewInterceptor(slog.Default(), testStore.GetTenantApiserverClient())
				called            = false

				interceptors = []connect.ServerInterceptor{
					(&tokenInjector{token: tt.token}).Interceptor(),
					tenantInterceptor.Interceptor(),
				}
			)

			test.CreateTenants(t, testStore, tt.existingTenants)
			test.CreateProjects(t, testStore, tt.existingProjects)

			defer func() {
				testStore.DeleteProjects()
				testStore.DeleteTenants()
			}()

			server := connect.NewServer(interceptors...)
			server.Register(tt.handler(t, tt.method, func(ctx context.Context) {
				called = true

				user := auditing.GetUserFromContext(ctx)
				assert.Equal(t, tt.wantUser, user)
			}))

			mux := http.NewServeMux()
			connecthttp.Mount(mux, server)

			httpServer := httptest.NewServer(mux)
			defer httpServer.Close()

			c, err := client.New(&client.DialConfig{
				BaseURL: httpServer.URL,
				Log:     log,
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
	return func(t *testing.T, procedure string, test func(context.Context)) connect.Method {
		return connect.Method{
			Spec: connect.Spec{
				Procedure:  procedure,
				StreamType: connect.StreamTypeUnary,
				Schema:     methodDescriptor(t, procedure),
			},
			Handler: func(ctx context.Context, _ connect.Spec, stream connect.ServerStream) error {
				var req Req
				if err := stream.Receive(&req); err != nil {
					return err
				}
				test(ctx)
				var zero Resp
				return stream.Send(&zero)
			},
		}
	}
}

func methodDescriptor(t *testing.T, procedure string) protoreflect.MethodDescriptor {
	t.Helper()

	serviceName, methodName, ok := strings.Cut(strings.TrimPrefix(procedure, "/"), "/")
	require.True(t, ok, "invalid procedure %q", procedure)

	descriptor, err := protoregistry.GlobalFiles.FindDescriptorByName(protoreflect.FullName(serviceName))
	require.NoError(t, err)

	service, ok := descriptor.(protoreflect.ServiceDescriptor)
	require.True(t, ok, "descriptor for %q is not a service", serviceName)

	method := service.Methods().ByName(protoreflect.Name(methodName))
	require.NotNil(t, method, "method %q not found in service %q", methodName, serviceName)

	return method
}

type tokenInjector struct {
	token *apiv2.Token
}

func (t *tokenInjector) Interceptor() connect.ServerInterceptor {
	return func(next connect.ServerFunc) connect.ServerFunc {
		return func(ctx context.Context, spec connect.Spec, stream connect.ServerStream) error {
			if t.token != nil {
				ctx = token.ContextWithToken(ctx, t.token)
			}
			return next(ctx, spec, stream)
		}
	}
}
