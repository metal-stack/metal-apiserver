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
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var (
	kubies = "00000000-0000-0000-0000-000000000000"
	token1 = "00000000-0000-0000-0000-000000000000"
)

func Test_Create(t *testing.T) {
	t.Parallel()
	type state struct {
		adminSubjects []string
		projectRoles  map[string]apiv2.ProjectRole
		tenantRoles   map[string]apiv2.TenantRole
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
				Labels: &apiv2.Labels{
					Labels: map[string]string{"a": "b"},
				},
			},
			state: state{
				adminSubjects: []string{},
			},
			wantToken: &apiv2.Token{
				User:        "phippy",
				Description: "empty token",
				TokenType:   apiv2.TokenType_TOKEN_TYPE_API,
				Meta: &apiv2.Meta{
					Labels: &apiv2.Labels{
						Labels: map[string]string{
							"a": "b",
						},
					},
				},
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
				adminSubjects: []string{},
			},
			wantErr:        true,
			wantErrMessage: `permission_denied: the following method "/metalstack.api.v2.IPService/Create" is not allowed`,
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
				adminSubjects: []string{},
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
				Meta:        &apiv2.Meta{},
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
				adminSubjects: []string{},
				projectRoles:  map[string]apiv2.ProjectRole{},
			},
			wantErr:        true,
			wantErrMessage: `permission_denied: the following method "/metalstack.api.v2.IPService/Create" is not allowed`,
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
				adminSubjects: []string{},
				projectRoles: map[string]apiv2.ProjectRole{
					kubies: apiv2.ProjectRole_PROJECT_ROLE_EDITOR,
				},
			},
			wantErr:        true,
			wantErrMessage: `permission_denied: the following method "/metalstack.api.v2.IPService/Create" is not allowed on any of the requested subjects: [00000000-0000-0000-0000-000000000000]`,
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
				adminSubjects: []string{"phippy"},
			},
			wantToken: &apiv2.Token{
				User:         "phippy",
				Description:  "admin token",
				TokenType:    apiv2.TokenType_TOKEN_TYPE_API,
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
				Meta:         &apiv2.Meta{},
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
				adminSubjects: []string{"phippy"},
			},
			wantToken: &apiv2.Token{
				Meta:         &apiv2.Meta{},
				User:         "phippy",
				Description:  "admin token",
				TokenType:    apiv2.TokenType_TOKEN_TYPE_API,
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
				AdminRole:    apiv2.AdminRole_ADMIN_ROLE_VIEWER.Enum(),
			},
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
				adminSubjects: []string{"blippy"},
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
				adminSubjects: []string{"phippy"},
			},
			wantToken: &apiv2.Token{
				Meta:         &apiv2.Meta{},
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
				adminSubjects: []string{},
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
				adminSubjects: []string{},
			},
			wantErr:        true,
			wantErrMessage: `permission_denied: the following method "/metalstack.api.v2.ProjectService/Create" is not allowed`,
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
				adminSubjects: []string{},
				tenantRoles: map[string]apiv2.TenantRole{
					"mascots": apiv2.TenantRole_TENANT_ROLE_EDITOR,
				},
			},
			wantToken: &apiv2.Token{
				Meta:         &apiv2.Meta{},
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
			name: "user without but token with tenant access cannot create tenant token",
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
				adminSubjects: []string{},
				projectRoles:  map[string]apiv2.ProjectRole{},
			},
			wantErr:        true,
			wantErrMessage: `permission_denied: the following method "/metalstack.api.v2.ProjectService/Create" is not allowed`,
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
				adminSubjects: []string{},
				projectRoles:  map[string]apiv2.ProjectRole{},
				tenantRoles: map[string]apiv2.TenantRole{
					"mascots": apiv2.TenantRole_TENANT_ROLE_EDITOR,
				},
			},
			wantErr:        true,
			wantErrMessage: `permission_denied: the following method "/metalstack.api.v2.ProjectService/Create" is not allowed on any of the requested subjects: [mascots]`,
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
				adminSubjects:            tt.state.adminSubjects,
				projectsAndTenantsGetter: projectsAndTenantsGetter,
				tokenCreator: token.NewWithPermissionCheck(&token.TokenWithPermissionCheckConfig{
					TokenWithoutPermissionCheckConfig: token.TokenWithoutPermissionCheckConfig{
						Certs:  certStore,
						Tokens: tokenStore,
						Issuer: "http://test",
					},
					Log:                      log,
					AdminSubjects:            tt.state.adminSubjects,
					Authorizer:               request.NewAuthorizer(log, projectsAndTenantsGetter),
					ProjectsAndTenantsGetter: projectsAndTenantsGetter,
				}),
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

				if diff := cmp.Diff(
					tt.wantToken, got,
					protocmp.Transform(),
					protocmp.IgnoreFields(
						&apiv2.Token{}, "issued_at", "uuid", "expires",
					),
					protocmp.IgnoreFields(
						&apiv2.Meta{}, "created_at", "updated_at",
					),
				); diff != "" {
					t.Errorf("diff: %s", diff)
				}
			}
		})
	}
}

func Test_Update(t *testing.T) {
	t.Parallel()
	type state struct {
		adminSubjects []string
		projectRoles  map[string]apiv2.ProjectRole
		tenantRoles   map[string]apiv2.TenantRole
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
				adminSubjects: []string{},
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
				adminSubjects: []string{},
			},
			wantErr:        true,
			wantErrMessage: `permission_denied: the following method "/metalstack.api.v2.IPService/Create" is not allowed`,
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
				adminSubjects: []string{},
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
				adminSubjects: []string{},
				projectRoles:  map[string]apiv2.ProjectRole{},
			},
			wantErr:        true,
			wantErrMessage: `permission_denied: the following method "/metalstack.api.v2.IPService/Create" is not allowed`,
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
				adminSubjects: []string{},
				projectRoles: map[string]apiv2.ProjectRole{
					kubies: apiv2.ProjectRole_PROJECT_ROLE_EDITOR,
				},
			},
			wantErr:        true,
			wantErrMessage: `permission_denied: the following method "/metalstack.api.v2.IPService/Create" is not allowed on any of the requested subjects: [00000000-0000-0000-0000-000000000000]`,
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
				adminSubjects: []string{"phippy"},
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
				adminSubjects: []string{},
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
				adminSubjects: []string{},
			},
			wantErr:        true,
			wantErrMessage: `permission_denied: the following method "/metalstack.api.v2.ProjectService/Create" is not allowed`,
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
				adminSubjects: []string{},
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
				adminSubjects: []string{},
				projectRoles:  map[string]apiv2.ProjectRole{},
			},
			wantErr:        true,
			wantErrMessage: `permission_denied: the following method "/metalstack.api.v2.ProjectService/Create" is not allowed`,
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
				adminSubjects: []string{},
				projectRoles:  map[string]apiv2.ProjectRole{},
				tenantRoles: map[string]apiv2.TenantRole{
					"mascots": apiv2.TenantRole_TENANT_ROLE_EDITOR,
				},
			},
			wantErr:        true,
			wantErrMessage: `permission_denied: the following method "/metalstack.api.v2.ProjectService/Create" is not allowed on any of the requested subjects: [mascots]`,
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
				adminSubjects: []string{},
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
				adminSubjects:            tt.state.adminSubjects,
				projectsAndTenantsGetter: projectsAndTenantsGetter,
				tokenCreator: token.NewWithPermissionCheck(&token.TokenWithPermissionCheckConfig{
					TokenWithoutPermissionCheckConfig: token.TokenWithoutPermissionCheckConfig{
						Certs:  certStore,
						Tokens: tokenStore,
						Issuer: "http://test",
					},
					Log:                      log,
					AdminSubjects:            tt.state.adminSubjects,
					Authorizer:               request.NewAuthorizer(log, projectsAndTenantsGetter),
					ProjectsAndTenantsGetter: projectsAndTenantsGetter,
				}),
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
			}
		})
	}
}

func Test_Refresh(t *testing.T) {
	t.Parallel()
	iat := time.Now()
	exp := iat.Add(time.Hour)
	type state struct {
		adminSubjects []string
		projectRoles  map[string]apiv2.ProjectRole
		tenantRoles   map[string]apiv2.TenantRole
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
				adminSubjects: []string{},
			},
			wantToken: &apiv2.Token{
				Uuid:         token1,
				User:         "phippy",
				Permissions:  nil,
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
				TokenType:    apiv2.TokenType_TOKEN_TYPE_API,
				IssuedAt:     timestamppb.New(exp),
				Expires:      timestamppb.New(exp.Add(time.Hour)),
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
				adminSubjects: []string{},
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
				adminSubjects:            tt.state.adminSubjects,
				projectsAndTenantsGetter: projectsAndTenantsGetter,
				tokenCreator: token.NewWithPermissionCheck(&token.TokenWithPermissionCheckConfig{
					TokenWithoutPermissionCheckConfig: token.TokenWithoutPermissionCheckConfig{
						Certs:  certStore,
						Tokens: tokenStore,
						Issuer: "http://test",
					},
					Log:                      log,
					AdminSubjects:            tt.state.adminSubjects,
					Authorizer:               request.NewAuthorizer(log, projectsAndTenantsGetter),
					ProjectsAndTenantsGetter: projectsAndTenantsGetter,
				}),
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
			}
		})
	}
}

func Test_List(t *testing.T) {
	t.Parallel()

	log := slog.Default()

	type state struct {
		existingTokens []*apiv2.Token
		adminSubjects  []string
		projectRoles   map[string]apiv2.ProjectRole
		tenantRoles    map[string]apiv2.TenantRole
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

			for _, tok := range tt.state.existingTokens {
				require.NoError(t, tokenStore.Set(t.Context(), tok))
			}

			service := tokenService{
				log:                      log,
				tokens:                   tokenStore,
				certs:                    certStore,
				issuer:                   "http://test",
				adminSubjects:            tt.state.adminSubjects,
				projectsAndTenantsGetter: projectsAndTenantsGetter,
				tokenCreator: token.NewWithPermissionCheck(&token.TokenWithPermissionCheckConfig{
					TokenWithoutPermissionCheckConfig: token.TokenWithoutPermissionCheckConfig{
						Certs:  certStore,
						Tokens: tokenStore,
						Issuer: "http://test",
					},
					Log:                      log,
					AdminSubjects:            tt.state.adminSubjects,
					Authorizer:               request.NewAuthorizer(log, projectsAndTenantsGetter),
					ProjectsAndTenantsGetter: projectsAndTenantsGetter,
				}),
			}

			if tt.wantErr == false {
				// Execute proto based validation
				err := protovalidate.Validate(tt.req)
				require.NoError(t, err)
			}

			response, err := service.List(ctx, tt.req)

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
				if diff := cmp.Diff(
					tt.want, response.Tokens,
					protocmp.Transform(),
					protocmp.IgnoreFields(
						&apiv2.Token{}, "issued_at", "uuid", "expires",
					),
					protocmp.IgnoreFields(
						&apiv2.Meta{}, "created_at", "updated_at",
					),
				); diff != "" {
					t.Errorf("diff: %s", diff)
				}
			}
		})
	}
}
