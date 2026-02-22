package token

import (
	"time"

	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type token struct {
	// Uuid of the jwt token, used to reference it by revoke
	Uuid string `json:"uuid,omitempty"`
	// User who created this token
	User string `json:"user,omitempty"`
	// Description is a user given description of this token.
	Description string `json:"description,omitempty"`
	// Permissions is a list of service methods this token can be used for
	Permissions []methodPermission `json:"permissions,omitempty"`
	// Expires gives the date in the future after which this token can not be used anymore
	Expires *time.Time `json:"expires,omitempty"`
	// IssuedAt gives the date when this token was created
	IssuedAt *time.Time `json:"issued_at,omitempty"`
	// TokenType describes the type of this token
	TokenType int32 `json:"token_type,omitempty"`
	// ProjectRoles associates a project id with the corresponding role of the token owner
	ProjectRoles map[string]string `json:"project_roles,omitempty"`
	// TenantRoles associates a tenant id with the corresponding role of the token owner
	TenantRoles map[string]string `json:"tenant_roles,omitempty"`
	// AdminRole defines the admin role of the token owner
	AdminRole *string `json:"admin_role,omitempty"`
	// InfraRole defines the infra role of the token owner
	InfraRole *string `json:"infra_role,omitempty"`
	// MachineRoles associates a machine uuid with the corresponding role of the token owner
	MachineRoles map[string]string `json:"machine_roles,omitempty"`
}

type methodPermission struct {
	// Subject maybe either the project or the organization
	// for which the methods should be allowed
	Subject string `json:"subject,omitempty"`
	// Methods which should be accessible
	Methods []string `json:"methods,omitempty"`
}

func toInternal(t *apiv2.Token) *token {
	var permissions []methodPermission
	for _, p := range t.Permissions {
		permissions = append(permissions, methodPermission{
			Subject: p.Subject,
			Methods: p.Methods,
		})
	}

	var (
		projectRoles = map[string]string{}
		tenantRoles  = map[string]string{}

		expires  *time.Time
		issuedAt *time.Time

		adminRole    *string
		infraRole    *string
		machineRoles = map[string]string{}
	)

	if t.Expires != nil {
		expires = new(t.Expires.AsTime())
	}
	if t.IssuedAt != nil {
		issuedAt = new(t.IssuedAt.AsTime())
	}

	for id, role := range t.ProjectRoles {
		projectRoles[id] = role.String()
	}
	for id, role := range t.TenantRoles {
		tenantRoles[id] = role.String()
	}

	if t.AdminRole != nil {
		adminRole = new(t.AdminRole.String())
	}

	if t.InfraRole != nil {
		infraRole = new(t.InfraRole.String())
	}

	for id, role := range t.MachineRoles {
		machineRoles[id] = role.String()
	}

	return &token{
		Uuid:         t.Uuid,
		User:         t.User,
		Description:  t.Description,
		Permissions:  permissions,
		Expires:      expires,
		IssuedAt:     issuedAt,
		TokenType:    int32(t.TokenType),
		ProjectRoles: projectRoles,
		TenantRoles:  tenantRoles,
		AdminRole:    adminRole,
		InfraRole:    infraRole,
		MachineRoles: machineRoles,
	}
}

func toExternal(t *token) *apiv2.Token {
	var permissions []*apiv2.MethodPermission
	for _, p := range t.Permissions {
		permissions = append(permissions, &apiv2.MethodPermission{
			Subject: p.Subject,
			Methods: p.Methods,
		})
	}

	var (
		projectRoles = map[string]apiv2.ProjectRole{}
		tenantRoles  = map[string]apiv2.TenantRole{}

		expires  *timestamppb.Timestamp
		issuedAt *timestamppb.Timestamp

		adminRole    *apiv2.AdminRole
		infraRole    *apiv2.InfraRole
		machineRoles = map[string]apiv2.MachineRole{}
	)

	if t.Expires != nil {
		expires = timestamppb.New(*t.Expires)
	}
	if t.IssuedAt != nil {
		issuedAt = timestamppb.New(*t.IssuedAt)
	}

	for id, role := range t.ProjectRoles {
		projectRoles[id] = apiv2.ProjectRole(apiv2.ProjectRole_value[role])
	}
	for id, role := range t.TenantRoles {
		tenantRoles[id] = apiv2.TenantRole(apiv2.TenantRole_value[role])
	}

	if t.AdminRole != nil {
		adminRole = new(apiv2.AdminRole(apiv2.AdminRole_value[*t.AdminRole]))
	}

	if t.InfraRole != nil {
		infraRole = new(apiv2.InfraRole(apiv2.InfraRole_value[*t.InfraRole]))
	}

	for id, role := range t.MachineRoles {
		machineRoles[id] = apiv2.MachineRole(apiv2.MachineRole_value[role])
	}

	return &apiv2.Token{
		Uuid:         t.Uuid,
		User:         t.User,
		Description:  t.Description,
		Permissions:  permissions,
		Expires:      expires,
		IssuedAt:     issuedAt,
		TokenType:    apiv2.TokenType(t.TokenType),
		ProjectRoles: projectRoles,
		TenantRoles:  tenantRoles,
		AdminRole:    adminRole,
		InfraRole:    infraRole,
		MachineRoles: machineRoles,
	}
}
