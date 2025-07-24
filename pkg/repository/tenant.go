package repository

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	mdcv1 "github.com/metal-stack/masterdata-api/api/v1"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	"github.com/metal-stack/metal-apiserver/pkg/token"
	"github.com/metal-stack/metal-lib/pkg/pointer"
	wrapperspb "google.golang.org/protobuf/types/known/wrapperspb"
)

const (
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

func (t *tenantRepository) matchScope(e *mdcv1.Tenant) bool {
	return true
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
		TagCreator: tok.UserId,
	}

	if c.Email != nil {
		ann[TagEmail] = *c.Email
	}
	if c.AvatarUrl != nil {
		ann[TagAvatarURL] = *c.AvatarUrl
	}
	if c.PhoneNumber != nil {
		ann[TagPhoneNumber] = *c.PhoneNumber
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
	_, err := t.s.mdc.Tenant().Delete(ctx, &mdcv1.TenantDeleteRequest{Id: e.Meta.Id})
	if err != nil {
		return errorutil.Convert(err)
	}

	return nil
}

func (t *tenantRepository) find(ctx context.Context, query *apiv2.TenantServiceListRequest) (*mdcv1.Tenant, error) {
	tenants, err := t.list(ctx, query)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	switch len(tenants) {
	case 0:
		return nil, errorutil.NotFound("cannot find tenant")
	case 1:
		return tenants[0], nil
	default:
		return nil, fmt.Errorf("more than one tenant exists")
	}
}

func (t *tenantRepository) get(ctx context.Context, id string) (*mdcv1.Tenant, error) {
	resp, err := t.s.mdc.Tenant().Get(ctx, &mdcv1.TenantGetRequest{Id: id})
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return resp.Tenant, nil
}

func (t *tenantRepository) list(ctx context.Context, query *apiv2.TenantServiceListRequest) ([]*mdcv1.Tenant, error) {
	var (
		id   *wrapperspb.StringValue
		name *wrapperspb.StringValue
	)

	if query.Id != nil {
		id = &wrapperspb.StringValue{Value: *query.Id}
	}
	if query.Name != nil {
		name = &wrapperspb.StringValue{Value: *query.Name}
	}

	resp, err := t.s.mdc.Tenant().Find(ctx, &mdcv1.TenantFindRequest{
		Id:   id,
		Name: name,
	})
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return resp.GetTenants(), nil
}

func (t *tenantRepository) update(ctx context.Context, e *mdcv1.Tenant, msg *apiv2.TenantServiceUpdateRequest) (*mdcv1.Tenant, error) {
	if msg.Name != nil {
		e.Name = *msg.Name
	}
	if msg.Description != nil {
		e.Description = *msg.Description
	}

	ann := e.Meta.Annotations

	if msg.Email != nil {
		ann[TagEmail] = *msg.Email
	}
	if msg.AvatarUrl != nil {
		ann[TagAvatarURL] = *msg.AvatarUrl
	}
	// TODO: add phone number to update request?
	// if msg. != nil {
	// 	ann[tutil.TagPhoneNumber] = *msg.PhoneNumber
	// }

	resp, err := t.s.mdc.Tenant().Update(ctx, &mdcv1.TenantUpdateRequest{Tenant: e})
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return resp.Tenant, nil
}

func (t *tenantRepository) convertToInternal(tenant *apiv2.Tenant) (*mdcv1.Tenant, error) {
	ann := map[string]string{
		TagEmail:     tenant.Email,
		TagAvatarURL: tenant.AvatarUrl,
		// tutil.TagPhoneNumber:  tenant.,
	}

	return &mdcv1.Tenant{
		Meta: &mdcv1.Meta{
			Id:          tenant.Login,
			Kind:        "Tenant",
			Annotations: ann,
		},
		Name:        tenant.Name,
		Description: tenant.Description,
	}, nil
}

func (te *tenantRepository) convertToProto(t *mdcv1.Tenant) (*apiv2.Tenant, error) {
	return &apiv2.Tenant{
		Login:       t.Meta.Id,
		Name:        t.Name,
		Description: t.Description,
		Email:       t.Meta.Annotations[TagEmail],
		AvatarUrl:   t.Meta.Annotations[TagAvatarURL],
		Meta: &apiv2.Meta{
			CreatedAt: t.Meta.CreatedTime,
			UpdatedAt: t.Meta.UpdatedTime,
		},
	}, nil
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

func (r *tenantRepository) EnsureProviderTenant(ctx context.Context, providerTenantID string) error {
	_, err := r.s.Tenant().Get(ctx, providerTenantID)
	if err != nil && !errorutil.IsNotFound(err) {
		return fmt.Errorf("unable to get tenant %q: %w", providerTenantID, err)
	}

	if err != nil && errorutil.IsNotFound(err) {
		_, err := r.CreateWithID(ctx, &apiv2.TenantServiceCreateRequest{
			Name:        providerTenantID,
			Description: pointer.Pointer("initial provider tenant for metal-stack"),
		}, providerTenantID)
		if err != nil {
			return fmt.Errorf("unable to create tenant:%s %w", providerTenantID, err)
		}
	}

	_, err = r.Member(providerTenantID).Get(ctx, providerTenantID)
	if err == nil {
		return nil
	}

	if connect.CodeOf(err) != connect.CodeNotFound {
		return err
	}

	_, err = r.Member(providerTenantID).Create(ctx, &TenantMemberCreateRequest{
		MemberID: providerTenantID,
		Role:     apiv2.TenantRole_TENANT_ROLE_OWNER,
	})
	if err != nil {
		return err
	}

	return nil
}
