package token

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"buf.build/go/protovalidate"
	"github.com/alicebob/miniredis/v2"
	"github.com/google/go-cmp/cmp"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/certs"
	"github.com/metal-stack/metal-apiserver/pkg/repository/api"
	"github.com/metal-stack/metal-apiserver/pkg/request"
	"github.com/metal-stack/metal-apiserver/pkg/token"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var (
	kubies = "00000000-0000-0000-0000-000000000000"
	token1 = "00000000-0000-0000-0000-000000000000"
)

func Test_Update(t *testing.T) {
	t.Parallel()
	type state struct {
		providerTenant string
		projectRoles   map[string]apiv2.ProjectRole
		tenantRoles    map[string]apiv2.TenantRole
	}
	tests := []struct {
		name           string
		sessionToken   *apiv2.Token
		tokenToUpdate  *apiv2.Token
		req            *apiv2.TokenServiceUpdateRequest
		state          state
		wantErr        bool
		wantErrMessage string
		wantToken      *apiv2.Token
	}{
		{
			name: "can update bare token",
			sessionToken: &apiv2.Token{
				User:         "phippy",
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
			},
			tokenToUpdate: &apiv2.Token{
				Uuid:         token1,
				User:         "phippy",
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
				TokenType:    apiv2.TokenType_TOKEN_TYPE_API,
			},
			req: &apiv2.TokenServiceUpdateRequest{
				Uuid:        token1,
				Description: new("update!"),
			},
			state: state{
				providerTenant: "metal-stack",
			},
			wantToken: &apiv2.Token{
				Uuid:        token1,
				User:        "phippy",
				Description: "update!",
				TokenType:   apiv2.TokenType_TOKEN_TYPE_API,
			},
		},
		{
			name: "user and token without project access cannot update project token",
			sessionToken: &apiv2.Token{
				User:         "phippy",
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
			},
			tokenToUpdate: &apiv2.Token{
				Uuid:         token1,
				User:         "phippy",
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
			},
			req: &apiv2.TokenServiceUpdateRequest{
				Uuid: token1,
				ProjectRoles: map[string]apiv2.ProjectRole{
					kubies: apiv2.ProjectRole_PROJECT_ROLE_EDITOR,
				},
				TenantRoles: map[string]apiv2.TenantRole{},
			},
			state: state{
				providerTenant: "metal-stack",
			},
			wantErr:        true,
			wantErrMessage: `permission_denied: requested project roles are not allowed: [00000000-0000-0000-0000-000000000000]`,
		},
		{
			name: "user and token with project access can update project token",
			sessionToken: &apiv2.Token{
				User:        "phippy",
				Permissions: []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{
					kubies: apiv2.ProjectRole_PROJECT_ROLE_EDITOR,
				},
				TenantRoles: map[string]apiv2.TenantRole{},
			},
			tokenToUpdate: &apiv2.Token{
				Uuid:         token1,
				User:         "phippy",
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
				TokenType:    apiv2.TokenType_TOKEN_TYPE_API,
			},
			req: &apiv2.TokenServiceUpdateRequest{
				Uuid: token1,
				ProjectRoles: map[string]apiv2.ProjectRole{
					kubies: apiv2.ProjectRole_PROJECT_ROLE_EDITOR,
				},
				TenantRoles: map[string]apiv2.TenantRole{},
			},
			state: state{
				providerTenant: "metal-stack",
				projectRoles: map[string]apiv2.ProjectRole{
					kubies: apiv2.ProjectRole_PROJECT_ROLE_EDITOR,
				},
			},
			wantToken: &apiv2.Token{
				Uuid:      token1,
				User:      "phippy",
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
				ProjectRoles: map[string]apiv2.ProjectRole{
					kubies: apiv2.ProjectRole_PROJECT_ROLE_EDITOR,
				},
				TenantRoles: map[string]apiv2.TenantRole{},
			},
		},
		{
			name: "user without but token with project access cannot update project token",
			sessionToken: &apiv2.Token{
				User:        "phippy",
				Permissions: []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{
					kubies: apiv2.ProjectRole_PROJECT_ROLE_EDITOR,
				},
				TenantRoles: map[string]apiv2.TenantRole{},
			},
			tokenToUpdate: &apiv2.Token{
				Uuid:         token1,
				User:         "phippy",
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
			},
			req: &apiv2.TokenServiceUpdateRequest{
				Uuid: token1,
				ProjectRoles: map[string]apiv2.ProjectRole{
					kubies: apiv2.ProjectRole_PROJECT_ROLE_EDITOR,
				},
				TenantRoles: map[string]apiv2.TenantRole{},
			},
			state: state{
				providerTenant: "metal-stack",
				projectRoles:   map[string]apiv2.ProjectRole{},
			},
			wantErr:        true,
			wantErrMessage: `permission_denied: requested project roles are not allowed: [00000000-0000-0000-0000-000000000000]`,
		},
		{
			name: "project without but user with project access cannot create project token",
			sessionToken: &apiv2.Token{
				User:         "phippy",
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
			},
			tokenToUpdate: &apiv2.Token{
				Uuid:         token1,
				User:         "phippy",
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
			},
			req: &apiv2.TokenServiceUpdateRequest{
				Uuid: token1,
				ProjectRoles: map[string]apiv2.ProjectRole{
					kubies: apiv2.ProjectRole_PROJECT_ROLE_EDITOR,
				},
				TenantRoles: map[string]apiv2.TenantRole{},
			},
			state: state{
				providerTenant: "metal-stack",
				projectRoles: map[string]apiv2.ProjectRole{
					kubies: apiv2.ProjectRole_PROJECT_ROLE_EDITOR,
				},
			},
			wantErr:        true,
			wantErrMessage: `permission_denied: requested project roles are not allowed: [00000000-0000-0000-0000-000000000000]`,
		},
		{
			name: "admin user and token can update admin token",
			sessionToken: &apiv2.Token{
				User:         "phippy",
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
				AdminRole:    apiv2.AdminRole_ADMIN_ROLE_EDITOR.Enum(),
			},
			tokenToUpdate: &apiv2.Token{
				Uuid:         token1,
				User:         "phippy",
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
				AdminRole:    apiv2.AdminRole_ADMIN_ROLE_VIEWER.Enum(),
				TokenType:    apiv2.TokenType_TOKEN_TYPE_API,
			},
			req: &apiv2.TokenServiceUpdateRequest{
				Uuid:         token1,
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
				AdminRole:    apiv2.AdminRole_ADMIN_ROLE_EDITOR.Enum(),
			},
			state: state{
				providerTenant: "phippy",
				tenantRoles: map[string]apiv2.TenantRole{
					"phippy": apiv2.TenantRole_TENANT_ROLE_OWNER,
				},
			},
			wantToken: &apiv2.Token{
				Uuid:         token1,
				User:         "phippy",
				TokenType:    apiv2.TokenType_TOKEN_TYPE_API,
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
				AdminRole:    apiv2.AdminRole_ADMIN_ROLE_EDITOR.Enum(),
			},
		},
		{
			name: "admin token but user cannot update admin token",
			sessionToken: &apiv2.Token{
				User:         "phippy",
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
				AdminRole:    apiv2.AdminRole_ADMIN_ROLE_EDITOR.Enum(),
			},
			tokenToUpdate: &apiv2.Token{
				Uuid:      token1,
				User:      "phippy",
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
			},
			req: &apiv2.TokenServiceUpdateRequest{
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
				AdminRole:    apiv2.AdminRole_ADMIN_ROLE_EDITOR.Enum(),
			},
			state: state{
				providerTenant: "metal-stack",
			},
			wantErr:        true,
			wantErrMessage: `permission_denied: the following method "/grpc.reflection.v1.ServerReflection/ServerReflectionInfo" is not allowed on any of the requested subjects: [*]`,
		},
		{
			name: "user and token without tenant access cannot update tenant token",
			sessionToken: &apiv2.Token{
				User:         "phippy",
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
			},
			tokenToUpdate: &apiv2.Token{
				Uuid:      token1,
				User:      "phippy",
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
			},
			req: &apiv2.TokenServiceUpdateRequest{
				Uuid:         token1,
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles: map[string]apiv2.TenantRole{
					"mascots": apiv2.TenantRole_TENANT_ROLE_EDITOR,
				},
			},
			state: state{
				providerTenant: "metal-stack",
			},
			wantErr:        true,
			wantErrMessage: `permission_denied: requested tenant roles are not allowed: [mascots]`,
		},
		{
			name: "user and token with tenant access can update tenant token",
			sessionToken: &apiv2.Token{
				User:         "phippy",
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles: map[string]apiv2.TenantRole{
					"mascots": apiv2.TenantRole_TENANT_ROLE_EDITOR,
				},
			},
			tokenToUpdate: &apiv2.Token{
				Uuid:         token1,
				User:         "phippy",
				TokenType:    apiv2.TokenType_TOKEN_TYPE_API,
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
			},
			req: &apiv2.TokenServiceUpdateRequest{
				Uuid:         token1,
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles: map[string]apiv2.TenantRole{
					"mascots": apiv2.TenantRole_TENANT_ROLE_EDITOR,
				},
			},
			state: state{
				providerTenant: "metal-stack",
				tenantRoles: map[string]apiv2.TenantRole{
					"mascots": apiv2.TenantRole_TENANT_ROLE_EDITOR,
				},
			},
			wantToken: &apiv2.Token{
				Uuid:         token1,
				User:         "phippy",
				TokenType:    *apiv2.TokenType_TOKEN_TYPE_API.Enum(),
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles: map[string]apiv2.TenantRole{
					"mascots": apiv2.TenantRole_TENANT_ROLE_EDITOR,
				},
			},
		},
		{
			name: "user without but token with tenant access cannot update tenant token",
			sessionToken: &apiv2.Token{
				User:         "phippy",
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles: map[string]apiv2.TenantRole{
					"mascots": apiv2.TenantRole_TENANT_ROLE_EDITOR,
				},
			},
			tokenToUpdate: &apiv2.Token{
				Uuid:      token1,
				User:      "phippy",
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
			},
			req: &apiv2.TokenServiceUpdateRequest{
				Uuid:         token1,
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles: map[string]apiv2.TenantRole{
					"mascots": apiv2.TenantRole_TENANT_ROLE_EDITOR,
				},
			},
			state: state{
				providerTenant: "metal-stack",
				projectRoles:   map[string]apiv2.ProjectRole{},
			},
			wantErr:        true,
			wantErrMessage: `permission_denied: requested tenant roles are not allowed: [mascots]`,
		},
		{
			name: "user and token with machine access can update machine token",
			sessionToken: &apiv2.Token{
				User:         "phippy",
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
				MachineRoles: map[string]apiv2.MachineRole{
					"de240964-ff9f-4e3d-95b2-8a96e43788f1": apiv2.MachineRole_MACHINE_ROLE_EDITOR,
				},
			},
			tokenToUpdate: &apiv2.Token{
				Uuid:         token1,
				User:         "phippy",
				TokenType:    apiv2.TokenType_TOKEN_TYPE_API,
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
				MachineRoles: map[string]apiv2.MachineRole{},
			},
			req: &apiv2.TokenServiceUpdateRequest{
				Uuid: token1,
				MachineRoles: map[string]apiv2.MachineRole{
					"de240964-ff9f-4e3d-95b2-8a96e43788f1": apiv2.MachineRole_MACHINE_ROLE_EDITOR,
				},
			},
			state: state{
				providerTenant: "metal-stack",
			},
			wantToken: &apiv2.Token{
				Uuid:      token1,
				User:      "phippy",
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
				MachineRoles: map[string]apiv2.MachineRole{
					"de240964-ff9f-4e3d-95b2-8a96e43788f1": apiv2.MachineRole_MACHINE_ROLE_EDITOR,
				},
			},
		},
		{
			name: "user and token without machine access cannot update machine token",
			sessionToken: &apiv2.Token{
				User:         "phippy",
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
			},
			tokenToUpdate: &apiv2.Token{
				Uuid:      token1,
				User:      "phippy",
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
			},
			req: &apiv2.TokenServiceUpdateRequest{
				Uuid: token1,
				MachineRoles: map[string]apiv2.MachineRole{
					"de240964-ff9f-4e3d-95b2-8a96e43788f1": apiv2.MachineRole_MACHINE_ROLE_EDITOR,
				},
			},
			state: state{
				providerTenant: "metal-stack",
			},
			wantErr:        true,
			wantErrMessage: `permission_denied: requested machine roles are not allowed: [de240964-ff9f-4e3d-95b2-8a96e43788f1]`,
		},
		{
			name: "token without but user with tenant access cannot update tenant token",
			sessionToken: &apiv2.Token{
				User:         "phippy",
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
			},
			tokenToUpdate: &apiv2.Token{
				Uuid:      token1,
				User:      "phippy",
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
			},
			req: &apiv2.TokenServiceUpdateRequest{
				Uuid:         token1,
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles: map[string]apiv2.TenantRole{
					"mascots": apiv2.TenantRole_TENANT_ROLE_EDITOR,
				},
			},
			state: state{
				providerTenant: "metal-stack",
				projectRoles:   map[string]apiv2.ProjectRole{},
				tenantRoles: map[string]apiv2.TenantRole{
					"mascots": apiv2.TenantRole_TENANT_ROLE_EDITOR,
				},
			},
			wantErr:        true,
			wantErrMessage: `permission_denied: requested tenant roles are not allowed: [mascots]`,
		},
		{
			name: "token does not exist in database",
			sessionToken: &apiv2.Token{
				User:        "phippy",
				Permissions: []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{
					kubies: apiv2.ProjectRole_PROJECT_ROLE_EDITOR,
				},
			},
			tokenToUpdate: &apiv2.Token{
				Uuid:      token1,
				User:      "phippy",
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
			},
			req: &apiv2.TokenServiceUpdateRequest{
				Uuid: "222",
				ProjectRoles: map[string]apiv2.ProjectRole{
					kubies: apiv2.ProjectRole_PROJECT_ROLE_EDITOR,
				},
				TenantRoles: map[string]apiv2.TenantRole{},
			},
			state: state{
				providerTenant: "metal-stack",
				projectRoles: map[string]apiv2.ProjectRole{
					kubies: apiv2.ProjectRole_PROJECT_ROLE_EDITOR,
				},
				tenantRoles: map[string]apiv2.TenantRole{},
			},
			wantErr:        true,
			wantErrMessage: `not_found: token not found`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(token.ContextWithToken(t.Context(), tt.sessionToken))
			defer cancel()

			s := miniredis.RunT(t)
			c := redis.NewClient(&redis.Options{Addr: s.Addr()})

			tokenStore := token.NewRedisStore(c)
			certStore := certs.NewRedisStore(&certs.Config{
				RedisClient: c,
			})

			if tt.tokenToUpdate != nil {
				err := tokenStore.Set(ctx, tt.tokenToUpdate)
				require.NoError(t, err)
			}

			projectsAndTenantsGetter := func(ctx context.Context, userId string) (*api.ProjectsAndTenants, error) {
				return &api.ProjectsAndTenants{
					ProjectRoles: tt.state.projectRoles,
					TenantRoles:  tt.state.tenantRoles,
				}, nil
			}
			log := slog.Default()
			service := tokenService{
				log:                      log,
				tokens:                   tokenStore,
				certs:                    certStore,
				issuer:                   "http://test",
				providerTenant:           tt.state.providerTenant,
				projectsAndTenantsGetter: projectsAndTenantsGetter,
				authorizer:               request.NewAuthorizer(log, projectsAndTenantsGetter),
			}

			if tt.wantErr == false {
				// Execute proto based validation
				err := protovalidate.Validate(tt.req)
				require.NoError(t, err)
			}

			response, err := service.Update(ctx, tt.req)
			switch {
			case tt.wantErr && err != nil:
				if dff := cmp.Diff(tt.wantErrMessage, err.Error()); dff != "" {
					t.Fatal(dff)
				}
			case tt.wantErr && err == nil:
				t.Fatalf("want error %q, got response %q", tt.wantErrMessage, response)
			case err != nil:
				t.Fatalf("want response, got error %q", err)

			default:
				got := response.Token
				assert.Equal(t, tt.wantToken.Uuid, got.Uuid, "uuid")
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

func Test_Refresh(t *testing.T) {
	t.Parallel()
	iat := time.Now()
	exp := iat.Add(time.Hour)
	type state struct {
		providerTenant string
		projectRoles   map[string]apiv2.ProjectRole
		tenantRoles    map[string]apiv2.TenantRole
	}
	tests := []struct {
		name           string
		sessionToken   *apiv2.Token
		existingToken  *apiv2.Token
		state          state
		wantErr        bool
		wantErrMessage string
		wantToken      *apiv2.Token
	}{
		{
			name: "can update bare token",
			sessionToken: &apiv2.Token{
				User:         "phippy",
				Uuid:         token1,
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
			},
			existingToken: &apiv2.Token{
				Uuid:         token1,
				User:         "phippy",
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
				TokenType:    apiv2.TokenType_TOKEN_TYPE_API,
				IssuedAt:     timestamppb.New(iat),
				Expires:      timestamppb.New(exp),
			},
			state: state{
				providerTenant: "metal-stack",
			},
			wantToken: &apiv2.Token{
				Uuid:         token1,
				User:         "phippy",
				Permissions:  nil,
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
				MachineRoles: map[string]apiv2.MachineRole{},
				TokenType:    apiv2.TokenType_TOKEN_TYPE_API,
				IssuedAt:     timestamppb.New(exp),
				Expires:      timestamppb.New(exp.Add(time.Hour)),
			},
		},
		{
			name: "refresh preserves machine roles from existing token",
			sessionToken: &apiv2.Token{
				User:         "phippy",
				Uuid:         token1,
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
				MachineRoles: map[string]apiv2.MachineRole{
					"de240964-ff9f-4e3d-95b2-8a96e43788f1": apiv2.MachineRole_MACHINE_ROLE_EDITOR,
				},
			},
			existingToken: &apiv2.Token{
				Uuid:         token1,
				User:         "phippy",
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
				MachineRoles: map[string]apiv2.MachineRole{
					"de240964-ff9f-4e3d-95b2-8a96e43788f1": apiv2.MachineRole_MACHINE_ROLE_EDITOR,
				},
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
				IssuedAt:  timestamppb.New(iat),
				Expires:   timestamppb.New(exp),
			},
			state: state{
				providerTenant: "metal-stack",
			},
			wantToken: &apiv2.Token{
				Uuid:         token1,
				User:         "phippy",
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
				MachineRoles: map[string]apiv2.MachineRole{
					"de240964-ff9f-4e3d-95b2-8a96e43788f1": apiv2.MachineRole_MACHINE_ROLE_EDITOR,
				},
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
				IssuedAt:  timestamppb.New(exp),
				Expires:   timestamppb.New(exp.Add(time.Hour)),
			},
		},
		// FIXME more tests
		{
			name: "token does not exist in database",
			sessionToken: &apiv2.Token{
				User:        "phippy",
				Permissions: []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{
					kubies: apiv2.ProjectRole_PROJECT_ROLE_EDITOR,
				},
			},
			state: state{
				providerTenant: "metal-stack",
				projectRoles: map[string]apiv2.ProjectRole{
					kubies: apiv2.ProjectRole_PROJECT_ROLE_EDITOR,
				},
				tenantRoles: map[string]apiv2.TenantRole{},
			},
			wantErr:        true,
			wantErrMessage: `not_found: token not found`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(token.ContextWithToken(t.Context(), tt.sessionToken))
			defer cancel()

			s := miniredis.RunT(t)
			c := redis.NewClient(&redis.Options{Addr: s.Addr()})

			tokenStore := token.NewRedisStore(c)
			certStore := certs.NewRedisStore(&certs.Config{
				RedisClient: c,
			})

			if tt.existingToken != nil {
				err := tokenStore.Set(ctx, tt.existingToken)
				require.NoError(t, err)
			}

			projectsAndTenantsGetter := func(ctx context.Context, userId string) (*api.ProjectsAndTenants, error) {
				return &api.ProjectsAndTenants{
					ProjectRoles: tt.state.projectRoles,
					TenantRoles:  tt.state.tenantRoles,
				}, nil
			}
			log := slog.Default()
			service := tokenService{
				log:                      log,
				tokens:                   tokenStore,
				certs:                    certStore,
				issuer:                   "http://test",
				providerTenant:           tt.state.providerTenant,
				projectsAndTenantsGetter: projectsAndTenantsGetter,
				authorizer:               request.NewAuthorizer(log, projectsAndTenantsGetter),
			}

			response, err := service.Refresh(ctx, &apiv2.TokenServiceRefreshRequest{})
			switch {
			case tt.wantErr && err != nil:
				if dff := cmp.Diff(tt.wantErrMessage, err.Error()); dff != "" {
					t.Fatal(dff)
				}
			case tt.wantErr && err == nil:
				t.Fatalf("want error %q, got response %q", tt.wantErrMessage, response)
			case err != nil:
				t.Fatalf("want response, got error %q", err)

			default:
				got := response.Token
				assert.Equal(t, tt.wantToken.User, got.User, "userId")
				assert.Equal(t, tt.wantToken.Description, got.Description, "description")
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
