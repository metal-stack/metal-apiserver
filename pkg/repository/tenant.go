package repository

import (
	"context"

	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	v1 "github.com/metal-stack/masterdata-api/api/v1"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
)

type tenantRepository struct {
	r *Store
}

// ValidateCreate implements Tenant.
func (t *tenantRepository) ValidateCreate(ctx context.Context, create *apiv2.TenantServiceCreateRequest) (*Validated[*apiv2.TenantServiceCreateRequest], error) {
	panic("unimplemented")
}

// ValidateDelete implements Tenant.
func (t *tenantRepository) ValidateDelete(ctx context.Context, e *v1.Tenant) (*Validated[*v1.Tenant], error) {
	panic("unimplemented")
}

// ValidateUpdate implements Tenant.
func (t *tenantRepository) ValidateUpdate(ctx context.Context, msg *apiv2.TenantServiceUpdateRequest) (*Validated[*apiv2.TenantServiceUpdateRequest], error) {
	panic("unimplemented")
}

// Create implements Tenant.
func (t *tenantRepository) Create(ctx context.Context, c *Validated[*apiv2.TenantServiceCreateRequest]) (*v1.Tenant, error) {
	// FIXME howto set the avatarurl during create ??
	tenant := &v1.Tenant{
		Name: c.message.Name,
	}

	if c.message.Description != nil {
		tenant.Description = *c.message.Description
	}

	resp, err := t.r.mdc.Tenant().Create(ctx, &v1.TenantCreateRequest{Tenant: tenant})
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return resp.Tenant, nil
}

// Delete implements Tenant.
func (t *tenantRepository) Delete(ctx context.Context, e *Validated[*v1.Tenant]) (*v1.Tenant, error) {
	panic("unimplemented")
}

// Find implements Tenant.
func (t *tenantRepository) Find(ctx context.Context, query *apiv2.TenantServiceListRequest) (*v1.Tenant, error) {
	panic("unimplemented")
}

// Get implements Tenant.
func (t *tenantRepository) Get(ctx context.Context, id string) (*v1.Tenant, error) {
	resp, err := t.r.mdc.Tenant().Get(ctx, &v1.TenantGetRequest{Id: id})
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return resp.Tenant, nil
}

// List implements Tenant.
func (t *tenantRepository) List(ctx context.Context, query *apiv2.TenantServiceListRequest) ([]*v1.Tenant, error) {
	panic("unimplemented")
}

// MatchScope implements Tenant.
func (t *tenantRepository) MatchScope(e *v1.Tenant) error {
	panic("unimplemented")
}

// Update implements Tenant.
func (t *tenantRepository) Update(ctx context.Context, msg *Validated[*apiv2.TenantServiceUpdateRequest]) (*v1.Tenant, error) {
	panic("unimplemented")
}

// ConvertToInternal implements Tenant.
func (t *tenantRepository) ConvertToInternal(msg *apiv2.Tenant) (*v1.Tenant, error) {
	panic("unimplemented")
}

// ConvertToProto implements Tenant.
func (t *tenantRepository) ConvertToProto(e *v1.Tenant) (*apiv2.Tenant, error) {
	panic("unimplemented")
}
