package repository

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	mdcv1 "github.com/metal-stack/masterdata-api/api/v1"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	tutil "github.com/metal-stack/metal-apiserver/pkg/tenant"
	"github.com/metal-stack/metal-apiserver/pkg/token"
	"github.com/metal-stack/metal-lib/pkg/pointer"
)

type tenantRepository struct {
	r *Store
}

func (t *tenantRepository) ValidateCreate(ctx context.Context, create *apiv2.TenantServiceCreateRequest) (*Validated[*apiv2.TenantServiceCreateRequest], error) {
	return &Validated[*apiv2.TenantServiceCreateRequest]{
		message: create,
	}, nil
}

func (t *tenantRepository) ValidateDelete(ctx context.Context, e *mdcv1.Tenant) (*Validated[*mdcv1.Tenant], error) {
	projects, err := t.r.UnscopedProject().List(ctx, &apiv2.ProjectServiceListRequest{
		Tenant: &e.Meta.Id,
	})
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	if len(projects) > 0 {
		return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("there are still projects associated with this tenant, you need to delete them first"))
	}

	return &Validated[*mdcv1.Tenant]{
		message: e,
	}, nil
}

func (t *tenantRepository) ValidateUpdate(ctx context.Context, msg *apiv2.TenantServiceUpdateRequest) (*Validated[*apiv2.TenantServiceUpdateRequest], error) {
	return &Validated[*apiv2.TenantServiceUpdateRequest]{
		message: msg,
	}, nil
}

func (t *tenantRepository) Create(ctx context.Context, c *Validated[*apiv2.TenantServiceCreateRequest]) (*mdcv1.Tenant, error) {
	return t.CreateWithID(ctx, c, "")
}

func (t *tenantRepository) CreateWithID(ctx context.Context, c *Validated[*apiv2.TenantServiceCreateRequest], id string) (*mdcv1.Tenant, error) {
	tok, ok := token.TokenFromContext(ctx)

	if !ok || tok == nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("no token found in request"))
	}

	ann := map[string]string{
		tutil.TagCreator: tok.UserId,
	}

	if c.message.Email != nil {
		ann[tutil.TagEmail] = *c.message.Email
	}
	if c.message.AvatarUrl != nil {
		ann[tutil.TagAvatarURL] = *c.message.AvatarUrl
	}
	if c.message.PhoneNumber != nil {
		ann[tutil.TagPhoneNumber] = *c.message.PhoneNumber
	}

	tenant := &mdcv1.Tenant{
		Meta: &mdcv1.Meta{
			Id:          id,
			Annotations: ann,
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

func (t *tenantRepository) Delete(ctx context.Context, e *Validated[*mdcv1.Tenant]) (*mdcv1.Tenant, error) {
	panic("unimplemented")
}

func (t *tenantRepository) Find(ctx context.Context, query *apiv2.TenantServiceListRequest) (*mdcv1.Tenant, error) {
	panic("unimplemented")
}

func (t *tenantRepository) Get(ctx context.Context, id string) (*mdcv1.Tenant, error) {
	resp, err := t.r.mdc.Tenant().Get(ctx, &mdcv1.TenantGetRequest{Id: id})
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return resp.Tenant, nil
}

func (t *tenantRepository) List(ctx context.Context, query *apiv2.TenantServiceListRequest) ([]*mdcv1.Tenant, error) {
	panic("unimplemented")
}

// MatchScope implements Tenant.
func (t *tenantRepository) MatchScope(e *mdcv1.Tenant) error {
	panic("unimplemented")
}

func (t *tenantRepository) Update(ctx context.Context, msg *Validated[*apiv2.TenantServiceUpdateRequest]) (*mdcv1.Tenant, error) {
	panic("unimplemented")
}

func (t *tenantRepository) ConvertToInternal(msg *apiv2.Tenant) (*mdcv1.Tenant, error) {
	panic("unimplemented")
}

func (t *tenantRepository) ConvertToProto(e *mdcv1.Tenant) (*apiv2.Tenant, error) {
	panic("unimplemented")
}

func (t *tenantRepository) Member(tenantID string) TenantMember {
	return &tenantMemberRepository{
		r:     t.r,
		scope: &TenantScope{tenantID: tenantID},
	}
}

func (t *tenantRepository) ListTenantMembers(ctx context.Context, tenant string, includeInherited bool) ([]*mdcv1.TenantWithMembershipAnnotations, error) {
	resp, err := t.r.mdc.Tenant().ListTenantMembers(ctx, &mdcv1.ListTenantMembersRequest{
		TenantId:         tenant,
		IncludeInherited: pointer.Pointer(includeInherited),
	})
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return resp.Tenants, nil
}

func (r *tenantRepository) FindParticipatingTenants(ctx context.Context, tenant string, includeInherited bool) ([]*mdcv1.TenantWithMembershipAnnotations, error) {
	resp, err := r.r.mdc.Tenant().FindParticipatingTenants(ctx, &mdcv1.FindParticipatingTenantsRequest{
		TenantId:         tenant,
		IncludeInherited: pointer.Pointer(includeInherited),
	})
	if err != nil {
		return nil, err
	}

	return resp.Tenants, nil
}
