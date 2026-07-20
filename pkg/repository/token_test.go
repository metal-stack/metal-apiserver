package repository

import (
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/go-cmp/cmp"
	"github.com/metal-stack/api/go/metalstack/admin/v2/adminv2connect"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/api/go/metalstack/api/v2/apiv2connect"
	"github.com/metal-stack/api/go/metalstack/infra/v2/infrav2connect"
	"github.com/metal-stack/metal-apiserver/pkg/certs"
	"github.com/metal-stack/metal-apiserver/pkg/token"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/testing/protocmp"
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
			log:            slog.Default(),
			certs:          certStore,
			tokens:         tokenStore,
			issuer:         "http://test",
			providerTenant: "metal-stack",
		},
	}

	got, err := repo.CreateUserTokenWithoutPermissionCheck(ctx, "test", new(1*time.Minute))
	require.NoError(t, err)
	// verifying response

	require.NotNil(t, got)
	require.NotNil(t, got)
	require.NotNil(t, got.GetToken())

	assert.NotEmpty(t, got.GetSecret())
	assert.True(t, strings.HasPrefix(got.GetSecret(), "ey"), "not a valid jwt token") // jwt always starts with "ey" because it's b64 encoded JSON
	claims, err := parseJWTToken(got.GetSecret())
	require.NoError(t, err, "token claims not parsable")
	require.NotNil(t, claims)

	assert.NotEmpty(t, got.GetToken().GetUuid())
	assert.Equal(t, "test", got.GetToken().GetUser())

	// verifying keydb entry
	err = tokenStore.Set(ctx, got.GetToken())
	require.NoError(t, err)

	// listing tokens

	tokenList, err := tokenStore.List(ctx, got.Token.User)
	require.NoError(t, err)

	require.NotNil(t, tokenList)
	require.NotNil(t, tokenList)

	require.Len(t, tokenList, 1)

	// Check still present
	_, err = tokenStore.Get(ctx, got.GetToken().GetUser(), got.GetToken().GetUuid())
	require.NoError(t, err)

	// Check unpresent after revocation
	err = tokenStore.Revoke(ctx, got.GetToken().GetUser(), got.GetToken().GetUuid())
	require.NoError(t, err)

	_, err = tokenStore.Get(ctx, got.GetToken().GetUser(), got.GetToken().GetUuid())
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

func TestCompactTypedMethodPermissions(t *testing.T) {
	tests := []struct {
		name  string
		perms []*apiv2.TypedMethodPermission
		want  []*apiv2.TypedMethodPermission
	}{
		{
			name: "combine admin perms",
			perms: []*apiv2.TypedMethodPermission{
				{
					Permissiontype: &apiv2.TypedMethodPermission_Admin{
						Admin: &apiv2.AdminPermissions{
							Methods: []string{adminv2connect.AuditServiceGetProcedure, adminv2connect.AuditServiceListProcedure},
						},
					},
				},
				{
					Permissiontype: &apiv2.TypedMethodPermission_Admin{
						Admin: &apiv2.AdminPermissions{
							Methods: []string{adminv2connect.AuditServiceGetProcedure, adminv2connect.ComponentServiceGetProcedure},
						},
					},
				},
			},
			want: []*apiv2.TypedMethodPermission{
				{
					Permissiontype: &apiv2.TypedMethodPermission_Admin{
						Admin: &apiv2.AdminPermissions{
							Methods: []string{
								adminv2connect.AuditServiceGetProcedure,
								adminv2connect.AuditServiceListProcedure,
								adminv2connect.ComponentServiceGetProcedure,
							},
						},
					},
				},
			},
		},
		{
			name: "all different types",
			perms: []*apiv2.TypedMethodPermission{
				{
					Permissiontype: &apiv2.TypedMethodPermission_Admin{
						Admin: &apiv2.AdminPermissions{
							Methods: []string{adminv2connect.AuditServiceGetProcedure, adminv2connect.AuditServiceListProcedure},
						},
					},
				},
				{
					Permissiontype: &apiv2.TypedMethodPermission_Infra{
						Infra: &apiv2.InfraPermissions{
							Methods: []string{infrav2connect.BMCServiceBMCCommandDoneProcedure},
						},
					},
				},
				{
					Permissiontype: &apiv2.TypedMethodPermission_Machine{
						Machine: &apiv2.MachinePermissions{
							Uuid:    "123",
							Methods: []string{infrav2connect.BootServiceRegisterProcedure},
						},
					},
				},
				{
					Permissiontype: &apiv2.TypedMethodPermission_Project{
						Project: &apiv2.ProjectPermissions{
							Project: "a",
							Methods: []string{apiv2connect.IPServiceCreateProcedure},
						},
					},
				},
				{
					Permissiontype: &apiv2.TypedMethodPermission_Public{
						Public: &apiv2.PublicPermissions{
							Methods: []string{apiv2connect.HealthServiceGetProcedure},
						},
					},
				},
				{
					Permissiontype: &apiv2.TypedMethodPermission_Self{
						Self: &apiv2.SelfPermissions{
							Methods: []string{apiv2connect.TenantServiceListProcedure},
						},
					},
				},
				{
					Permissiontype: &apiv2.TypedMethodPermission_Tenant{
						Tenant: &apiv2.TenantPermissions{
							Login:   "tenant-a",
							Methods: []string{apiv2connect.AuditServiceListProcedure},
						},
					},
				},
				// just repeat the first part here to check that compaction works properly
				{
					Permissiontype: &apiv2.TypedMethodPermission_Admin{
						Admin: &apiv2.AdminPermissions{
							Methods: []string{adminv2connect.AuditServiceGetProcedure, adminv2connect.AuditServiceListProcedure},
						},
					},
				},
				{
					Permissiontype: &apiv2.TypedMethodPermission_Infra{
						Infra: &apiv2.InfraPermissions{
							Methods: []string{infrav2connect.BMCServiceBMCCommandDoneProcedure},
						},
					},
				},
				{
					Permissiontype: &apiv2.TypedMethodPermission_Machine{
						Machine: &apiv2.MachinePermissions{
							Uuid:    "123",
							Methods: []string{infrav2connect.BootServiceRegisterProcedure},
						},
					},
				},
				{
					Permissiontype: &apiv2.TypedMethodPermission_Project{
						Project: &apiv2.ProjectPermissions{
							Project: "a",
							Methods: []string{apiv2connect.IPServiceCreateProcedure},
						},
					},
				},
				{
					Permissiontype: &apiv2.TypedMethodPermission_Public{
						Public: &apiv2.PublicPermissions{
							Methods: []string{apiv2connect.HealthServiceGetProcedure},
						},
					},
				},
				{
					Permissiontype: &apiv2.TypedMethodPermission_Self{
						Self: &apiv2.SelfPermissions{
							Methods: []string{apiv2connect.TenantServiceListProcedure},
						},
					},
				},
				{
					Permissiontype: &apiv2.TypedMethodPermission_Tenant{
						Tenant: &apiv2.TenantPermissions{
							Login:   "tenant-a",
							Methods: []string{apiv2connect.AuditServiceListProcedure},
						},
					},
				},
			},
			want: []*apiv2.TypedMethodPermission{
				{
					Permissiontype: &apiv2.TypedMethodPermission_Admin{
						Admin: &apiv2.AdminPermissions{
							Methods: []string{adminv2connect.AuditServiceGetProcedure, adminv2connect.AuditServiceListProcedure},
						},
					},
				},
				{
					Permissiontype: &apiv2.TypedMethodPermission_Infra{
						Infra: &apiv2.InfraPermissions{
							Methods: []string{infrav2connect.BMCServiceBMCCommandDoneProcedure},
						},
					},
				},
				{
					Permissiontype: &apiv2.TypedMethodPermission_Machine{
						Machine: &apiv2.MachinePermissions{
							Uuid:    "123",
							Methods: []string{infrav2connect.BootServiceRegisterProcedure},
						},
					},
				},
				{
					Permissiontype: &apiv2.TypedMethodPermission_Project{
						Project: &apiv2.ProjectPermissions{
							Project: "a",
							Methods: []string{apiv2connect.IPServiceCreateProcedure},
						},
					},
				},
				{
					Permissiontype: &apiv2.TypedMethodPermission_Public{
						Public: &apiv2.PublicPermissions{
							Methods: []string{apiv2connect.HealthServiceGetProcedure},
						},
					},
				},
				{
					Permissiontype: &apiv2.TypedMethodPermission_Self{
						Self: &apiv2.SelfPermissions{
							Methods: []string{apiv2connect.TenantServiceListProcedure},
						},
					},
				},
				{
					Permissiontype: &apiv2.TypedMethodPermission_Tenant{
						Tenant: &apiv2.TenantPermissions{
							Login:   "tenant-a",
							Methods: []string{apiv2connect.AuditServiceListProcedure},
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CompactTypedMethodPermissions(tt.perms)
			if diff := cmp.Diff(tt.want, got, protocmp.Transform()); diff != "" {
				t.Errorf("diff = %s", diff)
			}
		})
	}
}
