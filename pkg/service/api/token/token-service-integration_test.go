package token_test

import (
	"context"
	"log/slog"
	"testing"

	"buf.build/go/protovalidate"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/metal-stack/api/go/errorutil"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/repository/api"
	tokenservice "github.com/metal-stack/metal-apiserver/pkg/service/api/token"
	"github.com/metal-stack/metal-apiserver/pkg/test"
	"github.com/metal-stack/metal-apiserver/pkg/token"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/testing/protocmp"
)

var (
	kubies = "00000000-0000-0000-0000-000000000000"
)

func Test_Create(t *testing.T) {
	t.Parallel()

	log := slog.Default()

	testStore, closer := test.StartRepositoryWithCleanup(t, log, test.WithValkey(true), test.WithPostgres(true))
	defer closer()

	type state struct {
		providerTenant string
		projectRoles   map[string]apiv2.ProjectRole
		tenantRoles    map[string]apiv2.TenantRole
	}
	tests := []struct {
		name           string
		sessionToken   *apiv2.Token
		req            *apiv2.TokenServiceCreateRequest
		state          state
		wantErr        bool
		wantErrMessage string
		wantToken      *apiv2.Token
	}{
		{
			name: "can create bare token",
			sessionToken: &apiv2.Token{
				User:         "phippy",
				TokenType:    apiv2.TokenType_TOKEN_TYPE_API,
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
			},
			req: &apiv2.TokenServiceCreateRequest{
				Description: "empty token",
			},
			state: state{
				providerTenant: test.DefaultProviderTenant,
				tenantRoles:    map[string]apiv2.TenantRole{},
			},
			wantToken: &apiv2.Token{
				User:        "phippy",
				Description: "empty token",
				TokenType:   apiv2.TokenType_TOKEN_TYPE_API,
			},
		},
		{
			name: "user and token without project access cannot create project token",
			sessionToken: &apiv2.Token{
				User:         "phippy",
				TokenType:    apiv2.TokenType_TOKEN_TYPE_API,
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
			},
			req: &apiv2.TokenServiceCreateRequest{
				Description: "project token",
				ProjectRoles: map[string]apiv2.ProjectRole{
					kubies: apiv2.ProjectRole_PROJECT_ROLE_EDITOR,
				},
				TenantRoles: map[string]apiv2.TenantRole{},
			},
			state: state{
				providerTenant: test.DefaultProviderTenant,
			},
			wantErr:        true,
			wantErrMessage: `permission_denied: requested project roles are not allowed: [00000000-0000-0000-0000-000000000000]`,
		},
		{
			name: "user and token with project access can create project token",
			sessionToken: &apiv2.Token{
				User:        "phippy",
				TokenType:   apiv2.TokenType_TOKEN_TYPE_API,
				Permissions: []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{
					kubies: apiv2.ProjectRole_PROJECT_ROLE_EDITOR,
				},
				TenantRoles: map[string]apiv2.TenantRole{},
			},
			req: &apiv2.TokenServiceCreateRequest{
				Description: "project token",
				ProjectRoles: map[string]apiv2.ProjectRole{
					kubies: apiv2.ProjectRole_PROJECT_ROLE_EDITOR,
				},
				TenantRoles: map[string]apiv2.TenantRole{},
			},
			state: state{
				providerTenant: test.DefaultProviderTenant,
				projectRoles: map[string]apiv2.ProjectRole{
					kubies: apiv2.ProjectRole_PROJECT_ROLE_EDITOR,
				},
			},
			wantToken: &apiv2.Token{
				User:        "phippy",
				Description: "project token",
				TokenType:   apiv2.TokenType_TOKEN_TYPE_API,
				ProjectRoles: map[string]apiv2.ProjectRole{
					kubies: apiv2.ProjectRole_PROJECT_ROLE_EDITOR,
				},
				TenantRoles: map[string]apiv2.TenantRole{},
			},
		},
		{
			name: "user without but token with project access cannot create project token",
			sessionToken: &apiv2.Token{
				User:        "phippy",
				TokenType:   apiv2.TokenType_TOKEN_TYPE_API,
				Permissions: []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{
					kubies: apiv2.ProjectRole_PROJECT_ROLE_EDITOR,
				},
				TenantRoles: map[string]apiv2.TenantRole{},
			},
			req: &apiv2.TokenServiceCreateRequest{
				Description: "project token",
				ProjectRoles: map[string]apiv2.ProjectRole{
					kubies: apiv2.ProjectRole_PROJECT_ROLE_EDITOR,
				},
				TenantRoles: map[string]apiv2.TenantRole{},
			},
			state: state{
				providerTenant: test.DefaultProviderTenant,
				projectRoles:   map[string]apiv2.ProjectRole{},
			},
			wantErr:        true,
			wantErrMessage: `permission_denied: requested project roles are not allowed: [00000000-0000-0000-0000-000000000000]`,
		},
		{
			name: "project without but user with project access cannot create project token",
			sessionToken: &apiv2.Token{
				User:         "phippy",
				TokenType:    apiv2.TokenType_TOKEN_TYPE_API,
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
			},
			req: &apiv2.TokenServiceCreateRequest{
				Description: "project token",
				ProjectRoles: map[string]apiv2.ProjectRole{
					kubies: apiv2.ProjectRole_PROJECT_ROLE_EDITOR,
				},
				TenantRoles: map[string]apiv2.TenantRole{},
			},
			state: state{
				providerTenant: test.DefaultProviderTenant,
				projectRoles: map[string]apiv2.ProjectRole{
					kubies: apiv2.ProjectRole_PROJECT_ROLE_EDITOR,
				},
			},
			wantErr:        true,
			wantErrMessage: `permission_denied: requested project roles are not allowed: [00000000-0000-0000-0000-000000000000]`,
		},
		{
			name: "normal user with provider-tenant owner membership can create new admin editor token",
			sessionToken: &apiv2.Token{
				User:         "phippy",
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
				TokenType:    apiv2.TokenType_TOKEN_TYPE_USER,
			},
			req: &apiv2.TokenServiceCreateRequest{
				Description:  "admin token",
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
				AdminRole:    apiv2.AdminRole_ADMIN_ROLE_EDITOR.Enum(),
			},
			state: state{
				providerTenant: test.DefaultProviderTenant,
				tenantRoles: map[string]apiv2.TenantRole{
					test.DefaultProviderTenant: apiv2.TenantRole_TENANT_ROLE_OWNER,
				},
			},
			wantToken: &apiv2.Token{
				User:         "phippy",
				Description:  "admin token",
				TokenType:    apiv2.TokenType_TOKEN_TYPE_API,
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
				AdminRole:    apiv2.AdminRole_ADMIN_ROLE_EDITOR.Enum(),
			},
		},
		{
			name: "normal user with provider-tenant viewer membership can create new admin viewer token",
			sessionToken: &apiv2.Token{
				User:         "phippy",
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
				TokenType:    apiv2.TokenType_TOKEN_TYPE_USER,
			},
			req: &apiv2.TokenServiceCreateRequest{
				Description:  "admin token",
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
				AdminRole:    apiv2.AdminRole_ADMIN_ROLE_VIEWER.Enum(),
			},
			state: state{
				providerTenant: test.DefaultProviderTenant,
				tenantRoles: map[string]apiv2.TenantRole{
					test.DefaultProviderTenant: apiv2.TenantRole_TENANT_ROLE_VIEWER,
				},
			},
			wantToken: &apiv2.Token{
				User:         "phippy",
				Description:  "admin token",
				TokenType:    apiv2.TokenType_TOKEN_TYPE_API,
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
				AdminRole:    apiv2.AdminRole_ADMIN_ROLE_VIEWER.Enum(),
			},
		},
		{
			name: "provider tenant user owner can create new admin editor token",
			sessionToken: &apiv2.Token{
				User:         test.DefaultProviderTenant,
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
				TokenType:    apiv2.TokenType_TOKEN_TYPE_USER,
			},
			req: &apiv2.TokenServiceCreateRequest{
				Description:  "admin token",
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
				AdminRole:    apiv2.AdminRole_ADMIN_ROLE_EDITOR.Enum(),
			},
			state: state{
				providerTenant: test.DefaultProviderTenant,
			},
			wantToken: &apiv2.Token{
				User:         test.DefaultProviderTenant,
				Description:  "admin token",
				TokenType:    apiv2.TokenType_TOKEN_TYPE_API,
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
				AdminRole:    apiv2.AdminRole_ADMIN_ROLE_EDITOR.Enum(),
			},
		},
		{
			name: "provider tenant user viewer can create new admin viewer token",
			sessionToken: &apiv2.Token{
				User:         test.DefaultProviderTenant,
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
				TokenType:    apiv2.TokenType_TOKEN_TYPE_USER,
			},
			req: &apiv2.TokenServiceCreateRequest{
				Description:  "admin token",
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
				AdminRole:    apiv2.AdminRole_ADMIN_ROLE_VIEWER.Enum(),
			},
			state: state{
				providerTenant: test.DefaultProviderTenant,
				tenantRoles: map[string]apiv2.TenantRole{
					test.DefaultProviderTenant: apiv2.TenantRole_TENANT_ROLE_VIEWER,
				},
			},
			wantToken: &apiv2.Token{
				User:         test.DefaultProviderTenant,
				Description:  "admin token",
				TokenType:    apiv2.TokenType_TOKEN_TYPE_API,
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
				AdminRole:    apiv2.AdminRole_ADMIN_ROLE_VIEWER.Enum(),
			},
		},
		{
			name: "admin viewer cannot create admin editor token",
			sessionToken: &apiv2.Token{
				User:         "phippy",
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
				TokenType:    apiv2.TokenType_TOKEN_TYPE_USER,
			},
			req: &apiv2.TokenServiceCreateRequest{
				Description:  "admin token",
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
				AdminRole:    apiv2.AdminRole_ADMIN_ROLE_EDITOR.Enum(),
			},
			state: state{
				providerTenant: test.DefaultProviderTenant,
				tenantRoles: map[string]apiv2.TenantRole{
					test.DefaultProviderTenant: apiv2.TenantRole_TENANT_ROLE_VIEWER,
				},
			},
			wantToken:      nil,
			wantErr:        true,
			wantErrMessage: `permission_denied: your provider tenant membership only allows "ADMIN_ROLE_VIEWER", but you requested "ADMIN_ROLE_EDITOR"`,
		},
		{
			name: "normal user which is not member in the provider tenant can not create new admin viewer token",
			sessionToken: &apiv2.Token{
				User:         "phippy",
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
				TokenType:    apiv2.TokenType_TOKEN_TYPE_USER,
			},
			req: &apiv2.TokenServiceCreateRequest{
				Description:  "admin token",
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
				AdminRole:    apiv2.AdminRole_ADMIN_ROLE_VIEWER.Enum(),
			},
			state: state{
				providerTenant: "blippy",
			},
			wantToken:      nil,
			wantErr:        true,
			wantErrMessage: `permission_denied: the following method "/metalstack.admin.v2.AuditService/Get" is not allowed on any of the requested subjects: [*]`,
		},
		{
			name: "admin user and token can create new admin token",
			sessionToken: &apiv2.Token{
				User:         "phippy",
				TokenType:    apiv2.TokenType_TOKEN_TYPE_API,
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
				AdminRole:    apiv2.AdminRole_ADMIN_ROLE_EDITOR.Enum(),
			},
			req: &apiv2.TokenServiceCreateRequest{
				Description:  "admin token",
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
				AdminRole:    apiv2.AdminRole_ADMIN_ROLE_EDITOR.Enum(),
			},
			state: state{
				providerTenant: test.DefaultProviderTenant,
				tenantRoles: map[string]apiv2.TenantRole{
					test.DefaultProviderTenant: apiv2.TenantRole_TENANT_ROLE_OWNER,
				},
			},
			wantToken: &apiv2.Token{
				User:         "phippy",
				Description:  "admin token",
				TokenType:    apiv2.TokenType_TOKEN_TYPE_API,
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
				AdminRole:    apiv2.AdminRole_ADMIN_ROLE_EDITOR.Enum(),
			},
		},
		{
			name: "admin token but not user cannot create new admin token",
			sessionToken: &apiv2.Token{
				User:         "phippy",
				TokenType:    apiv2.TokenType_TOKEN_TYPE_API,
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
				AdminRole:    apiv2.AdminRole_ADMIN_ROLE_EDITOR.Enum(),
			},
			req: &apiv2.TokenServiceCreateRequest{
				Description:  "admin token",
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
				AdminRole:    apiv2.AdminRole_ADMIN_ROLE_EDITOR.Enum(),
			},
			state: state{
				providerTenant: test.DefaultProviderTenant,
			},
			wantErr:        true,
			wantErrMessage: `permission_denied: the following method "/grpc.reflection.v1.ServerReflection/ServerReflectionInfo" is not allowed on any of the requested subjects: [*]`,
		},
		{
			name: "user and token without tenant access cannot create tenant token",
			sessionToken: &apiv2.Token{
				User:         "phippy",
				TokenType:    apiv2.TokenType_TOKEN_TYPE_API,
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
			},
			req: &apiv2.TokenServiceCreateRequest{
				Description:  "project token",
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles: map[string]apiv2.TenantRole{
					"mascots": apiv2.TenantRole_TENANT_ROLE_EDITOR,
				},
			},
			state: state{
				providerTenant: test.DefaultProviderTenant,
			},
			wantErr:        true,
			wantErrMessage: `permission_denied: requested tenant roles are not allowed: [mascots]`,
		},
		{
			name: "user and token with tenant access can create tenant token",
			sessionToken: &apiv2.Token{
				User:         "phippy",
				TokenType:    apiv2.TokenType_TOKEN_TYPE_API,
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles: map[string]apiv2.TenantRole{
					"mascots": apiv2.TenantRole_TENANT_ROLE_EDITOR,
				},
			},
			req: &apiv2.TokenServiceCreateRequest{
				Description:  "project token",
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles: map[string]apiv2.TenantRole{
					"mascots": apiv2.TenantRole_TENANT_ROLE_EDITOR,
				},
			},
			state: state{
				providerTenant: test.DefaultProviderTenant,
				tenantRoles: map[string]apiv2.TenantRole{
					"mascots": apiv2.TenantRole_TENANT_ROLE_EDITOR,
				},
			},
			wantToken: &apiv2.Token{
				User:         "phippy",
				Description:  "project token",
				TokenType:    *apiv2.TokenType_TOKEN_TYPE_API.Enum(),
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles: map[string]apiv2.TenantRole{
					"mascots": apiv2.TenantRole_TENANT_ROLE_EDITOR,
				},
			},
		},
		{
			name: "user requests token for mascots but in the database does not have required tenant membership for mascots",
			sessionToken: &apiv2.Token{
				User:         "phippy",
				TokenType:    apiv2.TokenType_TOKEN_TYPE_API,
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles: map[string]apiv2.TenantRole{
					"mascots": apiv2.TenantRole_TENANT_ROLE_EDITOR,
				},
			},
			req: &apiv2.TokenServiceCreateRequest{
				Description:  "project token",
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles: map[string]apiv2.TenantRole{
					"mascots": apiv2.TenantRole_TENANT_ROLE_EDITOR,
				},
			},
			state: state{
				providerTenant: test.DefaultProviderTenant,
				projectRoles:   map[string]apiv2.ProjectRole{},
				tenantRoles: map[string]apiv2.TenantRole{
					"phippy": apiv2.TenantRole_TENANT_ROLE_OWNER,
				},
			},
			wantErr:        true,
			wantErrMessage: `permission_denied: requested tenant roles are not allowed: [mascots]`,
		},
		{
			name: "user requests token for mascots but neither has mascots in his token permissions nor in the database",
			sessionToken: &apiv2.Token{
				User:         "phippy",
				TokenType:    apiv2.TokenType_TOKEN_TYPE_API,
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles: map[string]apiv2.TenantRole{
					"phippy": apiv2.TenantRole_TENANT_ROLE_OWNER,
				},
			},
			req: &apiv2.TokenServiceCreateRequest{
				Description:  "project token",
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles: map[string]apiv2.TenantRole{
					"mascots": apiv2.TenantRole_TENANT_ROLE_EDITOR,
				},
			},
			state: state{
				providerTenant: test.DefaultProviderTenant,
				projectRoles:   map[string]apiv2.ProjectRole{},
				tenantRoles: map[string]apiv2.TenantRole{
					"phippy": apiv2.TenantRole_TENANT_ROLE_OWNER,
				},
			},
			wantErr:        true,
			wantErrMessage: `permission_denied: requested tenant roles are not allowed: [mascots]`,
		},
		{
			name: "token without but user with tenant access cannot create tenant token",
			sessionToken: &apiv2.Token{
				User:         "phippy",
				TokenType:    apiv2.TokenType_TOKEN_TYPE_API,
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
			},
			req: &apiv2.TokenServiceCreateRequest{
				Description:  "project token",
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles: map[string]apiv2.TenantRole{
					"mascots": apiv2.TenantRole_TENANT_ROLE_EDITOR,
				},
			},
			state: state{
				providerTenant: test.DefaultProviderTenant,
				projectRoles:   map[string]apiv2.ProjectRole{},
				tenantRoles: map[string]apiv2.TenantRole{
					"mascots": apiv2.TenantRole_TENANT_ROLE_EDITOR,
				},
			},
			wantErr:        true,
			wantErrMessage: `permission_denied: requested tenant roles are not allowed: [mascots]`,
		},
		{
			name: "user and token without machine access cannot create machine token",
			sessionToken: &apiv2.Token{
				User:         "phippy",
				TokenType:    apiv2.TokenType_TOKEN_TYPE_API,
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
			},
			req: &apiv2.TokenServiceCreateRequest{
				Description: "machine token",
				MachineRoles: map[string]apiv2.MachineRole{
					"de240964-ff9f-4e3d-95b2-8a96e43788f1": apiv2.MachineRole_MACHINE_ROLE_EDITOR,
				},
				TenantRoles: map[string]apiv2.TenantRole{},
			},
			state: state{
				providerTenant: test.DefaultProviderTenant,
			},
			wantErr:        true,
			wantErrMessage: `permission_denied: requested machine roles are not allowed: [de240964-ff9f-4e3d-95b2-8a96e43788f1]`,
		},
		{
			name: "user and token with machine access can create machine token",
			sessionToken: &apiv2.Token{
				User:        "pixie-core",
				TokenType:   apiv2.TokenType_TOKEN_TYPE_API,
				Permissions: []*apiv2.MethodPermission{},
				MachineRoles: map[string]apiv2.MachineRole{
					"*": apiv2.MachineRole_MACHINE_ROLE_EDITOR,
				},
				TenantRoles: map[string]apiv2.TenantRole{},
			},
			req: &apiv2.TokenServiceCreateRequest{
				Description: "machine token",
				MachineRoles: map[string]apiv2.MachineRole{
					"de240964-ff9f-4e3d-95b2-8a96e43788f1": apiv2.MachineRole_MACHINE_ROLE_EDITOR,
				},
				TenantRoles: map[string]apiv2.TenantRole{},
			},
			state: state{
				providerTenant: test.DefaultProviderTenant,
			},
			wantToken: &apiv2.Token{
				User:        "pixie-core",
				Description: "machine token",
				TokenType:   apiv2.TokenType_TOKEN_TYPE_API,
				MachineRoles: map[string]apiv2.MachineRole{
					"de240964-ff9f-4e3d-95b2-8a96e43788f1": apiv2.MachineRole_MACHINE_ROLE_EDITOR,
				},
				TenantRoles: map[string]apiv2.TenantRole{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(innerT *testing.T) {
			defer testStore.Cleanup(t)

			ctx, cancel := context.WithCancel(token.ContextWithToken(innerT.Context(), tt.sessionToken))
			defer cancel()

			test.CreateTenants(innerT, testStore, []*apiv2.TenantServiceCreateRequest{
				{
					Name: tt.sessionToken.User,
				},
			})

			// every logged in users comes with default tenant owner membership
			test.CreateTenantMemberships(innerT, testStore, tt.sessionToken.User, []*api.TenantMemberCreateRequest{
				{
					MemberID: tt.sessionToken.User,
					Role:     apiv2.TenantRole_TENANT_ROLE_OWNER,
				},
			})

			for id, perm := range tt.state.tenantRoles {
				if id != tt.sessionToken.User {
					test.CreateTenants(innerT, testStore, []*apiv2.TenantServiceCreateRequest{
						{
							Name: id,
						},
					})
				}
				test.CreateTenantMemberships(innerT, testStore, id, []*api.TenantMemberCreateRequest{
					{
						MemberID: tt.sessionToken.User,
						Role:     perm,
					},
				})
			}

			for id, perm := range tt.state.projectRoles {
				test.CreateProjects(innerT, testStore, []*apiv2.ProjectServiceCreateRequest{
					{
						Login: tt.sessionToken.User,
						Name:  id,
					},
				})
				test.CreateProjectMemberships(innerT, testStore, id, []*api.ProjectMemberCreateRequest{
					{
						TenantId: tt.sessionToken.User,
						Role:     perm,
					},
				})
			}

			service := tokenservice.New(tokenservice.Config{
				Log:  log,
				Repo: testStore.Store,
			})

			if tt.wantErr == false {
				// Execute proto based validation
				err := protovalidate.Validate(tt.req)
				require.NoError(t, err)
			}

			response, err := service.Create(ctx, tt.req)
			switch {
			case tt.wantErr && err != nil:
				if diff := cmp.Diff(tt.wantErrMessage, err.Error()); diff != "" {
					t.Errorf("diff = %s", diff)
				}
			case tt.wantErr && err == nil:
				t.Fatalf("want error %q, got response %q", tt.wantErrMessage, response)
			case err != nil:
				t.Fatalf("want response, got error %q", err)

			default:
				if response.Secret == "" {
					t.Error("response secret for token may not be empty")
				}
				require.NotNil(t, tt.wantToken, "token returned, nil expected")

				got := response.Token
				assert.Equal(t, tt.wantToken.Description, got.Description, "description")
				assert.Equal(t, tt.wantToken.User, got.User, "user id")
				assert.Equal(t, tt.wantToken.TokenType, got.TokenType, "token type")
				assert.Equal(t, tt.wantToken.AdminRole, got.AdminRole, "admin role")
				assert.Equal(t, tt.wantToken.Permissions, got.Permissions, "permissions")
				assert.Equal(t, tt.wantToken.ProjectRoles, got.ProjectRoles, "project roles")
				assert.Equal(t, tt.wantToken.TenantRoles, got.TenantRoles, "tenant roles")
				assert.Equal(t, tt.wantToken.MachineRoles, got.MachineRoles, "machine roles")
			}
		})
	}
}

func Test_Create_NoToken(t *testing.T) {
	t.Parallel()

	log := slog.Default()

	testStore, closer := test.StartRepositoryWithCleanup(t, log, test.WithValkey(true), test.WithPostgres(true))
	defer closer()

	service := tokenservice.New(tokenservice.Config{
		Log:            log,
		Repo:           testStore.Store,
		ProviderTenant: test.DefaultProviderTenant,
		Issuer:         "http://test",
	})

	_, err := service.Create(t.Context(), &apiv2.TokenServiceCreateRequest{})
	require.Error(t, err)
	require.Equal(t, "unauthenticated: no token found in request", err.Error())
}

func Test_List(t *testing.T) {
	t.Parallel()

	log := slog.Default()

	testStore, closer := test.StartRepositoryWithCleanup(t, log)
	defer closer()

	type state struct {
		existingTokens []*apiv2.Token
	}
	tests := []struct {
		name           string
		sessionToken   *apiv2.Token
		req            *apiv2.TokenServiceListRequest
		state          state
		wantErr        bool
		wantErrMessage string
		want           []*apiv2.Token
	}{
		{
			name: "no tokens",
			sessionToken: &apiv2.Token{
				User:         "phippy",
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
			},
			req:   &apiv2.TokenServiceListRequest{},
			state: state{},
			want:  nil,
		},
		{
			name: "list tokens",
			sessionToken: &apiv2.Token{
				User:         "phippy",
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
			},
			req: &apiv2.TokenServiceListRequest{},
			state: state{
				existingTokens: []*apiv2.Token{
					{
						Uuid: "c223af4d-b3f5-4df6-8815-52b80323930d",
						User: "phippy",
					},
					{
						Uuid: "8ff27ee2-209f-43e2-a15d-50143fb03229",
						User: "phippy",
					},
					{
						Uuid: "9baa8668-2212-4fa5-a2e4-167084d0552d",
						User: "not phippy",
					},
				},
			},
			want: []*apiv2.Token{
				{
					Uuid: "c223af4d-b3f5-4df6-8815-52b80323930d",
					User: "phippy",
					Meta: &apiv2.Meta{},
				},
				{
					Uuid: "8ff27ee2-209f-43e2-a15d-50143fb03229",
					User: "phippy",
					Meta: &apiv2.Meta{},
				},
			},
		},
		{
			name: "query uuid",
			sessionToken: &apiv2.Token{
				User:         "phippy",
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
			},
			req: &apiv2.TokenServiceListRequest{
				Query: &apiv2.TokenQuery{
					Uuid: new("c223af4d-b3f5-4df6-8815-52b80323930d"),
				},
			},
			state: state{
				existingTokens: []*apiv2.Token{
					{
						Uuid: "c223af4d-b3f5-4df6-8815-52b80323930d",
						User: "phippy",
					},
					{
						Uuid: "8ff27ee2-209f-43e2-a15d-50143fb03229",
						User: "phippy",
					},
					{
						Uuid: "9baa8668-2212-4fa5-a2e4-167084d0552d",
						User: "not phippy",
					},
				},
			},
			want: []*apiv2.Token{
				{
					Uuid: "c223af4d-b3f5-4df6-8815-52b80323930d",
					User: "phippy",
					Meta: &apiv2.Meta{},
				},
			},
		},
		{
			name: "query description and labels",
			sessionToken: &apiv2.Token{
				User:         "phippy",
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
			},
			req: &apiv2.TokenServiceListRequest{
				Query: &apiv2.TokenQuery{
					Labels: &apiv2.Labels{
						Labels: map[string]string{
							"a": "b",
							"c": "d",
						},
					},
					Description: new("test"),
				},
			},
			state: state{
				existingTokens: []*apiv2.Token{
					{
						Uuid:        "c223af4d-b3f5-4df6-8815-52b80323930d",
						User:        "phippy",
						Description: "test",
						Meta: &apiv2.Meta{
							Labels: &apiv2.Labels{
								Labels: map[string]string{
									"c": "d",
									"a": "b",
								},
							},
						},
					},
					{
						Uuid:        "8ff27ee2-209f-43e2-a15d-50143fb03229",
						User:        "phippy",
						Description: "nope",
						Meta: &apiv2.Meta{
							Labels: &apiv2.Labels{
								Labels: map[string]string{
									"a": "b",
									"c": "d",
								},
							},
						},
					},
					{
						Uuid:        "9baa8668-2212-4fa5-a2e4-167084d0552d",
						User:        "phippy",
						Description: "test",
						Meta: &apiv2.Meta{
							Labels: &apiv2.Labels{
								Labels: map[string]string{
									"a": "b",
									"c": "nope",
								},
							},
						},
					},
				},
			},
			want: []*apiv2.Token{
				{
					Uuid:        "c223af4d-b3f5-4df6-8815-52b80323930d",
					User:        "phippy",
					Description: "test",
					Meta: &apiv2.Meta{
						Labels: &apiv2.Labels{
							Labels: map[string]string{
								"a": "b",
								"c": "d",
							},
						},
					},
				},
			},
		},
		{
			name: "query user (does not see other users)",
			sessionToken: &apiv2.Token{
				User:         "phippy",
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
			},
			req: &apiv2.TokenServiceListRequest{
				Query: &apiv2.TokenQuery{
					User: new("not phippy"),
				},
			},
			state: state{
				existingTokens: []*apiv2.Token{
					{
						Uuid: "c223af4d-b3f5-4df6-8815-52b80323930d",
						User: "phippy",
					},
					{
						Uuid: "8ff27ee2-209f-43e2-a15d-50143fb03229",
						User: "phippy",
					},
					{
						Uuid: "9baa8668-2212-4fa5-a2e4-167084d0552d",
						User: "not phippy",
					},
				},
			},
			want: nil,
		},
		{
			name: "query token type",
			sessionToken: &apiv2.Token{
				User:         "phippy",
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
			},
			req: &apiv2.TokenServiceListRequest{
				Query: &apiv2.TokenQuery{
					TokenType: new(apiv2.TokenType_TOKEN_TYPE_API),
				},
			},
			state: state{
				existingTokens: []*apiv2.Token{
					{
						Uuid:      "c223af4d-b3f5-4df6-8815-52b80323930d",
						User:      "phippy",
						TokenType: apiv2.TokenType_TOKEN_TYPE_API,
					},
					{
						Uuid:      "8ff27ee2-209f-43e2-a15d-50143fb03229",
						User:      "phippy",
						TokenType: apiv2.TokenType_TOKEN_TYPE_USER,
					},
				},
			},
			want: []*apiv2.Token{
				{
					Uuid:      "c223af4d-b3f5-4df6-8815-52b80323930d",
					User:      "phippy",
					TokenType: apiv2.TokenType_TOKEN_TYPE_API,
					Meta:      &apiv2.Meta{},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(innerT *testing.T) {
			defer testStore.Cleanup(t)

			ctx, cancel := context.WithCancel(token.ContextWithToken(innerT.Context(), tt.sessionToken))
			defer cancel()

			for _, tok := range tt.state.existingTokens {
				err := testStore.GetTokenStore().Set(ctx, tok)
				require.NoError(innerT, err)
			}

			service := tokenservice.New(tokenservice.Config{
				Log:  log,
				Repo: testStore.Store,
			})

			if tt.wantErr == false {
				// Execute proto based validation
				err := protovalidate.Validate(tt.req)
				require.NoError(innerT, err)
			}

			response, err := service.List(ctx, tt.req)

			switch {
			case tt.wantErr && err != nil:
				if diff := cmp.Diff(tt.wantErrMessage, err.Error()); diff != "" {
					innerT.Errorf("diff = %s", diff)
				}

			case tt.wantErr && err == nil:
				innerT.Fatalf("want error %q, got response %q", tt.wantErrMessage, response)
			case err != nil:
				innerT.Fatalf("want response, got error %q", err)

			default:
				if diff := cmp.Diff(
					tt.want, response.Tokens,
					cmpopts.SortSlices(func(a, b *apiv2.Token) bool {
						return a.Uuid < b.Uuid
					}),
					protocmp.Transform(),
					protocmp.IgnoreFields(
						&apiv2.Token{}, "issued_at", "expires",
					),
					protocmp.IgnoreFields(
						&apiv2.Meta{}, "created_at", "updated_at",
					),
				); diff != "" {
					innerT.Errorf("diff: %s", diff)
				}
			}
		})
	}
}

func Test_Get(t *testing.T) {
	t.Parallel()

	log := slog.Default()

	testStore, closer := test.StartRepositoryWithCleanup(t, log)
	defer closer()

	type state struct {
		existingTokens []*apiv2.Token
	}
	tests := []struct {
		name              string
		sessionToken      *apiv2.Token
		req               *apiv2.TokenServiceGetRequest
		state             state
		wantErr           error
		wantValidationErr string
		want              *apiv2.Token
	}{
		{
			name: "missing id in request",
			sessionToken: &apiv2.Token{
				User:         "phippy",
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
			},
			req: &apiv2.TokenServiceGetRequest{},
			state: state{
				existingTokens: []*apiv2.Token{
					{
						Uuid: "c223af4d-b3f5-4df6-8815-52b80323930d",
						User: "phippy",
					},
					{
						Uuid: "8ff27ee2-209f-43e2-a15d-50143fb03229",
						User: "phippy",
					},
					{
						Uuid: "9baa8668-2212-4fa5-a2e4-167084d0552d",
						User: "not phippy",
					},
				},
			},
			want:              nil,
			wantValidationErr: "validation error: uuid: value is empty, which is not a valid UUID",
		},
		{
			name: "get non-existing",
			sessionToken: &apiv2.Token{
				User:         "phippy",
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
			},
			req: &apiv2.TokenServiceGetRequest{
				Uuid: "57460ff2-30e9-45e5-93c8-7f9ca85a92af",
			},
			state: state{
				existingTokens: []*apiv2.Token{
					{
						Uuid: "c223af4d-b3f5-4df6-8815-52b80323930d",
						User: "phippy",
					},
					{
						Uuid: "8ff27ee2-209f-43e2-a15d-50143fb03229",
						User: "phippy",
					},
					{
						Uuid: "9baa8668-2212-4fa5-a2e4-167084d0552d",
						User: "not phippy",
					},
				},
			},
			wantErr: errorutil.NotFound("token not found"),
		},
		{
			name: "cannot get another user's token",
			sessionToken: &apiv2.Token{
				User:         "phippy",
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
			},
			req: &apiv2.TokenServiceGetRequest{
				Uuid: "9baa8668-2212-4fa5-a2e4-167084d0552d",
			},
			state: state{
				existingTokens: []*apiv2.Token{
					{
						Uuid: "c223af4d-b3f5-4df6-8815-52b80323930d",
						User: "phippy",
					},
					{
						Uuid: "8ff27ee2-209f-43e2-a15d-50143fb03229",
						User: "phippy",
					},
					{
						Uuid: "9baa8668-2212-4fa5-a2e4-167084d0552d",
						User: "not phippy",
					},
				},
			},
			wantErr: errorutil.NotFound("token not found"),
		},
		{
			name: "get existing",
			sessionToken: &apiv2.Token{
				User:         "phippy",
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
			},
			req: &apiv2.TokenServiceGetRequest{
				Uuid: "8ff27ee2-209f-43e2-a15d-50143fb03229",
			},
			state: state{
				existingTokens: []*apiv2.Token{
					{
						Uuid: "c223af4d-b3f5-4df6-8815-52b80323930d",
						User: "phippy",
					},
					{
						Uuid: "8ff27ee2-209f-43e2-a15d-50143fb03229",
						User: "phippy",
					},
					{
						Uuid: "9baa8668-2212-4fa5-a2e4-167084d0552d",
						User: "not phippy",
					},
				},
			},
			want: &apiv2.Token{
				Uuid: "8ff27ee2-209f-43e2-a15d-50143fb03229",
				User: "phippy",
				Meta: &apiv2.Meta{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(innerT *testing.T) {
			defer testStore.Cleanup(t)

			ctx, cancel := context.WithCancel(token.ContextWithToken(innerT.Context(), tt.sessionToken))
			defer cancel()

			for _, tok := range tt.state.existingTokens {
				err := testStore.GetTokenStore().Set(ctx, tok)
				require.NoError(innerT, err)
			}

			service := tokenservice.New(tokenservice.Config{
				Log:  log,
				Repo: testStore.Store,
			})

			// Execute proto based validation
			err := protovalidate.Validate(tt.req)
			if tt.wantValidationErr != "" {
				if diff := cmp.Diff(tt.wantValidationErr, err.Error(), errorutil.ErrorStringComparer()); diff != "" {
					innerT.Errorf("diff = %s", diff)
				}

				return
			}

			require.NoError(innerT, err)

			response, err := service.Get(ctx, tt.req)

			switch {
			case tt.wantErr != nil && err != nil:
				if diff := cmp.Diff(tt.wantErr, err, errorutil.ErrorStringComparer()); diff != "" {
					innerT.Errorf("diff = %s", diff)
				}

			case tt.wantErr != nil && err == nil:
				innerT.Fatalf("want error %q, got response %q", tt.wantErr, response)
			case err != nil:
				innerT.Fatalf("want response, got error %q", err)

			default:
				if diff := cmp.Diff(
					tt.want, response.Token,
					protocmp.Transform(),
					protocmp.IgnoreFields(
						&apiv2.Token{}, "issued_at", "expires",
					),
					protocmp.IgnoreFields(
						&apiv2.Meta{}, "created_at", "updated_at",
					),
				); diff != "" {
					innerT.Errorf("diff: %s", diff)
				}
			}
		})
	}
}

func Test_Revoke(t *testing.T) {
	t.Parallel()

	log := slog.Default()

	testStore, closer := test.StartRepositoryWithCleanup(t, log)
	defer closer()

	type state struct {
		existingTokens []*apiv2.Token
	}
	tests := []struct {
		name              string
		sessionToken      *apiv2.Token
		req               *apiv2.TokenServiceRevokeRequest
		state             state
		wantErr           error
		wantValidationErr string
		wantRemaining     []*apiv2.Token
	}{
		{
			name: "missing id in request",
			sessionToken: &apiv2.Token{
				User:         "phippy",
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
			},
			req: &apiv2.TokenServiceRevokeRequest{},
			state: state{
				existingTokens: []*apiv2.Token{
					{
						Uuid: "c223af4d-b3f5-4df6-8815-52b80323930d",
						User: "phippy",
					},
					{
						Uuid: "8ff27ee2-209f-43e2-a15d-50143fb03229",
						User: "phippy",
					},
					{
						Uuid: "9baa8668-2212-4fa5-a2e4-167084d0552d",
						User: "not phippy",
					},
				},
			},
			wantRemaining: []*apiv2.Token{
				{
					Uuid: "c223af4d-b3f5-4df6-8815-52b80323930d",
					User: "phippy",
					Meta: &apiv2.Meta{},
				},
				{
					Uuid: "8ff27ee2-209f-43e2-a15d-50143fb03229",
					User: "phippy",
					Meta: &apiv2.Meta{},
				},
				{
					Uuid: "9baa8668-2212-4fa5-a2e4-167084d0552d",
					User: "not phippy",
					Meta: &apiv2.Meta{},
				},
			},
			wantValidationErr: "validation error: uuid: value is empty, which is not a valid UUID",
		},
		{
			name: "revoke non-existing",
			sessionToken: &apiv2.Token{
				User:         "phippy",
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
			},
			req: &apiv2.TokenServiceRevokeRequest{
				Uuid: "57460ff2-30e9-45e5-93c8-7f9ca85a92af",
			},
			state: state{
				existingTokens: []*apiv2.Token{
					{
						Uuid: "c223af4d-b3f5-4df6-8815-52b80323930d",
						User: "phippy",
					},
					{
						Uuid: "8ff27ee2-209f-43e2-a15d-50143fb03229",
						User: "phippy",
					},
					{
						Uuid: "9baa8668-2212-4fa5-a2e4-167084d0552d",
						User: "not phippy",
					},
				},
			},
			wantRemaining: []*apiv2.Token{
				{
					Uuid: "c223af4d-b3f5-4df6-8815-52b80323930d",
					User: "phippy",
					Meta: &apiv2.Meta{},
				},
				{
					Uuid: "8ff27ee2-209f-43e2-a15d-50143fb03229",
					User: "phippy",
					Meta: &apiv2.Meta{},
				},
				{
					Uuid: "9baa8668-2212-4fa5-a2e4-167084d0552d",
					User: "not phippy",
					Meta: &apiv2.Meta{},
				},
			},
			wantErr: errorutil.NotFound("token not found"),
		},
		{
			name: "cannot revoke another user's token",
			sessionToken: &apiv2.Token{
				User:         "phippy",
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
			},
			req: &apiv2.TokenServiceRevokeRequest{
				Uuid: "9baa8668-2212-4fa5-a2e4-167084d0552d",
			},
			state: state{
				existingTokens: []*apiv2.Token{
					{
						Uuid: "c223af4d-b3f5-4df6-8815-52b80323930d",
						User: "phippy",
					},
					{
						Uuid: "8ff27ee2-209f-43e2-a15d-50143fb03229",
						User: "phippy",
					},
					{
						Uuid: "9baa8668-2212-4fa5-a2e4-167084d0552d",
						User: "not phippy",
					},
				},
			},
			wantRemaining: []*apiv2.Token{
				{
					Uuid: "c223af4d-b3f5-4df6-8815-52b80323930d",
					User: "phippy",
					Meta: &apiv2.Meta{},
				},
				{
					Uuid: "8ff27ee2-209f-43e2-a15d-50143fb03229",
					User: "phippy",
					Meta: &apiv2.Meta{},
				},
				{
					Uuid: "9baa8668-2212-4fa5-a2e4-167084d0552d",
					User: "not phippy",
					Meta: &apiv2.Meta{},
				},
			},
			wantErr: errorutil.NotFound("token not found"),
		},
		{
			name: "revoke existing",
			sessionToken: &apiv2.Token{
				User:         "phippy",
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
			},
			req: &apiv2.TokenServiceRevokeRequest{
				Uuid: "8ff27ee2-209f-43e2-a15d-50143fb03229",
			},
			state: state{
				existingTokens: []*apiv2.Token{
					{
						Uuid: "c223af4d-b3f5-4df6-8815-52b80323930d",
						User: "phippy",
					},
					{
						Uuid: "8ff27ee2-209f-43e2-a15d-50143fb03229",
						User: "phippy",
					},
					{
						Uuid: "9baa8668-2212-4fa5-a2e4-167084d0552d",
						User: "not phippy",
					},
				},
			},
			wantRemaining: []*apiv2.Token{
				{
					Uuid: "c223af4d-b3f5-4df6-8815-52b80323930d",
					User: "phippy",
					Meta: &apiv2.Meta{},
				},
				{
					Uuid: "9baa8668-2212-4fa5-a2e4-167084d0552d",
					User: "not phippy",
					Meta: &apiv2.Meta{},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(innerT *testing.T) {
			defer testStore.Cleanup(t)

			ctx, cancel := context.WithCancel(token.ContextWithToken(innerT.Context(), tt.sessionToken))
			defer cancel()

			for _, tok := range tt.state.existingTokens {
				err := testStore.GetTokenStore().Set(ctx, tok)
				require.NoError(innerT, err)
			}

			service := tokenservice.New(tokenservice.Config{
				Log:  log,
				Repo: testStore.Store,
			})

			// Execute proto based validation
			err := protovalidate.Validate(tt.req)
			if tt.wantValidationErr != "" {
				if diff := cmp.Diff(tt.wantValidationErr, err.Error(), errorutil.ErrorStringComparer()); diff != "" {
					innerT.Errorf("diff = %s", diff)
				}

				return
			}

			require.NoError(innerT, err)

			response, err := service.Revoke(ctx, tt.req)

			switch {
			case tt.wantErr != nil && err != nil:
				if diff := cmp.Diff(tt.wantErr, err, errorutil.ErrorStringComparer()); diff != "" {
					innerT.Errorf("diff = %s", diff)
				}

			case tt.wantErr != nil && err == nil:
				innerT.Fatalf("want error %q, got response %q", tt.wantErr, response)
			case err != nil:
				innerT.Fatalf("want response, got error %q", err)

			default:
				remaining, err := testStore.GetTokenStore().AdminList(ctx)
				require.NoError(innerT, err)

				if diff := cmp.Diff(
					tt.wantRemaining, remaining,
					protocmp.Transform(),
					protocmp.IgnoreFields(
						&apiv2.Token{}, "issued_at", "expires",
					),
					protocmp.IgnoreFields(
						&apiv2.Meta{}, "created_at", "updated_at",
					),
					cmpopts.SortSlices(func(a, b *apiv2.Token) bool {
						return a.Uuid < b.Uuid
					}),
				); diff != "" {
					innerT.Errorf("diff: %s", diff)
				}
			}
		})
	}
}
