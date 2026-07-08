package repository

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/metal-stack/api/go/errorutil"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/api/go/tag"
	"github.com/metal-stack/metal-apiserver/pkg/repository/api"
	"github.com/metal-stack/metal-apiserver/pkg/tags"
	"github.com/metal-stack/metal-apiserver/pkg/token"
	tenantv1 "github.com/metal-stack/tenant-api/go/api/v1"
)

type (
	tenantEntity struct {
		*tenantv1.Tenant
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

type tenantCreateOpts any
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
		api.TenantTagCreator: creator,
	}

	if c.Email != nil {
		ann[api.TenantTagEmail] = *c.Email
	}
	if c.AvatarUrl != nil {
		ann[api.TenantTagAvatarURL] = *c.AvatarUrl
	}

	var labels []string
	if c.Labels != nil && len(c.Labels.Labels) > 0 {
		labels = tags.ToTags(c.Labels.Labels)
	}

	tenant := &tenantv1.Tenant{
		Meta: &tenantv1.Meta{
			Id:          id,
			Annotations: ann,
			Labels:      labels,
		},
		Name: c.Name,
	}

	if c.Description != nil {
		tenant.Description = *c.Description
	}

	resp, err := t.s.tc.Apiv1().Tenant().Create(ctx, &tenantv1.TenantServiceCreateRequest{Tenant: tenant})
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return &tenantEntity{Tenant: resp.Tenant}, nil
}

func (t *tenantRepository) delete(ctx context.Context, e *tenantEntity) (*deleteInfo, error) {
	_, err := t.s.tc.Apiv1().Tenant().Delete(ctx, &tenantv1.TenantServiceDeleteRequest{Id: e.Meta.Id})
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return nil, nil
}

func (t *tenantRepository) find(ctx context.Context, query *apiv2.TenantQuery) (*tenantEntity, error) {
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
	resp, err := t.s.tc.Apiv1().Tenant().Get(ctx, &tenantv1.TenantServiceGetRequest{Id: id})
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return &tenantEntity{Tenant: resp.Tenant}, nil
}

func (t *tenantRepository) list(ctx context.Context, query *apiv2.TenantQuery) ([]*tenantEntity, error) {
	if query == nil {
		query = &apiv2.TenantQuery{}
	}

	req := &tenantv1.TenantServiceListRequest{
		Id:   query.Login,
		Name: query.Name,
	}

	if query.Labels != nil && len(query.Labels.Labels) > 0 {
		req.Labels = tags.ToTags(query.Labels.Labels)
	}
	if query.Paging != nil {
		req.Paging = &tenantv1.Paging{
			Page:  query.Paging.Page,
			Count: query.Paging.Count,
		}
	}

	resp, err := t.s.tc.Apiv1().Tenant().List(ctx, req)
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
		ann[api.TenantTagEmail] = *rq.Email
	}
	if rq.AvatarUrl != nil {
		ann[api.TenantTagAvatarURL] = *rq.AvatarUrl
	}

	if rq.Labels != nil {
		tenant.Meta.Labels = updateLabelsOnSlice(rq.Labels, tenant.Meta.Labels)
	}

	resp, err := t.s.tc.Apiv1().Tenant().Update(ctx, &tenantv1.TenantServiceUpdateRequest{Tenant: tenant.Tenant})
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return &tenantEntity{Tenant: resp.Tenant}, nil
}

func (t *tenantRepository) convertToInternal(ctx context.Context, tenant *apiv2.Tenant) (*tenantEntity, error) {
	ann := map[string]string{
		api.TenantTagEmail:     tenant.Email,
		api.TenantTagAvatarURL: tenant.AvatarUrl,
		api.TenantTagCreator:   tenant.CreatedBy,
	}

	var labels []string
	if tenant.Meta != nil && tenant.Meta.Labels != nil && len(tenant.Meta.Labels.Labels) > 0 {
		labels = tags.ToTags(tenant.Meta.Labels.Labels)
	}

	return &tenantEntity{Tenant: &tenantv1.Tenant{
		Meta: &tenantv1.Meta{
			Id:          tenant.Login,
			Kind:        "Tenant",
			Annotations: ann,
			Labels:      labels,
		},
		Name:        tenant.Name,
		Description: tenant.Description,
	}}, nil
}

func (t *tenantRepository) convertToProto(ctx context.Context, tenant *tenantEntity) (*apiv2.Tenant, error) {
	var labels *apiv2.Labels

	if tenant.Meta != nil && tenant.Meta.Labels != nil && len(tenant.Meta.Labels) > 0 {
		labels = &apiv2.Labels{
			Labels: tags.ToLabels(tenant.Meta.Labels),
		}
	}

	return &apiv2.Tenant{
		Login:       tenant.Meta.Id,
		Name:        tenant.Name,
		Description: tenant.Description,
		Email:       tenant.Meta.Annotations[api.TenantTagEmail],
		AvatarUrl:   tenant.Meta.Annotations[api.TenantTagAvatarURL],
		CreatedBy:   tenant.Meta.Annotations[api.TenantTagCreator],
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

	return &store[*tenantMemberRepository, *tenantMemberEntity, *apiv2.TenantMember, *api.TenantMemberCreateRequest, *api.TenantMemberUpdateRequest, *api.TenantMemberQuery]{
		repository: repository,
		typed:      repository,
	}
}

func (t *tenantRepository) ListTenantMembers(ctx context.Context, tenant string, includeInherited bool) ([]*api.TenantWithMembershipAnnotations, error) {
	resp, err := t.s.tc.Apiv1().Tenant().ListTenantMembers(ctx, &tenantv1.TenantServiceListTenantMembersRequest{
		TenantId:         tenant,
		IncludeInherited: new(includeInherited),
	})
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	var res []*api.TenantWithMembershipAnnotations

	for _, tenant := range resp.Tenants {
		converted, err := t.convertToProto(ctx, &tenantEntity{Tenant: tenant.Tenant})
		if err != nil {
			return nil, err
		}

		res = append(res, &api.TenantWithMembershipAnnotations{
			Tenant:             converted,
			ProjectAnnotations: tenant.ProjectAnnotations,
			TenantAnnotations:  tenant.TenantAnnotations,
			ProjectIds:         tenant.ProjectIds,
		})
	}

	return res, nil
}

func (t *tenantRepository) FindParticipatingTenants(ctx context.Context, tenant string, includeInherited bool) ([]*api.TenantWithMembershipAnnotations, error) {
	resp, err := t.s.tc.Apiv1().Tenant().FindParticipatingTenants(ctx, &tenantv1.TenantServiceFindParticipatingTenantsRequest{
		TenantId:         tenant,
		IncludeInherited: new(includeInherited),
	})
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	var res []*api.TenantWithMembershipAnnotations

	for _, tenant := range resp.Tenants {
		converted, err := t.convertToProto(ctx, &tenantEntity{Tenant: tenant.Tenant})
		if err != nil {
			return nil, err
		}

		res = append(res, &api.TenantWithMembershipAnnotations{
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
		annotation = annotations[api.TenantRoleAnnotation]
		tenantRole = apiv2.TenantRole(apiv2.TenantRole_value[annotation])
	)

	return tenantRole
}

func (t *tenantRepository) EnsureProviderTenant(ctx context.Context, providerTenantID string) error {
	providerTenant, err := t.s.Tenant().Find(ctx, &apiv2.TenantQuery{
		Labels: &apiv2.Labels{
			Labels: map[string]string{
				tag.ProviderTenant: strconv.FormatBool(true),
			},
		},
	})

	switch {
	case err == nil:
		// noop
	case errorutil.IsNotFound(err):
		providerTenant, err = t.CreateWithID(ctx, &apiv2.TenantServiceCreateRequest{
			Name:        providerTenantID,
			Description: new("initial provider tenant for metal-stack"),
			Labels: &apiv2.Labels{
				Labels: map[string]string{
					tag.ProviderTenant: strconv.FormatBool(true),
				},
			},
		}, providerTenantID, NewTenantCreateOptWithCreator(providerTenantID))
		if err != nil && !errorutil.IsConflict(err) {
			return errorutil.Convert(fmt.Errorf("unable to create tenant %q: %w", providerTenantID, err))
		}

		if err != nil && errorutil.IsConflict(err) {
			t.s.log.Info("provider tenant already exists, but missing provider tenant label; patching tenant")

			providerTenant, err = t.s.Tenant().Update(ctx, providerTenantID, &apiv2.TenantServiceUpdateRequest{
				UpdateMeta: &apiv2.UpdateMeta{
					LockingStrategy: apiv2.OptimisticLockingStrategy_OPTIMISTIC_LOCKING_STRATEGY_SERVER,
				},
				Labels: &apiv2.UpdateLabels{
					Strategy: &apiv2.UpdateLabels_Patch{
						Patch: &apiv2.LabelsPatch{
							Update: &apiv2.Labels{
								Labels: map[string]string{
									tag.ProviderTenant: strconv.FormatBool(true),
								},
							},
						},
					},
				},
			})
			if err != nil {
				return errorutil.Internal("unable to patch provider tenant label to existing provider tenant entity: %w", err)
			}
		}

	default:
		return errorutil.Convert(fmt.Errorf("unable to find unique provider tenant %q: %w", providerTenantID, err))
	}

	if providerTenant.Login != providerTenantID {
		return errorutil.InvalidArgument("provider tenant %q already exists, refusing to create another one with id %q", providerTenant.Login, providerTenantID)
	}

	_, err = t.Member(providerTenantID).Get(ctx, providerTenantID)
	if err == nil {
		return nil
	}

	if !errorutil.IsNotFound(err) {
		return errorutil.Convert(err)
	}

	_, err = t.Member(providerTenantID).Create(ctx, &api.TenantMemberCreateRequest{
		MemberID: providerTenantID,
		Role:     apiv2.TenantRole_TENANT_ROLE_OWNER,
	})
	if err != nil {
		return errorutil.Convert(err)
	}

	return nil
}
