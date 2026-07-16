package request

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"connectrpc.com/connect"
	"github.com/metal-stack/api/go/errorutil"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/api/go/metalstack/api/v2/apiv2connect"
	"github.com/metal-stack/api/go/metalstack/infra/v2/infrav2connect"
	"github.com/metal-stack/metal-apiserver/pkg/repository/api"
	"github.com/stretchr/testify/require"
)

func Test_authorizer_allowed(t *testing.T) {
	tests := []struct {
		name               string
		token              *apiv2.Token
		projectsAndTenants *api.ProjectsAndTenants
		method             string
		subject            string
		wantErr            error
	}{
		{
			name:    "nil token, access to public endpoint allowed",
			token:   nil,
			method:  apiv2connect.VersionServiceGetProcedure,
			wantErr: nil,
		},
		{
			name:    "nil token, access to non public endpoint is not allowed",
			token:   nil,
			method:  "/metalstack.api.v2.PartitionService/List",
			wantErr: errorutil.PermissionDenied("access to:\"/metalstack.api.v2.PartitionService/List\" is not allowed because it is not part of the token permissions"),
		},
		{
			name: "one permission, api token",
			token: &apiv2.Token{
				User:      "user-a",
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
				Permissions: []*apiv2.MethodPermission{
					{Subject: "project-a", Methods: []string{"/metalstack.api.v2.IPService/Get"}},
				},
			},
			projectsAndTenants: &api.ProjectsAndTenants{
				ProjectRoles: map[string]apiv2.ProjectRole{
					"project-a": apiv2.ProjectRole_PROJECT_ROLE_VIEWER,
				},
			},
			method:  apiv2connect.IPServiceGetProcedure,
			subject: "project-a",
			wantErr: nil,
		},
		{
			name: "one infra permission, api token",
			token: &apiv2.Token{
				User: "user-a",
				Permissions: []*apiv2.MethodPermission{
					{Subject: "*", Methods: []string{infrav2connect.SwitchServiceRegisterProcedure}},
				},
			},
			method:  infrav2connect.SwitchServiceRegisterProcedure,
			subject: "switch01",
			wantErr: nil,
		},
		{
			name: "one permission, api token, access not allowed, wrong method",
			token: &apiv2.Token{
				User:      "user-a",
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
				Permissions: []*apiv2.MethodPermission{
					{Subject: "project-a", Methods: []string{"/metalstack.api.v2.IPService/Get"}},
				},
			},
			method:  apiv2connect.IPServiceCreateProcedure,
			subject: "project-a",
			wantErr: errorutil.PermissionDenied("access to:\"/metalstack.api.v2.IPService/Create\" is not allowed because it is not part of the token permissions"),
		},
		{
			name: "one permission, api token, access not allowed, wrong project",
			token: &apiv2.Token{
				User:      "user-a",
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
				Permissions: []*apiv2.MethodPermission{
					{Subject: "project-a", Methods: []string{"/metalstack.api.v2.IPService/Get"}},
				},
			},
			projectsAndTenants: &api.ProjectsAndTenants{
				ProjectRoles: map[string]apiv2.ProjectRole{
					"project-a": apiv2.ProjectRole_PROJECT_ROLE_VIEWER,
				},
			},
			method:  apiv2connect.IPServiceGetProcedure,
			subject: "project-b",
			wantErr: errorutil.PermissionDenied("access to:\"/metalstack.api.v2.IPService/Get\" with subject:\"project-b\" is not allowed because it is not part of the token permissions, allowed subjects are:[\"project-a\"]"),
		},
		{
			name: "admin editor access",
			token: &apiv2.Token{
				User:      "admin",
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
				AdminRole: apiv2.AdminRole_ADMIN_ROLE_EDITOR.Enum(),
			},
			method:  apiv2connect.IPServiceGetProcedure,
			subject: "project-b",
			wantErr: nil,
		},
		{
			name: "admin viewer access",
			token: &apiv2.Token{
				User:      "admin",
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
				AdminRole: apiv2.AdminRole_ADMIN_ROLE_VIEWER.Enum(),
			},
			method:  apiv2connect.IPServiceGetProcedure,
			subject: "project-b",
			wantErr: nil,
		},
		{
			name: "infra editor access",
			token: &apiv2.Token{
				User:      "metal-core",
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
				InfraRole: apiv2.InfraRole_INFRA_ROLE_EDITOR.Enum(),
			},
			method:  infrav2connect.SwitchServiceRegisterProcedure,
			subject: "",
			wantErr: nil,
		},
		{
			name: "infra viewer access",
			token: &apiv2.Token{
				User:      "metal-core",
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
				InfraRole: apiv2.InfraRole_INFRA_ROLE_VIEWER.Enum(),
			},
			method:  infrav2connect.SwitchServiceRegisterProcedure,
			subject: "project-b",
			wantErr: errorutil.PermissionDenied("access to:\"/metalstack.infra.v2.SwitchService/Register\" is not allowed because it is not part of the token permissions"),
		},
		{
			name: "infra viewer access",
			token: &apiv2.Token{
				User:      "metal-core",
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
				InfraRole: apiv2.InfraRole_INFRA_ROLE_VIEWER.Enum(),
			},
			method:  infrav2connect.SwitchServiceGetProcedure,
			subject: "project-b",
			wantErr: nil,
		},
		{
			name: "user token, tenant owner with inherited project viewer",
			token: &apiv2.Token{
				User:      "user-b",
				TokenType: apiv2.TokenType_TOKEN_TYPE_USER,
				TenantRoles: map[string]apiv2.TenantRole{
					"tenant-a": apiv2.TenantRole_TENANT_ROLE_OWNER,
				},
			},
			method:  apiv2connect.IPServiceGetProcedure,
			subject: "project-b",
			projectsAndTenants: &api.ProjectsAndTenants{
				ProjectRoles: map[string]apiv2.ProjectRole{
					"project-b": apiv2.ProjectRole_PROJECT_ROLE_EDITOR,
				},
			},
			wantErr: nil,
		},
		{
			name: "api token, tenant owner with inherited project viewer",
			token: &apiv2.Token{
				User:      "user-b",
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
				TenantRoles: map[string]apiv2.TenantRole{
					"tenant-a": apiv2.TenantRole_TENANT_ROLE_OWNER,
				},
			},
			method:  apiv2connect.IPServiceGetProcedure,
			subject: "project-b",
			projectsAndTenants: &api.ProjectsAndTenants{
				ProjectRoles: map[string]apiv2.ProjectRole{
					"project-b": apiv2.ProjectRole_PROJECT_ROLE_EDITOR,
				},
			},
			wantErr: errorutil.PermissionDenied(`access to:"/metalstack.api.v2.IPService/Get" is not allowed because it is not part of the token permissions`),
		},
		{
			name: "api token, permissions to projects failed",
			token: &apiv2.Token{
				User:      "user-b",
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
				Permissions: []*apiv2.MethodPermission{
					{
						Subject: "*",
						Methods: []string{"/metalstack.api.v2.MachineService/Create"},
					},
				},
			},
			method:  apiv2connect.MachineServiceCreateProcedure,
			subject: "project-b",
			projectsAndTenants: &api.ProjectsAndTenants{
				Projects: []*apiv2.Project{
					{Uuid: "project-a"},
				},
				ProjectRoles: map[string]apiv2.ProjectRole{
					"project-a": apiv2.ProjectRole_PROJECT_ROLE_EDITOR,
				},
			},
			wantErr: errorutil.PermissionDenied(`access to:"/metalstack.api.v2.MachineService/Create" with subject:"project-b" is not allowed because it is not part of the token permissions, allowed subjects are:["project-a"]`),
		},
		{
			name: "api token, permissions to projects",
			token: &apiv2.Token{
				User:      "user-b",
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
				Permissions: []*apiv2.MethodPermission{
					{
						Subject: "*",
						Methods: []string{"/metalstack.api.v2.MachineService/Create"},
					},
				},
			},
			method:  apiv2connect.MachineServiceCreateProcedure,
			subject: "project-a",
			projectsAndTenants: &api.ProjectsAndTenants{
				Projects: []*apiv2.Project{
					{Uuid: "project-a"},
				},
				ProjectRoles: map[string]apiv2.ProjectRole{
					"project-a": apiv2.ProjectRole_PROJECT_ROLE_EDITOR,
				},
			},
			wantErr: nil,
		},
		{
			name: "api token, permissions to tenants failed",
			token: &apiv2.Token{
				User:      "user-b",
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
				Permissions: []*apiv2.MethodPermission{
					{
						Subject: "*",
						Methods: []string{"/metalstack.api.v2.MachineService/Create"},
					},
					{
						Subject: "*",
						Methods: []string{"/metalstack.api.v2.ProjectService/Create"},
					},
				},
			},
			method:  apiv2connect.ProjectServiceCreateProcedure,
			subject: "tenant-b",
			projectsAndTenants: &api.ProjectsAndTenants{
				Projects: []*apiv2.Project{
					{Uuid: "project-a"},
				},
				Tenants: []*apiv2.Tenant{
					{Login: "tenant-a"},
				},
				TenantRoles: map[string]apiv2.TenantRole{
					"tenant-a": apiv2.TenantRole_TENANT_ROLE_OWNER,
				},
			},
			wantErr: errorutil.PermissionDenied(`access to:"/metalstack.api.v2.ProjectService/Create" with subject:"tenant-b" is not allowed because it is not part of the token permissions, allowed subjects are:["tenant-a"]`),
		},
		{
			name: "api token, permissions to tenants",
			token: &apiv2.Token{
				User:      "user-b",
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
				Permissions: []*apiv2.MethodPermission{
					{
						Subject: "*",
						Methods: []string{"/metalstack.api.v2.MachineService/Create"},
					},
					{
						Subject: "*",
						Methods: []string{"/metalstack.api.v2.ProjectService/Create"},
					},
				},
			},
			method:  apiv2connect.ProjectServiceCreateProcedure,
			subject: "tenant-a",
			projectsAndTenants: &api.ProjectsAndTenants{
				Projects: []*apiv2.Project{
					{Uuid: "project-a"},
				},
				Tenants: []*apiv2.Tenant{
					{Login: "tenant-a"},
				},
				TenantRoles: map[string]apiv2.TenantRole{
					"tenant-a": apiv2.TenantRole_TENANT_ROLE_OWNER,
				},
			},
			wantErr: nil,
		},
		{
			name: "api token, multiple permissions with different subjects allowed",
			token: &apiv2.Token{
				User:      "user-c",
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
				Permissions: []*apiv2.MethodPermission{
					{Subject: "project-a", Methods: []string{"/metalstack.api.v2.IPService/Get"}},
					{Subject: "project-b", Methods: []string{"/metalstack.api.v2.IPService/Get"}},
				},
			},
			method:  apiv2connect.IPServiceGetProcedure,
			subject: "project-a",
			projectsAndTenants: &api.ProjectsAndTenants{
				ProjectRoles: map[string]apiv2.ProjectRole{
					"project-a": apiv2.ProjectRole_PROJECT_ROLE_VIEWER,
					"project-b": apiv2.ProjectRole_PROJECT_ROLE_VIEWER,
				},
			},
			wantErr: nil,
		},
		{
			name: "api token, multiple permissions with different subjects denied",
			token: &apiv2.Token{
				User:      "user-c",
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
				Permissions: []*apiv2.MethodPermission{
					{Subject: "project-a", Methods: []string{"/metalstack.api.v2.IPService/Get"}},
					{Subject: "project-b", Methods: []string{"/metalstack.api.v2.IPService/Get"}},
				},
			},
			method:  apiv2connect.IPServiceGetProcedure,
			subject: "project-c",
			projectsAndTenants: &api.ProjectsAndTenants{
				ProjectRoles: map[string]apiv2.ProjectRole{
					"project-a": apiv2.ProjectRole_PROJECT_ROLE_VIEWER,
					"project-b": apiv2.ProjectRole_PROJECT_ROLE_VIEWER,
				},
			},
			wantErr: errorutil.PermissionDenied("access to:\"/metalstack.api.v2.IPService/Get\" with subject:\"project-c\" is not allowed because it is not part of the token permissions, allowed subjects are:[\"project-a\" \"project-b\"]"),
		},
		{
			name: "api token, empty permissions list denied",
			token: &apiv2.Token{
				User:        "user-d",
				TokenType:   apiv2.TokenType_TOKEN_TYPE_API,
				Permissions: []*apiv2.MethodPermission{},
			},
			method:  apiv2connect.IPServiceGetProcedure,
			subject: "project-a",
			wantErr: errorutil.PermissionDenied("access to:\"/metalstack.api.v2.IPService/Get\" is not allowed because it is not part of the token permissions"),
		},
		{
			name: "user token without tenant roles on non-public method denied",
			token: &apiv2.Token{
				User:      "user-e",
				TokenType: apiv2.TokenType_TOKEN_TYPE_USER,
			},
			method:  apiv2connect.IPServiceGetProcedure,
			subject: "project-a",
			projectsAndTenants: &api.ProjectsAndTenants{
				ProjectRoles: map[string]apiv2.ProjectRole{},
			},
			wantErr: errorutil.PermissionDenied("access to:\"/metalstack.api.v2.IPService/Get\" is not allowed because it is not part of the token permissions"),
		},
		{
			name: "admin editor all methods allowed regardless of permission",
			token: &apiv2.Token{
				User:      "admin-x",
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
				AdminRole: apiv2.AdminRole_ADMIN_ROLE_EDITOR.Enum(),
			},
			method:  apiv2connect.MachineServiceCreateProcedure,
			subject: "project-f",
			wantErr: nil,
		},
		{
			name: "admin viewer on viewer method allowed",
			token: &apiv2.Token{
				User:      "admin-v",
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
				AdminRole: apiv2.AdminRole_ADMIN_ROLE_VIEWER.Enum(),
			},
			method:  apiv2connect.IPServiceGetProcedure,
			subject: "project-g",
			wantErr: nil,
		},
		{
			name: "infra viewer on infra viewer method allowed",
			token: &apiv2.Token{
				User:      "metal-core",
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
				InfraRole: apiv2.InfraRole_INFRA_ROLE_VIEWER.Enum(),
			},
			method:  infrav2connect.SwitchServiceGetProcedure,
			subject: "",
			wantErr: nil,
		},
		{
			name: "api token with wildcard subject and specific method allowed",
			token: &apiv2.Token{
				User:      "user-wildcard",
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
				Permissions: []*apiv2.MethodPermission{
					{Subject: "*", Methods: []string{"/metalstack.api.v2.IPService/Get"}},
				},
			},
			method:  apiv2connect.IPServiceGetProcedure,
			subject: "project-h",
			projectsAndTenants: &api.ProjectsAndTenants{
				Projects: []*apiv2.Project{
					{Uuid: "project-h"},
				},
				ProjectRoles: map[string]apiv2.ProjectRole{
					"project-h": apiv2.ProjectRole_PROJECT_ROLE_VIEWER,
				},
			},
			wantErr: nil,
		},
		{
			name: "api token wildcard project subject but project not in user scope denied",
			token: &apiv2.Token{
				User:      "user-scoped",
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
				Permissions: []*apiv2.MethodPermission{
					{Subject: "*", Methods: []string{"/metalstack.api.v2.IPService/Get"}},
				},
			},
			method:  apiv2connect.IPServiceGetProcedure,
			subject: "project-unauthorized",
			projectsAndTenants: &api.ProjectsAndTenants{
				ProjectRoles: map[string]apiv2.ProjectRole{
					"project-h": apiv2.ProjectRole_PROJECT_ROLE_VIEWER,
				},
			},
			wantErr: errorutil.PermissionDenied("access to:\"/metalstack.api.v2.IPService/Get\" with subject:\"project-unauthorized\" is not allowed because it is not part of the token permissions, allowed subjects are:[\"project-h\"]"),
		},
		{
			name: "api token combined tenant and project scope with * permissions",
			token: &apiv2.Token{
				User:      "user-multi",
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
				Permissions: []*apiv2.MethodPermission{
					{Subject: "*", Methods: []string{"/metalstack.api.v2.ProjectService/Create"}},
					{Subject: "*", Methods: []string{"/metalstack.api.v2.MachineService/Create"}},
				},
			},
			method:  apiv2connect.ProjectServiceCreateProcedure,
			subject: "tenant-multi",
			projectsAndTenants: &api.ProjectsAndTenants{
				Projects: []*apiv2.Project{
					{Uuid: "project-multi"},
				},
				Tenants: []*apiv2.Tenant{
					{Login: "tenant-multi"},
				},
				TenantRoles: map[string]apiv2.TenantRole{
					"tenant-multi": apiv2.TenantRole_TENANT_ROLE_OWNER,
				},
				ProjectRoles: map[string]apiv2.ProjectRole{
					"project-multi": apiv2.ProjectRole_PROJECT_ROLE_EDITOR,
				},
			},
			wantErr: nil,
		},
		{
			name:    "nil token on public version method allowed",
			token:   nil,
			method:  apiv2connect.VersionServiceGetProcedure,
			wantErr: nil,
		},
		{
			name: "empty string subject with wildcard permission allowed",
			token: &apiv2.Token{
				User:      "user-empty",
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
				Permissions: []*apiv2.MethodPermission{
					{Subject: "*", Methods: []string{infrav2connect.SwitchServiceRegisterProcedure}},
				},
			},
			method:  infrav2connect.SwitchServiceRegisterProcedure,
			subject: "",
			wantErr: nil,
		},
		{
			name: "api token with wildcard permission to non-project method no subject",
			token: &apiv2.Token{
				User:      "user-nonproj",
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
				Permissions: []*apiv2.MethodPermission{
					{Subject: "*", Methods: []string{"/metalstack.api.v2.SelfService/Me"}},
				},
			},
			method:  "/metalstack.api.v2.SelfService/Me",
			subject: "",
			wantErr: errorutil.PermissionDenied(`requested procedure "/metalstack.api.v2.SelfService/Me" is not known`),
		},
		{
			name: "user token with tenant viewer role on tenant scoped method",
			token: &apiv2.Token{
				User:      "user-tenant",
				TokenType: apiv2.TokenType_TOKEN_TYPE_USER,
				TenantRoles: map[string]apiv2.TenantRole{
					"tenant-viewer": apiv2.TenantRole_TENANT_ROLE_VIEWER,
				},
			},
			method:  apiv2connect.ProjectServiceListProcedure,
			subject: "tenant-viewer",
			projectsAndTenants: &api.ProjectsAndTenants{
				TenantRoles: map[string]apiv2.TenantRole{
					"tenant-viewer": apiv2.TenantRole_TENANT_ROLE_VIEWER,
				},
			},
			wantErr: nil,
		},
		{
			name: "api token with machine editor role allowed on boot service",
			token: &apiv2.Token{
				User:         "user-machine",
				TokenType:    apiv2.TokenType_TOKEN_TYPE_API,
				MachineRoles: map[string]apiv2.MachineRole{"machine-02": apiv2.MachineRole_MACHINE_ROLE_EDITOR},
			},
			method:  "/metalstack.infra.v2.BootService/SuperUserPassword",
			subject: "machine-02",
			wantErr: nil,
		},
		{
			name: "api token with machine editor role denied on non-boot method",
			token: &apiv2.Token{
				User:         "user-machine",
				TokenType:    apiv2.TokenType_TOKEN_TYPE_API,
				MachineRoles: map[string]apiv2.MachineRole{"machine-02": apiv2.MachineRole_MACHINE_ROLE_EDITOR},
			},
			method:  apiv2connect.MachineServiceDeleteProcedure,
			subject: "machine-02",
			wantErr: errorutil.PermissionDenied("access to:\"/metalstack.api.v2.MachineService/Delete\" is not allowed because it is not part of the token permissions"),
		},
		{
			name: "api token user with project viewer role allowed",
			token: &apiv2.Token{
				User:      "user-proj",
				TokenType: apiv2.TokenType_TOKEN_TYPE_USER,
				ProjectRoles: map[string]apiv2.ProjectRole{
					"proj-viewer": apiv2.ProjectRole_PROJECT_ROLE_VIEWER,
				},
			},
			method:  apiv2connect.IPServiceGetProcedure,
			subject: "proj-viewer",
			projectsAndTenants: &api.ProjectsAndTenants{
				ProjectRoles: map[string]apiv2.ProjectRole{
					"proj-viewer": apiv2.ProjectRole_PROJECT_ROLE_VIEWER,
				},
			},
			wantErr: nil,
		},
		{
			name: "admin viewer not allowed on editor-only method",
			token: &apiv2.Token{
				User:      "admin-wrong",
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
				AdminRole: apiv2.AdminRole_ADMIN_ROLE_VIEWER.Enum(),
			},
			method:  apiv2connect.MachineServiceCreateProcedure,
			subject: "project-x",
			wantErr: errorutil.PermissionDenied("access to:\"/metalstack.api.v2.MachineService/Create\" is not allowed because it is not part of the token permissions"),
		},
		{
			name: "api token with machine editor role allowed on BootServiceRegister with correct UUID",
			token: &apiv2.Token{
				User:         "machine-prov",
				TokenType:    apiv2.TokenType_TOKEN_TYPE_API,
				MachineRoles: map[string]apiv2.MachineRole{"abc-def-123": apiv2.MachineRole_MACHINE_ROLE_EDITOR},
			},
			method:  infrav2connect.BootServiceRegisterProcedure,
			subject: "abc-def-123",
			wantErr: nil,
		},
		{
			name: "api token with machine editor role denied on BootServiceRegister with wrong UUID",
			token: &apiv2.Token{
				User:         "machine-prov",
				TokenType:    apiv2.TokenType_TOKEN_TYPE_API,
				MachineRoles: map[string]apiv2.MachineRole{"abc-def-123": apiv2.MachineRole_MACHINE_ROLE_EDITOR},
			},
			method:  infrav2connect.BootServiceRegisterProcedure,
			subject: "wrong-uuid-456",
			wantErr: errorutil.PermissionDenied("access to:\"/metalstack.infra.v2.BootService/Register\" with subject:\"wrong-uuid-456\" is not allowed because it is not part of the token permissions, allowed subjects are:[\"abc-def-123\"]"),
		},
		{
			name: "api token with machine editor role denied on wrong method for a machine",
			token: &apiv2.Token{
				User:         "machine-prov",
				TokenType:    apiv2.TokenType_TOKEN_TYPE_API,
				MachineRoles: map[string]apiv2.MachineRole{"abc-def-123": apiv2.MachineRole_MACHINE_ROLE_EDITOR},
			},
			method:  apiv2connect.MachineServiceDeleteProcedure,
			subject: "abc-def-123",
			wantErr: errorutil.PermissionDenied("access to:\"/metalstack.api.v2.MachineService/Delete\" is not allowed because it is not part of the token permissions"),
		},
		{
			name: "api token with machine editor role denied on non-machine method regardless of UUID",
			token: &apiv2.Token{
				User:         "machine-prov",
				TokenType:    apiv2.TokenType_TOKEN_TYPE_API,
				MachineRoles: map[string]apiv2.MachineRole{"abc-def-123": apiv2.MachineRole_MACHINE_ROLE_EDITOR},
			},
			method:  infrav2connect.SwitchServiceRegisterProcedure,
			subject: "abc-def-123",
			wantErr: errorutil.PermissionDenied("access to:\"/metalstack.infra.v2.SwitchService/Register\" is not allowed because it is not part of the token permissions"),
		},
		{
			name: "api token with machine editor role allowed on BootServiceWait with correct UUID",
			token: &apiv2.Token{
				User:         "machine-prov2",
				TokenType:    apiv2.TokenType_TOKEN_TYPE_API,
				MachineRoles: map[string]apiv2.MachineRole{"xyz-789": apiv2.MachineRole_MACHINE_ROLE_EDITOR},
			},
			method:  infrav2connect.BootServiceWaitProcedure,
			subject: "xyz-789",
			wantErr: nil,
		},
		{
			name: "api token with machine editor role denied on non-editor machine method",
			token: &apiv2.Token{
				User:         "machine-prov2",
				TokenType:    apiv2.TokenType_TOKEN_TYPE_API,
				MachineRoles: map[string]apiv2.MachineRole{"xyz-789": apiv2.MachineRole_MACHINE_ROLE_EDITOR},
			},
			method:  infrav2connect.SwitchServiceRegisterProcedure,
			subject: "machine-uuid-different",
			wantErr: errorutil.PermissionDenied("access to:\"/metalstack.infra.v2.SwitchService/Register\" is not allowed because it is not part of the token permissions"),
		},
		{
			name: "multiple machine roles, access allowed for one UUID",
			token: &apiv2.Token{
				User:         "multi-machine",
				TokenType:    apiv2.TokenType_TOKEN_TYPE_API,
				MachineRoles: map[string]apiv2.MachineRole{"machine-a": apiv2.MachineRole_MACHINE_ROLE_EDITOR, "machine-b": apiv2.MachineRole_MACHINE_ROLE_EDITOR},
			},
			method:  infrav2connect.BootServiceRegisterProcedure,
			subject: "machine-b",
			wantErr: nil,
		},
		{
			name: "wildcard machine role, access allowed for arbitrary uuid",
			token: &apiv2.Token{
				User:         "multi-machine",
				TokenType:    apiv2.TokenType_TOKEN_TYPE_API,
				MachineRoles: map[string]apiv2.MachineRole{"*": apiv2.MachineRole_MACHINE_ROLE_EDITOR},
			},
			method:  infrav2connect.BootServiceRegisterProcedure,
			subject: "machine-b",
			wantErr: nil,
		},
		{
			name: "multiple machine roles, access denied for unknown UUID",
			token: &apiv2.Token{
				User:         "multi-machine",
				TokenType:    apiv2.TokenType_TOKEN_TYPE_API,
				MachineRoles: map[string]apiv2.MachineRole{"machine-a": apiv2.MachineRole_MACHINE_ROLE_EDITOR, "machine-b": apiv2.MachineRole_MACHINE_ROLE_EDITOR},
			},
			method:  infrav2connect.BootServiceRegisterProcedure,
			subject: "machine-c",
			wantErr: errorutil.PermissionDenied("access to:\"/metalstack.infra.v2.BootService/Register\" with subject:\"machine-c\" is not allowed because it is not part of the token permissions, allowed subjects are:[\"machine-a\" \"machine-b\"]"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &authorizer{
				log:                slog.Default(),
				adminViewerMethods: adminViewerMethods(),
			}
			a.projectsAndTenantsGetter = func(ctx context.Context, userId string) (*api.ProjectsAndTenants, error) {
				if tt.projectsAndTenants == nil {
					return &api.ProjectsAndTenants{}, nil
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
		projectsAndTenants *api.ProjectsAndTenants
		adminSubjects      []string
		req                *connect.Request[apiv2.IPServiceGetRequest]
		callFn             func()
		wantErr            error
	}{
		{
			name: "one permission, api token",
			token: &apiv2.Token{
				User:      "user-b",
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
				Permissions: []*apiv2.MethodPermission{
					{Subject: "project-a", Methods: []string{"/metalstack.api.v2.IPService/Get"}},
				},
			},
			projectsAndTenants: &api.ProjectsAndTenants{
				ProjectRoles: map[string]apiv2.ProjectRole{
					"project-a": apiv2.ProjectRole_PROJECT_ROLE_VIEWER,
				},
			},
			req:     connect.NewRequest(&apiv2.IPServiceGetRequest{Project: "project-a"}),
			wantErr: nil,
		},
		{
			name: "one permission, api token, access not allowed because this method is not allowed",
			token: &apiv2.Token{
				User:      "user-b",
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
				Permissions: []*apiv2.MethodPermission{
					{Subject: "project-a", Methods: []string{"/metalstack.api.v2.IPService/Create"}},
				},
			},
			projectsAndTenants: &api.ProjectsAndTenants{
				ProjectRoles: map[string]apiv2.ProjectRole{
					"project-a": apiv2.ProjectRole_PROJECT_ROLE_EDITOR,
				},
			},
			req:     connect.NewRequest(&apiv2.IPServiceGetRequest{Project: "project-a"}),
			wantErr: errorutil.PermissionDenied("access to:\"/metalstack.api.v2.IPService/Get\" is not allowed because it is not part of the token permissions"),
		},
		{
			name: "one permission, api token, access not allowed because the subject is not allowed",
			token: &apiv2.Token{
				User:      "user-b",
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
				Permissions: []*apiv2.MethodPermission{
					{Subject: "project-a", Methods: []string{"/metalstack.api.v2.IPService/Get"}},
				},
			},
			projectsAndTenants: &api.ProjectsAndTenants{
				ProjectRoles: map[string]apiv2.ProjectRole{
					"project-a": apiv2.ProjectRole_PROJECT_ROLE_EDITOR,
				},
			},
			req:     connect.NewRequest(&apiv2.IPServiceGetRequest{Project: "project-b"}),
			wantErr: errorutil.PermissionDenied("access to:\"/metalstack.api.v2.IPService/Get\" with subject:\"project-b\" is not allowed because it is not part of the token permissions, allowed subjects are:[\"project-a\"]"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &authorizer{
				log:                slog.Default(),
				adminViewerMethods: adminViewerMethods(),
			}
			a.projectsAndTenantsGetter = func(ctx context.Context, userId string) (*api.ProjectsAndTenants, error) {
				if tt.projectsAndTenants == nil {
					return &api.ProjectsAndTenants{}, nil
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
