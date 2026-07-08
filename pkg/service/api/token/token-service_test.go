package token

import (
	"context"
	"errors"
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
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var (
	kubies = "00000000-0000-0000-0000-000000000000"
	token1 = "00000000-0000-0000-0000-000000000000"
)

func Test_Create(t *testing.T) {
	t.Parallel()
	type state struct {
		providerTenant string
		projectRoles   map[string]apiv2.ProjectRole
		tenantRoles    map[string]apiv2.TenantRole
		getterErr      error
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
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
			},
			req: &apiv2.TokenServiceCreateRequest{
				Description: "empty token",
			},
			state: state{
				providerTenant: "metal-stack",
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
				providerTenant: "metal-stack",
			},
			wantErr:        true,
			wantErrMessage: `permission_denied: requested project roles are not allowed: [00000000-0000-0000-0000-000000000000]`,
		},
		{
			name: "user and token with project access can create project token",
			sessionToken: &apiv2.Token{
				User:        "phippy",
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
				providerTenant: "metal-stack",
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
			req: &apiv2.TokenServiceCreateRequest{
				Description: "project token",
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
			name: "normal user which is listed in admin-subjects can create new admin editor token",
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
				providerTenant: "phippy",
				tenantRoles: map[string]apiv2.TenantRole{
					"phippy": apiv2.TenantRole_TENANT_ROLE_OWNER,
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
			name: "normal user which is listed in admin-subjects can create new admin viewer token",
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
				providerTenant: "phippy",
				tenantRoles: map[string]apiv2.TenantRole{
					"phippy": apiv2.TenantRole_TENANT_ROLE_VIEWER,
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
				providerTenant: "phippy",
				tenantRoles: map[string]apiv2.TenantRole{
					"phippy": apiv2.TenantRole_TENANT_ROLE_VIEWER,
				},
			},
			wantToken:      nil,
			wantErr:        true,
			wantErrMessage: `permission_denied: your provider tenant membership only allows "ADMIN_ROLE_VIEWER", but you requested "ADMIN_ROLE_EDITOR"`,
		},
		{
			name: "normal user which is not listed in admin-subjects can not create new admin viewer token",
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
				providerTenant: "phippy",
				tenantRoles: map[string]apiv2.TenantRole{
					"phippy": apiv2.TenantRole_TENANT_ROLE_OWNER,
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
				providerTenant: "metal-stack",
			},
			wantErr:        true,
			wantErrMessage: `permission_denied: the following method "/grpc.reflection.v1.ServerReflection/ServerReflectionInfo" is not allowed on any of the requested subjects: [*]`,
		},

		{
			name: "user and token without tenant access cannot create tenant token",
			sessionToken: &apiv2.Token{
				User:         "phippy",
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
				providerTenant: "metal-stack",
			},
			wantErr:        true,
			wantErrMessage: `permission_denied: requested tenant roles are not allowed: [mascots]`,
		},
		{
			name: "user and token with tenant access can create tenant token",
			sessionToken: &apiv2.Token{
				User:         "phippy",
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
				providerTenant: "metal-stack",
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
				providerTenant: "metal-stack",
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
				providerTenant: "metal-stack",
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
			name: "expiration exceeds max expiration",
			sessionToken: &apiv2.Token{
				User:         "phippy",
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
			},
			req: &apiv2.TokenServiceCreateRequest{
				Description: "token with long expiry",
				Expires:     durationpb.New(366 * 24 * time.Hour),
			},
			state: state{
				providerTenant: "metal-stack",
			},
			wantErr:        true,
			wantErrMessage: `requested expiration duration: "8784h0m0s" exceeds max expiration: "8760h0m0s"`,
		},
		{
			name: "user and token without machine access cannot create machine token",
			sessionToken: &apiv2.Token{
				User:         "phippy",
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
				providerTenant: "metal-stack",
			},
			wantErr:        true,
			wantErrMessage: `permission_denied: requested machine roles are not allowed: [de240964-ff9f-4e3d-95b2-8a96e43788f1]`,
		},
		{
			name: "user and token with machine access can create machine token",
			sessionToken: &apiv2.Token{
				User:        "pixie-core",
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
				providerTenant: "metal-stack",
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
		{
			name: "projects and tenants getter fails",
			sessionToken: &apiv2.Token{
				User:         "phippy",
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
			},
			req: &apiv2.TokenServiceCreateRequest{
				Description: "empty token",
			},
			state: state{
				providerTenant: "metal-stack",
				getterErr:      errors.New("getter failed"),
			},
			wantErr:        true,
			wantErrMessage: `internal: getter failed`,
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

			projectsAndTenantsGetter := func(ctx context.Context, userId string) (*api.ProjectsAndTenants, error) {
				if tt.state.getterErr != nil {
					return nil, tt.state.getterErr
				}
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

	s := miniredis.RunT(t)
	c := redis.NewClient(&redis.Options{Addr: s.Addr()})

	tokenStore := token.NewRedisStore(c)
	certStore := certs.NewRedisStore(&certs.Config{
		RedisClient: c,
	})

	projectsAndTenantsGetter := func(ctx context.Context, userId string) (*api.ProjectsAndTenants, error) {
		return &api.ProjectsAndTenants{}, nil
	}
	log := slog.Default()
	service := tokenService{
		log:                      log,
		tokens:                   tokenStore,
		certs:                    certStore,
		issuer:                   "http://test",
		providerTenant:           "metal-stack",
		projectsAndTenantsGetter: projectsAndTenantsGetter,
		authorizer:               request.NewAuthorizer(log, projectsAndTenantsGetter),
	}

	_, err := service.Create(t.Context(), &apiv2.TokenServiceCreateRequest{})
	require.Error(t, err)
	require.Equal(t, "unauthenticated: no token found in request", err.Error())
}

func Test_CreateForUser(t *testing.T) {
	t.Parallel()
	type state struct {
		providerTenant string
		projectRoles   map[string]apiv2.ProjectRole
		tenantRoles    map[string]apiv2.TenantRole
	}
	tests := []struct {
		name           string
		sessionToken   *apiv2.Token
		req            *apiv2.TokenServiceCreateRequest
		user           *string
		state          state
		wantErr        bool
		wantErrMessage string
		wantToken      *apiv2.Token
	}{
		{
			name: "phippy can create token for user foo",
			sessionToken: &apiv2.Token{
				User:         "phippy",
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
			},
			req: &apiv2.TokenServiceCreateRequest{
				Description: "empty token",
			},
			user: new("foo"),
			state: state{
				providerTenant: "phippy",
				tenantRoles: map[string]apiv2.TenantRole{
					"phippy": apiv2.TenantRole_TENANT_ROLE_OWNER,
				},
			},
			wantToken: &apiv2.Token{
				User:        "foo",
				Description: "empty token",
				TokenType:   apiv2.TokenType_TOKEN_TYPE_API,
			},
		},
		{
			name: "pixie-core can create token for metal-hammer with machine roles",
			sessionToken: &apiv2.Token{
				User:         "pixie-core",
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
				MachineRoles: map[string]apiv2.MachineRole{
					"de240964-ff9f-4e3d-95b2-8a96e43788f1": apiv2.MachineRole_MACHINE_ROLE_EDITOR,
				},
			},
			req: &apiv2.TokenServiceCreateRequest{
				Description: "machine token",
				MachineRoles: map[string]apiv2.MachineRole{
					"de240964-ff9f-4e3d-95b2-8a96e43788f1": apiv2.MachineRole_MACHINE_ROLE_EDITOR,
				},
			},
			user: new("metal-hammer"),
			state: state{
				providerTenant: "phippy",
				tenantRoles: map[string]apiv2.TenantRole{
					"phippy": apiv2.TenantRole_TENANT_ROLE_OWNER,
				},
			},
			wantToken: &apiv2.Token{
				User:        "metal-hammer",
				Description: "machine token",
				TokenType:   apiv2.TokenType_TOKEN_TYPE_API,
				MachineRoles: map[string]apiv2.MachineRole{
					"de240964-ff9f-4e3d-95b2-8a96e43788f1": apiv2.MachineRole_MACHINE_ROLE_EDITOR,
				},
			},
		},
		{
			name: "bar can not create token for user foo",
			sessionToken: &apiv2.Token{
				User:         "bar",
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
			},
			req: &apiv2.TokenServiceCreateRequest{
				Description: "empty token",
			},
			user: new("foo"),
			state: state{
				providerTenant: "phippy",
			},
			wantToken:      nil,
			wantErr:        true,
			wantErrMessage: "permission_denied: only admins can specify token user",
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

			response, err := service.CreateTokenForUser(ctx, tt.user, tt.req)
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

func Test_validateTokenRequest(t *testing.T) {
	t.Parallel()
	inOneHour := durationpb.New(time.Hour)
	tests := []struct {
		name           string
		pat            *api.ProjectsAndTenants
		token          *apiv2.Token
		req            *apiv2.TokenServiceCreateRequest
		providerTenant string
		wantErr        error
	}{
		{
			name: "simple token with empty permissions and roles",
			token: &apiv2.Token{
				User:      "test",
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
				Permissions: []*apiv2.MethodPermission{
					{
						Subject: "",
						Methods: []string{""},
					},
				},
			},
			req: &apiv2.TokenServiceCreateRequest{
				Description: "i don't need any permissions",
				Expires:     inOneHour,
			},
			providerTenant: "metal-stack",
			wantErr:        nil,
		},
		// Inherited Permissions
		{
			name: "simple token with no permissions but project role",
			pat: &api.ProjectsAndTenants{
				ProjectRoles: map[string]apiv2.ProjectRole{
					"ae8d2493-41ec-4efd-bbb4-81085b20b6fe": apiv2.ProjectRole_PROJECT_ROLE_OWNER,
				},
			},
			token: &apiv2.Token{
				User:      "test",
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
				ProjectRoles: map[string]apiv2.ProjectRole{
					"ae8d2493-41ec-4efd-bbb4-81085b20b6fe": apiv2.ProjectRole_PROJECT_ROLE_OWNER,
				},
			},
			req: &apiv2.TokenServiceCreateRequest{
				Description: "i want to get a cluster for this project",
				Permissions: []*apiv2.MethodPermission{
					{
						Subject: "ae8d2493-41ec-4efd-bbb4-81085b20b6fe",
						Methods: []string{
							"/metalstack.api.v2.IPService/Get",
						},
					},
				},
				Expires: inOneHour,
			},
			providerTenant: "metal-stack",
			wantErr:        nil,
		},
		// Permissions from Token
		{
			name: "simple token with one project and permission",
			pat: &api.ProjectsAndTenants{
				ProjectRoles: map[string]apiv2.ProjectRole{
					"abc": apiv2.ProjectRole_PROJECT_ROLE_OWNER,
				},
			},
			token: &apiv2.Token{
				User:      "test",
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
				Permissions: []*apiv2.MethodPermission{
					{
						Subject: "abc",
						Methods: []string{"/metalstack.api.v2.IPService/Get"},
					},
				},
			},
			req: &apiv2.TokenServiceCreateRequest{
				Description: "i want to get a cluster",
				Permissions: []*apiv2.MethodPermission{
					{
						Subject: "abc",
						Methods: []string{"/metalstack.api.v2.IPService/Get"},
					},
				},
				Expires: inOneHour,
			},
			providerTenant: "metal-stack",
			wantErr:        nil,
		},
		{
			name: "simple token with unknown method",
			pat: &api.ProjectsAndTenants{
				ProjectRoles: map[string]apiv2.ProjectRole{
					"abc": apiv2.ProjectRole_PROJECT_ROLE_EDITOR,
				},
			},
			token: &apiv2.Token{
				User:      "test",
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
				Permissions: []*apiv2.MethodPermission{
					{
						Subject: "abc",
						Methods: []string{"/metalstack.api.v2.IPService/Get"},
					},
				},
			},
			req: &apiv2.TokenServiceCreateRequest{
				Description: "i want to get a cluster",
				Permissions: []*apiv2.MethodPermission{
					{
						Subject: "abc",
						Methods: []string{"/metalstack.api.v2.UnknownService/Get"},
					},
				},
				Expires: inOneHour,
			},
			providerTenant: "metal-stack",
			wantErr:        errors.New("unknown method \"/metalstack.api.v2.UnknownService/Get\""),
		},
		{
			name: "simple token with one project and permission, wrong project given",
			pat: &api.ProjectsAndTenants{
				ProjectRoles: map[string]apiv2.ProjectRole{
					"abc": apiv2.ProjectRole_PROJECT_ROLE_EDITOR,
					"cde": apiv2.ProjectRole_PROJECT_ROLE_EDITOR,
				},
			},
			token: &apiv2.Token{
				User:      "sfs",
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
				Permissions: []*apiv2.MethodPermission{
					{
						Subject: "abc",
						Methods: []string{"/metalstack.api.v2.IPService/Get"},
					},
				},
			},
			req: &apiv2.TokenServiceCreateRequest{
				Description: "i want to get a cluster",
				Permissions: []*apiv2.MethodPermission{
					{
						Subject: "cde",
						Methods: []string{"/metalstack.api.v2.IPService/Get"},
					},
				},
				Expires: inOneHour,
			},
			providerTenant: "metal-stack",
			wantErr:        errors.New("method \"/metalstack.api.v2.IPService/Get\" is not allowed on subject \"cde\" with your current user permissions"),
		},
		{
			name: "simple token with one project and permission, wrong message given",
			pat: &api.ProjectsAndTenants{
				ProjectRoles: map[string]apiv2.ProjectRole{
					"abc": apiv2.ProjectRole_PROJECT_ROLE_EDITOR,
				},
			},
			token: &apiv2.Token{
				User:      "sfs",
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
				Permissions: []*apiv2.MethodPermission{
					{
						Subject: "abc",
						Methods: []string{"/metalstack.api.v2.IPService/Get"},
					},
				},
			},
			req: &apiv2.TokenServiceCreateRequest{
				Description: "i want to list clusters",
				Permissions: []*apiv2.MethodPermission{
					{
						Subject: "abc",
						Methods: []string{"/metalstack.api.v2.IPService/List"},
					},
				},
				Expires: inOneHour,
			},
			providerTenant: "metal-stack",
			wantErr:        errors.New("the following method \"/metalstack.api.v2.IPService/List\" is not allowed on any of the requested subjects: [abc]"),
		},
		{
			name: "simple token with one project and permission, wrong messages given",
			pat: &api.ProjectsAndTenants{
				ProjectRoles: map[string]apiv2.ProjectRole{
					"abc": apiv2.ProjectRole_PROJECT_ROLE_EDITOR,
				},
			},
			token: &apiv2.Token{
				User:      "sfs",
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
				Permissions: []*apiv2.MethodPermission{
					{
						Subject: "abc",
						Methods: []string{
							"/metalstack.api.v2.IPService/Create",
							"/metalstack.api.v2.IPService/Get",
							"/metalstack.api.v2.IPService/Delete",
						},
					},
				},
			},
			req: &apiv2.TokenServiceCreateRequest{
				Description: "i want to get and list clusters",
				Permissions: []*apiv2.MethodPermission{
					{
						Subject: "abc",
						Methods: []string{
							"/metalstack.api.v2.IPService/Get",
							"/metalstack.api.v2.IPService/List",
						},
					},
				},
				Expires: inOneHour,
			},
			providerTenant: "metal-stack",
			wantErr:        errors.New("the following method \"/metalstack.api.v2.IPService/List\" is not allowed on any of the requested subjects: [abc]"),
		},
		// Roles from Token
		{
			name: "token has no role",
			pat: &api.ProjectsAndTenants{
				ProjectRoles: map[string]apiv2.ProjectRole{
					"abc": apiv2.ProjectRole_PROJECT_ROLE_OWNER,
				},
			},
			token: &apiv2.Token{
				User:      "test",
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
				Permissions: []*apiv2.MethodPermission{
					{
						Subject: "abc",
						Methods: []string{"/metalstack.api.v2.IPService/Get"},
					},
				},
			},
			req: &apiv2.TokenServiceCreateRequest{
				Description: "i want to get a cluster",
				Permissions: []*apiv2.MethodPermission{
					{
						Subject: "abc",
						Methods: []string{"/metalstack.api.v2.IPService/Get"},
					},
				},
				TenantRoles: map[string]apiv2.TenantRole{
					"john@github": apiv2.TenantRole_TENANT_ROLE_OWNER,
				},
				Expires: inOneHour,
			},
			providerTenant: "metal-stack",
			wantErr:        errors.New("requested tenant roles are not allowed: [john@github]"),
		},
		{
			name: "token has to low role",
			pat: &api.ProjectsAndTenants{
				ProjectRoles: map[string]apiv2.ProjectRole{
					"abc": apiv2.ProjectRole_PROJECT_ROLE_OWNER,
				},
			},
			token: &apiv2.Token{
				User:      "test",
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
				Permissions: []*apiv2.MethodPermission{
					{
						Subject: "abc",
						Methods: []string{"/metalstack.api.v2.IPService/Get"},
					},
				},
				TenantRoles: map[string]apiv2.TenantRole{
					"company-a@github": apiv2.TenantRole_TENANT_ROLE_VIEWER,
				},
			},
			req: &apiv2.TokenServiceCreateRequest{
				Description: "i want to get a cluster",
				Permissions: []*apiv2.MethodPermission{
					{
						Subject: "abc",
						Methods: []string{"/metalstack.api.v2.IPService/Get"},
					},
				},
				TenantRoles: map[string]apiv2.TenantRole{
					"company-a@github": apiv2.TenantRole_TENANT_ROLE_EDITOR,
				},
				Expires: inOneHour,
			},
			providerTenant: "metal-stack",
			wantErr:        errors.New("the following method \"/metalstack.api.v2.ProjectService/Create\" is not allowed"),
		},
		{
			name: "token request has unspecified role",
			pat: &api.ProjectsAndTenants{
				ProjectRoles: map[string]apiv2.ProjectRole{
					"abc": apiv2.ProjectRole_PROJECT_ROLE_OWNER,
				},
				TenantRoles: map[string]apiv2.TenantRole{
					"company-a@github": apiv2.TenantRole_TENANT_ROLE_VIEWER,
				},
			},
			token: &apiv2.Token{
				User:      "test",
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
				Permissions: []*apiv2.MethodPermission{
					{
						Subject: "abc",
						Methods: []string{"/metalstack.api.v2.IPService/Get"},
					},
				},
				TenantRoles: map[string]apiv2.TenantRole{
					"company-a@github": apiv2.TenantRole_TENANT_ROLE_VIEWER,
				},
			},
			req: &apiv2.TokenServiceCreateRequest{
				Description: "i want to get a cluster",
				Permissions: []*apiv2.MethodPermission{
					{
						Subject: "abc",
						Methods: []string{"/metalstack.api.v2.IPService/Get"},
					},
				},
				TenantRoles: map[string]apiv2.TenantRole{
					"company-a@github": apiv2.TenantRole_TENANT_ROLE_UNSPECIFIED,
				},
				Expires: inOneHour,
			},
			providerTenant: "metal-stack",
			wantErr:        errors.New("requested tenant role: \"TENANT_ROLE_UNSPECIFIED\" is not allowed"),
		},
		// AdminSubjects
		{
			name:           "requested admin role but is not allowed",
			providerTenant: "metal-stack",
			pat: &api.ProjectsAndTenants{
				TenantRoles: map[string]apiv2.TenantRole{
					"company-a@github": apiv2.TenantRole_TENANT_ROLE_EDITOR,
				},
			},
			token: &apiv2.Token{
				User:      "test",
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
				TenantRoles: map[string]apiv2.TenantRole{
					"company-a@github": apiv2.TenantRole_TENANT_ROLE_EDITOR,
				},
			},
			req: &apiv2.TokenServiceCreateRequest{
				Description: "i want to get admin access",
				AdminRole:   new(apiv2.AdminRole_ADMIN_ROLE_VIEWER),
				Expires:     inOneHour,
			},
			wantErr: errors.New("the following method \"/grpc.reflection.v1.ServerReflection/ServerReflectionInfo\" is not allowed on any of the requested subjects: [*]"),
		},
		{
			name:           "requested admin role but is only viewer of admin orga",
			providerTenant: "company-a@github",
			pat: &api.ProjectsAndTenants{
				TenantRoles: map[string]apiv2.TenantRole{
					"company-a@github": apiv2.TenantRole_TENANT_ROLE_EDITOR,
				},
			},
			token: &apiv2.Token{
				User:      "test",
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
				TenantRoles: map[string]apiv2.TenantRole{
					"company-a@github": apiv2.TenantRole_TENANT_ROLE_VIEWER,
				},
			},
			req: &apiv2.TokenServiceCreateRequest{
				Description: "i want to get admin access",
				AdminRole:   new(apiv2.AdminRole_ADMIN_ROLE_EDITOR),
				Expires:     inOneHour,
			},
			wantErr: errors.New("the following method \"/grpc.reflection.v1.ServerReflection/ServerReflectionInfo\" is not allowed on any of the requested subjects: [*]"),
		},
		{
			name:           "token requested admin role but is editor in admin orga",
			providerTenant: "company-a@github",
			pat: &api.ProjectsAndTenants{
				TenantRoles: map[string]apiv2.TenantRole{
					"company-a@github": apiv2.TenantRole_TENANT_ROLE_EDITOR,
				},
			},
			token: &apiv2.Token{
				User:      "company-a@github",
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
				TenantRoles: map[string]apiv2.TenantRole{
					"company-a@github": apiv2.TenantRole_TENANT_ROLE_EDITOR,
				},
			},
			req: &apiv2.TokenServiceCreateRequest{
				Description: "i want to get admin access",
				AdminRole:   new(apiv2.AdminRole_ADMIN_ROLE_EDITOR),
				Expires:     inOneHour,
			},
			wantErr: errors.New("the following method \"/grpc.reflection.v1.ServerReflection/ServerReflectionInfo\" is not allowed on any of the requested subjects: [*]"),
		},
		{
			name:           "token requested admin role and has admin role editor",
			providerTenant: "company-a@github",
			pat: &api.ProjectsAndTenants{
				TenantRoles: map[string]apiv2.TenantRole{
					"company-a@github": apiv2.TenantRole_TENANT_ROLE_EDITOR,
				},
			},
			token: &apiv2.Token{
				User:      "company-a@github",
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
				TenantRoles: map[string]apiv2.TenantRole{
					"company-a@github": apiv2.TenantRole_TENANT_ROLE_EDITOR,
				},
				AdminRole: new(apiv2.AdminRole_ADMIN_ROLE_EDITOR),
			},
			req: &apiv2.TokenServiceCreateRequest{
				Description: "i want to get admin access",
				AdminRole:   new(apiv2.AdminRole_ADMIN_ROLE_EDITOR),
				Expires:     inOneHour,
			},
			wantErr: nil,
		},
		// Infra Roles
		{
			name:           "admin editor requested infra editor",
			providerTenant: "company-admin@github",
			token: &apiv2.Token{
				User:      "company-admin@github",
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
				AdminRole: apiv2.AdminRole_ADMIN_ROLE_EDITOR.Enum(),
			},
			req: &apiv2.TokenServiceCreateRequest{
				Description: "metal-bmc token",
				InfraRole:   apiv2.InfraRole_INFRA_ROLE_EDITOR.Enum(),
				Expires:     inOneHour,
			},
			wantErr: nil,
		},
		{
			name:           "admin viewer requested infra editor",
			providerTenant: "company-admin@github",
			token: &apiv2.Token{
				User:      "company-admin@github",
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
				AdminRole: apiv2.AdminRole_ADMIN_ROLE_VIEWER.Enum(),
			},
			req: &apiv2.TokenServiceCreateRequest{
				Description: "metal-bmc token",
				InfraRole:   apiv2.InfraRole_INFRA_ROLE_EDITOR.Enum(),
				Expires:     inOneHour,
			},
			wantErr: errors.New("the following method \"/metalstack.infra.v2.BMCService/BMCCommandDone\" is not allowed on any of the requested subjects: [*]"),
		},
		// Mixed role and permissions
		{
			name: "token has no role",
			pat: &api.ProjectsAndTenants{
				ProjectRoles: map[string]apiv2.ProjectRole{
					"ae8d2493-41ec-4efd-bbb4-81085b20b6fe": apiv2.ProjectRole_PROJECT_ROLE_OWNER,
				},
			},
			token: &apiv2.Token{
				User:      "test",
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
				ProjectRoles: map[string]apiv2.ProjectRole{
					"ae8d2493-41ec-4efd-bbb4-81085b20b6fe": apiv2.ProjectRole_PROJECT_ROLE_OWNER,
				},
			},
			req: &apiv2.TokenServiceCreateRequest{
				Description: "i want to get a cluster",
				Permissions: []*apiv2.MethodPermission{
					{
						Subject: "ae8d2493-41ec-4efd-bbb4-81085b20b6fe",
						Methods: []string{"/metalstack.api.v2.IPService/Get"},
					},
					{
						Subject: "internet",
						Methods: []string{"/metalstack.admin.v2.NetworkService/Create"},
					},
				},
				TenantRoles: map[string]apiv2.TenantRole{
					"john@github": apiv2.TenantRole_TENANT_ROLE_OWNER,
				},
				Expires: inOneHour,
			},
			providerTenant: "metal-stack",
			wantErr:        errors.New("requested tenant roles are not allowed: [john@github]"),
		},

		// Machine Roles
		{
			name: "token has no machine role",
			token: &apiv2.Token{
				User:        "test",
				TokenType:   apiv2.TokenType_TOKEN_TYPE_API,
				Permissions: []*apiv2.MethodPermission{},
			},
			req: &apiv2.TokenServiceCreateRequest{
				Description: "i want to get access to a machine",
				MachineRoles: map[string]apiv2.MachineRole{
					"de240964-ff9f-4e3d-95b2-8a96e43788f1": apiv2.MachineRole_MACHINE_ROLE_EDITOR,
				},
				Expires: inOneHour,
			},
			providerTenant: "metal-stack",
			wantErr:        errors.New("requested machine roles are not allowed: [de240964-ff9f-4e3d-95b2-8a96e43788f1]"),
		},
		{
			name: "token has machine role, matching request succeeds",
			pat:  &api.ProjectsAndTenants{},
			token: &apiv2.Token{
				User:        "test",
				TokenType:   apiv2.TokenType_TOKEN_TYPE_API,
				Permissions: []*apiv2.MethodPermission{},
				MachineRoles: map[string]apiv2.MachineRole{
					"de240964-ff9f-4e3d-95b2-8a96e43788f1": apiv2.MachineRole_MACHINE_ROLE_EDITOR,
				},
			},
			req: &apiv2.TokenServiceCreateRequest{
				Description: "i want to get access to a machine",
				MachineRoles: map[string]apiv2.MachineRole{
					"de240964-ff9f-4e3d-95b2-8a96e43788f1": apiv2.MachineRole_MACHINE_ROLE_EDITOR,
				},
				Expires: inOneHour,
			},
			providerTenant: "metal-stack",
			wantErr:        nil,
		},
		{
			name: "token has different machine role, forbidden machine in request",
			pat:  &api.ProjectsAndTenants{},
			token: &apiv2.Token{
				User:        "test",
				TokenType:   apiv2.TokenType_TOKEN_TYPE_API,
				Permissions: []*apiv2.MethodPermission{},
				MachineRoles: map[string]apiv2.MachineRole{
					"de240964-ff9f-4e3d-95b2-8a96e43788f1": apiv2.MachineRole_MACHINE_ROLE_EDITOR,
				},
			},
			req: &apiv2.TokenServiceCreateRequest{
				Description: "i want to get access to a different machine",
				MachineRoles: map[string]apiv2.MachineRole{
					"de240964-ff9f-4e3d-95b2-8a96e43788f2": apiv2.MachineRole_MACHINE_ROLE_EDITOR,
				},
				Expires: inOneHour,
			},
			providerTenant: "metal-stack",
			wantErr:        errors.New("requested machine roles are not allowed: [de240964-ff9f-4e3d-95b2-8a96e43788f2]"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.token.User == "" {
				t.Errorf("no user in token specified")
			}

			if tt.wantErr == nil {
				// Execute proto based validation
				err := protovalidate.Validate(tt.req)
				require.NoError(t, err)
			}

			projectsAndTenantsGetter := func(ctx context.Context, userId string) (*api.ProjectsAndTenants, error) {
				return tt.pat, nil
			}
			log := slog.Default()
			service := tokenService{
				log:                      log,
				tokens:                   nil,
				certs:                    nil,
				issuer:                   "http://test",
				providerTenant:           tt.providerTenant,
				projectsAndTenantsGetter: projectsAndTenantsGetter,
				authorizer:               request.NewAuthorizer(log, projectsAndTenantsGetter),
			}

			gotErr := service.validateTokenRequest(t.Context(), tt.token, tt.req)

			t.Log(gotErr)
			if tt.wantErr != nil {
				require.EqualError(t, gotErr, tt.wantErr.Error())
			} else if gotErr != nil {
				require.NoError(t, gotErr)
			}
		})
	}
}

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
