package repository

import (
	"context"

	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	mdcv1 "github.com/metal-stack/masterdata-api/api/v1"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
)

// FIXME completely untested and incomplete

type tenantRepository struct {
	r *Store
}

// ValidateCreate implements Tenant.
func (t *tenantRepository) ValidateCreate(ctx context.Context, create *apiv2.TenantServiceCreateRequest) (*Validated[*apiv2.TenantServiceCreateRequest], error) {
	return &Validated[*apiv2.TenantServiceCreateRequest]{
		message: create,
	}, nil
}

// ValidateDelete implements Tenant.
func (t *tenantRepository) ValidateDelete(ctx context.Context, e *mdcv1.Tenant) (*Validated[*mdcv1.Tenant], error) {
	return &Validated[*mdcv1.Tenant]{
		message: e,
	}, nil
}

// ValidateUpdate implements Tenant.
func (t *tenantRepository) ValidateUpdate(ctx context.Context, msg *apiv2.TenantServiceUpdateRequest) (*ValidatedUpdate[*mdcv1.Tenant, *apiv2.TenantServiceUpdateRequest], error) {
	return &ValidatedUpdate[*mdcv1.Tenant, *apiv2.TenantServiceUpdateRequest]{
		message: msg,
	}, nil
}

// Create implements Tenant.
func (t *tenantRepository) Create(ctx context.Context, c *Validated[*apiv2.TenantServiceCreateRequest]) (*mdcv1.Tenant, error) {
	// FIXME howto set the avatarurl during create ??
	tenant := &mdcv1.Tenant{
		Meta: &mdcv1.Meta{
			Id: c.message.Name,
		},
		Name: c.message.Name,
	}

	if c.message.Description != nil {
		tenant.Description = *c.message.Description
	}

	resp, err := t.r.mdc.Tenant().Create(ctx, &mdcv1.TenantCreateRequest{Tenant: tenant})
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return resp.Tenant, nil
}

// Delete implements Tenant.
func (t *tenantRepository) Delete(ctx context.Context, e *Validated[*mdcv1.Tenant]) (*mdcv1.Tenant, error) {
	panic("unimplemented")
}

// Find implements Tenant.
func (t *tenantRepository) Find(ctx context.Context, query *apiv2.TenantServiceListRequest) (*mdcv1.Tenant, error) {
	panic("unimplemented")
}

// Get implements Tenant.
func (t *tenantRepository) Get(ctx context.Context, id string) (*mdcv1.Tenant, error) {
	resp, err := t.r.mdc.Tenant().Get(ctx, &mdcv1.TenantGetRequest{Id: id})
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return resp.Tenant, nil
}

// List implements Tenant.
func (t *tenantRepository) List(ctx context.Context, query *apiv2.TenantServiceListRequest) ([]*mdcv1.Tenant, error) {
	panic("unimplemented")
}

// MatchScope implements Tenant.
func (t *tenantRepository) MatchScope(e *mdcv1.Tenant) error {
	panic("unimplemented")
}

// Update implements Tenant.
func (t *tenantRepository) Update(ctx context.Context, msg *ValidatedUpdate[*mdcv1.Tenant, *apiv2.TenantServiceUpdateRequest]) (*mdcv1.Tenant, error) {
	panic("unimplemented")
}

// ConvertToInternal implements Tenant.
func (t *tenantRepository) ConvertToInternal(msg *apiv2.Tenant) (*mdcv1.Tenant, error) {
	panic("unimplemented")
}

// ConvertToProto implements Tenant.
func (t *tenantRepository) ConvertToProto(e *mdcv1.Tenant) (*apiv2.Tenant, error) {
	panic("unimplemented")
}
