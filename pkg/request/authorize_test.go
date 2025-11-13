package request

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"connectrpc.com/connect"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/api/go/metalstack/api/v2/apiv2connect"
	"github.com/metal-stack/api/go/metalstack/infra/v2/infrav2connect"
	"github.com/metal-stack/metal-apiserver/pkg/repository"
	"github.com/stretchr/testify/require"
)

func Test_authorizer_allowed(t *testing.T) {
	tests := []struct {
		name               string
		token              *apiv2.Token
		projectsAndTenants *repository.ProjectsAndTenants
		method             string
		subject            string
		wantErr            error
	}{
		{
			name:    "nil token, access to public endpoint allowed",
			token:   nil,
			method:  "/metalstack.api.v2.VersionService/Get",
			wantErr: nil,
		},
		{
			name:    "nil token, access to non public endpoint is not allowed",
			token:   nil,
			method:  "/metalstack.api.v2.PartitionService/List",
			wantErr: errors.New("permission_denied: access to:\"/metalstack.api.v2.PartitionService/List\" is not allowed because it is not part of the token permissions"),
		},
		{
			name: "one permission, api token",
			token: &apiv2.Token{
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
				Permissions: []*apiv2.MethodPermission{
					{Subject: "project-a", Methods: []string{"/metalstack.api.v2.IPService/Get"}},
				},
			},
			method:  "/metalstack.api.v2.IPService/Get",
			subject: "project-a",
			wantErr: nil,
		},
		{
			name: "one infra permission, api token",
			token: &apiv2.Token{
				Permissions: []*apiv2.MethodPermission{
					{Subject: "*", Methods: []string{infrav2connect.SwitchServiceRegisterProcedure}},
				},
			},
			method:  "/metalstack.infra.v2.SwitchService/Register",
			subject: "switch01",
			wantErr: nil,
		},
		{
			name: "one permission, api token, access not allowed",
			token: &apiv2.Token{
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
				Permissions: []*apiv2.MethodPermission{
					{Subject: "project-a", Methods: []string{"/metalstack.api.v2.IPService/Get"}},
				},
			},
			method:  "/metalstack.api.v2.IPService/Create",
			subject: "project-a",
			wantErr: errors.New("permission_denied: access to:\"/metalstack.api.v2.IPService/Create\" is not allowed because it is not part of the token permissions"),
		},
		{
			name: "one permission, api token, access not allowed",
			token: &apiv2.Token{
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
				Permissions: []*apiv2.MethodPermission{
					{Subject: "project-a", Methods: []string{"/metalstack.api.v2.IPService/Get"}},
				},
			},
			method:  "/metalstack.api.v2.IPService/Get",
			subject: "project-b",
			wantErr: errors.New("permission_denied: access to:\"/metalstack.api.v2.IPService/Get\" with subject:\"project-b\" is not allowed because it is not part of the token permissions, allowed subjects are:[\"project-a\"]"),
		},
		{
			name: "admin editor access",
			token: &apiv2.Token{
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
				AdminRole: apiv2.AdminRole_ADMIN_ROLE_EDITOR.Enum(),
			},
			method:  "/metalstack.api.v2.IPService/Get",
			subject: "project-b",
			wantErr: nil,
		},
		{
			name: "admin viewer access",
			token: &apiv2.Token{
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
				AdminRole: apiv2.AdminRole_ADMIN_ROLE_VIEWER.Enum(),
			},
			method:  "/metalstack.api.v2.IPService/Get",
			subject: "project-b",
			wantErr: nil,
		},
		{
			name: "user token, tenant owner with inherited project viewer",
			token: &apiv2.Token{
				TokenType: apiv2.TokenType_TOKEN_TYPE_USER,
				TenantRoles: map[string]apiv2.TenantRole{
					"tenant-a": apiv2.TenantRole_TENANT_ROLE_OWNER,
				},
			},
			method:  "/metalstack.api.v2.IPService/Get",
			subject: "project-b",
			projectsAndTenants: &repository.ProjectsAndTenants{
				ProjectRoles: map[string]apiv2.ProjectRole{
					"project-b": apiv2.ProjectRole_PROJECT_ROLE_EDITOR,
				},
			},
			wantErr: nil,
		},
		{
			name: "api token, tenant owner with inherited project viewer",
			token: &apiv2.Token{
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
				TenantRoles: map[string]apiv2.TenantRole{
					"tenant-a": apiv2.TenantRole_TENANT_ROLE_OWNER,
				},
			},
			method:  "/metalstack.api.v2.IPService/Get",
			subject: "project-b",
			projectsAndTenants: &repository.ProjectsAndTenants{
				ProjectRoles: map[string]apiv2.ProjectRole{
					"project-b": apiv2.ProjectRole_PROJECT_ROLE_EDITOR,
				},
			},
			wantErr: errors.New(`permission_denied: access to:"/metalstack.api.v2.IPService/Get" is not allowed because it is not part of the token permissions`),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &authorizer{
				log: slog.Default(),
			}
			a.projectsAndTenantsGetter = func(ctx context.Context, userId string) (*repository.ProjectsAndTenants, error) {
				if tt.projectsAndTenants == nil {
					return &repository.ProjectsAndTenants{}, nil
				}
				return tt.projectsAndTenants, nil
			}

			gotErr := a.authorize(t.Context(), tt.token, tt.method, tt.subject)

			if tt.wantErr != nil {
				require.EqualError(t, gotErr, tt.wantErr.Error())
			} else if gotErr != nil {
				require.NoError(t, gotErr)
			}
		})
	}
}

func Test_authorizer_Allowed(t *testing.T) {
	mux := http.NewServeMux()
	mux.Handle(apiv2connect.NewIPServiceHandler(apiv2connect.UnimplementedIPServiceHandler{}))
	server := httptest.NewTLSServer(mux)
	server.EnableHTTP2 = true
	defer func() {
		server.Close()
	}()

	tests := []struct {
		name               string
		token              *apiv2.Token
		projectsAndTenants *repository.ProjectsAndTenants
		adminSubjects      []string
		req                *connect.Request[apiv2.IPServiceGetRequest]
		callFn             func()
		wantErr            error
	}{
		{
			name: "one permission, api token",
			token: &apiv2.Token{
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
				Permissions: []*apiv2.MethodPermission{
					{Subject: "project-a", Methods: []string{"/metalstack.api.v2.IPService/Get"}},
				},
			},
			req:     connect.NewRequest(&apiv2.IPServiceGetRequest{Project: "project-a"}),
			wantErr: nil,
		},
		{
			name: "one permission, api token, access not allowed",
			token: &apiv2.Token{
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
				Permissions: []*apiv2.MethodPermission{
					{Subject: "project-a", Methods: []string{"/metalstack.api.v2.IPService/Create"}},
				},
			},
			req:     connect.NewRequest(&apiv2.IPServiceGetRequest{Project: "project-a"}),
			wantErr: errors.New("permission_denied: access to:\"/metalstack.api.v2.IPService/Get\" is not allowed because it is not part of the token permissions"),
		},
		{
			name: "one permission, api token, access not allowed",
			token: &apiv2.Token{
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
				Permissions: []*apiv2.MethodPermission{
					{Subject: "project-a", Methods: []string{"/metalstack.api.v2.IPService/Get"}},
				},
			},
			req:     connect.NewRequest(&apiv2.IPServiceGetRequest{Project: "project-b"}),
			wantErr: errors.New("permission_denied: access to:\"/metalstack.api.v2.IPService/Get\" with subject:\"project-b\" is not allowed because it is not part of the token permissions"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &authorizer{
				log: slog.Default(),
			}
			a.projectsAndTenantsGetter = func(ctx context.Context, userId string) (*repository.ProjectsAndTenants, error) {
				if tt.projectsAndTenants == nil {
					return &repository.ProjectsAndTenants{}, nil
				}
				return tt.projectsAndTenants, nil
			}

			client := apiv2connect.NewIPServiceClient(server.Client(), server.URL, connect.WithInterceptors(connect.UnaryInterceptorFunc(
				func(next connect.UnaryFunc) connect.UnaryFunc {
					return connect.UnaryFunc(func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
						gotErr := a.Authorize(t.Context(), tt.token, req)
						if tt.wantErr != nil {
							require.EqualError(t, gotErr, tt.wantErr.Error())
						} else if gotErr != nil {
							require.NoError(t, gotErr)
						}
						return next(ctx, req)
					})
				},
			)))

			// Swallow response and error, comparison is done inside the interceptor
			_, _ = client.Get(t.Context(), tt.req.Msg)
		})
	}
}
