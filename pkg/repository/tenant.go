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

type tenant struct {
	*mdcv1.Tenant
}

func (t *tenant) SetChanged(time time.Time) {
}

type tenantRepository struct {
	s *Store
}

func (t *tenantRepository) matchScope(e *tenant) bool {
	return true
}

func (t *tenantRepository) create(ctx context.Context, rq *apiv2.TenantServiceCreateRequest) (*tenant, error) {
	return t.CreateWithID(ctx, rq, "")
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

func (t *tenantRepository) CreateWithID(ctx context.Context, c *apiv2.TenantServiceCreateRequest, id string, opts ...tenantCreateOpts) (*tenant, error) {
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

	te := &mdcv1.Tenant{
		Meta: &mdcv1.Meta{
			Id:          id,
			Annotations: ann,
			Labels:      labels,
		},
		Name: c.Name,
	}

	if c.Description != nil {
		te.Description = *c.Description
	}

	resp, err := t.s.mdc.Tenant().Create(ctx, &mdcv1.TenantCreateRequest{Tenant: te})
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return &tenant{Tenant: resp.Tenant}, nil
}

func (t *tenantRepository) delete(ctx context.Context, e *tenant) error {
	_, err := t.s.mdc.Tenant().Delete(ctx, &mdcv1.TenantDeleteRequest{Id: e.Meta.Id})
	if err != nil {
		return errorutil.Convert(err)
	}

	return nil
}

func (t *tenantRepository) find(ctx context.Context, query *apiv2.TenantServiceListRequest) (*tenant, error) {
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

func (t *tenantRepository) get(ctx context.Context, id string) (*tenant, error) {
	resp, err := t.s.mdc.Tenant().Get(ctx, &mdcv1.TenantGetRequest{Id: id})
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return &tenant{Tenant: resp.Tenant}, nil
}

func (t *tenantRepository) list(ctx context.Context, query *apiv2.TenantServiceListRequest) ([]*tenant, error) {
	resp, err := t.s.mdc.Tenant().Find(ctx, &mdcv1.TenantFindRequest{
		Id:   query.Id,
		Name: query.Name,
	})
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	ts := make([]*tenant, 0, len(resp.Tenants))
	for _, t := range resp.Tenants {
		ts = append(ts, &tenant{Tenant: t})
	}

	return ts, nil
}

func (t *tenantRepository) update(ctx context.Context, te *tenant, rq *apiv2.TenantServiceUpdateRequest) (*tenant, error) {
	if rq.Name != nil {
		te.Name = *rq.Name
	}
	if rq.Description != nil {
		te.Description = *rq.Description
	}

	ann := te.Meta.Annotations
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
		te.Meta.Labels = updateLabelsOnSlice(rq.Labels, te.Meta.Labels)
	}

	resp, err := t.s.mdc.Tenant().Update(ctx, &mdcv1.TenantUpdateRequest{Tenant: te.Tenant})
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return &tenant{Tenant: resp.Tenant}, nil
}

func (t *tenantRepository) convertToInternal(ctx context.Context, te *apiv2.Tenant) (*tenant, error) {
	ann := map[string]string{
		TenantTagEmail:     te.Email,
		TenantTagAvatarURL: te.AvatarUrl,
		TenantTagCreator:   te.CreatedBy,
	}

	var labels []string
	if te.Meta != nil && te.Meta.Labels != nil && len(te.Meta.Labels.Labels) > 0 {
		labels = tag.TagMap(te.Meta.Labels.Labels).Slice()
	}

	return &tenant{Tenant: &mdcv1.Tenant{
		Meta: &mdcv1.Meta{
			Id:          te.Login,
			Kind:        "Tenant",
			Annotations: ann,
			Labels:      labels,
		},
		Name:        te.Name,
		Description: te.Description,
	}}, nil
}

func (te *tenantRepository) convertToProto(ctx context.Context, t *tenant) (*apiv2.Tenant, error) {
	var labels *apiv2.Labels
	if t.Meta != nil && t.Meta.Labels != nil && len(t.Meta.Labels) > 0 {
		labels = &apiv2.Labels{
			Labels: tag.NewTagMap(t.Meta.Labels),
		}
	}

	return &apiv2.Tenant{
		Login:       t.Meta.Id,
		Name:        t.Name,
		Description: t.Description,
		Email:       t.Meta.Annotations[TenantTagEmail],
		AvatarUrl:   t.Meta.Annotations[TenantTagAvatarURL],
		CreatedBy:   t.Meta.Annotations[TenantTagCreator],
		Meta: &apiv2.Meta{
			CreatedAt: t.Meta.CreatedTime,
			UpdatedAt: t.Meta.UpdatedTime,
			Labels:    labels,
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

	return &store[*tenantMemberRepository, *tenantMember, *apiv2.TenantMember, *TenantMemberCreateRequest, *TenantMemberUpdateRequest, *TenantMemberQuery]{
		repository: repository,
		typed:      repository,
	}
}

func (t *tenantRepository) ListTenantMembers(ctx context.Context, te string, includeInherited bool) ([]*mdcv1.TenantWithMembershipAnnotations, error) {
	resp, err := t.s.mdc.Tenant().ListTenantMembers(ctx, &mdcv1.ListTenantMembersRequest{
		TenantId:         te,
		IncludeInherited: pointer.Pointer(includeInherited),
	})
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return resp.Tenants, nil
}

func (r *tenantRepository) FindParticipatingTenants(ctx context.Context, te string, includeInherited bool) ([]*mdcv1.TenantWithMembershipAnnotations, error) {
	resp, err := r.s.mdc.Tenant().FindParticipatingTenants(ctx, &mdcv1.FindParticipatingTenantsRequest{
		TenantId:         te,
		IncludeInherited: pointer.Pointer(includeInherited),
	})
	if err != nil {
		return nil, errorutil.Convert(err)
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
		return errorutil.Convert(fmt.Errorf("unable to get tenant %q: %w", providerTenantID, err))
	}

	if err != nil && errorutil.IsNotFound(err) {
		_, err := r.CreateWithID(ctx, &apiv2.TenantServiceCreateRequest{
			Name:        providerTenantID,
			Description: pointer.Pointer("initial provider tenant for metal-stack"),
		}, providerTenantID)
		if err != nil {
			return errorutil.Convert(fmt.Errorf("unable to create tenant:%s %w", providerTenantID, err))
		}
	}

	_, err = r.Member(providerTenantID).Get(ctx, providerTenantID)
	if err == nil {
		return nil
	}

	if !errorutil.IsNotFound(err) {
		return errorutil.Convert(err)
	}

	_, err = r.Member(providerTenantID).Create(ctx, &TenantMemberCreateRequest{
		MemberID: providerTenantID,
		Role:     apiv2.TenantRole_TENANT_ROLE_OWNER,
	})
	if err != nil {
		return errorutil.Convert(err)
	}

	return nil
}
