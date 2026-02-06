package method_test

import (
	"log/slog"
	"os"
	"testing"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/testing/protocmp"

	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/api/go/metalstack/api/v2/apiv2connect"
	"github.com/metal-stack/metal-apiserver/pkg/repository"
	"github.com/metal-stack/metal-apiserver/pkg/token"

	"github.com/metal-stack/metal-apiserver/pkg/service/method"
	"github.com/stretchr/testify/require"

	"github.com/metal-stack/metal-apiserver/pkg/test"
)

func Test_methodServiceServer_List(t *testing.T) {
	t.Parallel()

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	testStore, closer := test.StartRepositoryWithCleanup(t, log, test.WithPostgres(true))
	defer closer()

	test.CreateTenants(t, testStore, []*apiv2.TenantServiceCreateRequest{
		{Name: "john.doe@github.com"},
		{Name: "foo.bar@github.com"},
		{Name: "viewer@github.com"},
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
	projectMap := test.CreateProjects(t, testStore, []*apiv2.ProjectServiceCreateRequest{
		{Login: "john.doe@github.com"},
	})
	test.CreateProjectMemberships(t, testStore, projectMap["john.doe@github.com"], []*repository.ProjectMemberCreateRequest{
		{TenantId: "foo.bar@github.com", Role: apiv2.ProjectRole_PROJECT_ROLE_VIEWER},
	})

	tests := []struct {
		name    string
		token   *apiv2.Token
		want    *apiv2.MethodServiceListResponse
		wantErr error
	}{
		{
			name:  "no token",
			token: nil,
			want: &apiv2.MethodServiceListResponse{
				Methods: []string{
					"/grpc.reflection.v1.ServerReflection/ServerReflectionInfo",
					"/grpc.reflection.v1alpha.ServerReflection/ServerReflectionInfo",
					"/metalstack.api.v2.HealthService/Get",
					"/metalstack.api.v2.MethodService/List",
					"/metalstack.api.v2.VersionService/Get",
				},
			},
		},
		{
			name: "api token with one permission",
			token: &apiv2.Token{
				User:      "foo.bar@github.com",
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
				Permissions: []*apiv2.MethodPermission{
					{
						Subject: "foo",
						Methods: []string{apiv2connect.IPServiceGetProcedure},
					},
				},
			},
			want: &apiv2.MethodServiceListResponse{
				Methods: []string{apiv2connect.IPServiceGetProcedure},
			},
			wantErr: nil,
		},
		{
			name: "user token",
			token: &apiv2.Token{
				User:      "foo.bar@github.com",
				TokenType: apiv2.TokenType_TOKEN_TYPE_USER,
			},
			want: &apiv2.MethodServiceListResponse{
				Methods: []string{
					"/grpc.reflection.v1.ServerReflection/ServerReflectionInfo",
					"/grpc.reflection.v1alpha.ServerReflection/ServerReflectionInfo",
					"/metalstack.api.v2.FilesystemService/Get",
					"/metalstack.api.v2.FilesystemService/List",
					"/metalstack.api.v2.FilesystemService/Match",
					"/metalstack.api.v2.HealthService/Get",
					"/metalstack.api.v2.IPService/Get",
					"/metalstack.api.v2.IPService/List",
					"/metalstack.api.v2.ImageService/Get",
					"/metalstack.api.v2.ImageService/Latest",
					"/metalstack.api.v2.ImageService/List",
					"/metalstack.api.v2.MachineService/Get",
					"/metalstack.api.v2.MachineService/List",
					"/metalstack.api.v2.MethodService/List",
					"/metalstack.api.v2.MethodService/TokenScopedList",
					"/metalstack.api.v2.NetworkService/Get",
					"/metalstack.api.v2.NetworkService/List",
					"/metalstack.api.v2.NetworkService/ListBaseNetworks",
					"/metalstack.api.v2.PartitionService/Get",
					"/metalstack.api.v2.PartitionService/List",
					"/metalstack.api.v2.ProjectService/Create",
					"/metalstack.api.v2.ProjectService/Get",
					"/metalstack.api.v2.ProjectService/InviteAccept",
					"/metalstack.api.v2.ProjectService/InviteGet",
					"/metalstack.api.v2.ProjectService/Leave",
					"/metalstack.api.v2.ProjectService/List",
					"/metalstack.api.v2.SizeService/Get",
					"/metalstack.api.v2.SizeService/List",
					"/metalstack.api.v2.TenantService/Create",
					"/metalstack.api.v2.TenantService/Delete",
					"/metalstack.api.v2.TenantService/Get",
					"/metalstack.api.v2.TenantService/Invite",
					"/metalstack.api.v2.TenantService/InviteAccept",
					"/metalstack.api.v2.TenantService/InviteDelete",
					"/metalstack.api.v2.TenantService/InviteGet",
					"/metalstack.api.v2.TenantService/InvitesList",
					"/metalstack.api.v2.TenantService/List",
					"/metalstack.api.v2.TenantService/RemoveMember",
					"/metalstack.api.v2.TenantService/Update",
					"/metalstack.api.v2.TenantService/UpdateMember",
					"/metalstack.api.v2.TokenService/Create",
					"/metalstack.api.v2.TokenService/Get",
					"/metalstack.api.v2.TokenService/List",
					"/metalstack.api.v2.TokenService/Refresh",
					"/metalstack.api.v2.TokenService/Revoke",
					"/metalstack.api.v2.TokenService/Update",
					"/metalstack.api.v2.UserService/Get",
					"/metalstack.api.v2.VersionService/Get",
				},
			},
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			m := method.New(log, testStore.Store)

			ctx := t.Context()
			if tt.token != nil {
				ctx = token.ContextWithToken(t.Context(), tt.token)
			}

			got, gotErr := m.List(ctx, nil)
			if diff := cmp.Diff(got, tt.want, protocmp.Transform()); diff != "" {
				t.Errorf("diff = %s", diff)
			}

			if tt.wantErr != nil {
				require.EqualError(t, gotErr, tt.wantErr.Error())
			} else {
				require.NoError(t, gotErr)
			}
		})
	}
}
