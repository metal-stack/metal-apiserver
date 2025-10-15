package repository

import (
	"context"
	"fmt"
	"time"

	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	mdcv1 "github.com/metal-stack/masterdata-api/api/v1"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	"github.com/metal-stack/metal-lib/pkg/pointer"
	"github.com/metal-stack/metal-lib/pkg/tag"
)

const (
	ProjectRoleAnnotation = "metal-stack.io/project-role"
	avatarURLAnnotation   = "avatarUrl"
)

type (
	projectRepository struct {
		s     *Store
		scope *ProjectScope
	}

	projectEntity struct {
		*mdcv1.Project
	}
)

func (t *projectEntity) SetChanged(time time.Time) {}

func (r *projectRepository) Member() ProjectMember {
	return r.projectMember(&ProjectScope{
		projectID: r.scope.projectID,
	})
}

func (r *projectRepository) UnscopedMember() ProjectMember {
	return r.projectMember(nil)
}

func (r *projectRepository) projectMember(scope *ProjectScope) ProjectMember {
	repository := &projectMemberRepository{
		s:     r.s,
		scope: scope,
	}

	return &store[*projectMemberRepository, *projectMemberEntity, *apiv2.ProjectMember, *ProjectMemberCreateRequest, *ProjectMemberUpdateRequest, *ProjectMemberQuery]{
		typed:      repository,
		repository: repository,
	}
}

func (*ProjectMemberUpdateRequest) GetUpdateMeta() *apiv2.UpdateMeta {
	return &apiv2.UpdateMeta{}
}

func (r *projectRepository) get(ctx context.Context, id string) (*projectEntity, error) {
	resp, err := r.s.mdc.Project().Get(ctx, &mdcv1.ProjectGetRequest{Id: id})
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	if resp.Project == nil || resp.Project.Meta == nil {
		return nil, errorutil.NotFound("project %q has no meta", id)
	}

	return &projectEntity{
		Project: resp.Project,
	}, nil
}

func (r *projectRepository) matchScope(p *projectEntity) bool {
	if r.scope == nil {
		return true
	}

	return r.scope.projectID == p.Meta.Id
}

func (r *projectRepository) create(ctx context.Context, e *apiv2.ProjectServiceCreateRequest) (*projectEntity, error) {
	return r.CreateWithID(ctx, e, "")
}

func (r *projectRepository) CreateWithID(ctx context.Context, e *apiv2.ProjectServiceCreateRequest, id string) (*projectEntity, error) {
	ann := map[string]string{}

	if e.AvatarUrl != nil {
		ann[avatarURLAnnotation] = *e.AvatarUrl
	}

	var labels []string
	if e.Labels != nil && len(e.Labels.Labels) > 0 {
		labels = tag.TagMap(e.Labels.Labels).Slice()
	}

	resp, err := r.s.mdc.Project().Create(ctx, &mdcv1.ProjectCreateRequest{Project: &mdcv1.Project{
		Meta: &mdcv1.Meta{
			Annotations: ann,
			Id:          id,
			Labels:      labels,
		},
		Name:        e.Name,
		Description: e.Description,
		TenantId:    e.Login,
	}})
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return &projectEntity{
		Project: resp.Project,
	}, nil
}

func (r *projectRepository) update(ctx context.Context, p *projectEntity, rq *apiv2.ProjectServiceUpdateRequest) (*projectEntity, error) {
	if rq.Description != nil {
		p.Description = *rq.Description
	}
	if rq.Name != nil {
		p.Name = *rq.Name
	}

	ann := p.Meta.Annotations
	if ann == nil {
		ann = map[string]string{}
	}

	if rq.AvatarUrl != nil {
		ann[avatarURLAnnotation] = *rq.AvatarUrl
	}

	if rq.Labels != nil {
		p.Meta.Labels = updateLabelsOnSlice(rq.Labels, p.Meta.Labels)
	}

	resp, err := r.s.mdc.Project().Update(ctx, &mdcv1.ProjectUpdateRequest{Project: p.Project})
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return &projectEntity{
		Project: resp.Project,
	}, nil
}

func (r *projectRepository) delete(ctx context.Context, e *projectEntity) error {
	_, err := r.s.mdc.Project().Delete(ctx, &mdcv1.ProjectDeleteRequest{Id: e.Meta.Id})
	if err != nil {
		return errorutil.Convert(err)
	}

	return nil
}

func (r *projectRepository) find(ctx context.Context, query *apiv2.ProjectServiceListRequest) (*projectEntity, error) {
	projects, err := r.list(ctx, query)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	switch len(projects) {
	case 0:
		return nil, errorutil.NotFound("cannot find project")
	case 1:
		return projects[0], nil
	default:
		return nil, errorutil.Internal("more than one project exists")
	}
}

func (r *projectRepository) list(ctx context.Context, query *apiv2.ProjectServiceListRequest) ([]*projectEntity, error) {
	resp, err := r.s.mdc.Project().Find(ctx, &mdcv1.ProjectFindRequest{
		Id:       query.Id,
		Name:     query.Name,
		TenantId: query.Tenant,
	})
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	ps := make([]*projectEntity, 0, len(resp.Projects))
	for _, p := range resp.Projects {
		ps = append(ps, &projectEntity{Project: p})
	}

	return ps, nil
}

func (r *projectRepository) convertToInternal(ctx context.Context, p *apiv2.Project) (*projectEntity, error) {
	var labels []string
	if p.Meta != nil && p.Meta.Labels != nil && len(p.Meta.Labels.Labels) > 0 {
		labels = tag.TagMap(p.Meta.Labels.Labels).Slice()
	}

	meta := &mdcv1.Meta{
		Id:          p.Uuid,
		CreatedTime: p.Meta.CreatedAt,
		UpdatedTime: p.Meta.UpdatedAt,
		Labels:      labels,
	}

	if p.AvatarUrl != nil {
		meta.Annotations["avatarUrl"] = *p.AvatarUrl
	}

	return &projectEntity{
		Project: &mdcv1.Project{
			Meta:        meta,
			Name:        p.Name,
			Description: p.Description,
			TenantId:    p.Tenant,
		},
	}, nil
}

func (r *projectRepository) convertToProto(ctx context.Context, p *projectEntity) (*apiv2.Project, error) {
	if p.Meta == nil {
		return nil, errorutil.Internal("project meta is nil")
	}

	var labels *apiv2.Labels
	if p.Meta != nil && p.Meta.Labels != nil && len(p.Meta.Labels) > 0 {
		labels = &apiv2.Labels{
			Labels: tag.NewTagMap(p.Meta.Labels),
		}
	}

	return &apiv2.Project{
		Uuid:        p.Meta.Id,
		Name:        p.Name,
		Description: p.Description,
		Tenant:      p.TenantId,
		Meta: &apiv2.Meta{
			CreatedAt: p.Meta.CreatedTime,
			UpdatedAt: p.Meta.UpdatedTime,
			Labels:    labels,
		},
		AvatarUrl: pointer.PointerOrNil(p.Meta.Annotations[avatarURLAnnotation]),
	}, nil

}

func projectRoleFromMap(annotations map[string]string) apiv2.ProjectRole {
	if annotations == nil {
		return apiv2.ProjectRole_PROJECT_ROLE_UNSPECIFIED
	}

	var (
		annotation  = annotations[ProjectRoleAnnotation]
		projectRole = apiv2.ProjectRole(apiv2.ProjectRole_value[annotation])
	)

	return projectRole
}

type ProjectsAndTenants struct {
	Projects      []*apiv2.Project
	Tenants       []*apiv2.Tenant
	DefaultTenant *apiv2.Tenant
	ProjectRoles  map[string]apiv2.ProjectRole
	TenantRoles   map[string]apiv2.TenantRole
}

// GetProjectsAndTenants returns all projects and tenants that the user is participating in
func (r *projectRepository) GetProjectsAndTenants(ctx context.Context, userId string) (*ProjectsAndTenants, error) {
	var (
		projectRoles = map[string]apiv2.ProjectRole{}
		projects     []*apiv2.Project

		tenantRoles   = map[string]apiv2.TenantRole{}
		tenants       []*apiv2.Tenant
		defaultTenant *apiv2.Tenant
	)

	projectResp, err := r.s.mdc.Tenant().FindParticipatingProjects(ctx, &mdcv1.FindParticipatingProjectsRequest{TenantId: userId, IncludeInherited: pointer.Pointer(true)})
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	tenantResp, err := r.s.mdc.Tenant().FindParticipatingTenants(ctx, &mdcv1.FindParticipatingTenantsRequest{TenantId: userId, IncludeInherited: pointer.Pointer(true)})
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	for _, projectWithAnnotations := range projectResp.Projects {
		p := projectWithAnnotations.Project

		apip, err := r.convertToProto(ctx, &projectEntity{Project: p})
		if err != nil {
			return nil, errorutil.Internal("unable to convert project %w", err)
		}

		projects = append(projects, apip)

		var (
			projectRole = projectRoleFromMap(projectWithAnnotations.ProjectAnnotations)
			tenantRole  = TenantRoleFromMap(projectWithAnnotations.TenantAnnotations)
		)

		switch {
		case projectRole == apiv2.ProjectRole_PROJECT_ROLE_OWNER, tenantRole == apiv2.TenantRole_TENANT_ROLE_OWNER:
			projectRole = apiv2.ProjectRole_PROJECT_ROLE_OWNER
		case projectRole == apiv2.ProjectRole_PROJECT_ROLE_EDITOR, tenantRole == apiv2.TenantRole_TENANT_ROLE_EDITOR:
			projectRole = apiv2.ProjectRole_PROJECT_ROLE_EDITOR
		case projectRole == apiv2.ProjectRole_PROJECT_ROLE_VIEWER, tenantRole == apiv2.TenantRole_TENANT_ROLE_VIEWER:
			projectRole = apiv2.ProjectRole_PROJECT_ROLE_VIEWER
		case tenantRole == apiv2.TenantRole_TENANT_ROLE_GUEST:
			// user has no access to this project, ignore
			continue
		default:
			// no roles associated with either tenant or project
			continue
		}

		projectRoles[p.Meta.GetId()] = projectRole
	}

	for _, tenantWithAnnotations := range tenantResp.Tenants {
		t := tenantWithAnnotations.Tenant

		apit, err := r.s.Tenant().ConvertToProto(ctx, &tenantEntity{Tenant: t})
		if err != nil {
			return nil, err
		}

		if t.Meta.Id == userId {
			defaultTenant = apit
		}

		tenants = append(tenants, apit)

		var (
			projectRole = projectRoleFromMap(tenantWithAnnotations.ProjectAnnotations)
			tenantRole  = TenantRoleFromMap(tenantWithAnnotations.TenantAnnotations)
		)

		if tenantRole == apiv2.TenantRole_TENANT_ROLE_UNSPECIFIED && projectRole > 0 {
			tenantRole = apiv2.TenantRole_TENANT_ROLE_GUEST
		}

		tenantRoles[t.Meta.GetId()] = tenantRole
	}

	if defaultTenant == nil {
		return nil, errorutil.Internal("unable to find a default tenant for user: %s", userId)
	}

	return &ProjectsAndTenants{
		Tenants:       tenants,
		Projects:      projects,
		DefaultTenant: defaultTenant,
		ProjectRoles:  projectRoles,
		TenantRoles:   tenantRoles,
	}, nil
}

func (r *projectRepository) EnsureProviderProject(ctx context.Context, providerTenantID string) error {
	ensureMembership := func(projectId string) error {
		_, err := r.s.Project(projectId).AdditionalMethods().Member().Get(ctx, providerTenantID)
		if err == nil {
			return nil
		}
		if !errorutil.IsNotFound(err) {
			return err
		}

		_, err = r.s.mdc.ProjectMember().Create(ctx, &mdcv1.ProjectMemberCreateRequest{
			ProjectMember: &mdcv1.ProjectMember{
				Meta: &mdcv1.Meta{
					Annotations: map[string]string{
						ProjectRoleAnnotation: apiv2.ProjectRole_PROJECT_ROLE_OWNER.String(),
					},
				},
				ProjectId: projectId,
				TenantId:  providerTenantID,
			},
		})

		return err
	}

	resp, err := r.s.mdc.Project().Find(ctx, &mdcv1.ProjectFindRequest{
		TenantId: &providerTenantID,
	})
	if err != nil {
		return errorutil.Convert(fmt.Errorf("unable to find project %q: %w", providerTenantID, err))
	}

	if len(resp.Projects) > 0 {
		return ensureMembership(resp.Projects[0].Meta.Id)
	}

	project, err := r.s.UnscopedProject().AdditionalMethods().CreateWithID(ctx, &apiv2.ProjectServiceCreateRequest{
		Name:        "Default Project",
		Description: "Default project of " + providerTenantID,
		Login:       providerTenantID,
	}, providerTenantID)
	if err != nil {
		return errorutil.Convert(fmt.Errorf("unable to create project: %w", err))
	}

	return ensureMembership(project.Meta.Id)
}
