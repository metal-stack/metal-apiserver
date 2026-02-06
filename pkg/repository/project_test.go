package repository_test

import (
	"log/slog"
	"os"
	"sort"
	"testing"

	"github.com/google/go-cmp/cmp"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	"github.com/metal-stack/metal-apiserver/pkg/repository"
	"github.com/metal-stack/metal-apiserver/pkg/test"
	"github.com/metal-stack/metal-lib/pkg/testcommon"
	"google.golang.org/protobuf/testing/protocmp"
)

func Test_projectRepository_GetProjectsAndTenants(t *testing.T) {
	t.Parallel()

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	testStore, closer := test.StartRepositoryWithCleanup(t, log, test.WithPostgres(true))
	defer closer()

	test.CreateTenants(t, testStore, []*apiv2.TenantServiceCreateRequest{
		{Name: "john.doe@github.com"},
		{Name: "viewer@github.com"},
		{Name: "guest@github.com"},
		{Name: "ron@github.com"},
		{Name: "maddy@github.com"},
	})
	test.CreateTenantMemberships(t, testStore, "john.doe@github.com", []*repository.TenantMemberCreateRequest{
		{MemberID: "john.doe@github.com", Role: apiv2.TenantRole_TENANT_ROLE_OWNER},
		{MemberID: "viewer@github.com", Role: apiv2.TenantRole_TENANT_ROLE_VIEWER},
		{MemberID: "ron@github.com", Role: apiv2.TenantRole_TENANT_ROLE_EDITOR},
		{MemberID: "maddy@github.com", Role: apiv2.TenantRole_TENANT_ROLE_VIEWER},
	})
	test.CreateTenantMemberships(t, testStore, "viewer@github.com", []*repository.TenantMemberCreateRequest{
		{MemberID: "viewer@github.com", Role: apiv2.TenantRole_TENANT_ROLE_OWNER},
	})
	test.CreateTenantMemberships(t, testStore, "guest@github.com", []*repository.TenantMemberCreateRequest{
		{MemberID: "guest@github.com", Role: apiv2.TenantRole_TENANT_ROLE_OWNER},
	})
	test.CreateTenantMemberships(t, testStore, "ron@github.com", []*repository.TenantMemberCreateRequest{
		{MemberID: "ron@github.com", Role: apiv2.TenantRole_TENANT_ROLE_OWNER},
	})
	test.CreateTenantMemberships(t, testStore, "maddy@github.com", []*repository.TenantMemberCreateRequest{
		{MemberID: "maddy@github.com", Role: apiv2.TenantRole_TENANT_ROLE_OWNER},
	})
	projectMap := test.CreateProjects(t, testStore, []*apiv2.ProjectServiceCreateRequest{
		{Login: "john.doe@github.com"},
	})
	test.CreateProjectMemberships(t, testStore, projectMap["john.doe@github.com"], []*repository.ProjectMemberCreateRequest{
		{TenantId: "guest@github.com", Role: apiv2.ProjectRole_PROJECT_ROLE_VIEWER},
		{TenantId: "ron@github.com", Role: apiv2.ProjectRole_PROJECT_ROLE_VIEWER},
		{TenantId: "maddy@github.com", Role: apiv2.ProjectRole_PROJECT_ROLE_EDITOR},
	})

	tests := []struct {
		name    string
		userId  string
		want    *repository.ProjectsAndTenants
		wantErr error
	}{
		{
			name:   "simple",
			userId: "john.doe@github.com",
			want: &repository.ProjectsAndTenants{
				Projects:      []*apiv2.Project{{Uuid: projectMap["john.doe@github.com"], Tenant: "john.doe@github.com"}},
				Tenants:       []*apiv2.Tenant{{Login: "john.doe@github.com", Name: "john.doe@github.com", CreatedBy: "john.doe@github.com"}},
				DefaultTenant: &apiv2.Tenant{Login: "john.doe@github.com", Name: "john.doe@github.com", CreatedBy: "john.doe@github.com"},
				ProjectRoles: map[string]apiv2.ProjectRole{
					projectMap["john.doe@github.com"]: apiv2.ProjectRole_PROJECT_ROLE_OWNER,
				},
				TenantRoles: map[string]apiv2.TenantRole{
					"john.doe@github.com": apiv2.TenantRole_TENANT_ROLE_OWNER,
				},
			},
			wantErr: nil,
		},
		{
			name:   "not simple",
			userId: "viewer@github.com",
			want: &repository.ProjectsAndTenants{
				Projects: []*apiv2.Project{{Uuid: projectMap["john.doe@github.com"], Tenant: "john.doe@github.com"}},
				Tenants: []*apiv2.Tenant{
					{Login: "john.doe@github.com", Name: "john.doe@github.com", CreatedBy: "john.doe@github.com"},
					{Login: "viewer@github.com", Name: "viewer@github.com", CreatedBy: "viewer@github.com"},
				},
				DefaultTenant: &apiv2.Tenant{Login: "viewer@github.com", Name: "viewer@github.com", CreatedBy: "viewer@github.com"},
				ProjectRoles: map[string]apiv2.ProjectRole{
					projectMap["john.doe@github.com"]: apiv2.ProjectRole_PROJECT_ROLE_VIEWER,
				},
				TenantRoles: map[string]apiv2.TenantRole{
					"viewer@github.com":   apiv2.TenantRole_TENANT_ROLE_OWNER,
					"john.doe@github.com": apiv2.TenantRole_TENANT_ROLE_VIEWER,
				},
			},
			wantErr: nil,
		},
		{
			name:   "even more complicated",
			userId: "guest@github.com",
			want: &repository.ProjectsAndTenants{
				Projects: []*apiv2.Project{{Uuid: projectMap["john.doe@github.com"], Tenant: "john.doe@github.com"}},
				Tenants: []*apiv2.Tenant{
					{Login: "guest@github.com", Name: "guest@github.com", CreatedBy: "guest@github.com"},
					{Login: "john.doe@github.com", Name: "john.doe@github.com", CreatedBy: "john.doe@github.com"},
				},
				DefaultTenant: &apiv2.Tenant{Login: "guest@github.com", Name: "guest@github.com", CreatedBy: "guest@github.com"},
				ProjectRoles: map[string]apiv2.ProjectRole{
					projectMap["john.doe@github.com"]: apiv2.ProjectRole_PROJECT_ROLE_VIEWER,
				},
				TenantRoles: map[string]apiv2.TenantRole{
					"guest@github.com":    apiv2.TenantRole_TENANT_ROLE_OWNER,
					"john.doe@github.com": apiv2.TenantRole_TENANT_ROLE_GUEST,
				},
			},
			wantErr: nil,
		},
		{
			name:   "higher tenant role overrules explicit project role",
			userId: "ron@github.com",
			want: &repository.ProjectsAndTenants{
				Projects: []*apiv2.Project{{Uuid: projectMap["john.doe@github.com"], Tenant: "john.doe@github.com"}},
				Tenants: []*apiv2.Tenant{
					{Login: "john.doe@github.com", Name: "john.doe@github.com", CreatedBy: "john.doe@github.com"},
					{Login: "ron@github.com", Name: "ron@github.com", CreatedBy: "ron@github.com"},
				},
				DefaultTenant: &apiv2.Tenant{Login: "ron@github.com", Name: "ron@github.com", CreatedBy: "ron@github.com"},
				ProjectRoles: map[string]apiv2.ProjectRole{
					projectMap["john.doe@github.com"]: apiv2.ProjectRole_PROJECT_ROLE_EDITOR,
				},
				TenantRoles: map[string]apiv2.TenantRole{
					"ron@github.com":      apiv2.TenantRole_TENANT_ROLE_OWNER,
					"john.doe@github.com": apiv2.TenantRole_TENANT_ROLE_EDITOR,
				},
			},
			wantErr: nil,
		},
		{
			name:   "higher project role overrules tenant role",
			userId: "maddy@github.com",
			want: &repository.ProjectsAndTenants{
				Projects: []*apiv2.Project{{Uuid: projectMap["john.doe@github.com"], Tenant: "john.doe@github.com"}},
				Tenants: []*apiv2.Tenant{
					{Login: "john.doe@github.com", Name: "john.doe@github.com", CreatedBy: "john.doe@github.com"},
					{Login: "maddy@github.com", Name: "maddy@github.com", CreatedBy: "maddy@github.com"},
				},
				DefaultTenant: &apiv2.Tenant{Login: "maddy@github.com", Name: "maddy@github.com", CreatedBy: "maddy@github.com"},
				ProjectRoles: map[string]apiv2.ProjectRole{
					projectMap["john.doe@github.com"]: apiv2.ProjectRole_PROJECT_ROLE_EDITOR,
				},
				TenantRoles: map[string]apiv2.TenantRole{
					"maddy@github.com":    apiv2.TenantRole_TENANT_ROLE_OWNER,
					"john.doe@github.com": apiv2.TenantRole_TENANT_ROLE_VIEWER,
				},
			},
			wantErr: nil,
		},
		{
			name:   "unknown user",
			userId: "bud@github.com",
			want:   nil,
			// FIXME this error should be a NotFound, but this would require a additional masterdata call
			wantErr: errorutil.Internal("unable to find a default tenant for user: bud@github.com"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, gotErr := testStore.UnscopedProject().AdditionalMethods().GetProjectsAndTenants(t.Context(), tt.userId)

			if diff := cmp.Diff(tt.wantErr, gotErr, testcommon.ErrorStringComparer()); diff != "" {
				t.Errorf("GetProjectsAndTenants() failed: %v", diff)
			}

			if tt.want == nil && got == nil {
				return
			}

			sort.SliceStable(got.Tenants, func(i, j int) bool {
				return got.Tenants[i].Login < got.Tenants[j].Login
			})

			if diff := cmp.Diff(tt.want, got, protocmp.Transform(),
				protocmp.IgnoreFields(
					&apiv2.Project{}, "meta",
				),
				protocmp.IgnoreFields(
					&apiv2.Tenant{}, "meta",
				),
			); diff != "" {
				t.Errorf("GetProjectsAndTenants() failed: %v", diff)
			}
		})
	}
}
