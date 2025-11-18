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
	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	"github.com/metal-stack/api/go/metalstack/admin/v2/adminv2connect"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/api/go/metalstack/api/v2/apiv2connect"
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
		{Name: "foo.bar@github.com"},
		{Name: "viewer@github.com"},
		{Name: "metal-image-cache-sync"},
	})
	test.CreateTenantMemberships(t, testStore, "john.doe@github.com", []*repository.TenantMemberCreateRequest{
		{MemberID: "john.doe@github.com", Role: apiv2.TenantRole_TENANT_ROLE_OWNER},
		{MemberID: "viewer@github.com", Role: apiv2.TenantRole_TENANT_ROLE_VIEWER},
	})
	test.CreateTenantMemberships(t, testStore, "viewer@github.com", []*repository.TenantMemberCreateRequest{
		{MemberID: "viewer@github.com", Role: apiv2.TenantRole_TENANT_ROLE_OWNER},
	})
	test.CreateTenantMemberships(t, testStore, "foo.bar@github.com", []*repository.TenantMemberCreateRequest{
		{MemberID: "foo.bar@github.com", Role: apiv2.TenantRole_TENANT_ROLE_OWNER},
	})
	test.CreateTenantMemberships(t, testStore, "metal-image-cache-sync", []*repository.TenantMemberCreateRequest{
		{MemberID: "metal-image-cache-sync", Role: apiv2.TenantRole_TENANT_ROLE_OWNER},
	})
	projectMap := test.CreateProjects(t, testStore.Store, []*apiv2.ProjectServiceCreateRequest{
		{Login: "john.doe@github.com"},
	})
	test.CreateProjectMemberships(t, testStore, projectMap["john.doe@github.com"], []*repository.ProjectMemberCreateRequest{
		{TenantId: "foo.bar@github.com", Role: apiv2.ProjectRole_PROJECT_ROLE_VIEWER},
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
			method:  apiv2connect.MachineServiceGetProcedure,
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
			method:  apiv2connect.MachineServiceGetProcedure,
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
			method:  apiv2connect.MachineServiceGetProcedure,
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
			name:    "machine gest not known with API token",
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
			method:  apiv2connect.MachineServiceGetProcedure,
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
			method:  apiv2connect.MachineServiceListProcedure,
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
			method:  apiv2connect.MachineServiceCreateProcedure,
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
			method:  apiv2connect.MachineServiceCreateProcedure,
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
			method:  apiv2connect.MachineServiceCreateProcedure,
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
			wantErr: errorutil.PermissionDenied("access to:\"/metalstack.api.v2.MachineService/Create\" with subject:\"unknown project\" is not allowed because it is not part of the token permissions, allowed subjects are:%q", []string{projectMap["john.doe@github.com"]}),
		},
		{
			name:    "admin api tenantlist is not allowed with MethodPermissions and wrong subject",
			method:  adminv2connect.TenantServiceListProcedure,
			handler: handler[adminv2.TenantServiceListRequest, adminv2.TenantServiceListResponse](),
			reqFn: func(ctx context.Context, c client.Client) error {
				_, err := c.Adminv2().Tenant().List(ctx, &adminv2.TenantServiceListRequest{})
				return err
			},
			token: &apiv2.Token{
				User:      "john.doe@github.com",
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
				Permissions: []*apiv2.MethodPermission{
					{
						Subject: "john.doe@github.com",
						Methods: []string{"/metalstack.admin.v2.TenantService/List"},
					},
				},
			},
			wantErr: errorutil.PermissionDenied("access to:\"/metalstack.admin.v2.TenantService/List\" with subject:\"\" is not allowed because it is not part of the token permissions, allowed subjects are:[\"john.doe@github.com\"]"),
		},
		{
			name:    "admin api tenantlist is allowed",
			method:  adminv2connect.TenantServiceListProcedure,
			handler: handler[adminv2.TenantServiceListRequest, adminv2.TenantServiceListResponse](),
			reqFn: func(ctx context.Context, c client.Client) error {
				_, err := c.Adminv2().Tenant().List(ctx, &adminv2.TenantServiceListRequest{})
				return err
			},
			token: &apiv2.Token{
				User:      "john.doe@github.com",
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
				AdminRole: apiv2.AdminRole_ADMIN_ROLE_EDITOR.Enum(),
			},
			wantErr: nil,
		},
		{
			name:    "admin editor api tenant invitesList is allowed",
			method:  apiv2connect.TenantServiceInvitesListProcedure,
			handler: handler[apiv2.TenantServiceInvitesListRequest, apiv2.TenantServiceInvitesListResponse](),
			reqFn: func(ctx context.Context, c client.Client) error {
				_, err := c.Apiv2().Tenant().InvitesList(ctx, &apiv2.TenantServiceInvitesListRequest{})
				return err
			},
			token: &apiv2.Token{
				User:      "john.doe@github.com",
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
				AdminRole: apiv2.AdminRole_ADMIN_ROLE_EDITOR.Enum(),
			},
			wantErr: nil,
		},
		{
			name:    "admin viewer api tenant invite is not allowed",
			method:  apiv2connect.TenantServiceInviteProcedure,
			handler: handler[apiv2.TenantServiceInviteRequest, apiv2.TenantServiceInviteResponse](),
			reqFn: func(ctx context.Context, c client.Client) error {
				_, err := c.Apiv2().Tenant().Invite(ctx, &apiv2.TenantServiceInviteRequest{})
				return err
			},
			token: &apiv2.Token{
				User:      "john.doe@github.com",
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
				AdminRole: apiv2.AdminRole_ADMIN_ROLE_VIEWER.Enum(),
			},
			wantErr: errorutil.PermissionDenied("access to:\"/metalstack.api.v2.TenantService/Invite\" is not allowed because it is not part of the token permissions"),
		},
		{
			name:    "admin viewer can access self methods",
			method:  apiv2connect.TenantServiceGetProcedure,
			handler: handler[apiv2.TenantServiceGetRequest, apiv2.TenantServiceGetResponse](),
			reqFn: func(ctx context.Context, c client.Client) error {
				_, err := c.Apiv2().Tenant().Get(ctx, &apiv2.TenantServiceGetRequest{})
				return err
			},
			token: &apiv2.Token{
				User:      "john.doe@github.com",
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
				AdminRole: apiv2.AdminRole_ADMIN_ROLE_VIEWER.Enum(),
			},
			wantErr: nil,
		},
		{
			name:    "ip get allowed for owner",
			method:  apiv2connect.IPServiceGetProcedure,
			handler: handler[apiv2.IPServiceGetRequest, apiv2.IPServiceGetResponse](),
			reqFn: func(ctx context.Context, c client.Client) error {
				_, err := c.Apiv2().IP().Get(ctx, &apiv2.IPServiceGetRequest{Project: projectMap["john.doe@github.com"]})
				return err
			},
			token: &apiv2.Token{
				User:      "john.doe@github.com",
				TokenType: apiv2.TokenType_TOKEN_TYPE_USER,
			},
			wantErr: nil,
		},
		{
			name:    "ip get allowed for viewer",
			method:  apiv2connect.IPServiceGetProcedure,
			handler: handler[apiv2.IPServiceGetRequest, apiv2.IPServiceGetResponse](),
			reqFn: func(ctx context.Context, c client.Client) error {
				_, err := c.Apiv2().IP().Get(ctx, &apiv2.IPServiceGetRequest{Project: projectMap["john.doe@github.com"]})
				return err
			},
			token: &apiv2.Token{
				User:      "foo.bar@github.com",
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
				ProjectRoles: map[string]apiv2.ProjectRole{
					projectMap["john.doe@github.com"]: apiv2.ProjectRole_PROJECT_ROLE_VIEWER,
				},
			},
			wantErr: nil,
		},
		{
			name:    "ip get allowed for viewer",
			method:  apiv2connect.IPServiceGetProcedure,
			handler: handler[apiv2.IPServiceGetRequest, apiv2.IPServiceGetResponse](),
			reqFn: func(ctx context.Context, c client.Client) error {
				_, err := c.Apiv2().IP().Get(ctx, &apiv2.IPServiceGetRequest{Project: projectMap["john.doe@github.com"]})
				return err
			},
			token: &apiv2.Token{
				User:      "foo.bar@github.com",
				TokenType: apiv2.TokenType_TOKEN_TYPE_USER,
			},
			wantErr: nil,
		},
		{
			name:    "tenant get allowed for viewer",
			method:  apiv2connect.TenantServiceGetProcedure,
			handler: handler[apiv2.TenantServiceGetRequest, apiv2.TenantServiceGetResponse](),
			reqFn: func(ctx context.Context, c client.Client) error {
				_, err := c.Apiv2().Tenant().Get(ctx, &apiv2.TenantServiceGetRequest{Login: "john.doe@github.com"})
				return err
			},
			token: &apiv2.Token{
				User:      "viewer@github.com",
				TokenType: apiv2.TokenType_TOKEN_TYPE_USER,
			},
			wantErr: nil,
		},
		{
			name:    "ip get not allowed, wrong project requested",
			method:  apiv2connect.IPServiceGetProcedure,
			handler: handler[apiv2.IPServiceGetRequest, apiv2.IPServiceGetResponse](),
			reqFn: func(ctx context.Context, c client.Client) error {
				_, err := c.Apiv2().IP().Get(ctx, &apiv2.IPServiceGetRequest{Project: "unknown-project"})
				return err
			},
			token: &apiv2.Token{
				User:      "john.doe@github.com",
				TokenType: apiv2.TokenType_TOKEN_TYPE_USER,
			},
			wantErr: errorutil.PermissionDenied("access to:\"/metalstack.api.v2.IPService/Get\" with subject:\"unknown-project\" is not allowed because it is not part of the token permissions, allowed subjects are:%q", []string{projectMap["john.doe@github.com"]}),
		},
		{
			name:    "ip create allowed for owner",
			method:  apiv2connect.IPServiceCreateProcedure,
			handler: handler[apiv2.IPServiceCreateRequest, apiv2.IPServiceCreateResponse](),
			reqFn: func(ctx context.Context, c client.Client) error {
				_, err := c.Apiv2().IP().Create(ctx, &apiv2.IPServiceCreateRequest{Project: projectMap["john.doe@github.com"]})
				return err
			},
			token: &apiv2.Token{
				User:      "john.doe@github.com",
				TokenType: apiv2.TokenType_TOKEN_TYPE_USER,
			},
			wantErr: nil,
		},
		{
			name:    "ip create not allowed for viewer",
			method:  apiv2connect.IPServiceCreateProcedure,
			handler: handler[apiv2.IPServiceCreateRequest, apiv2.IPServiceCreateResponse](),
			reqFn: func(ctx context.Context, c client.Client) error {
				_, err := c.Apiv2().IP().Create(ctx, &apiv2.IPServiceCreateRequest{Project: projectMap["john.doe@github.com"]})
				return err
			},
			token: &apiv2.Token{
				User:      "foo.bar@github.com",
				TokenType: apiv2.TokenType_TOKEN_TYPE_USER,
				ProjectRoles: map[string]apiv2.ProjectRole{
					projectMap["john.doe@github.com"]: apiv2.ProjectRole_PROJECT_ROLE_VIEWER,
				},
			},
			wantErr: errorutil.PermissionDenied("access to:\"/metalstack.api.v2.IPService/Create\" is not allowed because it is not part of the token permissions"),
		},
		{
			name:    "version service allowed without token because it is public visibility",
			method:  apiv2connect.VersionServiceGetProcedure,
			handler: handler[apiv2.VersionServiceGetRequest, apiv2.VersionServiceGetResponse](),
			reqFn: func(ctx context.Context, c client.Client) error {
				_, err := c.Apiv2().Version().Get(ctx, &apiv2.VersionServiceGetRequest{})
				return err
			},
			token:   nil,
			wantErr: nil,
		},
		{
			name:    "health service allowed without token because it is public visibility",
			method:  apiv2connect.HealthServiceGetProcedure,
			handler: handler[apiv2.HealthServiceGetRequest, apiv2.HealthServiceGetResponse](),
			reqFn: func(ctx context.Context, c client.Client) error {
				_, err := c.Apiv2().Health().Get(ctx, &apiv2.HealthServiceGetRequest{})
				return err
			},
			token:   nil,
			wantErr: nil,
		},
		{
			name:    "token service has visibility self",
			method:  apiv2connect.TokenServiceCreateProcedure,
			handler: handler[apiv2.TokenServiceCreateRequest, apiv2.TokenServiceCreateResponse](),
			reqFn: func(ctx context.Context, c client.Client) error {
				_, err := c.Apiv2().Token().Create(ctx, &apiv2.TokenServiceCreateRequest{})
				return err
			},
			token: &apiv2.Token{
				User:      "john.doe@github.com",
				TokenType: apiv2.TokenType_TOKEN_TYPE_USER,
				TenantRoles: map[string]apiv2.TenantRole{
					"john.doe@github": apiv2.TenantRole_TENANT_ROLE_OWNER,
				},
			},
			wantErr: nil,
		},
		{
			name:    "project list service has visibility self but wrong methodpermissions",
			method:  apiv2connect.ProjectServiceListProcedure,
			handler: handler[apiv2.ProjectServiceListRequest, apiv2.ProjectServiceListResponse](),
			reqFn: func(ctx context.Context, c client.Client) error {
				_, err := c.Apiv2().Project().List(ctx, &apiv2.ProjectServiceListRequest{})
				return err
			},
			token: &apiv2.Token{
				User:      "john.doe@github.com",
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
				Permissions: []*apiv2.MethodPermission{
					{
						Subject: "a-project",
						Methods: []string{"/metalstack.api.v2.IPService/List"},
					},
				},
			},
			wantErr: errorutil.PermissionDenied("access to:\"/metalstack.api.v2.ProjectService/List\" is not allowed because it is not part of the token permissions"),
		},
		{
			name:    "project list service has visibility self with api token and proper method permissions",
			method:  apiv2connect.ProjectServiceListProcedure,
			handler: handler[apiv2.ProjectServiceListRequest, apiv2.ProjectServiceListResponse](),
			reqFn: func(ctx context.Context, c client.Client) error {
				_, err := c.Apiv2().Project().List(ctx, &apiv2.ProjectServiceListRequest{})
				return err
			},
			token: &apiv2.Token{
				User:      "john.doe@github.com",
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
				Permissions: []*apiv2.MethodPermission{
					{
						Subject: "",
						Methods: []string{"/metalstack.api.v2.ProjectService/List"},
					},
				},
			},
			wantErr: nil,
		},
		{
			name:    "project list service has visibility self and console token",
			method:  apiv2connect.ProjectServiceListProcedure,
			handler: handler[apiv2.ProjectServiceListRequest, apiv2.ProjectServiceListResponse](),
			reqFn: func(ctx context.Context, c client.Client) error {
				_, err := c.Apiv2().Project().List(ctx, &apiv2.ProjectServiceListRequest{})
				return err
			},
			token: &apiv2.Token{
				User:      "john.doe@github.com",
				TokenType: apiv2.TokenType_TOKEN_TYPE_USER,
				TenantRoles: map[string]apiv2.TenantRole{
					"john.doe@github": apiv2.TenantRole_TENANT_ROLE_OWNER,
				},
			},
			wantErr: nil,
		},
		{
			name:    "project list service has visibility self but token has not permissions",
			method:  apiv2connect.ProjectServiceListProcedure,
			handler: handler[apiv2.ProjectServiceListRequest, apiv2.ProjectServiceListResponse](),
			reqFn: func(ctx context.Context, c client.Client) error {
				_, err := c.Apiv2().Project().List(ctx, &apiv2.ProjectServiceListRequest{})
				return err
			},
			token: &apiv2.Token{
				User:      "john.doe@github.com",
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
			},
			wantErr: errorutil.PermissionDenied("access to:\"/metalstack.api.v2.ProjectService/List\" is not allowed because it is not part of the token permissions"),
		},

		{
			name:    "project get service has not visibility self",
			method:  apiv2connect.ProjectServiceGetProcedure,
			handler: handler[apiv2.ProjectServiceGetRequest, apiv2.ProjectServiceGetResponse](),
			reqFn: func(ctx context.Context, c client.Client) error {
				_, err := c.Apiv2().Project().Get(ctx, &apiv2.ProjectServiceGetRequest{Project: "a-project"})
				return err
			},
			token: &apiv2.Token{
				User:      "john.doe@github.com",
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
				Permissions: []*apiv2.MethodPermission{
					{
						Subject: "a-project",
						Methods: []string{"/metalstack.api.v2.IPService/List"},
					},
				},
			},
			wantErr: errorutil.PermissionDenied("access to:\"/metalstack.api.v2.ProjectService/Get\" is not allowed because it is not part of the token permissions"),
		},
		{
			name:    "project with star permissions but not member of project in request",
			method:  apiv2connect.ProjectServiceGetProcedure,
			handler: handler[apiv2.ProjectServiceGetRequest, apiv2.ProjectServiceGetResponse](),
			reqFn: func(ctx context.Context, c client.Client) error {
				_, err := c.Apiv2().Project().Get(ctx, &apiv2.ProjectServiceGetRequest{Project: "a-project"})
				return err
			},
			token: &apiv2.Token{
				User:      "john.doe@github.com",
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
				Permissions: []*apiv2.MethodPermission{
					{
						Subject: "*",
						Methods: []string{"/metalstack.api.v2.ProjectService/Get"},
					},
				},
			},
			wantErr: errorutil.PermissionDenied("access to:\"/metalstack.api.v2.ProjectService/Get\" with subject:\"a-project\" is not allowed because it is not part of the token permissions, allowed subjects are:%q", []string{projectMap["john.doe@github.com"]}),
		},
		{
			name:    "access project with console token",
			method:  apiv2connect.ProjectServiceGetProcedure,
			handler: handler[apiv2.ProjectServiceGetRequest, apiv2.ProjectServiceGetResponse](),
			reqFn: func(ctx context.Context, c client.Client) error {
				_, err := c.Apiv2().Project().Get(ctx, &apiv2.ProjectServiceGetRequest{Project: projectMap["john.doe@github.com"]})
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
			name:    "metal-image-cache-sync token works",
			method:  apiv2connect.ImageServiceListProcedure,
			handler: handler[apiv2.ImageServiceListRequest, apiv2.ImageServiceListResponse](),
			reqFn: func(ctx context.Context, c client.Client) error {
				_, err := c.Apiv2().Image().List(ctx, &apiv2.ImageServiceListRequest{})
				return err
			},
			token: &apiv2.Token{
				User:      "metal-image-cache-sync",
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
				Permissions: []*apiv2.MethodPermission{
					{
						Subject: "",
						Methods: []string{
							"/metalstack.api.v2.ImageService/List",
							"/metalstack.api.v2.PartitionService/List",
							"/metalstack.api.v2.TokenService/Refresh",
						},
					},
				},
			},
			wantErr: nil,
		},
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
