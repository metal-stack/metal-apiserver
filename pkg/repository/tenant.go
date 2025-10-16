package repository

import (
	"context"
	"fmt"
	"time"

	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	mdcv1 "github.com/metal-stack/masterdata-api/api/v1"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	"github.com/metal-stack/metal-apiserver/pkg/token"
	"github.com/metal-stack/metal-lib/pkg/pointer"
	"github.com/metal-stack/metal-lib/pkg/tag"
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
	tenantEntity struct {
		*mdcv1.Tenant
	}

	TenantWithMembershipAnnotations struct {
		Tenant             *apiv2.Tenant
		ProjectAnnotations map[string]string
		TenantAnnotations  map[string]string
		ProjectIds         []string
	}

	tenantRepository struct {
		s *Store
	}
)

func (t *tenantEntity) SetChanged(time time.Time) {
}

func (t *tenantRepository) matchScope(e *tenantEntity) bool {
	return true
}

func (t *tenantRepository) create(ctx context.Context, rq *apiv2.TenantServiceCreateRequest) (*tenantEntity, error) {
	return t.createWithID(ctx, rq, "")
}

type tenantCreateOpts interface {
}
type tenantCreateOptWithCreator struct {
	creator string
}

func NewTenantCreateOptWithCreator(creator string) *tenantCreateOptWithCreator {
	return &tenantCreateOptWithCreator{
		creator: creator,
	}
}

func (t *tenantRepository) CreateWithID(ctx context.Context, c *apiv2.TenantServiceCreateRequest, id string, opts ...tenantCreateOpts) (*apiv2.Tenant, error) {
	tenant, err := t.createWithID(ctx, c, id, opts...)
	if err != nil {
		return nil, err
	}

	converted, err := t.convertToProto(ctx, tenant)
	if err != nil {
		return nil, err
	}

	return converted, nil
}

func (t *tenantRepository) createWithID(ctx context.Context, c *apiv2.TenantServiceCreateRequest, id string, opts ...tenantCreateOpts) (*tenantEntity, error) {
	var creator string

	for _, opt := range opts {
		switch o := opt.(type) {
		case *tenantCreateOptWithCreator:
			creator = o.creator
		default:
			return nil, errorutil.Internal("unknown tenantcreateopt:%T", o)
		}
	}

	if creator == "" {
		tok, ok := token.TokenFromContext(ctx)

		if !ok || tok == nil {
			return nil, errorutil.Unauthenticated("no token found in request")
		}
		creator = tok.User
	}

	ann := map[string]string{
		TenantTagCreator: creator,
	}

	if c.Email != nil {
		ann[TenantTagEmail] = *c.Email
	}
	if c.AvatarUrl != nil {
		ann[TenantTagAvatarURL] = *c.AvatarUrl
	}

	var labels []string
	if c.Labels != nil && len(c.Labels.Labels) > 0 {
		labels = tag.TagMap(c.Labels.Labels).Slice()
	}

	tenant := &mdcv1.Tenant{
		Meta: &mdcv1.Meta{
			Id:          id,
			Annotations: ann,
			Labels:      labels,
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

	return &tenantEntity{Tenant: resp.Tenant}, nil
}

func (t *tenantRepository) delete(ctx context.Context, e *tenantEntity) error {
	_, err := t.s.mdc.Tenant().Delete(ctx, &mdcv1.TenantDeleteRequest{Id: e.Meta.Id})
	if err != nil {
		return errorutil.Convert(err)
	}

	return nil
}

func (t *tenantRepository) find(ctx context.Context, query *apiv2.TenantServiceListRequest) (*tenantEntity, error) {
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
		return nil, errorutil.Internal("more than one tenant exists")
	}
}

func (t *tenantRepository) get(ctx context.Context, id string) (*tenantEntity, error) {
	resp, err := t.s.mdc.Tenant().Get(ctx, &mdcv1.TenantGetRequest{Id: id})
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return &tenantEntity{Tenant: resp.Tenant}, nil
}

func (t *tenantRepository) list(ctx context.Context, query *apiv2.TenantServiceListRequest) ([]*tenantEntity, error) {
	resp, err := t.s.mdc.Tenant().Find(ctx, &mdcv1.TenantFindRequest{
		Id:   query.Id,
		Name: query.Name,
	})
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	ts := make([]*tenantEntity, 0, len(resp.Tenants))
	for _, t := range resp.Tenants {
		ts = append(ts, &tenantEntity{Tenant: t})
	}

	return ts, nil
}

func (t *tenantRepository) update(ctx context.Context, tenant *tenantEntity, rq *apiv2.TenantServiceUpdateRequest) (*tenantEntity, error) {
	if rq.Name != nil {
		tenant.Name = *rq.Name
	}
	if rq.Description != nil {
		tenant.Description = *rq.Description
	}

	ann := tenant.Meta.Annotations
	if ann == nil {
		ann = map[string]string{}
	}

	if rq.Email != nil {
		ann[TenantTagEmail] = *rq.Email
	}
	if rq.AvatarUrl != nil {
		ann[TenantTagAvatarURL] = *rq.AvatarUrl
	}

	if rq.Labels != nil {
		tenant.Meta.Labels = updateLabelsOnSlice(rq.Labels, tenant.Meta.Labels)
	}

	resp, err := t.s.mdc.Tenant().Update(ctx, &mdcv1.TenantUpdateRequest{Tenant: tenant.Tenant})
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return &tenantEntity{Tenant: resp.Tenant}, nil
}

func (t *tenantRepository) convertToInternal(ctx context.Context, tenant *apiv2.Tenant, opts ...Option) (*tenantEntity, error) {
	ann := map[string]string{
		TenantTagEmail:     tenant.Email,
		TenantTagAvatarURL: tenant.AvatarUrl,
		TenantTagCreator:   tenant.CreatedBy,
	}

	var labels []string
	if tenant.Meta != nil && tenant.Meta.Labels != nil && len(tenant.Meta.Labels.Labels) > 0 {
		labels = tag.TagMap(tenant.Meta.Labels.Labels).Slice()
	}

	return &tenantEntity{Tenant: &mdcv1.Tenant{
		Meta: &mdcv1.Meta{
			Id:          tenant.Login,
			Kind:        "Tenant",
			Annotations: ann,
			Labels:      labels,
		},
		Name:        tenant.Name,
		Description: tenant.Description,
	}}, nil
}

func (t *tenantRepository) convertToProto(ctx context.Context, tenant *tenantEntity, opts ...Option) (*apiv2.Tenant, error) {
	var labels *apiv2.Labels
	if tenant.Meta != nil && tenant.Meta.Labels != nil && len(tenant.Meta.Labels) > 0 {
		labels = &apiv2.Labels{
			Labels: tag.NewTagMap(tenant.Meta.Labels),
		}
	}

	return &apiv2.Tenant{
		Login:       tenant.Meta.Id,
		Name:        tenant.Name,
		Description: tenant.Description,
		Email:       tenant.Meta.Annotations[TenantTagEmail],
		AvatarUrl:   tenant.Meta.Annotations[TenantTagAvatarURL],
		CreatedBy:   tenant.Meta.Annotations[TenantTagCreator],
		Meta: &apiv2.Meta{
			CreatedAt: tenant.Meta.CreatedTime,
			UpdatedAt: tenant.Meta.UpdatedTime,
			Labels:    labels,
		},
	}, nil
}

func (t *tenantRepository) Member(tenantID string) TenantMember {
	return t.tenantMember(&TenantScope{
		tenantID: tenantID,
	})
}

func (t *tenantRepository) UnscopedMember() TenantMember {
	return t.tenantMember(nil)
}

func (t *tenantRepository) tenantMember(scope *TenantScope) TenantMember {
	repository := &tenantMemberRepository{
		s:     t.s,
		scope: scope,
	}

	return &store[*tenantMemberRepository, *tenantMemberEntity, *apiv2.TenantMember, *TenantMemberCreateRequest, *TenantMemberUpdateRequest, *TenantMemberQuery]{
		repository: repository,
		typed:      repository,
	}
}

func (t *tenantRepository) ListTenantMembers(ctx context.Context, tenant string, includeInherited bool) ([]*TenantWithMembershipAnnotations, error) {
	resp, err := t.s.mdc.Tenant().ListTenantMembers(ctx, &mdcv1.ListTenantMembersRequest{
		TenantId:         tenant,
		IncludeInherited: pointer.Pointer(includeInherited),
	})
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	var res []*TenantWithMembershipAnnotations

	for _, tenant := range resp.Tenants {
		converted, err := t.convertToProto(ctx, &tenantEntity{Tenant: tenant.Tenant})
		if err != nil {
			return nil, err
		}

		res = append(res, &TenantWithMembershipAnnotations{
			Tenant:             converted,
			ProjectAnnotations: tenant.ProjectAnnotations,
			TenantAnnotations:  tenant.TenantAnnotations,
			ProjectIds:         tenant.ProjectIds,
		})
	}

	return res, nil
}

func (t *tenantRepository) FindParticipatingTenants(ctx context.Context, tenant string, includeInherited bool) ([]*TenantWithMembershipAnnotations, error) {
	resp, err := t.s.mdc.Tenant().FindParticipatingTenants(ctx, &mdcv1.FindParticipatingTenantsRequest{
		TenantId:         tenant,
		IncludeInherited: pointer.Pointer(includeInherited),
	})
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	var res []*TenantWithMembershipAnnotations

	for _, tenant := range resp.Tenants {
		converted, err := t.convertToProto(ctx, &tenantEntity{Tenant: tenant.Tenant})
		if err != nil {
			return nil, err
		}

		res = append(res, &TenantWithMembershipAnnotations{
			Tenant:             converted,
			ProjectAnnotations: tenant.ProjectAnnotations,
			TenantAnnotations:  tenant.TenantAnnotations,
			ProjectIds:         tenant.ProjectIds,
		})
	}

	return res, nil
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

func (t *tenantRepository) EnsureProviderTenant(ctx context.Context, providerTenantID string) error {
	_, err := t.s.Tenant().Get(ctx, providerTenantID)
	if err != nil && !errorutil.IsNotFound(err) {
		return errorutil.Convert(fmt.Errorf("unable to get tenant %q: %w", providerTenantID, err))
	}

	if err != nil && errorutil.IsNotFound(err) {
		_, err := t.CreateWithID(ctx, &apiv2.TenantServiceCreateRequest{
			Name:        providerTenantID,
			Description: pointer.Pointer("initial provider tenant for metal-stack"),
		}, providerTenantID)
		if err != nil {
			return errorutil.Convert(fmt.Errorf("unable to create tenant:%s %w", providerTenantID, err))
		}
	}

	_, err = t.Member(providerTenantID).Get(ctx, providerTenantID)
	if err == nil {
		return nil
	}

	if !errorutil.IsNotFound(err) {
		return errorutil.Convert(err)
	}

	_, err = t.Member(providerTenantID).Create(ctx, &TenantMemberCreateRequest{
		MemberID: providerTenantID,
		Role:     apiv2.TenantRole_TENANT_ROLE_OWNER,
	})
	if err != nil {
		return errorutil.Convert(err)
	}

	return nil
}
