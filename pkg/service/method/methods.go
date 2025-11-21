package method

import (
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/api/go/permissions"
)

func IsAdminToken(token *apiv2.Token) bool {
	return token.AdminRole != nil
}

func PermissionsBySubject(token *apiv2.Token) map[string]*apiv2.MethodPermission {
	res := map[string]*apiv2.MethodPermission{}
	for _, p := range token.Permissions {
		perm, ok := res[p.Subject]
		if !ok {
			perm = &apiv2.MethodPermission{
				Subject: p.Subject,
			}
		}

		perm.Methods = append(perm.Methods, p.Methods...)

		res[p.Subject] = perm
	}
	return res
}

func AllowedMethodsFromRoles(servicePermissions *permissions.ServicePermissions, token *apiv2.Token) map[string]*apiv2.MethodPermission {
	perms := map[string]*apiv2.MethodPermission{}

	for projectID, role := range token.ProjectRoles {
		perm, ok := perms[projectID]
		if !ok {
			perm = &apiv2.MethodPermission{
				Subject: projectID,
			}
		}

		perm.Methods = append(perm.Methods, servicePermissions.Roles.Project[role.String()]...)

		perms[projectID] = perm
	}

	for tenantID, role := range token.TenantRoles {
		perm, ok := perms[tenantID]
		if !ok {
			perm = &apiv2.MethodPermission{
				Subject: tenantID,
			}
		}

		perm.Methods = append(perm.Methods, servicePermissions.Roles.Tenant[role.String()]...)

		perms[tenantID] = perm
	}

	return perms
}
