package api

const (
	// TODO: Migrate to common fields introduced in https://github.com/metal-stack/tenant-api/pull/127
	TenantTagEmail     = "metal-stack.io/email"
	TenantTagAvatarURL = "metal-stack.io/avatarurl"
	TenantTagCreator   = "metal-stack.io/creator"

	// TODO: Use scoped memberships: https://github.com/metal-stack/tenant-api/issues/130
	TenantRoleAnnotation = "metal-stack.io/tenant-role"
)
