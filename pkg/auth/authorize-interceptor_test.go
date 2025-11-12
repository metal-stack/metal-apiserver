package auth

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"connectrpc.com/connect"
	"github.com/metal-stack/api/go/client"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	"github.com/metal-stack/metal-apiserver/pkg/repository"
	"github.com/metal-stack/metal-apiserver/pkg/test"
	"github.com/metal-stack/metal-apiserver/pkg/token"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type interceptorTestFn func(string, []connect.Interceptor, func(context.Context)) *connect.Handler

func Test_authorizeInterceptor_WrapUnary(t *testing.T) {
	t.Parallel()

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	testStore, closer := test.StartRepositoryWithCleanup(t, log, test.WithCockroach(true))
	defer closer()

	test.CreateTenants(t, testStore, []*apiv2.TenantServiceCreateRequest{
		{Name: "john.doe@github.com"},
	})
	test.CreateTenantMemberships(t, testStore, "john.doe@github.com", []*repository.TenantMemberCreateRequest{
		{MemberID: "john.doe@github.com", Role: apiv2.TenantRole_TENANT_ROLE_OWNER},
	})
	projectMap := test.CreateProjects(t, testStore.Store, []*apiv2.ProjectServiceCreateRequest{
		{Login: "john.doe@github.com"},
	})

	tests := []struct {
		name    string
		reqFn   func(ctx context.Context, c client.Client) error
		method  string
		handler interceptorTestFn
		token   *apiv2.Token

		wantErr error
	}{

		{
			name:    "machine get not allowed, nil token",
			method:  "/metalstack.api.v2.MachineService/Get",
			handler: handler[apiv2.MachineServiceGetRequest, apiv2.MachineServiceGetResponse](),
			reqFn: func(ctx context.Context, c client.Client) error {
				_, err := c.Apiv2().Machine().Get(ctx, &apiv2.MachineServiceGetRequest{})
				return err
			},
			token:   nil,
			wantErr: errorutil.PermissionDenied("access to:\"/metalstack.api.v2.MachineService/Get\" is not allowed because it is not part of the token permissions"),
		},
		{
			name:    "machine get not allowed, no token",
			method:  "/metalstack.api.v2.MachineService/Get",
			handler: handler[apiv2.MachineServiceGetRequest, apiv2.MachineServiceGetResponse](),
			reqFn: func(ctx context.Context, c client.Client) error {
				_, err := c.Apiv2().Machine().Get(ctx, &apiv2.MachineServiceGetRequest{})
				return err
			},
			token:   &apiv2.Token{},
			wantErr: errorutil.PermissionDenied("access to:\"/metalstack.api.v2.MachineService/Get\" is not allowed because it is not part of the token permissions"),
		},
		{
			name:    "machine get allowed with API token",
			method:  "/metalstack.api.v2.MachineService/Get",
			handler: handler[apiv2.MachineServiceGetRequest, apiv2.MachineServiceGetResponse](),
			reqFn: func(ctx context.Context, c client.Client) error {
				_, err := c.Apiv2().Machine().Get(ctx, &apiv2.MachineServiceGetRequest{Project: "john.doe-project"})
				return err
			},
			token: &apiv2.Token{
				User:      "john.doe@github.com",
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
				Permissions: []*apiv2.MethodPermission{
					{
						Subject: "john.doe-project",
						Methods: []string{"/metalstack.api.v2.MachineService/Get"},
					},
				},
			},
		},
		{
			name:    "machine not known with API token",
			method:  "/metalstack.api.v2.MachineService/Gest",
			handler: handler[apiv2.MachineServiceGetRequest, apiv2.MachineServiceGetResponse](),
			reqFn: func(ctx context.Context, c client.Client) error {
				_, err := c.Apiv2().Machine().Get(ctx, &apiv2.MachineServiceGetRequest{Project: "john.doe-project"})
				return err
			},
			token: &apiv2.Token{
				User:      "john.doe@github.com",
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
				Permissions: []*apiv2.MethodPermission{
					{
						Subject: "john.doe-project",
						Methods: []string{"/metalstack.api.v2.MachineService/Get"},
					},
				},
			},
			wantErr: errors.New("unimplemented: 404 Not Found"),
		},
		{
			name:    "machine get not allowed with API token",
			method:  "/metalstack.api.v2.MachineService/Get",
			handler: handler[apiv2.MachineServiceGetRequest, apiv2.MachineServiceGetResponse](),
			reqFn: func(ctx context.Context, c client.Client) error {
				_, err := c.Apiv2().Machine().Get(ctx, &apiv2.MachineServiceGetRequest{Project: "john.doe-project"})
				return err
			},
			token: &apiv2.Token{
				User:      "john.doe@github.com",
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
				Permissions: []*apiv2.MethodPermission{
					{
						Subject: "john.doe-project",
						Methods: []string{"/metalstack.api.v2.MachineService/List"},
					},
				},
			},
			wantErr: errorutil.PermissionDenied("access to:\"/metalstack.api.v2.MachineService/Get\" is not allowed because it is not part of the token permissions"),
		},
		{
			name:    "machine list allowed with API token",
			method:  "/metalstack.api.v2.MachineService/List",
			handler: handler[apiv2.MachineServiceListRequest, apiv2.MachineServiceListResponse](),
			reqFn: func(ctx context.Context, c client.Client) error {
				_, err := c.Apiv2().Machine().List(ctx, &apiv2.MachineServiceListRequest{Project: "john.doe-project"})
				return err
			},
			token: &apiv2.Token{
				User:      "john.doe@github.com",
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
				Permissions: []*apiv2.MethodPermission{
					{
						Subject: "john.doe-project",
						Methods: []string{"/metalstack.api.v2.MachineService/List"},
					},
				},
			},
			wantErr: nil,
		},
		{
			name:    "machine create allowed with API token",
			method:  "/metalstack.api.v2.MachineService/Create",
			handler: handler[apiv2.MachineServiceCreateRequest, apiv2.MachineServiceCreateResponse](),
			reqFn: func(ctx context.Context, c client.Client) error {
				_, err := c.Apiv2().Machine().Create(ctx, &apiv2.MachineServiceCreateRequest{Project: "john.doe-project"})
				return err
			},
			token: &apiv2.Token{
				User:      "john.doe@github.com",
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
				Permissions: []*apiv2.MethodPermission{
					{
						Subject: "john.doe-project",
						Methods: []string{
							"/metalstack.api.v2.MachineService/Create",
							"/metalstack.api.v2.MachineService/List",
						},
					},
				},
			},
			wantErr: nil,
		},
		{
			name:    "machine create allowed with USER token",
			method:  "/metalstack.api.v2.MachineService/Create",
			handler: handler[apiv2.MachineServiceCreateRequest, apiv2.MachineServiceCreateResponse](),
			reqFn: func(ctx context.Context, c client.Client) error {
				_, err := c.Apiv2().Machine().Create(ctx, &apiv2.MachineServiceCreateRequest{Project: projectMap["john.doe@github.com"]})
				return err
			},
			token: &apiv2.Token{
				User:      "john.doe@github.com",
				TokenType: apiv2.TokenType_TOKEN_TYPE_USER,
				ProjectRoles: map[string]apiv2.ProjectRole{
					projectMap["john.doe@github.com"]: apiv2.ProjectRole_PROJECT_ROLE_OWNER,
				},
			},
			wantErr: nil,
		},
		{
			name:    "machine create not allowed with USER token, wrong project",
			method:  "/metalstack.api.v2.MachineService/Create",
			handler: handler[apiv2.MachineServiceCreateRequest, apiv2.MachineServiceCreateResponse](),
			reqFn: func(ctx context.Context, c client.Client) error {
				_, err := c.Apiv2().Machine().Create(ctx, &apiv2.MachineServiceCreateRequest{Project: "unknown project"})
				return err
			},
			token: &apiv2.Token{
				User:      "john.doe@github.com",
				TokenType: apiv2.TokenType_TOKEN_TYPE_USER,
				ProjectRoles: map[string]apiv2.ProjectRole{
					projectMap["john.doe@github.com"]: apiv2.ProjectRole_PROJECT_ROLE_OWNER,
				},
			},
			wantErr: errorutil.PermissionDenied("access to:\"/metalstack.api.v2.MachineService/Create\" with subject:\"unknown project\" is not allowed because it is not part of the token permissions"),
		},

		// {
		// 	name:    "admin api tenantlist is not allowed with MethodPermissions",
		// 	subject: "john.doe@github",
		// 	method:  "/metalstack.admin.v2.TenantService/List",
		// 	req:     adminv2.TenantServiceListRequest{},
		// 	permissions: []*apiv2.MethodPermission{
		// 		{
		// 			Subject: "john.doe@github",
		// 			Methods: []string{"/metalstack.admin.v2.TenantService/List"},
		// 		},
		// 	},
		// 	wantErr: errorutil.PermissionDenied("not allowed to call: /metalstack.admin.v2.TenantService/List"),
		// },
		// {
		// 	name:        "admin api tenantlist is allowed",
		// 	subject:     "john.doe@github",
		// 	method:      "/metalstack.admin.v2.TenantService/List",
		// 	req:         adminv2.TenantServiceListRequest{},
		// 	permissions: []*apiv2.MethodPermission{},
		// },
		// {
		// 	name:        "admin api tenantlist is not allowed because he is not in the list of allowed admin subjects",
		// 	subject:     "hein.bloed@github",
		// 	method:      "/metalstack.admin.v2.TenantService/List",
		// 	req:         adminv2.TenantServiceListRequest{},
		// 	permissions: []*apiv2.MethodPermission{},
		// 	wantErr:     errorutil.PermissionDenied("not allowed to call: /metalstack.admin.v2.TenantService/List"),
		// },
		// {
		// 	name:        "admin editor accessed api/v1 methods tenant invite is allowed",
		// 	subject:     "john.doe@github",
		// 	method:      "/metalstack.api.v2.TenantService/Invite",
		// 	req:         apiv2.TenantServiceInvitesListRequest{},
		// 	permissions: []*apiv2.MethodPermission{},
		// },
		// {
		// 	name:        "admin viewer accessed api/v1 methods tenant invite is allowed",
		// 	subject:     "john.doe@github",
		// 	method:      "/metalstack.api.v2.TenantService/Invite",
		// 	req:         apiv2.TenantServiceInvitesListRequest{},
		// 	permissions: []*apiv2.MethodPermission{},
		// 	wantErr:     errorutil.PermissionDenied("not allowed to call: /metalstack.api.v2.TenantService/Invite"),
		// },
		// {
		// 	name:        "admin editor can access api/v1 self methods",
		// 	subject:     "john.doe@github",
		// 	method:      "/metalstack.api.v2.TenantService/InviteGet",
		// 	req:         apiv2.TenantServiceInviteGetRequest{},
		// 	permissions: []*apiv2.MethodPermission{},
		// },
		// // FIXME more admin roles defined in proto must be checked/implemented
		// {
		// 	name:        "ip get allowed for owner",
		// 	subject:     "john.doe@github",
		// 	method:      "/metalstack.api.v2.IPService/Get",
		// 	req:         apiv2.IPServiceGetRequest{Project: "project-a"},
		// 	permissions: []*apiv2.MethodPermission{},
		// 	projectsAndTenants: &request.ProjectsAndTenants{
		// 		ProjectRoles: map[string]apiv2.ProjectRole{
		// 			"project-a": apiv2.ProjectRole_PROJECT_ROLE_OWNER,
		// 		},
		// 	},
		// 	projectRoles: map[string]apiv2.ProjectRole{
		// 		"project-a": apiv2.ProjectRole_PROJECT_ROLE_OWNER,
		// 	},
		// },
		// {
		// 	name:        "ip get allowed for viewer",
		// 	subject:     "john.doe@github",
		// 	method:      "/metalstack.api.v2.IPService/Get",
		// 	req:         apiv2.IPServiceGetRequest{Project: "project-a"},
		// 	permissions: []*apiv2.MethodPermission{},
		// 	projectsAndTenants: &request.ProjectsAndTenants{
		// 		ProjectRoles: map[string]apiv2.ProjectRole{
		// 			"project-a": apiv2.ProjectRole_PROJECT_ROLE_VIEWER,
		// 		},
		// 	},
		// 	projectRoles: map[string]apiv2.ProjectRole{
		// 		"project-a": apiv2.ProjectRole_PROJECT_ROLE_VIEWER,
		// 	},
		// },
		// {
		// 	name:        "ip get not allowed, wrong project requested",
		// 	subject:     "john.doe@github",
		// 	method:      "/metalstack.api.v2.IPService/Get",
		// 	req:         apiv2.IPServiceGetRequest{Project: "project-b"},
		// 	permissions: []*apiv2.MethodPermission{},
		// 	projectRoles: map[string]apiv2.ProjectRole{
		// 		"project-a": apiv2.ProjectRole_PROJECT_ROLE_VIEWER,
		// 	},
		// 	wantErr: errorutil.PermissionDenied("not allowed to call: /metalstack.api.v2.IPService/Get"),
		// },
		// {
		// 	name:        "ip create allowed for owner",
		// 	subject:     "john.doe@github",
		// 	method:      "/metalstack.api.v2.IPService/Create",
		// 	req:         apiv2.IPServiceCreateRequest{Project: "project-a"},
		// 	permissions: []*apiv2.MethodPermission{},
		// 	projectsAndTenants: &request.ProjectsAndTenants{
		// 		ProjectRoles: map[string]apiv2.ProjectRole{
		// 			"project-a": apiv2.ProjectRole_PROJECT_ROLE_OWNER,
		// 		},
		// 	},
		// 	projectRoles: map[string]apiv2.ProjectRole{
		// 		"project-a": apiv2.ProjectRole_PROJECT_ROLE_OWNER,
		// 	},
		// },
		// {
		// 	name:        "ip create not allowed for viewer",
		// 	subject:     "john.doe@github",
		// 	method:      "/metalstack.api.v2.IPService/Create",
		// 	req:         apiv2.IPServiceCreateRequest{Project: "project-a"},
		// 	permissions: []*apiv2.MethodPermission{},
		// 	projectRoles: map[string]apiv2.ProjectRole{
		// 		"project-a": apiv2.ProjectRole_PROJECT_ROLE_VIEWER,
		// 	},
		// 	wantErr: errorutil.PermissionDenied("not allowed to call: /metalstack.api.v2.IPService/Create"),
		// },
		// {
		// 	name:    "version service allowed without token because it is public visibility",
		// 	subject: "",
		// 	method:  "/metalstack.api.v2.VersionService/Get",
		// 	req:     apiv2.VersionServiceGetRequest{},
		// 	userJwtMutateFn: func(_ *testing.T, _ string) string {
		// 		return ""
		// 	},
		// },
		// {
		// 	name:    "health service allowed without token because it is public visibility",
		// 	subject: "",
		// 	method:  "/metalstack.api.v2.HealthService/Get",
		// 	req:     apiv2.HealthServiceGetRequest{},
		// 	userJwtMutateFn: func(_ *testing.T, _ string) string {
		// 		return ""
		// 	},
		// },
		// {
		// 	name:    "token service has visibility self",
		// 	subject: "john.doe@github",
		// 	method:  "/metalstack.api.v2.TokenService/Create",
		// 	req:     apiv2.TokenServiceCreateRequest{},
		// 	projectsAndTenants: &request.ProjectsAndTenants{
		// 		TenantRoles: map[string]apiv2.TenantRole{
		// 			"john.doe@github": apiv2.TenantRole_TENANT_ROLE_OWNER,
		// 		},
		// 	},
		// 	tenantRoles: map[string]apiv2.TenantRole{
		// 		"john.doe@github": apiv2.TenantRole_TENANT_ROLE_OWNER,
		// 	},
		// },
		// {
		// 	name:    "token service malformed token",
		// 	subject: "john.doe@github",
		// 	method:  "/metalstack.api.v2.TokenService/Create",
		// 	req:     apiv2.TokenServiceCreateRequest{},
		// 	userJwtMutateFn: func(_ *testing.T, jwt string) string {
		// 		return jwt + "foo"
		// 	},
		// 	tenantRoles: map[string]apiv2.TenantRole{
		// 		"john.doe@github": apiv2.TenantRole_TENANT_ROLE_OWNER,
		// 	},
		// 	wantErr: errorutil.Unauthenticated("invalid token"),
		// },
		// {
		// 	name:    "project list service has visibility self but wrong methodpermissions",
		// 	subject: "john.doe@github",
		// 	method:  "/metalstack.api.v2.ProjectService/List",
		// 	req:     apiv2.ProjectServiceListRequest{},
		// 	projectsAndTenants: &request.ProjectsAndTenants{
		// 		TenantRoles: map[string]apiv2.TenantRole{
		// 			"john.doe@github": apiv2.TenantRole_TENANT_ROLE_OWNER,
		// 		},
		// 	},
		// 	permissions: []*apiv2.MethodPermission{
		// 		{
		// 			Subject: "a-project",
		// 			Methods: []string{"/metalstack.api.v2.IPService/List"},
		// 		},
		// 	},
		// 	wantErr: errorutil.PermissionDenied("not allowed to call: /metalstack.api.v2.ProjectService/List"),
		// },
		// {
		// 	name:      "project list service has visibility self and console token",
		// 	subject:   "john.doe@github",
		// 	method:    "/metalstack.api.v2.ProjectService/List",
		// 	tokenType: apiv2.TokenType_TOKEN_TYPE_USER,
		// 	req:       apiv2.ProjectServiceListRequest{},
		// 	projectsAndTenants: &request.ProjectsAndTenants{
		// 		TenantRoles: map[string]apiv2.TenantRole{
		// 			"john.doe@github": apiv2.TenantRole_TENANT_ROLE_OWNER,
		// 		},
		// 	},
		// },
		// {
		// 	name:      "project list service has visibility self with api token and proper method permissions",
		// 	subject:   "john.doe@github",
		// 	method:    "/metalstack.api.v2.ProjectService/List",
		// 	tokenType: apiv2.TokenType_TOKEN_TYPE_API,
		// 	req:       apiv2.ProjectServiceListRequest{},
		// 	// FIXME this is weird, if a api token is created for specific methods, but still tenant or project roles are defined
		// 	// self methods can not be called
		// 	// projectsAndTenants: &putil.ProjectsAndTenants{
		// 	// 	TenantRoles: map[string]v2.TenantRole{
		// 	// 		"john.doe@github":apiv2.TenantRole_TENANT_ROLE_OWNER,
		// 	// 	},
		// 	// },
		// 	permissions: []*apiv2.MethodPermission{
		// 		{
		// 			Methods: []string{"/metalstack.api.v2.ProjectService/List"},
		// 		},
		// 	},
		// },
		// {
		// 	name:    "project list service has visibility self but token has not permissions",
		// 	subject: "john.doe@github",
		// 	method:  "/metalstack.api.v2.ProjectService/List",
		// 	req:     apiv2.ProjectServiceListRequest{},
		// 	wantErr: errorutil.PermissionDenied("not allowed to call: /metalstack.api.v2.ProjectService/List"),
		// },
		// {
		// 	name:    "project get service has not visibility self",
		// 	subject: "john.doe@github",
		// 	method:  "/metalstack.api.v2.ProjectService/Get",
		// 	req:     apiv2.ProjectServiceGetRequest{Project: "a-project"},
		// 	permissions: []*apiv2.MethodPermission{
		// 		{
		// 			Subject: "a-project",
		// 			Methods: []string{"/metalstack.api.v2.IPService/List"},
		// 		},
		// 	},
		// 	wantErr: errorutil.PermissionDenied("not allowed to call: /metalstack.api.v2.ProjectService/Get"),
		// },
		// {
		// 	name:      "access project with console token",
		// 	subject:   "john.doe@github",
		// 	method:    "/metalstack.api.v2.ProjectService/Get",
		// 	req:       apiv2.ProjectServiceGetRequest{Project: "project-a"},
		// 	tokenType: apiv2.TokenType_TOKEN_TYPE_USER,
		// 	projectsAndTenants: &request.ProjectsAndTenants{
		// 		ProjectRoles: map[string]apiv2.ProjectRole{
		// 			"project-a": apiv2.ProjectRole_PROJECT_ROLE_OWNER,
		// 		},
		// 	},
		// },
		// {
		// 	name:      "metal-image-cache-sync token works",
		// 	subject:   "metal-image-cache-sync@metal-stack.io",
		// 	method:    "/metalstack.api.v2.ImageService/List",
		// 	req:       apiv2.ImageServiceListRequest{},
		// 	tokenType: apiv2.TokenType_TOKEN_TYPE_API,
		// 	permissions: []*apiv2.MethodPermission{
		// 		{
		// 			Subject: "a-project",
		// 			Methods: []string{
		// 				"/metalstack.api.v2.ImageService/List",
		// 				"/metalstack.api.v2.PartitionService/List",
		// 				"/metalstack.api.v2.TokenService/Refresh",
		// 			},
		// 		},
		// 	},
		// 	wantErr: nil,
		// },
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				authorizeinterceptor = NewAuthorizeInterceptor(log, testStore.Store)
				called               = false

				interceptors = []connect.Interceptor{
					&tokenInjector{token: tt.token},
					authorizeinterceptor,
				}
			)

			// test.CreateTenants(t, testStore, tt.existingTenants)
			// test.CreateProjects(t, testStore.Store, tt.existingProjects)

			// defer func() {
			// 	testStore.DeleteProjects()
			// 	testStore.DeleteTenants()
			// }()

			mux := http.NewServeMux()
			mux.Handle(tt.method, tt.handler(tt.method, interceptors, func(ctx context.Context) {
				called = true
			}))

			server := httptest.NewServer(mux)
			defer server.Close()

			c, err := client.New(&client.DialConfig{
				BaseURL: server.URL,
				Log:     log,
			})
			require.NoError(t, err)

			require.NotNil(t, tt.reqFn)
			err = tt.reqFn(t.Context(), c)
			if tt.wantErr != nil {
				require.EqualError(t, err, tt.wantErr.Error())
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
