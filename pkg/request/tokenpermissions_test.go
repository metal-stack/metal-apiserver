package request

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/google/go-cmp/cmp"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/api/go/metalstack/infra/v2/infrav2connect"
	"github.com/metal-stack/metal-apiserver/pkg/repository"
	"github.com/stretchr/testify/require"
)

func Test_getTokenPermissions(t *testing.T) {
	tests := []struct {
		name               string
		token              *apiv2.Token
		projectsAndTenants *repository.ProjectsAndTenants
		want               tokenPermissions
		wantErr            error
	}{
		{
			name:    "unknown admin role",
			token:   &apiv2.Token{User: "admin", AdminRole: apiv2.AdminRole_ADMIN_ROLE_UNSPECIFIED.Enum()},
			wantErr: errors.New("given admin role:ADMIN_ROLE_UNSPECIFIED is not valid"),
		},
		{
			name:  "empty token",
			token: nil,
			want: tokenPermissions{
				"/grpc.reflection.v1.ServerReflection/ServerReflectionInfo":      {"*": entry{}},
				"/grpc.reflection.v1alpha.ServerReflection/ServerReflectionInfo": {"*": entry{}},
				"/metalstack.api.v2.HealthService/Get":                           {"*": entry{}},
				"/metalstack.api.v2.MethodService/List":                          {"*": entry{}},
				"/metalstack.api.v2.VersionService/Get":                          {"*": entry{}},
			},
		},
		{
			name: "admin role editor",
			token: &apiv2.Token{
				User:      "admin",
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
				AdminRole: apiv2.AdminRole_ADMIN_ROLE_EDITOR.Enum(),
			},
			want: tokenPermissions{
				"/grpc.reflection.v1.ServerReflection/ServerReflectionInfo":      {"*": entry{}},
				"/grpc.reflection.v1alpha.ServerReflection/ServerReflectionInfo": {"*": entry{}},
				"/metalstack.admin.v2.FilesystemService/Create":                  {"*": entry{}},
				"/metalstack.admin.v2.FilesystemService/Delete":                  {"*": entry{}},
				"/metalstack.admin.v2.FilesystemService/Update":                  {"*": entry{}},
				"/metalstack.admin.v2.IPService/List":                            {"*": entry{}},
				"/metalstack.admin.v2.ImageService/Create":                       {"*": entry{}},
				"/metalstack.admin.v2.ImageService/Delete":                       {"*": entry{}},
				"/metalstack.admin.v2.ImageService/Update":                       {"*": entry{}},
				"/metalstack.admin.v2.ImageService/Usage":                        {"*": entry{}},
				"/metalstack.admin.v2.MachineService/Get":                        {"*": entry{}},
				"/metalstack.admin.v2.MachineService/List":                       {"*": entry{}},
				"/metalstack.admin.v2.NetworkService/Create":                     {"*": entry{}},
				"/metalstack.admin.v2.NetworkService/Delete":                     {"*": entry{}},
				"/metalstack.admin.v2.NetworkService/Get":                        {"*": entry{}},
				"/metalstack.admin.v2.NetworkService/List":                       {"*": entry{}},
				"/metalstack.admin.v2.NetworkService/Update":                     {"*": entry{}},
				"/metalstack.admin.v2.PartitionService/Capacity":                 {"*": entry{}},
				"/metalstack.admin.v2.PartitionService/Create":                   {"*": entry{}},
				"/metalstack.admin.v2.PartitionService/Delete":                   {"*": entry{}},
				"/metalstack.admin.v2.PartitionService/Update":                   {"*": entry{}},
				"/metalstack.admin.v2.ProjectService/List":                       {"*": entry{}},
				"/metalstack.admin.v2.SizeService/Create":                        {"*": entry{}},
				"/metalstack.admin.v2.SizeService/Delete":                        {"*": entry{}},
				"/metalstack.admin.v2.SizeService/Update":                        {"*": entry{}},
				"/metalstack.admin.v2.SwitchService/Delete":                      {"*": entry{}},
				"/metalstack.admin.v2.SwitchService/Get":                         {"*": entry{}},
				"/metalstack.admin.v2.SwitchService/List":                        {"*": entry{}},
				"/metalstack.admin.v2.SwitchService/Migrate":                     {"*": entry{}},
				"/metalstack.admin.v2.SwitchService/Port":                        {"*": entry{}},
				"/metalstack.admin.v2.SwitchService/Update":                      {"*": entry{}},
				"/metalstack.admin.v2.TenantService/Create":                      {"*": entry{}},
				"/metalstack.admin.v2.TenantService/List":                        {"*": entry{}},
				"/metalstack.admin.v2.TokenService/Create":                       {"*": entry{}},
				"/metalstack.admin.v2.TokenService/List":                         {"*": entry{}},
				"/metalstack.admin.v2.TokenService/Revoke":                       {"*": entry{}},
				"/metalstack.api.v2.FilesystemService/Get":                       {"*": entry{}},
				"/metalstack.api.v2.FilesystemService/List":                      {"*": entry{}},
				"/metalstack.api.v2.FilesystemService/Match":                     {"*": entry{}},
				"/metalstack.api.v2.HealthService/Get":                           {"*": entry{}},
				"/metalstack.api.v2.IPService/Create":                            {"*": entry{}},
				"/metalstack.api.v2.IPService/Delete":                            {"*": entry{}},
				"/metalstack.api.v2.IPService/Get":                               {"*": entry{}},
				"/metalstack.api.v2.IPService/List":                              {"*": entry{}},
				"/metalstack.api.v2.IPService/Update":                            {"*": entry{}},
				"/metalstack.api.v2.ImageService/Get":                            {"*": entry{}},
				"/metalstack.api.v2.ImageService/Latest":                         {"*": entry{}},
				"/metalstack.api.v2.ImageService/List":                           {"*": entry{}},
				"/metalstack.api.v2.MachineService/Create":                       {"*": entry{}},
				"/metalstack.api.v2.MachineService/Delete":                       {"*": entry{}},
				"/metalstack.api.v2.MachineService/Get":                          {"*": entry{}},
				"/metalstack.api.v2.MachineService/List":                         {"*": entry{}},
				"/metalstack.api.v2.MachineService/Update":                       {"*": entry{}},
				"/metalstack.api.v2.MethodService/List":                          {"*": entry{}},
				"/metalstack.api.v2.MethodService/TokenScopedList":               {"*": entry{}},
				"/metalstack.api.v2.NetworkService/Create":                       {"*": entry{}},
				"/metalstack.api.v2.NetworkService/Delete":                       {"*": entry{}},
				"/metalstack.api.v2.NetworkService/Get":                          {"*": entry{}},
				"/metalstack.api.v2.NetworkService/List":                         {"*": entry{}},
				"/metalstack.api.v2.NetworkService/ListBaseNetworks":             {"*": entry{}},
				"/metalstack.api.v2.NetworkService/Update":                       {"*": entry{}},
				"/metalstack.api.v2.PartitionService/Get":                        {"*": entry{}},
				"/metalstack.api.v2.PartitionService/List":                       {"*": entry{}},
				"/metalstack.api.v2.ProjectService/Create":                       {"*": entry{}},
				"/metalstack.api.v2.ProjectService/Delete":                       {"*": entry{}},
				"/metalstack.api.v2.ProjectService/Get":                          {"*": entry{}},
				"/metalstack.api.v2.ProjectService/Invite":                       {"*": entry{}},
				"/metalstack.api.v2.ProjectService/InviteAccept":                 {"*": entry{}},
				"/metalstack.api.v2.ProjectService/InviteDelete":                 {"*": entry{}},
				"/metalstack.api.v2.ProjectService/InviteGet":                    {"*": entry{}},
				"/metalstack.api.v2.ProjectService/InvitesList":                  {"*": entry{}},
				"/metalstack.api.v2.ProjectService/Leave":                        {"*": entry{}},
				"/metalstack.api.v2.ProjectService/List":                         {"*": entry{}},
				"/metalstack.api.v2.ProjectService/RemoveMember":                 {"*": entry{}},
				"/metalstack.api.v2.ProjectService/Update":                       {"*": entry{}},
				"/metalstack.api.v2.ProjectService/UpdateMember":                 {"*": entry{}},
				"/metalstack.api.v2.SizeService/Get":                             {"*": entry{}},
				"/metalstack.api.v2.SizeService/List":                            {"*": entry{}},
				"/metalstack.api.v2.TenantService/Create":                        {"*": entry{}},
				"/metalstack.api.v2.TenantService/Delete":                        {"*": entry{}},
				"/metalstack.api.v2.TenantService/Get":                           {"*": entry{}},
				"/metalstack.api.v2.TenantService/Invite":                        {"*": entry{}},
				"/metalstack.api.v2.TenantService/InviteAccept":                  {"*": entry{}},
				"/metalstack.api.v2.TenantService/InviteDelete":                  {"*": entry{}},
				"/metalstack.api.v2.TenantService/InviteGet":                     {"*": entry{}},
				"/metalstack.api.v2.TenantService/InvitesList":                   {"*": entry{}},
				"/metalstack.api.v2.TenantService/Leave":                         {"*": entry{}},
				"/metalstack.api.v2.TenantService/List":                          {"*": entry{}},
				"/metalstack.api.v2.TenantService/RemoveMember":                  {"*": entry{}},
				"/metalstack.api.v2.TenantService/Update":                        {"*": entry{}},
				"/metalstack.api.v2.TenantService/UpdateMember":                  {"*": entry{}},
				"/metalstack.api.v2.TokenService/Create":                         {"*": entry{}},
				"/metalstack.api.v2.TokenService/Get":                            {"*": entry{}},
				"/metalstack.api.v2.TokenService/List":                           {"*": entry{}},
				"/metalstack.api.v2.TokenService/Refresh":                        {"*": entry{}},
				"/metalstack.api.v2.TokenService/Revoke":                         {"*": entry{}},
				"/metalstack.api.v2.TokenService/Update":                         {"*": entry{}},
				"/metalstack.api.v2.UserService/Get":                             {"*": entry{}},
				"/metalstack.api.v2.VersionService/Get":                          {"*": entry{}},
				"/metalstack.infra.v2.BMCService/UpdateBMCInfo":                  {"*": entry{}},
				"/metalstack.infra.v2.SwitchService/Get":                         {"*": entry{}},
				"/metalstack.infra.v2.SwitchService/Heartbeat":                   {"*": entry{}},
				"/metalstack.infra.v2.SwitchService/Register":                    {"*": entry{}},
			},
			wantErr: nil,
		},
		{
			name: "admin role viewer",
			token: &apiv2.Token{
				User:      "admin",
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
				AdminRole: apiv2.AdminRole_ADMIN_ROLE_VIEWER.Enum(),
			},
			want: tokenPermissions{
				"/grpc.reflection.v1.ServerReflection/ServerReflectionInfo":      {"*": entry{}},
				"/grpc.reflection.v1alpha.ServerReflection/ServerReflectionInfo": {"*": entry{}},
				"/metalstack.admin.v2.IPService/List":                            {"*": entry{}},
				"/metalstack.admin.v2.ImageService/Usage":                        {"*": entry{}},
				"/metalstack.admin.v2.MachineService/Get":                        {"*": entry{}},
				"/metalstack.admin.v2.MachineService/List":                       {"*": entry{}},
				"/metalstack.admin.v2.NetworkService/Get":                        {"*": entry{}},
				"/metalstack.admin.v2.NetworkService/List":                       {"*": entry{}},
				"/metalstack.admin.v2.PartitionService/Capacity":                 {"*": entry{}},
				"/metalstack.admin.v2.ProjectService/List":                       {"*": entry{}},
				"/metalstack.admin.v2.SwitchService/Get":                         {"*": entry{}},
				"/metalstack.admin.v2.SwitchService/List":                        {"*": entry{}},
				"/metalstack.admin.v2.TenantService/List":                        {"*": entry{}},
				"/metalstack.admin.v2.TokenService/List":                         {"*": entry{}},
				"/metalstack.api.v2.FilesystemService/Get":                       {"*": entry{}},
				"/metalstack.api.v2.FilesystemService/List":                      {"*": entry{}},
				"/metalstack.api.v2.FilesystemService/Match":                     {"*": entry{}},
				"/metalstack.api.v2.HealthService/Get":                           {"*": entry{}},
				"/metalstack.api.v2.IPService/Get":                               {"*": entry{}},
				"/metalstack.api.v2.IPService/List":                              {"*": entry{}},
				"/metalstack.api.v2.ImageService/Get":                            {"*": entry{}},
				"/metalstack.api.v2.ImageService/Latest":                         {"*": entry{}},
				"/metalstack.api.v2.ImageService/List":                           {"*": entry{}},
				"/metalstack.api.v2.MachineService/Get":                          {"*": entry{}},
				"/metalstack.api.v2.MachineService/List":                         {"*": entry{}},
				"/metalstack.api.v2.MethodService/List":                          {"*": entry{}},
				"/metalstack.api.v2.MethodService/TokenScopedList":               {"*": entry{}},
				"/metalstack.api.v2.NetworkService/Get":                          {"*": entry{}},
				"/metalstack.api.v2.NetworkService/List":                         {"*": entry{}},
				"/metalstack.api.v2.NetworkService/ListBaseNetworks":             {"*": entry{}},
				"/metalstack.api.v2.PartitionService/Get":                        {"*": entry{}},
				"/metalstack.api.v2.PartitionService/List":                       {"*": entry{}},
				"/metalstack.api.v2.ProjectService/Get":                          {"*": entry{}},
				"/metalstack.api.v2.ProjectService/InviteAccept":                 {"*": entry{}},
				"/metalstack.api.v2.ProjectService/InviteGet":                    {"*": entry{}},
				"/metalstack.api.v2.ProjectService/Leave":                        {"*": entry{}},
				"/metalstack.api.v2.ProjectService/List":                         {"*": entry{}},
				"/metalstack.api.v2.SizeService/Get":                             {"*": entry{}},
				"/metalstack.api.v2.SizeService/List":                            {"*": entry{}},
				"/metalstack.api.v2.TenantService/Create":                        {"*": entry{}},
				"/metalstack.api.v2.TenantService/Get":                           {"*": entry{}},
				"/metalstack.api.v2.TenantService/InviteAccept":                  {"*": entry{}},
				"/metalstack.api.v2.TenantService/InviteGet":                     {"*": entry{}},
				"/metalstack.api.v2.TenantService/Leave":                         {"*": entry{}},
				"/metalstack.api.v2.TenantService/List":                          {"*": entry{}},
				"/metalstack.api.v2.TokenService/Create":                         {"*": entry{}},
				"/metalstack.api.v2.TokenService/Get":                            {"*": entry{}},
				"/metalstack.api.v2.TokenService/List":                           {"*": entry{}},
				"/metalstack.api.v2.TokenService/Refresh":                        {"*": entry{}},
				"/metalstack.api.v2.TokenService/Revoke":                         {"*": entry{}},
				"/metalstack.api.v2.TokenService/Update":                         {"*": entry{}},
				"/metalstack.api.v2.UserService/Get":                             {"*": entry{}},
				"/metalstack.api.v2.VersionService/Get":                          {"*": entry{}},
				"/metalstack.infra.v2.SwitchService/Get":                         {"*": entry{}},
			},
			wantErr: nil,
		},
		{
			name: "infra role editor",
			token: &apiv2.Token{
				User:      "metal-core",
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
				InfraRole: apiv2.InfraRole_INFRA_ROLE_EDITOR.Enum(),
			},
			want: tokenPermissions{
				"/metalstack.infra.v2.BMCService/UpdateBMCInfo": {"*": entry{}},
				"/metalstack.infra.v2.SwitchService/Get":        {"*": entry{}},
				"/metalstack.infra.v2.SwitchService/Heartbeat":  {"*": entry{}},
				"/metalstack.infra.v2.SwitchService/Register":   {"*": entry{}},
			},
		},
		{
			name: "infra role viewer",
			token: &apiv2.Token{
				User:      "metal-core",
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
				InfraRole: apiv2.InfraRole_INFRA_ROLE_VIEWER.Enum(),
			},
			want: tokenPermissions{
				"/metalstack.infra.v2.SwitchService/Get": {"*": entry{}},
			},
		},
		{
			name: "only permissions",
			token: &apiv2.Token{
				User:      "user-a",
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
				Permissions: []*apiv2.MethodPermission{
					{Subject: "a", Methods: []string{"/metalstack.api.v2.IPService/Create"}},
					{Subject: "a", Methods: []string{"/metalstack.api.v2.IPService/Delete"}},
					{Subject: "b", Methods: []string{"/metalstack.api.v2.IPService/Delete"}},
				},
			},
			projectsAndTenants: &repository.ProjectsAndTenants{
				ProjectRoles: map[string]apiv2.ProjectRole{
					"a": apiv2.ProjectRole_PROJECT_ROLE_EDITOR,
					"b": apiv2.ProjectRole_PROJECT_ROLE_OWNER,
				},
			},
			want: tokenPermissions{
				"/metalstack.api.v2.IPService/Create": {"a": entry{}},
				"/metalstack.api.v2.IPService/Delete": {"a": entry{}, "b": entry{}},
			},
		},
		{
			name: "infra permissions",
			token: &apiv2.Token{
				User: "metal-core",
				Permissions: []*apiv2.MethodPermission{
					{Subject: "*", Methods: []string{infrav2connect.SwitchServiceRegisterProcedure}},
				},
			},
			want: tokenPermissions{
				"/metalstack.infra.v2.SwitchService/Register": {"*": entry{}},
			},
		},
		{
			name: "tenant roles, token type api",
			token: &apiv2.Token{
				User:      "user-b",
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
				TenantRoles: map[string]apiv2.TenantRole{
					"a": apiv2.TenantRole_TENANT_ROLE_GUEST,
					"b": apiv2.TenantRole_TENANT_ROLE_EDITOR,
					"c": apiv2.TenantRole_TENANT_ROLE_OWNER,
				},
			},
			projectsAndTenants: &repository.ProjectsAndTenants{
				TenantRoles: map[string]apiv2.TenantRole{
					"a": apiv2.TenantRole_TENANT_ROLE_GUEST,
					"b": apiv2.TenantRole_TENANT_ROLE_EDITOR,
					"c": apiv2.TenantRole_TENANT_ROLE_OWNER,
				},
			},
			want: tokenPermissions{
				"/metalstack.api.v2.ProjectService/Create":      {"b": entry{}, "c": entry{}},
				"/metalstack.api.v2.TenantService/Get":          {"b": entry{}, "a": entry{}, "c": entry{}},
				"/metalstack.api.v2.TenantService/Update":       {"b": entry{}, "c": entry{}},
				"/metalstack.api.v2.TenantService/Delete":       {"b": entry{}, "c": entry{}},
				"/metalstack.api.v2.TenantService/RemoveMember": {"c": entry{}},
				"/metalstack.api.v2.TenantService/UpdateMember": {"c": entry{}},
				"/metalstack.api.v2.TenantService/Invite":       {"c": entry{}},
				"/metalstack.api.v2.TenantService/InviteDelete": {"c": entry{}},
				"/metalstack.api.v2.TenantService/InvitesList":  {"c": entry{}},
			},
		},
		{
			name: "tenant roles, token type user",
			token: &apiv2.Token{
				User:      "user-b",
				TokenType: apiv2.TokenType_TOKEN_TYPE_USER,
			},
			projectsAndTenants: &repository.ProjectsAndTenants{
				TenantRoles: map[string]apiv2.TenantRole{
					"a": apiv2.TenantRole_TENANT_ROLE_GUEST,
					"b": apiv2.TenantRole_TENANT_ROLE_EDITOR,
					"c": apiv2.TenantRole_TENANT_ROLE_OWNER,
				},
			},
			want: tokenPermissions{
				"/grpc.reflection.v1.ServerReflection/ServerReflectionInfo":      {"*": entry{}},
				"/grpc.reflection.v1alpha.ServerReflection/ServerReflectionInfo": {"*": entry{}},
				"/metalstack.api.v2.FilesystemService/Get":                       {"*": entry{}},
				"/metalstack.api.v2.FilesystemService/List":                      {"*": entry{}},
				"/metalstack.api.v2.FilesystemService/Match":                     {"*": entry{}},
				"/metalstack.api.v2.HealthService/Get":                           {"*": entry{}},
				"/metalstack.api.v2.ImageService/Get":                            {"*": entry{}},
				"/metalstack.api.v2.ImageService/Latest":                         {"*": entry{}},
				"/metalstack.api.v2.ImageService/List":                           {"*": entry{}},
				"/metalstack.api.v2.MethodService/List":                          {"*": entry{}},
				"/metalstack.api.v2.MethodService/TokenScopedList":               {"*": entry{}},
				"/metalstack.api.v2.PartitionService/Get":                        {"*": entry{}},
				"/metalstack.api.v2.PartitionService/List":                       {"*": entry{}},
				"/metalstack.api.v2.ProjectService/Create":                       {"b": entry{}, "c": entry{}},
				"/metalstack.api.v2.ProjectService/InviteAccept":                 {"*": entry{}},
				"/metalstack.api.v2.ProjectService/InviteGet":                    {"*": entry{}},
				"/metalstack.api.v2.ProjectService/List":                         {"*": entry{}},
				"/metalstack.api.v2.SizeService/Get":                             {"*": entry{}},
				"/metalstack.api.v2.SizeService/List":                            {"*": entry{}},
				"/metalstack.api.v2.TenantService/Create":                        {"*": entry{}},
				"/metalstack.api.v2.TenantService/Get":                           {"b": entry{}, "a": entry{}, "c": entry{}},
				"/metalstack.api.v2.TenantService/Update":                        {"b": entry{}, "c": entry{}},
				"/metalstack.api.v2.TenantService/Delete":                        {"b": entry{}, "c": entry{}},
				"/metalstack.api.v2.TenantService/RemoveMember":                  {"c": entry{}},
				"/metalstack.api.v2.TenantService/UpdateMember":                  {"c": entry{}},
				"/metalstack.api.v2.TenantService/Invite":                        {"c": entry{}},
				"/metalstack.api.v2.TenantService/InviteAccept":                  {"*": entry{}},
				"/metalstack.api.v2.TenantService/InviteDelete":                  {"c": entry{}},
				"/metalstack.api.v2.TenantService/InviteGet":                     {"*": entry{}},
				"/metalstack.api.v2.TenantService/InvitesList":                   {"c": entry{}},
				"/metalstack.api.v2.TenantService/List":                          {"*": entry{}},
				"/metalstack.api.v2.TokenService/Create":                         {"*": entry{}},
				"/metalstack.api.v2.TokenService/Get":                            {"*": entry{}},
				"/metalstack.api.v2.TokenService/List":                           {"*": entry{}},
				"/metalstack.api.v2.TokenService/Refresh":                        {"*": entry{}},
				"/metalstack.api.v2.TokenService/Revoke":                         {"*": entry{}},
				"/metalstack.api.v2.TokenService/Update":                         {"*": entry{}},
				"/metalstack.api.v2.UserService/Get":                             {"*": entry{}},
				"/metalstack.api.v2.VersionService/Get":                          {"*": entry{}},
			},
		},
		{
			name: "project roles, token type api",
			token: &apiv2.Token{
				User:      "user-c",
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
				ProjectRoles: map[string]apiv2.ProjectRole{
					"a": apiv2.ProjectRole_PROJECT_ROLE_VIEWER,
					"b": apiv2.ProjectRole_PROJECT_ROLE_EDITOR,
					"c": apiv2.ProjectRole_PROJECT_ROLE_OWNER,
				},
			},
			projectsAndTenants: &repository.ProjectsAndTenants{
				ProjectRoles: map[string]apiv2.ProjectRole{
					"a": apiv2.ProjectRole_PROJECT_ROLE_VIEWER,
					"b": apiv2.ProjectRole_PROJECT_ROLE_EDITOR,
					"c": apiv2.ProjectRole_PROJECT_ROLE_OWNER,
				},
			},
			want: tokenPermissions{
				"/metalstack.api.v2.IPService/Get":                   {"a": entry{}, "b": entry{}, "c": entry{}},
				"/metalstack.api.v2.IPService/Create":                {"b": entry{}, "c": entry{}},
				"/metalstack.api.v2.IPService/Update":                {"b": entry{}, "c": entry{}},
				"/metalstack.api.v2.IPService/List":                  {"a": entry{}, "b": entry{}, "c": entry{}},
				"/metalstack.api.v2.IPService/Delete":                {"b": entry{}, "c": entry{}},
				"/metalstack.api.v2.MachineService/Get":              {"a": entry{}, "b": entry{}, "c": entry{}},
				"/metalstack.api.v2.MachineService/Create":           {"b": entry{}, "c": entry{}},
				"/metalstack.api.v2.MachineService/Update":           {"b": entry{}, "c": entry{}},
				"/metalstack.api.v2.MachineService/List":             {"a": entry{}, "b": entry{}, "c": entry{}},
				"/metalstack.api.v2.MachineService/Delete":           {"b": entry{}, "c": entry{}},
				"/metalstack.api.v2.NetworkService/Get":              {"a": entry{}, "b": entry{}, "c": entry{}},
				"/metalstack.api.v2.NetworkService/Create":           {"b": entry{}, "c": entry{}},
				"/metalstack.api.v2.NetworkService/Update":           {"b": entry{}, "c": entry{}},
				"/metalstack.api.v2.NetworkService/List":             {"a": entry{}, "b": entry{}, "c": entry{}},
				"/metalstack.api.v2.NetworkService/ListBaseNetworks": {"a": entry{}, "b": entry{}, "c": entry{}},
				"/metalstack.api.v2.NetworkService/Delete":           {"b": entry{}, "c": entry{}},
				"/metalstack.api.v2.ProjectService/Get":              {"a": entry{}, "b": entry{}, "c": entry{}},
				"/metalstack.api.v2.ProjectService/Delete":           {"c": entry{}},
				"/metalstack.api.v2.ProjectService/Update":           {"b": entry{}, "c": entry{}},
				"/metalstack.api.v2.ProjectService/RemoveMember":     {"c": entry{}},
				"/metalstack.api.v2.ProjectService/UpdateMember":     {"c": entry{}},
				"/metalstack.api.v2.ProjectService/Invite":           {"c": entry{}},
				"/metalstack.api.v2.ProjectService/InviteDelete":     {"c": entry{}},
				"/metalstack.api.v2.ProjectService/InvitesList":      {"c": entry{}},
				"/metalstack.api.v2.ProjectService/Leave":            {"a": entry{}},
			},
		},
		{
			name: "project roles with user token",
			token: &apiv2.Token{
				TokenType: apiv2.TokenType_TOKEN_TYPE_USER,
				User:      "user-a",
			},
			projectsAndTenants: &repository.ProjectsAndTenants{
				ProjectRoles: map[string]apiv2.ProjectRole{
					"a": apiv2.ProjectRole_PROJECT_ROLE_VIEWER,
					"b": apiv2.ProjectRole_PROJECT_ROLE_EDITOR,
					"c": apiv2.ProjectRole_PROJECT_ROLE_OWNER,
				},
			},
			want: tokenPermissions{
				"/grpc.reflection.v1.ServerReflection/ServerReflectionInfo":      {"*": entry{}},
				"/grpc.reflection.v1alpha.ServerReflection/ServerReflectionInfo": {"*": entry{}},
				"/metalstack.api.v2.FilesystemService/Get":                       {"*": entry{}},
				"/metalstack.api.v2.FilesystemService/List":                      {"*": entry{}},
				"/metalstack.api.v2.FilesystemService/Match":                     {"*": entry{}},
				"/metalstack.api.v2.HealthService/Get":                           {"*": entry{}},
				"/metalstack.api.v2.IPService/Get":                               {"a": entry{}, "b": entry{}, "c": entry{}},
				"/metalstack.api.v2.IPService/Create":                            {"b": entry{}, "c": entry{}},
				"/metalstack.api.v2.IPService/Update":                            {"b": entry{}, "c": entry{}},
				"/metalstack.api.v2.IPService/List":                              {"a": entry{}, "b": entry{}, "c": entry{}},
				"/metalstack.api.v2.IPService/Delete":                            {"b": entry{}, "c": entry{}},
				"/metalstack.api.v2.ImageService/Get":                            {"*": entry{}},
				"/metalstack.api.v2.ImageService/Latest":                         {"*": entry{}},
				"/metalstack.api.v2.ImageService/List":                           {"*": entry{}},
				"/metalstack.api.v2.MachineService/Get":                          {"a": entry{}, "b": entry{}, "c": entry{}},
				"/metalstack.api.v2.MachineService/Create":                       {"b": entry{}, "c": entry{}},
				"/metalstack.api.v2.MachineService/Update":                       {"b": entry{}, "c": entry{}},
				"/metalstack.api.v2.MachineService/List":                         {"a": entry{}, "b": entry{}, "c": entry{}},
				"/metalstack.api.v2.MachineService/Delete":                       {"b": entry{}, "c": entry{}},
				"/metalstack.api.v2.MethodService/List":                          {"*": entry{}},
				"/metalstack.api.v2.MethodService/TokenScopedList":               {"*": entry{}},
				"/metalstack.api.v2.NetworkService/Get":                          {"a": entry{}, "b": entry{}, "c": entry{}},
				"/metalstack.api.v2.NetworkService/Create":                       {"b": entry{}, "c": entry{}},
				"/metalstack.api.v2.NetworkService/Update":                       {"b": entry{}, "c": entry{}},
				"/metalstack.api.v2.NetworkService/List":                         {"a": entry{}, "b": entry{}, "c": entry{}},
				"/metalstack.api.v2.NetworkService/ListBaseNetworks":             {"a": entry{}, "b": entry{}, "c": entry{}},
				"/metalstack.api.v2.NetworkService/Delete":                       {"b": entry{}, "c": entry{}},
				"/metalstack.api.v2.PartitionService/Get":                        {"*": entry{}},
				"/metalstack.api.v2.PartitionService/List":                       {"*": entry{}},
				"/metalstack.api.v2.ProjectService/Get":                          {"a": entry{}, "b": entry{}, "c": entry{}},
				"/metalstack.api.v2.ProjectService/Delete":                       {"c": entry{}},
				"/metalstack.api.v2.ProjectService/Update":                       {"b": entry{}, "c": entry{}},
				"/metalstack.api.v2.ProjectService/RemoveMember":                 {"c": entry{}},
				"/metalstack.api.v2.ProjectService/UpdateMember":                 {"c": entry{}},
				"/metalstack.api.v2.ProjectService/Invite":                       {"c": entry{}},
				"/metalstack.api.v2.ProjectService/InviteAccept":                 {"*": entry{}},
				"/metalstack.api.v2.ProjectService/InviteDelete":                 {"c": entry{}},
				"/metalstack.api.v2.ProjectService/InviteGet":                    {"*": entry{}},
				"/metalstack.api.v2.ProjectService/InvitesList":                  {"c": entry{}},
				"/metalstack.api.v2.ProjectService/Leave":                        {"a": entry{}},
				"/metalstack.api.v2.ProjectService/List":                         {"*": entry{}},
				"/metalstack.api.v2.SizeService/Get":                             {"*": entry{}},
				"/metalstack.api.v2.SizeService/List":                            {"*": entry{}},
				"/metalstack.api.v2.TenantService/Create":                        {"*": entry{}},
				"/metalstack.api.v2.TenantService/InviteAccept":                  {"*": entry{}},
				"/metalstack.api.v2.TenantService/InviteGet":                     {"*": entry{}},
				"/metalstack.api.v2.TenantService/List":                          {"*": entry{}},
				"/metalstack.api.v2.TokenService/Create":                         {"*": entry{}},
				"/metalstack.api.v2.TokenService/Get":                            {"*": entry{}},
				"/metalstack.api.v2.TokenService/List":                           {"*": entry{}},
				"/metalstack.api.v2.TokenService/Refresh":                        {"*": entry{}},
				"/metalstack.api.v2.TokenService/Revoke":                         {"*": entry{}},
				"/metalstack.api.v2.TokenService/Update":                         {"*": entry{}},
				"/metalstack.api.v2.UserService/Get":                             {"*": entry{}},
				"/metalstack.api.v2.VersionService/Get":                          {"*": entry{}},
			},
		},
		{
			name: "project roles with user token",
			token: &apiv2.Token{
				TokenType: apiv2.TokenType_TOKEN_TYPE_USER,
				User:      "user-a",
			},
			projectsAndTenants: &repository.ProjectsAndTenants{
				ProjectRoles: map[string]apiv2.ProjectRole{
					"a": apiv2.ProjectRole_PROJECT_ROLE_VIEWER,
				},
			},
			want: tokenPermissions{
				"/grpc.reflection.v1.ServerReflection/ServerReflectionInfo":      {"*": entry{}},
				"/grpc.reflection.v1alpha.ServerReflection/ServerReflectionInfo": {"*": entry{}},
				"/metalstack.api.v2.FilesystemService/Get":                       {"*": entry{}},
				"/metalstack.api.v2.FilesystemService/List":                      {"*": entry{}},
				"/metalstack.api.v2.FilesystemService/Match":                     {"*": entry{}},
				"/metalstack.api.v2.HealthService/Get":                           {"*": entry{}},
				"/metalstack.api.v2.IPService/Get":                               {"a": entry{}},
				"/metalstack.api.v2.IPService/List":                              {"a": entry{}},
				"/metalstack.api.v2.ImageService/Get":                            {"*": entry{}},
				"/metalstack.api.v2.ImageService/Latest":                         {"*": entry{}},
				"/metalstack.api.v2.ImageService/List":                           {"*": entry{}},
				"/metalstack.api.v2.MachineService/Get":                          {"a": entry{}},
				"/metalstack.api.v2.MachineService/List":                         {"a": entry{}},
				"/metalstack.api.v2.MethodService/List":                          {"*": entry{}},
				"/metalstack.api.v2.MethodService/TokenScopedList":               {"*": entry{}},
				"/metalstack.api.v2.NetworkService/Get":                          {"a": entry{}},
				"/metalstack.api.v2.NetworkService/List":                         {"a": entry{}},
				"/metalstack.api.v2.NetworkService/ListBaseNetworks":             {"a": entry{}},
				"/metalstack.api.v2.PartitionService/Get":                        {"*": entry{}},
				"/metalstack.api.v2.PartitionService/List":                       {"*": entry{}},
				"/metalstack.api.v2.ProjectService/Get":                          {"a": entry{}},
				"/metalstack.api.v2.ProjectService/InviteAccept":                 {"*": entry{}},
				"/metalstack.api.v2.ProjectService/InviteGet":                    {"*": entry{}},
				"/metalstack.api.v2.ProjectService/Leave":                        {"a": entry{}},
				"/metalstack.api.v2.ProjectService/List":                         {"*": entry{}},
				"/metalstack.api.v2.SizeService/Get":                             {"*": entry{}},
				"/metalstack.api.v2.SizeService/List":                            {"*": entry{}},
				"/metalstack.api.v2.TenantService/Create":                        {"*": entry{}},
				"/metalstack.api.v2.TenantService/InviteAccept":                  {"*": entry{}},
				"/metalstack.api.v2.TenantService/InviteGet":                     {"*": entry{}},
				"/metalstack.api.v2.TenantService/List":                          {"*": entry{}},
				"/metalstack.api.v2.TokenService/Create":                         {"*": entry{}},
				"/metalstack.api.v2.TokenService/Get":                            {"*": entry{}},
				"/metalstack.api.v2.TokenService/List":                           {"*": entry{}},
				"/metalstack.api.v2.TokenService/Refresh":                        {"*": entry{}},
				"/metalstack.api.v2.TokenService/Revoke":                         {"*": entry{}},
				"/metalstack.api.v2.TokenService/Update":                         {"*": entry{}},
				"/metalstack.api.v2.UserService/Get":                             {"*": entry{}},
				"/metalstack.api.v2.VersionService/Get":                          {"*": entry{}},
			},
		},
		{
			name: "project roles with api token and permission",
			token: &apiv2.Token{
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
				User:      "user-a",
				Permissions: []*apiv2.MethodPermission{
					{
						Subject: "*",
						Methods: []string{"/metalstack.api.v2.MachineService/Create"},
					},
				},
			},
			projectsAndTenants: &repository.ProjectsAndTenants{
				Projects: []*apiv2.Project{
					{Uuid: "a"},
					{Uuid: "b"},
				},
				ProjectRoles: map[string]apiv2.ProjectRole{
					"a": apiv2.ProjectRole_PROJECT_ROLE_VIEWER,
					"b": apiv2.ProjectRole_PROJECT_ROLE_EDITOR,
				},
			},
			want: tokenPermissions{
				"/metalstack.api.v2.MachineService/Create": {"b": entry{}},
			},
		},
		{
			name: "tenant roles with api token and permission",
			token: &apiv2.Token{
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
				User:      "user-a",
				Permissions: []*apiv2.MethodPermission{
					{
						Subject: "*",
						Methods: []string{"/metalstack.api.v2.ProjectService/Create"},
					},
				},
			},
			projectsAndTenants: &repository.ProjectsAndTenants{
				Tenants: []*apiv2.Tenant{
					{Login: "user-a"},
				},
				TenantRoles: map[string]apiv2.TenantRole{
					"user-a": apiv2.TenantRole_TENANT_ROLE_EDITOR,
				},
			},
			want: tokenPermissions{
				"/metalstack.api.v2.ProjectService/Create": {"user-a": entry{}},
			},
		},
		{
			name: "tenant roles api tokens and permissions subject does not exist",
			token: &apiv2.Token{
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
				User:      "user-a",
				Permissions: []*apiv2.MethodPermission{
					{
						Subject: "foo",
						Methods: []string{"/metalstack.api.v2.ProjectService/Create"},
					},
					{
						Subject: "*",
						Methods: []string{"/metalstack.api.v2.ProjectService/Create"},
					},
				},
			},
			projectsAndTenants: &repository.ProjectsAndTenants{
				Tenants: []*apiv2.Tenant{
					{Login: "user-a"},
				},
				TenantRoles: map[string]apiv2.TenantRole{
					"user-a": apiv2.TenantRole_TENANT_ROLE_EDITOR,
				},
			},
			want: tokenPermissions{
				"/metalstack.api.v2.ProjectService/Create": {"user-a": entry{}},
			},
		},
		{
			name: "project roles api tokens and permissions subject does not exist",
			token: &apiv2.Token{
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
				User:      "user-a",
				Permissions: []*apiv2.MethodPermission{
					{
						Subject: "foo",
						Methods: []string{"/metalstack.api.v2.MachineService/Create"},
					},
					{
						Subject: "project-a",
						Methods: []string{"/metalstack.api.v2.MachineService/Create"},
					},
				},
			},
			projectsAndTenants: &repository.ProjectsAndTenants{
				Projects: []*apiv2.Project{
					{Uuid: "project-a"},
				},
				ProjectRoles: map[string]apiv2.ProjectRole{
					"project-a": apiv2.ProjectRole_PROJECT_ROLE_EDITOR,
				},
			},
			want: tokenPermissions{
				"/metalstack.api.v2.MachineService/Create": {"project-a": entry{}},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &authorizer{
				log: slog.Default(),
			}
			a.projectsAndTenantsGetter = func(ctx context.Context, userId string) (*repository.ProjectsAndTenants, error) {
				if tt.projectsAndTenants == nil {
					return &repository.ProjectsAndTenants{}, nil
				}
				return tt.projectsAndTenants, nil
			}

			got, _, gotErr := a.getTokenPermissions(t.Context(), tt.token)
			if diff := cmp.Diff(tt.want, got); diff != "" {
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
