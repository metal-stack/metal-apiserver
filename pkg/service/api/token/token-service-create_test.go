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
	"github.com/metal-stack/api/go/errorutil"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/certs"
	"github.com/metal-stack/metal-apiserver/pkg/repository/api"
	"github.com/metal-stack/metal-apiserver/pkg/request"
	"github.com/metal-stack/metal-apiserver/pkg/token"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/durationpb"
)

var (
	machineID1 = "de240964-ff9f-4e3d-95b2-8a96e43788f1"
	machineID2 = "fa350875-ee8f-4e3d-95b2-8a96e43788f2"
	project1   = "00000000-0000-0000-0000-000000000000"
	project2   = "11111111-1111-1111-1111-111111111111"
	tenant1    = "mascots"
	tenant2    = "animals"
)

func Test_Create_RoleAndPermissionCombinations(t *testing.T) {
	t.Parallel()
	type state struct {
		providerTenant string
		projectRoles   map[string]apiv2.ProjectRole
		tenantRoles    map[string]apiv2.TenantRole
		getterErr      error
	}
	tests := []struct {
		name         string
		sessionToken *apiv2.Token
		req          *apiv2.TokenServiceCreateRequest
		state        state
		wantError    error
		wantToken    *apiv2.Token
	}{
		// ============================================================================
		// 1. BASELINE — empty / no roles / no permissions
		// ============================================================================
		{
			name: "bare token with no roles or permissions",
			sessionToken: &apiv2.Token{
				User:         "phippy",
				TokenType:    apiv2.TokenType_TOKEN_TYPE_UNSPECIFIED,
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
			},
			req: &apiv2.TokenServiceCreateRequest{
				Description: "bare token",
			},
			state: state{
				providerTenant: "metal-stack",
			},
			wantError: errorutil.FailedPrecondition(`invalid token type for token creation: "TOKEN_TYPE_UNSPECIFIED"`),
		},
		{
			name: "USER token with empty roles can create empty token",
			sessionToken: &apiv2.Token{
				User:         "phippy",
				TokenType:    apiv2.TokenType_TOKEN_TYPE_USER,
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
			},
			req: &apiv2.TokenServiceCreateRequest{
				Description: "from user token",
			},
			state: state{
				providerTenant: "metal-stack",
			},
			wantToken: &apiv2.Token{
				User:        "phippy",
				Description: "from user token",
				TokenType:   apiv2.TokenType_TOKEN_TYPE_API,
			},
		},
		{
			name: "API token with no roles can create empty token",
			sessionToken: &apiv2.Token{
				User:         "phippy",
				TokenType:    apiv2.TokenType_TOKEN_TYPE_API,
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
			},
			req: &apiv2.TokenServiceCreateRequest{
				Description: "from api token",
			},
			state: state{
				providerTenant: "metal-stack",
			},
			wantToken: &apiv2.Token{
				User:        "phippy",
				Description: "from api token",
				TokenType:   apiv2.TokenType_TOKEN_TYPE_API,
			},
		},

		// ============================================================================
		// 2. ADMIN ROLE
		//   session nil/EDITOR/VIEWER × request nil/EDITOR/VIEWER
		//   PAT controls what level the user is allowed to request.
		// ============================================================================
		{
			name: "admin EDITOR session can create admin EDITOR token as provider tenant OWNER",
			sessionToken: &apiv2.Token{
				User:         "phippy",
				TokenType:    apiv2.TokenType_TOKEN_TYPE_API,
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
				AdminRole:    apiv2.AdminRole_ADMIN_ROLE_EDITOR.Enum(),
			},
			req: &apiv2.TokenServiceCreateRequest{
				Description:  "admin editor token",
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
				Description:  "admin editor token",
				TokenType:    apiv2.TokenType_TOKEN_TYPE_API,
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
				AdminRole:    apiv2.AdminRole_ADMIN_ROLE_EDITOR.Enum(),
			},
		},
		{
			name: "admin VIEWER session can create admin VIEWER token as provider tenant VIEWER",
			sessionToken: &apiv2.Token{
				User:         "phippy",
				TokenType:    apiv2.TokenType_TOKEN_TYPE_API,
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
				AdminRole:    apiv2.AdminRole_ADMIN_ROLE_VIEWER.Enum(),
			},
			req: &apiv2.TokenServiceCreateRequest{
				Description:  "admin viewer token",
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
				Description:  "admin viewer token",
				TokenType:    apiv2.TokenType_TOKEN_TYPE_API,
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
				AdminRole:    apiv2.AdminRole_ADMIN_ROLE_VIEWER.Enum(),
			},
		},
		{
			name: "admin VIEWER session cannot create admin EDITOR token as provider tenant VIEWER",
			sessionToken: &apiv2.Token{
				User:         "phippy",
				TokenType:    apiv2.TokenType_TOKEN_TYPE_API,
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
				AdminRole:    apiv2.AdminRole_ADMIN_ROLE_VIEWER.Enum(),
			},
			req: &apiv2.TokenServiceCreateRequest{
				Description:  "admin editor token",
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
			wantError: errorutil.PermissionDenied(`the following method "/metalstack.admin.v2.ComponentService/Delete" is not allowed on any of the requested subjects: [*]`),
		},
		{
			name: "admin VIEWER session cannot elevate to admin EDITOR token as provider tenant OWNER",
			sessionToken: &apiv2.Token{
				User:         "phippy",
				TokenType:    apiv2.TokenType_TOKEN_TYPE_API,
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
				AdminRole:    apiv2.AdminRole_ADMIN_ROLE_VIEWER.Enum(),
			},
			req: &apiv2.TokenServiceCreateRequest{
				Description:  "admin editor token",
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
			wantError: errorutil.PermissionDenied(`the following method "/metalstack.admin.v2.ComponentService/Delete" is not allowed on any of the requested subjects: [*]`),
		},
		{
			name: "admin EDITOR session can create admin VIEWER token as provider tenant OWNER",
			sessionToken: &apiv2.Token{
				User:         "phippy",
				TokenType:    apiv2.TokenType_TOKEN_TYPE_API,
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
				AdminRole:    apiv2.AdminRole_ADMIN_ROLE_EDITOR.Enum(),
			},
			req: &apiv2.TokenServiceCreateRequest{
				Description:  "admin viewer token",
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
				AdminRole:    apiv2.AdminRole_ADMIN_ROLE_VIEWER.Enum(),
			},
			state: state{
				providerTenant: "phippy",
				tenantRoles: map[string]apiv2.TenantRole{
					"phippy": apiv2.TenantRole_TENANT_ROLE_OWNER,
				},
			},
			wantToken: &apiv2.Token{
				User:         "phippy",
				Description:  "admin viewer token",
				TokenType:    apiv2.TokenType_TOKEN_TYPE_API,
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
				AdminRole:    apiv2.AdminRole_ADMIN_ROLE_VIEWER.Enum(),
			},
		},
		{
			name: "admin EDITOR session cannot create admin VIEWER token without provider tenant membership",
			sessionToken: &apiv2.Token{
				User:         "phippy",
				TokenType:    apiv2.TokenType_TOKEN_TYPE_API,
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
				AdminRole:    apiv2.AdminRole_ADMIN_ROLE_EDITOR.Enum(),
			},
			req: &apiv2.TokenServiceCreateRequest{
				Description:  "admin viewer token",
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
				AdminRole:    apiv2.AdminRole_ADMIN_ROLE_VIEWER.Enum(),
			},
			state: state{
				providerTenant: "metal-stack",
			},
			wantError: errorutil.PermissionDenied(`the following method "/grpc.reflection.v1.ServerReflection/ServerReflectionInfo" is not allowed on any of the requested subjects: [*]`),
		},
		{
			name: "USER token without admin can obtain admin EDITOR as provider tenant OWNER",
			sessionToken: &apiv2.Token{
				User:         "phippy",
				TokenType:    apiv2.TokenType_TOKEN_TYPE_USER,
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
			},
			req: &apiv2.TokenServiceCreateRequest{
				Description:  "become admin flow for provider tenant users",
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
				Description:  "become admin flow for provider tenant users",
				TokenType:    apiv2.TokenType_TOKEN_TYPE_API,
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
				AdminRole:    apiv2.AdminRole_ADMIN_ROLE_EDITOR.Enum(),
			},
		},
		{
			name: "USER token without admin can obtain admin VIEWER as provider tenant VIEWER",
			sessionToken: &apiv2.Token{
				User:         "phippy",
				TokenType:    apiv2.TokenType_TOKEN_TYPE_USER,
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
			},
			req: &apiv2.TokenServiceCreateRequest{
				Description:  "become admin viewer flow for provider tenant users",
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
				Description:  "become admin viewer flow for provider tenant users",
				TokenType:    apiv2.TokenType_TOKEN_TYPE_API,
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
				AdminRole:    apiv2.AdminRole_ADMIN_ROLE_VIEWER.Enum(),
			},
		},
		{
			name: "USER token without admin can obtain admin VIEWER as provider tenant EDITOR",
			sessionToken: &apiv2.Token{
				User:         "phippy",
				TokenType:    apiv2.TokenType_TOKEN_TYPE_USER,
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
			},
			req: &apiv2.TokenServiceCreateRequest{
				Description:  "become admin viewer flow for provider tenant users",
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
				AdminRole:    apiv2.AdminRole_ADMIN_ROLE_VIEWER.Enum(),
			},
			state: state{
				providerTenant: "phippy",
				tenantRoles: map[string]apiv2.TenantRole{
					"phippy": apiv2.TenantRole_TENANT_ROLE_EDITOR,
				},
			},
			wantToken: &apiv2.Token{
				User:         "phippy",
				Description:  "become admin viewer flow for provider tenant users",
				TokenType:    apiv2.TokenType_TOKEN_TYPE_API,
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
				AdminRole:    apiv2.AdminRole_ADMIN_ROLE_VIEWER.Enum(),
			},
		},
		{
			name: "USER token cannot obtain admin EDITOR as provider tenant VIEWER",
			sessionToken: &apiv2.Token{
				User:         "phippy",
				TokenType:    apiv2.TokenType_TOKEN_TYPE_USER,
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
			},
			req: &apiv2.TokenServiceCreateRequest{
				Description:  "cannot become editor",
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
			wantError: errorutil.PermissionDenied(`your provider tenant membership only allows "ADMIN_ROLE_VIEWER", but you requested "ADMIN_ROLE_EDITOR"`),
		},
		{
			name: "USER token cannot obtain admin EDITOR as provider tenant EDITOR",
			sessionToken: &apiv2.Token{
				User:         "phippy",
				TokenType:    apiv2.TokenType_TOKEN_TYPE_USER,
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
			},
			req: &apiv2.TokenServiceCreateRequest{
				Description:  "cannot become editor",
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
				AdminRole:    apiv2.AdminRole_ADMIN_ROLE_EDITOR.Enum(),
			},
			state: state{
				providerTenant: "phippy",
				tenantRoles: map[string]apiv2.TenantRole{
					"phippy": apiv2.TenantRole_TENANT_ROLE_EDITOR,
				},
			},
			wantError: errorutil.PermissionDenied(`your provider tenant membership only allows "ADMIN_ROLE_VIEWER", but you requested "ADMIN_ROLE_EDITOR"`),
		},
		{
			name: "USER token cannot obtain admin EDITOR as provider tenant GUEST",
			sessionToken: &apiv2.Token{
				User:         "phippy",
				TokenType:    apiv2.TokenType_TOKEN_TYPE_USER,
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
			},
			req: &apiv2.TokenServiceCreateRequest{
				Description:  "cannot become editor",
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
				AdminRole:    apiv2.AdminRole_ADMIN_ROLE_EDITOR.Enum(),
			},
			state: state{
				providerTenant: "phippy",
				tenantRoles: map[string]apiv2.TenantRole{
					"phippy": apiv2.TenantRole_TENANT_ROLE_GUEST,
				},
			},
			wantError: errorutil.PermissionDenied(`the following method "/metalstack.admin.v2.AuditService/Get" is not allowed on any of the requested subjects: [*]`),
		},
		{
			name: "USER token cannot obtain admin VIEWER when not in provider tenant",
			sessionToken: &apiv2.Token{
				User:         "phippy",
				TokenType:    apiv2.TokenType_TOKEN_TYPE_USER,
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
			},
			req: &apiv2.TokenServiceCreateRequest{
				Description:  "cannot become admin",
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
				AdminRole:    apiv2.AdminRole_ADMIN_ROLE_VIEWER.Enum(),
			},
			state: state{
				providerTenant: "metal-stack",
			},
			wantError: errorutil.PermissionDenied(`the following method "/metalstack.admin.v2.AuditService/Get" is not allowed on any of the requested subjects: [*]`),
		},
		{
			name: "API token with admin EDITOR cannot create admin token without provider tenant membership",
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
				providerTenant: "metal-stack",
			},
			wantError: errorutil.PermissionDenied(`the following method "/grpc.reflection.v1.ServerReflection/ServerReflectionInfo" is not allowed on any of the requested subjects: [*]`),
		},
		{
			name: "no admin role in session or request",
			sessionToken: &apiv2.Token{
				User:         "phippy",
				TokenType:    apiv2.TokenType_TOKEN_TYPE_API,
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
			},
			req: &apiv2.TokenServiceCreateRequest{
				Description:  "no admin",
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
			},
			state: state{
				providerTenant: "phippy",
			},
			wantToken: &apiv2.Token{
				User:         "phippy",
				Description:  "no admin",
				TokenType:    apiv2.TokenType_TOKEN_TYPE_API,
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
			},
		},

		// ============================================================================
		// 3. TENANT ROLE VARIATIONS
		//   single / multiple / wildcard / wrong subject / role escalation
		// ============================================================================
		{
			name: "session and request share same tenant role",
			sessionToken: &apiv2.Token{
				User:         "phippy",
				TokenType:    apiv2.TokenType_TOKEN_TYPE_API,
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles: map[string]apiv2.TenantRole{
					tenant1: apiv2.TenantRole_TENANT_ROLE_EDITOR,
				},
			},
			req: &apiv2.TokenServiceCreateRequest{
				Description:  "tenant token",
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles: map[string]apiv2.TenantRole{
					tenant1: apiv2.TenantRole_TENANT_ROLE_EDITOR,
				},
			},
			state: state{
				providerTenant: "metal-stack",
				tenantRoles: map[string]apiv2.TenantRole{
					tenant1: apiv2.TenantRole_TENANT_ROLE_EDITOR,
				},
			},
			wantToken: &apiv2.Token{
				User:         "phippy",
				Description:  "tenant token",
				TokenType:    *apiv2.TokenType_TOKEN_TYPE_API.Enum(),
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles: map[string]apiv2.TenantRole{
					tenant1: apiv2.TenantRole_TENANT_ROLE_EDITOR,
				},
			},
		},
		{
			name: "session lacks tenant subject, request has it — fails",
			sessionToken: &apiv2.Token{
				User:         "phippy",
				TokenType:    apiv2.TokenType_TOKEN_TYPE_API,
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
			},
			req: &apiv2.TokenServiceCreateRequest{
				Description:  "tenant token",
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles: map[string]apiv2.TenantRole{
					tenant1: apiv2.TenantRole_TENANT_ROLE_EDITOR,
				},
			},
			state: state{
				providerTenant: "metal-stack",
			},
			wantError: errorutil.PermissionDenied(`requested tenant roles are not allowed: [%s]`, tenant1),
		},
		{
			name: "session has tenant, PAT getter lacks it — fails second validation",
			sessionToken: &apiv2.Token{
				User:         "phippy",
				TokenType:    apiv2.TokenType_TOKEN_TYPE_API,
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles: map[string]apiv2.TenantRole{
					tenant1: apiv2.TenantRole_TENANT_ROLE_EDITOR,
				},
			},
			req: &apiv2.TokenServiceCreateRequest{
				Description:  "tenant token",
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles: map[string]apiv2.TenantRole{
					tenant1: apiv2.TenantRole_TENANT_ROLE_EDITOR,
				},
			},
			state: state{
				providerTenant: "metal-stack",
				tenantRoles:    map[string]apiv2.TenantRole{},
			},
			wantError: errorutil.PermissionDenied(`requested tenant roles are not allowed: [%s]`, tenant1),
		},
		{
			name: "session tenant wildcard allows any tenant subject",
			sessionToken: &apiv2.Token{
				User:         "phippy",
				TokenType:    apiv2.TokenType_TOKEN_TYPE_API,
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles: map[string]apiv2.TenantRole{
					"*": apiv2.TenantRole_TENANT_ROLE_EDITOR,
				},
			},
			req: &apiv2.TokenServiceCreateRequest{
				Description:  "wildcard tenant",
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles: map[string]apiv2.TenantRole{
					tenant1: apiv2.TenantRole_TENANT_ROLE_EDITOR,
				},
			},
			state: state{
				providerTenant: "metal-stack",
				tenantRoles: map[string]apiv2.TenantRole{
					tenant1: apiv2.TenantRole_TENANT_ROLE_EDITOR,
				},
			},
			wantToken: &apiv2.Token{
				User:         "phippy",
				Description:  "wildcard tenant",
				TokenType:    *apiv2.TokenType_TOKEN_TYPE_API.Enum(),
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles: map[string]apiv2.TenantRole{
					tenant1: apiv2.TenantRole_TENANT_ROLE_EDITOR,
				},
			},
		},
		{
			name: "tenant role escalation from VIEWER to EDITOR fails method check",
			sessionToken: &apiv2.Token{
				User:         "phippy",
				TokenType:    apiv2.TokenType_TOKEN_TYPE_API,
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles: map[string]apiv2.TenantRole{
					tenant1: apiv2.TenantRole_TENANT_ROLE_VIEWER,
				},
			},
			req: &apiv2.TokenServiceCreateRequest{
				Description:  "escalated tenant",
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles: map[string]apiv2.TenantRole{
					tenant1: apiv2.TenantRole_TENANT_ROLE_EDITOR,
				},
			},
			state: state{
				providerTenant: "metal-stack",
				tenantRoles: map[string]apiv2.TenantRole{
					tenant1: apiv2.TenantRole_TENANT_ROLE_EDITOR,
				},
			},
			wantError: errorutil.PermissionDenied(`the following method "/metalstack.api.v2.ProjectService/Create" is not allowed on any of the requested subjects: [%s]`, tenant1),
		},
		{
			name: "tenant role from OWNER to EDITOR (same subject) succeeds",
			sessionToken: &apiv2.Token{
				User:         "phippy",
				TokenType:    apiv2.TokenType_TOKEN_TYPE_API,
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles: map[string]apiv2.TenantRole{
					tenant1: apiv2.TenantRole_TENANT_ROLE_OWNER,
				},
			},
			req: &apiv2.TokenServiceCreateRequest{
				Description:  "downgraded tenant role",
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles: map[string]apiv2.TenantRole{
					tenant1: apiv2.TenantRole_TENANT_ROLE_EDITOR,
				},
			},
			state: state{
				providerTenant: "metal-stack",
				tenantRoles: map[string]apiv2.TenantRole{
					tenant1: apiv2.TenantRole_TENANT_ROLE_OWNER,
				},
			},
			wantToken: &apiv2.Token{
				User:         "phippy",
				Description:  "downgraded tenant role",
				TokenType:    *apiv2.TokenType_TOKEN_TYPE_API.Enum(),
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles: map[string]apiv2.TenantRole{
					tenant1: apiv2.TenantRole_TENANT_ROLE_EDITOR,
				},
			},
		},
		{
			name: "multiple tenants in session, one in request",
			sessionToken: &apiv2.Token{
				User:         "phippy",
				TokenType:    apiv2.TokenType_TOKEN_TYPE_API,
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles: map[string]apiv2.TenantRole{
					tenant1: apiv2.TenantRole_TENANT_ROLE_EDITOR,
					tenant2: apiv2.TenantRole_TENANT_ROLE_VIEWER,
				},
			},
			req: &apiv2.TokenServiceCreateRequest{
				Description:  "single of multiple",
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles: map[string]apiv2.TenantRole{
					tenant1: apiv2.TenantRole_TENANT_ROLE_EDITOR,
				},
			},
			state: state{
				providerTenant: "metal-stack",
				tenantRoles: map[string]apiv2.TenantRole{
					tenant1: apiv2.TenantRole_TENANT_ROLE_EDITOR,
					tenant2: apiv2.TenantRole_TENANT_ROLE_VIEWER,
				},
			},
			wantToken: &apiv2.Token{
				User:         "phippy",
				Description:  "single of multiple",
				TokenType:    *apiv2.TokenType_TOKEN_TYPE_API.Enum(),
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles: map[string]apiv2.TenantRole{
					tenant1: apiv2.TenantRole_TENANT_ROLE_EDITOR,
				},
			},
		},
		{
			name: "request for TENANT_ROLE_UNSPECIFIED fails",
			sessionToken: &apiv2.Token{
				User:         "phippy",
				TokenType:    apiv2.TokenType_TOKEN_TYPE_API,
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles: map[string]apiv2.TenantRole{
					tenant1: apiv2.TenantRole_TENANT_ROLE_EDITOR,
				},
			},
			req: &apiv2.TokenServiceCreateRequest{
				Description:  "unspecified tenant role",
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles: map[string]apiv2.TenantRole{
					tenant1: apiv2.TenantRole_TENANT_ROLE_UNSPECIFIED,
				},
			},
			state: state{
				providerTenant: "metal-stack",
				tenantRoles: map[string]apiv2.TenantRole{
					tenant1: apiv2.TenantRole_TENANT_ROLE_EDITOR,
				},
			},
			wantError: errorutil.PermissionDenied(`requested tenant role: "TENANT_ROLE_UNSPECIFIED" is not allowed`),
		},

		// ============================================================================
		// 4. PROJECT ROLE VARIATIONS
		// ============================================================================
		{
			name: "session and request share same project role",
			sessionToken: &apiv2.Token{
				User:        "phippy",
				TokenType:   apiv2.TokenType_TOKEN_TYPE_API,
				Permissions: []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{
					project1: apiv2.ProjectRole_PROJECT_ROLE_EDITOR,
				},
				TenantRoles: map[string]apiv2.TenantRole{},
			},
			req: &apiv2.TokenServiceCreateRequest{
				Description: "project token",
				ProjectRoles: map[string]apiv2.ProjectRole{
					project1: apiv2.ProjectRole_PROJECT_ROLE_EDITOR,
				},
				TenantRoles: map[string]apiv2.TenantRole{},
			},
			state: state{
				providerTenant: "metal-stack",
				projectRoles: map[string]apiv2.ProjectRole{
					project1: apiv2.ProjectRole_PROJECT_ROLE_EDITOR,
				},
			},
			wantToken: &apiv2.Token{
				User:        "phippy",
				Description: "project token",
				TokenType:   apiv2.TokenType_TOKEN_TYPE_API,
				ProjectRoles: map[string]apiv2.ProjectRole{
					project1: apiv2.ProjectRole_PROJECT_ROLE_EDITOR,
				},
				TenantRoles: map[string]apiv2.TenantRole{},
			},
		},
		{
			name: "session lacks project subject — fails",
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
					project1: apiv2.ProjectRole_PROJECT_ROLE_EDITOR,
				},
				TenantRoles: map[string]apiv2.TenantRole{},
			},
			state: state{
				providerTenant: "metal-stack",
			},
			wantError: errorutil.PermissionDenied(`requested project roles are not allowed: [%s]`, project1),
		},
		{
			name: "session has project wildcard allows any project",
			sessionToken: &apiv2.Token{
				User:        "phippy",
				TokenType:   apiv2.TokenType_TOKEN_TYPE_API,
				Permissions: []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{
					"*": apiv2.ProjectRole_PROJECT_ROLE_EDITOR,
				},
				TenantRoles: map[string]apiv2.TenantRole{},
			},
			req: &apiv2.TokenServiceCreateRequest{
				Description: "wildcard project",
				ProjectRoles: map[string]apiv2.ProjectRole{
					project1: apiv2.ProjectRole_PROJECT_ROLE_EDITOR,
				},
				TenantRoles: map[string]apiv2.TenantRole{},
			},
			state: state{
				providerTenant: "metal-stack",
				projectRoles: map[string]apiv2.ProjectRole{
					project1: apiv2.ProjectRole_PROJECT_ROLE_EDITOR,
				},
			},
			wantToken: &apiv2.Token{
				User:        "phippy",
				Description: "wildcard project",
				TokenType:   apiv2.TokenType_TOKEN_TYPE_API,
				ProjectRoles: map[string]apiv2.ProjectRole{
					project1: apiv2.ProjectRole_PROJECT_ROLE_EDITOR,
				},
				TenantRoles: map[string]apiv2.TenantRole{},
			},
		},
		{
			name: "project role escalation from VIEWER to EDITOR fails",
			sessionToken: &apiv2.Token{
				User:        "phippy",
				TokenType:   apiv2.TokenType_TOKEN_TYPE_API,
				Permissions: []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{
					project1: apiv2.ProjectRole_PROJECT_ROLE_VIEWER,
				},
				TenantRoles: map[string]apiv2.TenantRole{},
			},
			req: &apiv2.TokenServiceCreateRequest{
				Description: "escalated project",
				ProjectRoles: map[string]apiv2.ProjectRole{
					project1: apiv2.ProjectRole_PROJECT_ROLE_EDITOR,
				},
				TenantRoles: map[string]apiv2.TenantRole{},
			},
			state: state{
				providerTenant: "metal-stack",
				projectRoles: map[string]apiv2.ProjectRole{
					project1: apiv2.ProjectRole_PROJECT_ROLE_EDITOR,
				},
			},
			wantError: errorutil.PermissionDenied(`the following method "/metalstack.api.v2.IPService/Create" is not allowed on any of the requested subjects: [%s]`, project1),
		},
		{
			name: "project OWNER to EDITOR downgrade succeeds",
			sessionToken: &apiv2.Token{
				User:        "phippy",
				TokenType:   apiv2.TokenType_TOKEN_TYPE_API,
				Permissions: []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{
					project1: apiv2.ProjectRole_PROJECT_ROLE_OWNER,
				},
				TenantRoles: map[string]apiv2.TenantRole{},
			},
			req: &apiv2.TokenServiceCreateRequest{
				Description: "downgraded project",
				ProjectRoles: map[string]apiv2.ProjectRole{
					project1: apiv2.ProjectRole_PROJECT_ROLE_EDITOR,
				},
				TenantRoles: map[string]apiv2.TenantRole{},
			},
			state: state{
				providerTenant: "metal-stack",
				projectRoles: map[string]apiv2.ProjectRole{
					project1: apiv2.ProjectRole_PROJECT_ROLE_OWNER,
				},
			},
			wantToken: &apiv2.Token{
				User:        "phippy",
				Description: "downgraded project",
				TokenType:   apiv2.TokenType_TOKEN_TYPE_API,
				ProjectRoles: map[string]apiv2.ProjectRole{
					project1: apiv2.ProjectRole_PROJECT_ROLE_EDITOR,
				},
				TenantRoles: map[string]apiv2.TenantRole{},
			},
		},

		// ============================================================================
		// 5. MACHINE ROLE VARIATIONS
		// ============================================================================
		{
			name: "session and request share same machine role",
			sessionToken: &apiv2.Token{
				User:        "pixie-core",
				TokenType:   apiv2.TokenType_TOKEN_TYPE_API,
				Permissions: []*apiv2.MethodPermission{},
				MachineRoles: map[string]apiv2.MachineRole{
					machineID1: apiv2.MachineRole_MACHINE_ROLE_EDITOR,
				},
				TenantRoles: map[string]apiv2.TenantRole{},
			},
			req: &apiv2.TokenServiceCreateRequest{
				Description: "machine token",
				MachineRoles: map[string]apiv2.MachineRole{
					machineID1: apiv2.MachineRole_MACHINE_ROLE_EDITOR,
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
					machineID1: apiv2.MachineRole_MACHINE_ROLE_EDITOR,
				},
				TenantRoles: map[string]apiv2.TenantRole{},
			},
		},
		{
			name: "session lacks machine — fails",
			sessionToken: &apiv2.Token{
				User:         "pixie-core",
				TokenType:    apiv2.TokenType_TOKEN_TYPE_API,
				Permissions:  []*apiv2.MethodPermission{},
				MachineRoles: map[string]apiv2.MachineRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
			},
			req: &apiv2.TokenServiceCreateRequest{
				Description: "machine token",
				MachineRoles: map[string]apiv2.MachineRole{
					machineID1: apiv2.MachineRole_MACHINE_ROLE_EDITOR,
				},
				TenantRoles: map[string]apiv2.TenantRole{},
			},
			state: state{
				providerTenant: "metal-stack",
			},
			wantError: errorutil.PermissionDenied(`requested machine roles are not allowed: [%s]`, machineID1),
		},
		{
			name: "session machine wildcard allows any machine",
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
				Description: "wildcard machine",
				MachineRoles: map[string]apiv2.MachineRole{
					machineID2: apiv2.MachineRole_MACHINE_ROLE_EDITOR,
				},
				TenantRoles: map[string]apiv2.TenantRole{},
			},
			state: state{
				providerTenant: "metal-stack",
			},
			wantToken: &apiv2.Token{
				User:        "pixie-core",
				Description: "wildcard machine",
				TokenType:   apiv2.TokenType_TOKEN_TYPE_API,
				MachineRoles: map[string]apiv2.MachineRole{
					machineID2: apiv2.MachineRole_MACHINE_ROLE_EDITOR,
				},
				TenantRoles: map[string]apiv2.TenantRole{},
			},
		},
		{
			name: "session has one machine, request asks for different machine — fails",
			sessionToken: &apiv2.Token{
				User:        "pixie-core",
				TokenType:   apiv2.TokenType_TOKEN_TYPE_API,
				Permissions: []*apiv2.MethodPermission{},
				MachineRoles: map[string]apiv2.MachineRole{
					machineID1: apiv2.MachineRole_MACHINE_ROLE_EDITOR,
				},
				TenantRoles: map[string]apiv2.TenantRole{},
			},
			req: &apiv2.TokenServiceCreateRequest{
				Description: "different machine",
				MachineRoles: map[string]apiv2.MachineRole{
					machineID2: apiv2.MachineRole_MACHINE_ROLE_VIEWER,
				},
				TenantRoles: map[string]apiv2.TenantRole{},
			},
			state: state{
				providerTenant: "metal-stack",
			},
			wantError: errorutil.PermissionDenied(`requested machine roles are not allowed: [%s]`, machineID2),
		},
		{
			name: "machine VIEWER cannot request machine EDITOR (same subject)",
			sessionToken: &apiv2.Token{
				User:        "pixie-core",
				TokenType:   apiv2.TokenType_TOKEN_TYPE_API,
				Permissions: []*apiv2.MethodPermission{},
				MachineRoles: map[string]apiv2.MachineRole{
					machineID1: apiv2.MachineRole_MACHINE_ROLE_VIEWER,
				},
				TenantRoles: map[string]apiv2.TenantRole{},
			},
			req: &apiv2.TokenServiceCreateRequest{
				Description: "machine role escalation",
				MachineRoles: map[string]apiv2.MachineRole{
					machineID1: apiv2.MachineRole_MACHINE_ROLE_EDITOR,
				},
				TenantRoles: map[string]apiv2.TenantRole{},
			},
			state: state{
				providerTenant: "metal-stack",
			},
			wantError: errorutil.PermissionDenied(`the following method "/metalstack.infra.v2.BootService/InstallationSucceeded" is not allowed on any of the requested subjects: [%s]`, machineID1),
		},

		// ============================================================================
		// 6. INFRA ROLE VARIATIONS
		// ============================================================================
		{
			name: "admin EDITOR session can create infra EDITOR token",
			sessionToken: &apiv2.Token{
				User:         "phippy",
				TokenType:    apiv2.TokenType_TOKEN_TYPE_API,
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
				AdminRole:    apiv2.AdminRole_ADMIN_ROLE_EDITOR.Enum(),
			},
			req: &apiv2.TokenServiceCreateRequest{
				Description:  "infra editor token",
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
				InfraRole:    apiv2.InfraRole_INFRA_ROLE_EDITOR.Enum(),
			},
			state: state{
				providerTenant: "phippy",
				tenantRoles: map[string]apiv2.TenantRole{
					"phippy": apiv2.TenantRole_TENANT_ROLE_OWNER,
				},
			},
			wantToken: &apiv2.Token{
				User:         "phippy",
				Description:  "infra editor token",
				TokenType:    apiv2.TokenType_TOKEN_TYPE_API,
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
				InfraRole:    apiv2.InfraRole_INFRA_ROLE_EDITOR.Enum(),
			},
		},
		{
			name: "admin VIEWER session requesting infra EDITOR fails",
			sessionToken: &apiv2.Token{
				User:         "phippy",
				TokenType:    apiv2.TokenType_TOKEN_TYPE_API,
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
				AdminRole:    apiv2.AdminRole_ADMIN_ROLE_VIEWER.Enum(),
			},
			req: &apiv2.TokenServiceCreateRequest{
				Description:  "infra editor token",
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
				InfraRole:    apiv2.InfraRole_INFRA_ROLE_EDITOR.Enum(),
			},
			state: state{
				providerTenant: "phippy",
				tenantRoles: map[string]apiv2.TenantRole{
					"phippy": apiv2.TenantRole_TENANT_ROLE_VIEWER,
				},
			},
			wantError: errorutil.PermissionDenied(`the following method "/metalstack.infra.v2.BMCService/BMCCommandDone" is not allowed on any of the requested subjects: [*]`),
		},
		{
			name: "non-admin session with infra role directly fails",
			sessionToken: &apiv2.Token{
				User:         "phippy",
				TokenType:    apiv2.TokenType_TOKEN_TYPE_API,
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
			},
			req: &apiv2.TokenServiceCreateRequest{
				Description:  "infra alone",
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
				InfraRole:    apiv2.InfraRole_INFRA_ROLE_EDITOR.Enum(),
			},
			state: state{
				providerTenant: "metal-stack",
			},
			wantError: errorutil.PermissionDenied(`the following method "/metalstack.infra.v2.BMCService/BMCCommandDone" is not allowed on any of the requested subjects: [*]`),
		},
		{
			name: "infra VIEWER can create infra VIEWER token",
			sessionToken: &apiv2.Token{
				User:         "phippy",
				TokenType:    apiv2.TokenType_TOKEN_TYPE_API,
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
				AdminRole:    apiv2.AdminRole_ADMIN_ROLE_EDITOR.Enum(),
				InfraRole:    apiv2.InfraRole_INFRA_ROLE_VIEWER.Enum(),
			},
			req: &apiv2.TokenServiceCreateRequest{
				Description:  "infra viewer",
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
				InfraRole:    apiv2.InfraRole_INFRA_ROLE_VIEWER.Enum(),
			},
			state: state{
				providerTenant: "phippy",
				tenantRoles: map[string]apiv2.TenantRole{
					"phippy": apiv2.TenantRole_TENANT_ROLE_OWNER,
				},
			},
			wantToken: &apiv2.Token{
				User:         "phippy",
				Description:  "infra viewer",
				TokenType:    apiv2.TokenType_TOKEN_TYPE_API,
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
				InfraRole:    apiv2.InfraRole_INFRA_ROLE_VIEWER.Enum(),
			},
		},

		// ============================================================================
		// 7. EXPLICIT PERMISSION VARIATIONS
		//   The second validation pass uses fullUserToken which only gets
		//   ProjectRoles/TenantRoles from PAT + MachineRoles/InfraRole from session.
		//   Explicit Permissions are NOT carried into fullUserToken.
		//   So the PAT must provide role coverage for any requested permission.
		// ============================================================================
		{
			name: "session has permission, request same permission",
			sessionToken: &apiv2.Token{
				User:      "phippy",
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
				Permissions: []*apiv2.MethodPermission{
					{
						Subject: project1,
						Methods: []string{"/metalstack.api.v2.IPService/Get"},
					},
				},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
			},
			req: &apiv2.TokenServiceCreateRequest{
				Description: "permission token",
				Permissions: []*apiv2.MethodPermission{
					{
						Subject: project1,
						Methods: []string{"/metalstack.api.v2.IPService/Get"},
					},
				},
			},
			state: state{
				providerTenant: "metal-stack",
				projectRoles: map[string]apiv2.ProjectRole{
					project1: apiv2.ProjectRole_PROJECT_ROLE_EDITOR,
				},
			},
			wantToken: &apiv2.Token{
				User:        "phippy",
				Description: "permission token",
				TokenType:   apiv2.TokenType_TOKEN_TYPE_API,
				Permissions: []*apiv2.MethodPermission{
					{
						Subject: project1,
						Methods: []string{"/metalstack.api.v2.IPService/Get"},
					},
				},
			},
		},
		{
			name: "session lacks permission method — fails",
			sessionToken: &apiv2.Token{
				User:         "phippy",
				TokenType:    apiv2.TokenType_TOKEN_TYPE_API,
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
			},
			req: &apiv2.TokenServiceCreateRequest{
				Description: "unknown method",
				Permissions: []*apiv2.MethodPermission{
					{
						Subject: project1,
						Methods: []string{"/metalstack.api.v2.IPService/Get"},
					},
				},
			},
			state: state{
				providerTenant: "metal-stack",
			},
			wantError: errorutil.PermissionDenied(`the following method "/metalstack.api.v2.IPService/Get" is not allowed`),
		},
		{
			name: "session permission with wildcard subject grants any subject",
			sessionToken: &apiv2.Token{
				User:      "phippy",
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
				Permissions: []*apiv2.MethodPermission{
					{
						Subject: "*",
						Methods: []string{"/metalstack.api.v2.IPService/Get"},
					},
				},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
			},
			req: &apiv2.TokenServiceCreateRequest{
				Description: "wildcard permission",
				Permissions: []*apiv2.MethodPermission{
					{
						Subject: project1,
						Methods: []string{"/metalstack.api.v2.IPService/Get"},
					},
				},
			},
			state: state{
				providerTenant: "metal-stack",
				projectRoles: map[string]apiv2.ProjectRole{
					project1: apiv2.ProjectRole_PROJECT_ROLE_EDITOR,
				},
			},
			wantToken: &apiv2.Token{
				User:        "phippy",
				Description: "wildcard permission",
				TokenType:   apiv2.TokenType_TOKEN_TYPE_API,
				Permissions: []*apiv2.MethodPermission{
					{
						Subject: project1,
						Methods: []string{"/metalstack.api.v2.IPService/Get"},
					},
				},
			},
		},
		{
			name: "session permission for subject A, request for subject B — fails",
			sessionToken: &apiv2.Token{
				User:      "phippy",
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
				Permissions: []*apiv2.MethodPermission{
					{
						Subject: project1,
						Methods: []string{"/metalstack.api.v2.IPService/Get"},
					},
				},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
			},
			req: &apiv2.TokenServiceCreateRequest{
				Description: "wrong subject",
				Permissions: []*apiv2.MethodPermission{
					{
						Subject: project2,
						Methods: []string{"/metalstack.api.v2.IPService/Get"},
					},
				},
			},
			state: state{
				providerTenant: "metal-stack",
				projectRoles: map[string]apiv2.ProjectRole{
					project2: apiv2.ProjectRole_PROJECT_ROLE_EDITOR,
				},
			},
			wantError: errorutil.PermissionDenied(`method "/metalstack.api.v2.IPService/Get" is not allowed on subject %q with your current user permissions`, project2),
		},
		{
			name: "multiple permissions in request, all allowed",
			sessionToken: &apiv2.Token{
				User:      "phippy",
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
				Permissions: []*apiv2.MethodPermission{
					{
						Subject: "*",
						Methods: []string{
							"/metalstack.api.v2.IPService/Get",
							"/metalstack.api.v2.IPService/List",
						},
					},
				},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
			},
			req: &apiv2.TokenServiceCreateRequest{
				Description: "multiple permissions",
				Permissions: []*apiv2.MethodPermission{
					{
						Subject: project1,
						Methods: []string{"/metalstack.api.v2.IPService/Get"},
					},
					{
						Subject: project1,
						Methods: []string{"/metalstack.api.v2.IPService/List"},
					},
				},
			},
			state: state{
				providerTenant: "metal-stack",
				projectRoles: map[string]apiv2.ProjectRole{
					project1: apiv2.ProjectRole_PROJECT_ROLE_EDITOR,
				},
			},
			wantToken: &apiv2.Token{
				User:        "phippy",
				Description: "multiple permissions",
				TokenType:   apiv2.TokenType_TOKEN_TYPE_API,
				Permissions: []*apiv2.MethodPermission{
					{
						Subject: project1,
						Methods: []string{"/metalstack.api.v2.IPService/Get"},
					},
					{
						Subject: project1,
						Methods: []string{"/metalstack.api.v2.IPService/List"},
					},
				},
			},
		},

		// ============================================================================
		// 8. MIXED ROLE COMBINATIONS
		// ============================================================================
		{
			name: "admin + project role combination",
			sessionToken: &apiv2.Token{
				User:        "phippy",
				TokenType:   apiv2.TokenType_TOKEN_TYPE_API,
				Permissions: []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{
					project1: apiv2.ProjectRole_PROJECT_ROLE_OWNER,
				},
				TenantRoles: map[string]apiv2.TenantRole{},
				AdminRole:   apiv2.AdminRole_ADMIN_ROLE_EDITOR.Enum(),
			},
			req: &apiv2.TokenServiceCreateRequest{
				Description: "admin with project",
				ProjectRoles: map[string]apiv2.ProjectRole{
					project1: apiv2.ProjectRole_PROJECT_ROLE_OWNER,
				},
				TenantRoles: map[string]apiv2.TenantRole{},
				AdminRole:   apiv2.AdminRole_ADMIN_ROLE_EDITOR.Enum(),
			},
			state: state{
				providerTenant: "phippy",
				projectRoles: map[string]apiv2.ProjectRole{
					project1: apiv2.ProjectRole_PROJECT_ROLE_OWNER,
				},
				tenantRoles: map[string]apiv2.TenantRole{
					"phippy": apiv2.TenantRole_TENANT_ROLE_OWNER,
				},
			},
			wantToken: &apiv2.Token{
				User:        "phippy",
				Description: "admin with project",
				TokenType:   apiv2.TokenType_TOKEN_TYPE_API,
				ProjectRoles: map[string]apiv2.ProjectRole{
					project1: apiv2.ProjectRole_PROJECT_ROLE_OWNER,
				},
				TenantRoles: map[string]apiv2.TenantRole{},
				AdminRole:   apiv2.AdminRole_ADMIN_ROLE_EDITOR.Enum(),
			},
		},
		{
			name: "tenant + project roles together",
			sessionToken: &apiv2.Token{
				User:        "phippy",
				Permissions: []*apiv2.MethodPermission{},
				TokenType:   apiv2.TokenType_TOKEN_TYPE_API,
				ProjectRoles: map[string]apiv2.ProjectRole{
					project1: apiv2.ProjectRole_PROJECT_ROLE_EDITOR,
				},
				TenantRoles: map[string]apiv2.TenantRole{
					tenant1: apiv2.TenantRole_TENANT_ROLE_EDITOR,
				},
			},
			req: &apiv2.TokenServiceCreateRequest{
				Description: "tenant and project",
				ProjectRoles: map[string]apiv2.ProjectRole{
					project1: apiv2.ProjectRole_PROJECT_ROLE_EDITOR,
				},
				TenantRoles: map[string]apiv2.TenantRole{
					tenant1: apiv2.TenantRole_TENANT_ROLE_EDITOR,
				},
			},
			state: state{
				providerTenant: "metal-stack",
				projectRoles: map[string]apiv2.ProjectRole{
					project1: apiv2.ProjectRole_PROJECT_ROLE_EDITOR,
				},
				tenantRoles: map[string]apiv2.TenantRole{
					tenant1: apiv2.TenantRole_TENANT_ROLE_EDITOR,
				},
			},
			wantToken: &apiv2.Token{
				User:        "phippy",
				Description: "tenant and project",
				TokenType:   *apiv2.TokenType_TOKEN_TYPE_API.Enum(),
				ProjectRoles: map[string]apiv2.ProjectRole{
					project1: apiv2.ProjectRole_PROJECT_ROLE_EDITOR,
				},
				TenantRoles: map[string]apiv2.TenantRole{
					tenant1: apiv2.TenantRole_TENANT_ROLE_EDITOR,
				},
			},
		},
		{
			name: "machine + tenant role combination",
			sessionToken: &apiv2.Token{
				User:        "pixie-core",
				TokenType:   apiv2.TokenType_TOKEN_TYPE_API,
				Permissions: []*apiv2.MethodPermission{},
				MachineRoles: map[string]apiv2.MachineRole{
					machineID1: apiv2.MachineRole_MACHINE_ROLE_EDITOR,
				},
				TenantRoles: map[string]apiv2.TenantRole{
					tenant1: apiv2.TenantRole_TENANT_ROLE_VIEWER,
				},
			},
			req: &apiv2.TokenServiceCreateRequest{
				Description: "machine and tenant",
				MachineRoles: map[string]apiv2.MachineRole{
					machineID1: apiv2.MachineRole_MACHINE_ROLE_EDITOR,
				},
				TenantRoles: map[string]apiv2.TenantRole{
					tenant1: apiv2.TenantRole_TENANT_ROLE_VIEWER,
				},
			},
			state: state{
				providerTenant: "metal-stack",
				tenantRoles: map[string]apiv2.TenantRole{
					tenant1: apiv2.TenantRole_TENANT_ROLE_VIEWER,
				},
			},
			wantToken: &apiv2.Token{
				User:        "pixie-core",
				Description: "machine and tenant",
				TokenType:   *apiv2.TokenType_TOKEN_TYPE_API.Enum(),
				MachineRoles: map[string]apiv2.MachineRole{
					machineID1: apiv2.MachineRole_MACHINE_ROLE_EDITOR,
				},
				TenantRoles: map[string]apiv2.TenantRole{
					tenant1: apiv2.TenantRole_TENANT_ROLE_VIEWER,
				},
			},
		},
		{
			name: "permission + project role combination",
			sessionToken: &apiv2.Token{
				User:      "phippy",
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
				Permissions: []*apiv2.MethodPermission{
					{
						Subject: project1,
						Methods: []string{"/metalstack.api.v2.IPService/Get"},
					},
				},
				ProjectRoles: map[string]apiv2.ProjectRole{
					project1: apiv2.ProjectRole_PROJECT_ROLE_EDITOR,
				},
				TenantRoles: map[string]apiv2.TenantRole{},
			},
			req: &apiv2.TokenServiceCreateRequest{
				Description: "permission and project",
				Permissions: []*apiv2.MethodPermission{
					{
						Subject: project1,
						Methods: []string{"/metalstack.api.v2.IPService/Get"},
					},
				},
				ProjectRoles: map[string]apiv2.ProjectRole{
					project1: apiv2.ProjectRole_PROJECT_ROLE_EDITOR,
				},
				TenantRoles: map[string]apiv2.TenantRole{},
			},
			state: state{
				providerTenant: "metal-stack",
				projectRoles: map[string]apiv2.ProjectRole{
					project1: apiv2.ProjectRole_PROJECT_ROLE_EDITOR,
				},
			},
			wantToken: &apiv2.Token{
				User:        "phippy",
				Description: "permission and project",
				TokenType:   apiv2.TokenType_TOKEN_TYPE_API,
				Permissions: []*apiv2.MethodPermission{
					{
						Subject: project1,
						Methods: []string{"/metalstack.api.v2.IPService/Get"},
					},
				},
				ProjectRoles: map[string]apiv2.ProjectRole{
					project1: apiv2.ProjectRole_PROJECT_ROLE_EDITOR,
				},
				TenantRoles: map[string]apiv2.TenantRole{},
			},
		},
		{
			name: "all role types empty request succeeds",
			sessionToken: &apiv2.Token{
				User:         "phippy",
				TokenType:    apiv2.TokenType_TOKEN_TYPE_API,
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
			},
			req: &apiv2.TokenServiceCreateRequest{
				Description:  "all empty",
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
				MachineRoles: map[string]apiv2.MachineRole{},
			},
			state: state{
				providerTenant: "metal-stack",
			},
			wantToken: &apiv2.Token{
				User:         "phippy",
				Description:  "all empty",
				TokenType:    apiv2.TokenType_TOKEN_TYPE_API,
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
				MachineRoles: map[string]apiv2.MachineRole{},
			},
		},

		// ============================================================================
		// 9. EDGE CASES
		// ============================================================================
		{
			name: "expiration exceeds max",
			sessionToken: &apiv2.Token{
				User:         "phippy",
				TokenType:    apiv2.TokenType_TOKEN_TYPE_API,
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
			},
			req: &apiv2.TokenServiceCreateRequest{
				Description: "too long",
				Expires:     durationpb.New(366 * 24 * time.Hour),
			},
			state: state{
				providerTenant: "metal-stack",
			},
			wantError: errors.New(`requested expiration duration: "8784h0m0s" exceeds max expiration: "8760h0m0s"`),
		},
		{
			name: "PAT getter fails",
			sessionToken: &apiv2.Token{
				User:         "phippy",
				TokenType:    apiv2.TokenType_TOKEN_TYPE_API,
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
			},
			req: &apiv2.TokenServiceCreateRequest{
				Description: "getter fails",
			},
			state: state{
				providerTenant: "metal-stack",
				getterErr:      errors.New("getter failed"),
			},
			wantError: errorutil.Internal(`getter failed`),
		},
		{
			name: "unknown method in request permissions",
			sessionToken: &apiv2.Token{
				User:      "phippy",
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
				Permissions: []*apiv2.MethodPermission{
					{
						Subject: "*",
						Methods: []string{"/metalstack.api.v2.IPService/Get"},
					},
				},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
			},
			req: &apiv2.TokenServiceCreateRequest{
				Description: "unknown method",
				Permissions: []*apiv2.MethodPermission{
					{
						Subject: project1,
						Methods: []string{"/metalstack.api.v2.UnknownService/Get"},
					},
				},
			},
			state: state{
				providerTenant: "metal-stack",
			},
			wantError: errorutil.PermissionDenied(`unknown method "/metalstack.api.v2.UnknownService/Get"`),
		},
		{
			name: "USER token creates token with project from PAT",
			sessionToken: &apiv2.Token{
				User:         "phippy",
				TokenType:    apiv2.TokenType_TOKEN_TYPE_USER,
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
			},
			req: &apiv2.TokenServiceCreateRequest{
				Description: "project token",
				ProjectRoles: map[string]apiv2.ProjectRole{
					project1: apiv2.ProjectRole_PROJECT_ROLE_EDITOR,
				},
				TenantRoles: map[string]apiv2.TenantRole{},
			},
			state: state{
				providerTenant: "metal-stack",
				projectRoles: map[string]apiv2.ProjectRole{
					project1: apiv2.ProjectRole_PROJECT_ROLE_EDITOR,
				},
			},
			wantToken: &apiv2.Token{
				User:        "phippy",
				Description: "project token",
				TokenType:   apiv2.TokenType_TOKEN_TYPE_API,
				ProjectRoles: map[string]apiv2.ProjectRole{
					project1: apiv2.ProjectRole_PROJECT_ROLE_EDITOR,
				},
				TenantRoles: map[string]apiv2.TenantRole{},
			},
		},
		{
			name: "token create with only machine role and tenant role succeeds",
			// FIXME review
			sessionToken: &apiv2.Token{
				User:        "pixie-core",
				TokenType:   apiv2.TokenType_TOKEN_TYPE_API,
				Permissions: []*apiv2.MethodPermission{},
				MachineRoles: map[string]apiv2.MachineRole{
					machineID1: apiv2.MachineRole_MACHINE_ROLE_VIEWER,
				},
				TenantRoles: map[string]apiv2.TenantRole{
					tenant1: apiv2.TenantRole_TENANT_ROLE_VIEWER,
				},
			},
			req: &apiv2.TokenServiceCreateRequest{
				Description: "machine and tenant viewer",
				MachineRoles: map[string]apiv2.MachineRole{
					machineID1: apiv2.MachineRole_MACHINE_ROLE_VIEWER,
				},
				TenantRoles: map[string]apiv2.TenantRole{
					tenant1: apiv2.TenantRole_TENANT_ROLE_VIEWER,
				},
			},
			state: state{
				providerTenant: "metal-stack",
				tenantRoles: map[string]apiv2.TenantRole{
					tenant1: apiv2.TenantRole_TENANT_ROLE_VIEWER,
				},
			},
			wantToken: &apiv2.Token{
				User:        "pixie-core",
				Description: "machine and tenant viewer",
				TokenType:   *apiv2.TokenType_TOKEN_TYPE_API.Enum(),
				MachineRoles: map[string]apiv2.MachineRole{
					machineID1: apiv2.MachineRole_MACHINE_ROLE_VIEWER,
				},
				TenantRoles: map[string]apiv2.TenantRole{
					tenant1: apiv2.TenantRole_TENANT_ROLE_VIEWER,
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

			if tt.wantError == nil {
				err := protovalidate.Validate(tt.req)
				require.NoError(t, err)
			}

			response, err := service.Create(ctx, tt.req)
			switch {
			case tt.wantError != nil && err != nil:
				if diff := cmp.Diff(tt.wantError.Error(), err.Error()); diff != "" {
					t.Errorf("diff = %s", diff)
				}
			case tt.wantError != nil && err == nil:
				t.Fatalf("want error %q, got response %q", tt.wantError, response)
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
				if diff := cmp.Diff(tt.wantToken.Permissions, got.Permissions, protocmp.Transform()); diff != "" {
					t.Errorf("permissions diff = %s", diff)
				}
				assert.Equal(t, tt.wantToken.ProjectRoles, got.ProjectRoles, "project roles")
				assert.Equal(t, tt.wantToken.TenantRoles, got.TenantRoles, "tenant roles")
				assert.Equal(t, tt.wantToken.MachineRoles, got.MachineRoles, "machine roles")
				if tt.wantToken.InfraRole != nil {
					assert.Equal(t, tt.wantToken.InfraRole, got.InfraRole, "infra role")
				}
			}
		})
	}
}
