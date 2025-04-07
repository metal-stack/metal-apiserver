package repository

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	mdcv1 "github.com/metal-stack/masterdata-api/api/v1"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	putil "github.com/metal-stack/metal-apiserver/pkg/project"
	tutil "github.com/metal-stack/metal-apiserver/pkg/tenant"
	"github.com/metal-stack/metal-lib/pkg/pointer"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// FIXME completely untested and incomplete

const (
	ProjectRoleAnnotation = "metal-stack.io/project-role"
	avatarURLAnnotation   = "avatarUrl"
)

type projectRepository struct {
	r     *Store
	scope *ProjectScope
}

func (r *projectRepository) ValidateCreate(ctx context.Context, req *apiv2.ProjectServiceCreateRequest) (*Validated[*apiv2.ProjectServiceCreateRequest], error) {
	return &Validated[*apiv2.ProjectServiceCreateRequest]{
		message: req,
	}, nil
}

func (r *projectRepository) ValidateUpdate(ctx context.Context, req *apiv2.ProjectServiceUpdateRequest) (*Validated[*apiv2.ProjectServiceUpdateRequest], error) {
	return &Validated[*apiv2.ProjectServiceUpdateRequest]{
		message: req,
	}, nil
}

func (r *projectRepository) ValidateDelete(ctx context.Context, req *mdcv1.Project) (*Validated[*mdcv1.Project], error) {
	return &Validated[*mdcv1.Project]{
		message: req,
	}, nil
}

func (r *projectRepository) Get(ctx context.Context, id string) (*mdcv1.Project, error) {
	resp, err := r.r.mdc.Project().Get(ctx, &mdcv1.ProjectGetRequest{Id: id})
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	if resp.Project == nil || resp.Project.Meta == nil {
		return nil, errorutil.NotFound("project %q has no meta", id)
	}

	err = r.MatchScope(resp.Project)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return resp.Project, nil
}

func (r *projectRepository) MatchScope(p *mdcv1.Project) error {
	if r.scope == nil {
		return nil
	}

	if r.scope.projectID == p.Meta.Id {
		return nil
	}

	return errorutil.NotFound("project:%s not found", p.Meta.Id)
}

func (r *projectRepository) Create(ctx context.Context, e *Validated[*apiv2.ProjectServiceCreateRequest]) (*mdcv1.Project, error) {
	return r.CreateWithID(ctx, e, "")
}

func (r *projectRepository) CreateWithID(ctx context.Context, e *Validated[*apiv2.ProjectServiceCreateRequest], id string) (*mdcv1.Project, error) {
	ann := map[string]string{}

	if e.message.AvatarUrl != nil {
		ann[putil.AvatarURLAnnotation] = *e.message.AvatarUrl
	}

	project := &mdcv1.Project{
		Meta: &mdcv1.Meta{
			Annotations: ann,
			Id:          id,
		},
		Name:        e.message.Name,
		Description: e.message.Description,
		TenantId:    e.message.Login,
	}

	resp, err := r.r.mdc.Project().Create(ctx, &mdcv1.ProjectCreateRequest{Project: project})
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return resp.Project, nil
}

func (r *projectRepository) Update(ctx context.Context, e *Validated[*apiv2.ProjectServiceUpdateRequest]) (*mdcv1.Project, error) {
	panic("unimplemented")
}

func (r *projectRepository) Delete(ctx context.Context, e *Validated[*mdcv1.Project]) (*mdcv1.Project, error) {
	panic("unimplemented")
}

func (r *projectRepository) Find(ctx context.Context, query *apiv2.ProjectServiceListRequest) (*mdcv1.Project, error) {
	panic("unimplemented")
}

func (r *projectRepository) List(ctx context.Context, query *apiv2.ProjectServiceListRequest) ([]*mdcv1.Project, error) {
	panic("unimplemented")
}

func (t *projectRepository) Member() ProjectMember {
	return &projectMemberRepository{
		r:     t.r,
		scope: t.scope,
	}
}

func (r *projectRepository) ConvertToInternal(p *apiv2.Project) (*mdcv1.Project, error) {
	meta := &mdcv1.Meta{
		Id:          p.Uuid,
		CreatedTime: p.Meta.CreatedAt,
		UpdatedTime: p.Meta.UpdatedAt,
	}
	if p.AvatarUrl != nil {
		meta.Annotations["avatarUrl"] = *p.AvatarUrl
	}
	return &mdcv1.Project{
		Meta:        meta,
		Name:        p.Name,
		Description: p.Description,
		TenantId:    p.Tenant,
	}, nil
}

func (r *projectRepository) ConvertToProto(p *mdcv1.Project) (*apiv2.Project, error) {
	if p.Meta == nil {
		return nil, errors.New("project meta is nil")
	}
	avatarUrl := p.Meta.Annotations[avatarURLAnnotation]

	return &apiv2.Project{
		Uuid:        p.Meta.Id,
		Name:        p.Name,
		Description: p.Description,
		Tenant:      p.TenantId,
		Meta: &apiv2.Meta{
			CreatedAt: p.Meta.CreatedTime,
			UpdatedAt: p.Meta.UpdatedTime,
		},
		AvatarUrl: &avatarUrl,
	}, nil

}

func ProjectRoleFromMap(annotations map[string]string) apiv2.ProjectRole {
	if annotations == nil {
		return apiv2.ProjectRole_PROJECT_ROLE_UNSPECIFIED
	}

	var (
		annotation  = annotations[ProjectRoleAnnotation]
		projectRole = apiv2.ProjectRole(apiv2.ProjectRole_value[annotation])
	)

	return projectRole
}

func ToProject(p *mdcv1.Project) (*apiv2.Project, error) {
	if p.Meta == nil {
		return nil, fmt.Errorf("project meta is nil")
	}

	avatarUrl := p.Meta.Annotations[avatarURLAnnotation]

	return &apiv2.Project{
		Uuid:        p.Meta.Id,
		Name:        p.Name,
		Description: p.Description,
		Tenant:      p.TenantId,
		Meta: &apiv2.Meta{
			CreatedAt: p.Meta.CreatedTime,
			UpdatedAt: p.Meta.UpdatedTime,
		},
		AvatarUrl: &avatarUrl,
	}, nil
}

// func (r *projectRepository) GetMember(ctx context.Context, projectID, tenantID string) (*mdcv1.ProjectMember, *mdcv1.Project, error) {
// 	getResp, err := r.r.mdc.Project().Get(ctx, &mdcv1.ProjectGetRequest{
// 		Id: projectID,
// 	})
// 	if err != nil {
// 		return nil, nil, connect.NewError(connect.CodeInternal, fmt.Errorf("no project found with id %q: %w", projectID, err))
// 	}

// 	memberships, err := r.r.mdc.ProjectMember().Find(ctx, &mdcv1.ProjectMemberFindRequest{
// 		ProjectId: &projectID,
// 		TenantId:  &tenantID,
// 	})
// 	if err != nil {
// 		return nil, nil, connect.NewError(connect.CodeInternal, err)
// 	}

// 	switch len(memberships.ProjectMembers) {
// 	case 0:
// 		return nil, nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("tenant %s is not a member of project %s", tenantID, projectID))
// 	case 1:
// 		// fallthrough
// 	default:
// 		return nil, nil, connect.NewError(connect.CodeInternal, fmt.Errorf("found multiple membership associations for a member to a project"))
// 	}

// 	return memberships.GetProjectMembers()[0], getResp.Project, nil
// }

// type DefaultProjectRequirement bool

// const (
// 	DefaultProjectRequired    DefaultProjectRequirement = true
// 	DefaultProjectNotRequired DefaultProjectRequirement = false
// )

type ProjectsAndTenants struct {
	Projects       []*apiv2.Project
	DefaultProject *apiv2.Project
	Tenants        []*apiv2.Tenant
	DefaultTenant  *apiv2.Tenant
	ProjectRoles   map[string]apiv2.ProjectRole
	TenantRoles    map[string]apiv2.TenantRole
}

// GetProjectsAndTenants returns all projects and tenants that the user is participating in
func (r *projectRepository) GetProjectsAndTenants(ctx context.Context, userId string) (*ProjectsAndTenants, error) {
	var (
		projectRoles   = map[string]apiv2.ProjectRole{}
		projects       []*apiv2.Project
		defaultProject *apiv2.Project

		tenantRoles   = map[string]apiv2.TenantRole{}
		tenants       []*apiv2.Tenant
		defaultTenant *apiv2.Tenant
	)

	projectResp, err := r.r.mdc.Tenant().FindParticipatingProjects(ctx, &mdcv1.FindParticipatingProjectsRequest{TenantId: userId, IncludeInherited: pointer.Pointer(true)})
	if err != nil {
		return nil, err
	}

	tenantResp, err := r.r.mdc.Tenant().FindParticipatingTenants(ctx, &mdcv1.FindParticipatingTenantsRequest{TenantId: userId, IncludeInherited: pointer.Pointer(true)})
	if err != nil {
		return nil, err
	}

	for _, projectWithAnnotations := range projectResp.Projects {
		p := projectWithAnnotations.Project

		apip, err := ToProject(p)
		if err != nil {
			return nil, fmt.Errorf("unable to convert project %w", err)
		}

		projects = append(projects, apip)

		var (
			projectRole = ProjectRoleFromMap(projectWithAnnotations.ProjectAnnotations)
			tenantRole  = tutil.TenantRoleFromMap(projectWithAnnotations.TenantAnnotations)
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

		apit := tutil.ConvertFromTenant(t)

		if t.Meta.Id == userId {
			defaultTenant = apit
		}

		tenants = append(tenants, apit)

		var (
			projectRole = ProjectRoleFromMap(tenantWithAnnotations.ProjectAnnotations)
			tenantRole  = tutil.TenantRoleFromMap(tenantWithAnnotations.TenantAnnotations)
		)

		if tenantRole == apiv2.TenantRole_TENANT_ROLE_UNSPECIFIED && projectRole > 0 {
			tenantRole = apiv2.TenantRole_TENANT_ROLE_GUEST
		}

		tenantRoles[t.Meta.GetId()] = tenantRole
	}

	if defaultTenant == nil {
		return nil, fmt.Errorf("unable to find a default tenant for user: %s", userId)
	}

	return &ProjectsAndTenants{
		Tenants:        tenants,
		Projects:       projects,
		DefaultTenant:  defaultTenant,
		DefaultProject: defaultProject,
		ProjectRoles:   projectRoles,
		TenantRoles:    tenantRoles,
	}, nil
}

func (r *projectRepository) EnsureProviderProject(ctx context.Context, providerTenantID string) error {
	ensureMembership := func(projectId string) error {
		_, err := r.r.Project(projectId).Member().Get(ctx, providerTenantID)
		if err == nil {
			return nil
		}
		if connect.CodeOf(err) != connect.CodeNotFound {
			return err
		}

		_, err = r.r.mdc.ProjectMember().Create(ctx, &mdcv1.ProjectMemberCreateRequest{
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

	resp, err := r.r.mdc.Project().Find(ctx, &mdcv1.ProjectFindRequest{
		TenantId: wrapperspb.String(providerTenantID),
	})
	if err != nil {
		return fmt.Errorf("unable to get find project %q: %w", providerTenantID, err)
	}

	if len(resp.Projects) > 0 {
		return ensureMembership(resp.Projects[0].Meta.Id)
	}

	project, err := r.r.mdc.Project().Create(ctx, &mdcv1.ProjectCreateRequest{
		Project: &mdcv1.Project{
			Name:        "Default Project",
			TenantId:    providerTenantID,
			Description: "Default project of " + providerTenantID,
		},
	})
	if err != nil {
		return fmt.Errorf("unable to create project: %w", err)
	}

	return ensureMembership(project.Project.Meta.Id)
}
