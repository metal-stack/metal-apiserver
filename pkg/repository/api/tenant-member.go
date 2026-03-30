package api

import (
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
)

const (
	// TODO: Migrate to common fields introduced in https://github.com/metal-stack/masterdata-api/pull/127
	TenantTagEmail     = "metal-stack.io/email"
	TenantTagAvatarURL = "metal-stack.io/avatarurl"
	TenantTagCreator   = "metal-stack.io/creator"

	// TODO: Use scoped memberships: https://github.com/metal-stack/masterdata-api/issues/130
	TenantRoleAnnotation = "metal-stack.io/tenant-role"
)

type (
	TenantWithMembershipAnnotations struct {
		Tenant             *apiv2.Tenant
		ProjectAnnotations map[string]string
		TenantAnnotations  map[string]string
		ProjectIds         []string
	}

	TenantMemberCreateRequest struct {
		MemberID string
		Role     apiv2.TenantRole
	}

	TenantMemberUpdateRequest struct {
		Role apiv2.TenantRole
	}

	TenantMemberQuery struct {
		MemberId    *string
		Annotations map[string]string
	}
)

func (*TenantMemberUpdateRequest) GetUpdateMeta() *apiv2.UpdateMeta {
	return &apiv2.UpdateMeta{}
}
