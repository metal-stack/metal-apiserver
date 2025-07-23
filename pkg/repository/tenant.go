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
	s *Store
}

func (t *tenantRepository) validateCreate(ctx context.Context, create *apiv2.TenantServiceCreateRequest) error {
	return nil
}

func (t *tenantRepository) validateDelete(ctx context.Context, e *mdcv1.Tenant) error {
	projects, err := t.s.UnscopedProject().List(ctx, &apiv2.ProjectServiceListRequest{
		Tenant: &e.Meta.Id,
	})
	if err != nil {
		return errorutil.Convert(err)
	}

	if len(projects) > 0 {
		return connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("there are still projects associated with this tenant, you need to delete them first"))
	}

	return nil
}

func (t *tenantRepository) validateUpdate(ctx context.Context, msg *apiv2.TenantServiceUpdateRequest, _ *mdcv1.Tenant) error {
	return nil
}

func (t *tenantRepository) create(ctx context.Context, rq *apiv2.TenantServiceCreateRequest) (*mdcv1.Tenant, error) {
	return t.CreateWithID(ctx, rq, "")
}

func (t *tenantRepository) CreateWithID(ctx context.Context, c *apiv2.TenantServiceCreateRequest, id string) (*mdcv1.Tenant, error) {
	tok, ok := token.TokenFromContext(ctx)

	if !ok || tok == nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("no token found in request"))
	}

	ann := map[string]string{
		tutil.TagCreator: tok.UserId,
	}

	if c.Email != nil {
		ann[tutil.TagEmail] = *c.Email
	}
	if c.AvatarUrl != nil {
		ann[tutil.TagAvatarURL] = *c.AvatarUrl
	}
	if c.PhoneNumber != nil {
		ann[tutil.TagPhoneNumber] = *c.PhoneNumber
	}

	tenant := &mdcv1.Tenant{
		Meta: &mdcv1.Meta{
			Id:          id,
			Annotations: ann,
		},
		Name: c.Name,
	}

	if c.Description != nil {
		tenant.Description = *c.Description
	}

	resp, err := t.s.mdc.Tenant().Create(ctx, &mdcv1.TenantCreateRequest{Tenant: tenant})
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return resp.Tenant, nil
}

func (t *tenantRepository) delete(ctx context.Context, e *mdcv1.Tenant) error {
	panic("unimplemented")
}

func (t *tenantRepository) find(ctx context.Context, query *apiv2.TenantServiceListRequest) (*mdcv1.Tenant, error) {
	panic("unimplemented")
}

func (t *tenantRepository) get(ctx context.Context, id string) (*mdcv1.Tenant, error) {
	resp, err := t.s.mdc.Tenant().Get(ctx, &mdcv1.TenantGetRequest{Id: id})
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return resp.Tenant, nil
}

func (t *tenantRepository) list(ctx context.Context, query *apiv2.TenantServiceListRequest) ([]*mdcv1.Tenant, error) {
	panic("unimplemented")
}

// MatchScope implements Tenant.
func (t *tenantRepository) matchScope(e *mdcv1.Tenant) bool {
	panic("unimplemented")
}

func (t *tenantRepository) update(ctx context.Context, e *mdcv1.Tenant, msg *apiv2.TenantServiceUpdateRequest) (*mdcv1.Tenant, error) {
	panic("unimplemented")
}

func (t *tenantRepository) convertToInternal(msg *apiv2.Tenant) (*mdcv1.Tenant, error) {
	panic("unimplemented")
}

func (t *tenantRepository) convertToProto(e *mdcv1.Tenant) (*apiv2.Tenant, error) {
	panic("unimplemented")
}

func (r *tenantRepository) Member(tenantID string) TenantMember {
	return r.tenantMember(&TenantScope{
		tenantID: tenantID,
	})
}

func (r *tenantRepository) UnscopedMember() TenantMember {
	return r.tenantMember(nil)
}

func (r *tenantRepository) tenantMember(scope *TenantScope) TenantMember {
	repository := &tenantMemberRepository{
		s:     r.s,
		scope: scope,
	}

	return &store[*tenantMemberRepository, *mdcv1.TenantMember, *mdcv1.TenantMember, *TenantMemberCreateRequest, *TenantMemberUpdateRequest, *TenantMemberQuery]{
		repository: repository,
		typed:      repository,
	}
}

func (t *tenantRepository) ListTenantMembers(ctx context.Context, tenant string, includeInherited bool) ([]*mdcv1.TenantWithMembershipAnnotations, error) {
	resp, err := t.s.mdc.Tenant().ListTenantMembers(ctx, &mdcv1.ListTenantMembersRequest{
		TenantId:         tenant,
		IncludeInherited: pointer.Pointer(includeInherited),
	})
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return resp.Tenants, nil
}

func (r *tenantRepository) FindParticipatingTenants(ctx context.Context, tenant string, includeInherited bool) ([]*mdcv1.TenantWithMembershipAnnotations, error) {
	resp, err := r.s.mdc.Tenant().FindParticipatingTenants(ctx, &mdcv1.FindParticipatingTenantsRequest{
		TenantId:         tenant,
		IncludeInherited: pointer.Pointer(includeInherited),
	})
	if err != nil {
		return nil, err
	}

	return resp.Tenants, nil
}
