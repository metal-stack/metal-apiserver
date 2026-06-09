package repository

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"buf.build/go/protovalidate"
	"github.com/alicebob/miniredis/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/go-cmp/cmp"
	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/certs"
	"github.com/metal-stack/metal-apiserver/pkg/repository/api"
	"github.com/metal-stack/metal-apiserver/pkg/token"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/durationpb"
)

func Test_tokenRepository_CreateConsoleTokenWithoutPermissionCheck(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	s := miniredis.RunT(t)
	c := redis.NewClient(&redis.Options{Addr: s.Addr()})

	tokenStore := token.NewRedisStore(c)
	certStore := certs.NewRedisStore(&certs.Config{
		RedisClient: c,
	})

	repo := &tokenRepository{
		s: &Store{
			log:           slog.Default(),
			certs:         certStore,
			tokens:        tokenStore,
			issuer:        "http://test",
			adminSubjects: []string{},
		},
	}

	got, err := repo.CreateUserTokenWithoutPermissionCheck(ctx, "test", new(1*time.Minute))
	require.NoError(t, err)
	// verifying response

	require.NotNil(t, got)
	require.NotNil(t, got)
	require.NotNil(t, got.Token)

	assert.NotEmpty(t, got.Secret)
	assert.True(t, strings.HasPrefix(got.Secret, "ey"), "not a valid jwt token") // jwt always starts with "ey" because it's b64 encoded JSON
	claims, err := parseJWTToken(got.Secret)
	require.NoError(t, err, "token claims not parsable")
	require.NotNil(t, claims)

	assert.NotEmpty(t, got.Token.GetUuid())
	assert.Equal(t, "test", got.Token.GetUser())

	// verifying keydb entry
	err = tokenStore.Set(ctx, got.Token)
	require.NoError(t, err)

	// listing tokens

	tokenList, err := tokenStore.List(ctx, got.Token.User)
	require.NoError(t, err)

	require.NotNil(t, tokenList)
	require.NotNil(t, tokenList)

	require.Len(t, tokenList, 1)

	// Check still present
	_, err = tokenStore.Get(ctx, got.Token.GetUser(), got.Token.GetUuid())
	require.NoError(t, err)

	// Check unpresent after revocation
	err = tokenStore.Revoke(ctx, got.Token.GetUser(), got.Token.GetUuid())
	require.NoError(t, err)

	_, err = tokenStore.Get(ctx, got.Token.GetUser(), got.Token.GetUuid())
	require.Error(t, err)

	// List must now be empty
	tokenList, err = tokenStore.List(ctx, got.Token.User)
	require.NoError(t, err)

	require.Empty(t, tokenList)
}

// parseJWTToken unverified to Claims to get Issuer,Subject, Roles and Permissions
func parseJWTToken(tokenString string) (*token.Claims, error) {
	if tokenString == "" {
		return nil, nil
	}

	claims := &token.Claims{}
	parser := jwt.NewParser()
	_, _, err := parser.ParseUnverified(string(tokenString), claims)

	if err != nil {
		return nil, err
	}

	return claims, nil
}

func Test_tokenRepository_create(t *testing.T) {
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
				adminSubjects: []string{"phippy"},
			},
			wantToken: &apiv2.Token{
				User:        "foo",
				Description: "empty token",
				TokenType:   apiv2.TokenType_TOKEN_TYPE_API,
				Meta:        &apiv2.Meta{},
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
				adminSubjects: []string{"phippy"},
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

			tokenRepo := &tokenRepository{
				s: &Store{
					log:           log,
					certs:         certStore,
					tokens:        tokenStore,
					issuer:        "http://test",
					adminSubjects: tt.state.adminSubjects,
				},
				patg: projectsAndTenantsGetter,
				scope: &UserScope{
					user: *tt.user,
				},
			}

			if tt.wantErr == false {
				// Execute proto based validation
				err := protovalidate.Validate(tt.req)
				require.NoError(t, err)
			}

			req := &adminv2.TokenServiceCreateRequest{
				TokenCreateRequest: tt.req,
				User:               tt.user,
			}

			err := tokenRepo.validateCreate(ctx, req)
			switch {
			case tt.wantErr && err != nil:
				if dff := cmp.Diff(tt.wantErrMessage, err.Error()); dff != "" {
					t.Fatal(dff)
				}

				return
			case tt.wantErr && err == nil:
				t.Fatalf("want error %q, got nil", tt.wantErrMessage)
			case err != nil:
				t.Fatalf("want response, got error %q", err)
			}

			response, err := tokenRepo.create(ctx, req)
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

func Test_tokenRepository_validateTokenRequest(t *testing.T) {
	t.Parallel()
	inOneHour := durationpb.New(time.Hour)
	tests := []struct {
		name          string
		pat           *api.ProjectsAndTenants
		token         *apiv2.Token
		req           *apiv2.TokenServiceCreateRequest
		adminSubjects []string
		wantErr       error
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
			adminSubjects: []string{},
			wantErr:       nil,
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
			adminSubjects: []string{},
			wantErr:       nil,
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
			adminSubjects: []string{},
			wantErr:       nil,
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
			adminSubjects: []string{},
			wantErr:       errors.New("unknown method \"/metalstack.api.v2.UnknownService/Get\""),
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
			adminSubjects: []string{},
			wantErr:       errors.New("method \"/metalstack.api.v2.IPService/Get\" is not allowed on subject \"cde\" with your current user permissions"),
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
			adminSubjects: []string{},
			wantErr:       errors.New("the following method \"/metalstack.api.v2.IPService/List\" is not allowed on any of the requested subjects: [abc]"),
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
			adminSubjects: []string{},
			wantErr:       errors.New("the following method \"/metalstack.api.v2.IPService/List\" is not allowed on any of the requested subjects: [abc]"),
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
			adminSubjects: []string{},
			wantErr:       errors.New("the following method \"/metalstack.api.v2.AuditService/Get\" is not allowed"),
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
			adminSubjects: []string{},
			wantErr:       errors.New("the following method \"/metalstack.api.v2.ProjectService/Create\" is not allowed"),
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
			adminSubjects: []string{},
			wantErr:       errors.New("requested tenant role: \"TENANT_ROLE_UNSPECIFIED\" is not allowed"),
		},
		// AdminSubjects
		{
			name:          "requested admin role but is not allowed",
			adminSubjects: []string{},
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
			name: "requested admin role but is only viewer of admin orga",
			adminSubjects: []string{
				"company-a@github",
			},
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
			name: "token requested admin role but is editor in admin orga",
			adminSubjects: []string{
				"company-a@github",
			},
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
			name: "token requested admin role and has admin role editor",
			adminSubjects: []string{
				"company-a@github",
			},
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
			name: "admin editor requested infra editor",
			adminSubjects: []string{
				"company-admin@github",
			},
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
			name: "admin viewer requested infra editor",
			adminSubjects: []string{
				"company-admin@github",
			},
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
			adminSubjects: []string{},
			wantErr:       errors.New("the following method \"/metalstack.admin.v2.NetworkService/Create\" is not allowed on any of the requested subjects: [internet]"),
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

			tokenRepo := &tokenRepository{
				s: &Store{
					log:           log,
					certs:         nil,
					tokens:        nil,
					issuer:        "http://test",
					adminSubjects: tt.adminSubjects,
				},
				patg: projectsAndTenantsGetter,
			}

			gotErr := tokenRepo.validateTokenRequest(t.Context(), tt.token, tt.req)

			t.Log(gotErr)
			if tt.wantErr != nil {
				require.EqualError(t, gotErr, tt.wantErr.Error())
			} else if gotErr != nil {
				require.NoError(t, gotErr)
			}
		})
	}
}
