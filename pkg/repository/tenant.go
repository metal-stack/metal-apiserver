package repository

import (
	"context"

	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	mdcv1 "github.com/metal-stack/masterdata-api/api/v1"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
)

// FIXME completely untested and incomplete

type tenantRepository struct {
	s *Store
}

// ValidateCreate implements Tenant.
func (t *tenantRepository) validateCreate(ctx context.Context, create *apiv2.TenantServiceCreateRequest) error {
	return nil
}

// ValidateDelete implements Tenant.
func (t *tenantRepository) validateDelete(ctx context.Context, e *mdcv1.Tenant) error {
	return nil
}

// ValidateUpdate implements Tenant.
func (t *tenantRepository) validateUpdate(ctx context.Context, msg *apiv2.TenantServiceUpdateRequest, _ *mdcv1.Tenant) error {
	return nil
}

// Create implements Tenant.
func (t *tenantRepository) create(ctx context.Context, rq *apiv2.TenantServiceCreateRequest) (*mdcv1.Tenant, error) {
	// FIXME howto set the avatarurl during create ??
	tenant := &mdcv1.Tenant{
		Meta: &mdcv1.Meta{
			Id: rq.Name,
		},
		Name: rq.Name,
	}

	if rq.Description != nil {
		tenant.Description = *rq.Description
	}

	resp, err := t.s.mdc.Tenant().Create(ctx, &mdcv1.TenantCreateRequest{Tenant: tenant})
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return resp.Tenant, nil
}

// Delete implements Tenant.
func (t *tenantRepository) delete(ctx context.Context, e *mdcv1.Tenant) error {
	panic("unimplemented")
}

// Find implements Tenant.
func (t *tenantRepository) find(ctx context.Context, query *apiv2.TenantServiceListRequest) (*mdcv1.Tenant, error) {
	panic("unimplemented")
}

// Get implements Tenant.
func (t *tenantRepository) get(ctx context.Context, id string) (*mdcv1.Tenant, error) {
	resp, err := t.s.mdc.Tenant().Get(ctx, &mdcv1.TenantGetRequest{Id: id})
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return resp.Tenant, nil
}

// List implements Tenant.
func (t *tenantRepository) list(ctx context.Context, query *apiv2.TenantServiceListRequest) ([]*mdcv1.Tenant, error) {
	panic("unimplemented")
}

// MatchScope implements Tenant.
func (t *tenantRepository) matchScope(e *mdcv1.Tenant) bool {
	panic("unimplemented")
}

// Update implements Tenant.
func (t *tenantRepository) update(ctx context.Context, e *mdcv1.Tenant, msg *apiv2.TenantServiceUpdateRequest) (*mdcv1.Tenant, error) {
	panic("unimplemented")
}

// ConvertToInternal implements Tenant.
func (t *tenantRepository) convertToInternal(msg *apiv2.Tenant) (*mdcv1.Tenant, error) {
	panic("unimplemented")
}

// ConvertToProto implements Tenant.
func (t *tenantRepository) convertToProto(e *mdcv1.Tenant) (*apiv2.Tenant, error) {
	panic("unimplemented")
}
