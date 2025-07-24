package v1

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	mdcv1 "github.com/metal-stack/masterdata-api/api/v1"
	mdc "github.com/metal-stack/masterdata-api/pkg/client"
)

const (
	// TODO: maybe should move to metal-lib?
	// FIXME: overlaps with metalstack.cloud annotations
	TagEmail       = "metal-stack.io/email"
	TagPhoneNumber = "metal-stack.io/phone"
	TagAvatarURL   = "metal-stack.io/avatarurl"
	TagCreator     = "metal-stack.io/creator"

	TenantRoleAnnotation = "metal-stack.io/tenant-role"

	// Master Tenant which must be present on every metal-stack installation
	MasterTenant = "metal-stack"
	// Master Tenant Project ID must be present on every metal-stack installation
	MasterTenantProjectId = "00000000-0000-0000-0000-000000000000"
)

// FIXME this should go to tenant repository

func TenantRoleFromMap(annotations map[string]string) apiv2.TenantRole {
	if annotations == nil {
		return apiv2.TenantRole_TENANT_ROLE_UNSPECIFIED
	}

	var (
		annotation = annotations[TenantRoleAnnotation]
		tenantRole = apiv2.TenantRole(apiv2.TenantRole_value[annotation])
	)

	return tenantRole
}

func GetTenantMember(ctx context.Context, c mdc.Client, tenantID, memberID string) (*mdcv1.TenantMember, error) {
	memberships, err := c.TenantMember().Find(ctx, &mdcv1.TenantMemberFindRequest{
		MemberId: &memberID,
		TenantId: &tenantID,
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	switch len(memberships.TenantMembers) {
	case 0:
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("tenant %s is not a member of tenant %s", tenantID, memberID))
	case 1:
		// fallthrough
	default:
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("found multiple membership associations for a member to a tenant"))
	}

	return memberships.GetTenantMembers()[0], nil
}

func EnsureProviderTenant(ctx context.Context, c mdc.Client, providerTenantID string) error {
	_, err := c.Tenant().Get(ctx, &mdcv1.TenantGetRequest{
		Id: providerTenantID,
	})
	if err != nil && !mdcv1.IsNotFound(err) {
		return fmt.Errorf("unable to get tenant %q: %w", providerTenantID, err)
	}

	if err != nil && mdcv1.IsNotFound(err) {
		_, err := c.Tenant().Create(ctx, &mdcv1.TenantCreateRequest{
			Tenant: &mdcv1.Tenant{
				Meta: &mdcv1.Meta{
					Id: providerTenantID,
					Annotations: map[string]string{
						TagCreator: providerTenantID,
					},
				},
				Name:        providerTenantID,
				Description: "initial provider tenant for metal-stack",
			},
		})
		if err != nil {
			return fmt.Errorf("unable to create tenant:%s %w", providerTenantID, err)
		}
	}

	_, err = GetTenantMember(ctx, c, providerTenantID, providerTenantID)
	if err == nil {
		return nil
	}

	if connect.CodeOf(err) != connect.CodeNotFound {
		return err
	}

	_, err = c.TenantMember().Create(ctx, &mdcv1.TenantMemberCreateRequest{
		TenantMember: &mdcv1.TenantMember{
			Meta: &mdcv1.Meta{
				Annotations: map[string]string{
					TenantRoleAnnotation: apiv2.TenantRole_TENANT_ROLE_OWNER.String(),
				},
			},
			TenantId: providerTenantID,
			MemberId: providerTenantID,
		},
	})
	if err != nil {
		return err
	}

	return nil
}
