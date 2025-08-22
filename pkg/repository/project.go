package repository

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	mdcv1 "github.com/metal-stack/masterdata-api/api/v1"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	"github.com/metal-stack/metal-lib/pkg/pointer"
)

// FIXME completely untested and incomplete

const (
	ProjectRoleAnnotation = "metal-stack.io/project-role"
	avatarURLAnnotation   = "avatarUrl"
)

type projectRepository struct {
	s     *Store
	scope *ProjectScope
}

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

	return &store[*projectMemberRepository, *mdcv1.ProjectMember, *apiv2.ProjectMember, *ProjectMemberCreateRequest, *ProjectMemberUpdateRequest, *ProjectMemberQuery]{
		typed: repository,
	}
}

func (r *projectRepository) validateCreate(ctx context.Context, req *apiv2.ProjectServiceCreateRequest) error {
	return nil
}

func (r *projectRepository) validateUpdate(ctx context.Context, req *apiv2.ProjectServiceUpdateRequest, _ *mdcv1.Project) error {
	return nil
}

func (r *projectRepository) validateDelete(ctx context.Context, req *mdcv1.Project) error {
	// FIXME check for machines

	networks, err := r.s.Network(req.Meta.Id).List(ctx, &apiv2.NetworkQuery{Project: &req.Meta.Id})
	if err != nil {
		return err
	}

	if len(networks) > 0 {
		return connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("there are still networks associated with this project, you need to delete them first"))
	}

	ips, err := r.s.IP(req.Meta.Id).List(ctx, &apiv2.IPQuery{Project: &req.Meta.Id})
	if err != nil {
		return err
	}

	if len(ips) > 0 {
		return connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("there are still ips associated with this project, you need to delete them first"))
	}

	ms, err := r.s.Machine(req.Meta.Id).List(ctx, &apiv2.MachineQuery{
		Allocation: &apiv2.MachineAllocationQuery{
			Project: &req.Meta.Id,
		},
	})
	if err != nil {
		return err
	}

	if len(ms) > 0 {
		return connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("there are still machines associated with this project, you need to delete them first"))
	}

	// TODO: ensure project tokens are revoked / cleaned up

	return nil
}

func (r *projectRepository) get(ctx context.Context, id string) (*mdcv1.Project, error) {
	resp, err := r.s.mdc.Project().Get(ctx, &mdcv1.ProjectGetRequest{Id: id})
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	if resp.Project == nil || resp.Project.Meta == nil {
		return nil, errorutil.NotFound("project %q has no meta", id)
	}

	return resp.Project, nil
}

func (r *projectRepository) matchScope(p *mdcv1.Project) bool {
	if r.scope == nil {
		return true
	}

	if r.scope.projectID == p.Meta.Id {
		return true
	}
	return false
}

func (r *projectRepository) create(ctx context.Context, e *apiv2.ProjectServiceCreateRequest) (*mdcv1.Project, error) {
	return r.CreateWithID(ctx, e, "")
}

func (r *projectRepository) CreateWithID(ctx context.Context, e *apiv2.ProjectServiceCreateRequest, id string) (*mdcv1.Project, error) {
	ann := map[string]string{}

	if e.AvatarUrl != nil {
		ann[avatarURLAnnotation] = *e.AvatarUrl
	}

	project := &mdcv1.Project{
		Meta: &mdcv1.Meta{
			Annotations: ann,
			Id:          id,
		},
		Name:        e.Name,
		Description: e.Description,
		TenantId:    e.Login,
	}

	resp, err := r.s.mdc.Project().Create(ctx, &mdcv1.ProjectCreateRequest{Project: project})
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return resp.Project, nil
}

func (r *projectRepository) update(ctx context.Context, e *mdcv1.Project, msg *apiv2.ProjectServiceUpdateRequest) (*mdcv1.Project, error) {
	panic("unimplemented")
}

func (r *projectRepository) delete(ctx context.Context, e *mdcv1.Project) error {
	panic("unimplemented")
}

func (r *projectRepository) find(ctx context.Context, query *apiv2.ProjectServiceListRequest) (*mdcv1.Project, error) {
	panic("unimplemented")
}

func (r *projectRepository) list(ctx context.Context, query *apiv2.ProjectServiceListRequest) ([]*mdcv1.Project, error) {
	panic("unimplemented")
}

func (r *projectRepository) convertToInternal(p *apiv2.Project) (*mdcv1.Project, error) {

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

func (r *projectRepository) convertToProto(p *mdcv1.Project) (*apiv2.Project, error) {
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
		return nil, err
	}

	tenantResp, err := r.s.mdc.Tenant().FindParticipatingTenants(ctx, &mdcv1.FindParticipatingTenantsRequest{TenantId: userId, IncludeInherited: pointer.Pointer(true)})
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

		apit, err := r.s.Tenant().ConvertToProto(t)
		if err != nil {
			return nil, err
		}

		if t.Meta.Id == userId {
			defaultTenant = apit
		}

		tenants = append(tenants, apit)

		var (
			projectRole = ProjectRoleFromMap(tenantWithAnnotations.ProjectAnnotations)
			tenantRole  = TenantRoleFromMap(tenantWithAnnotations.TenantAnnotations)
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
		if connect.CodeOf(err) != connect.CodeNotFound {
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
		return fmt.Errorf("unable to get find project %q: %w", providerTenantID, err)
	}

	if len(resp.Projects) > 0 {
		return ensureMembership(resp.Projects[0].Meta.Id)
	}

	project, err := r.s.UnscopedProject().AdditionalMethods().CreateWithID(ctx, &apiv2.ProjectServiceCreateRequest{
		Name:        "Default Project",
		Description: "Default project of " + providerTenantID,
	}, providerTenantID)
	if err != nil {
		return fmt.Errorf("unable to create project: %w", err)
	}

	return ensureMembership(project.Meta.Id)
}
