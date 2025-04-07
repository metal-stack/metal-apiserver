package project

import (
	"fmt"

	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	mdcv1 "github.com/metal-stack/masterdata-api/api/v1"
)

const (
	DefaultProjectAnnotation = "metal-stack.io/default-project"
	ProjectRoleAnnotation    = "metal-stack.io/project-role"
	AvatarURLAnnotation      = "avatarUrl"
)

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
	avatarUrl := p.Meta.Annotations[AvatarURLAnnotation]

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

// func IsDefaultProject(p *mdcv1.Project) bool {
// 	value, ok := p.Meta.Annotations[DefaultProjectAnnotation]
// 	if !ok {
// 		return false
// 	}

// 	res, err := strconv.ParseBool(value)
// 	if err != nil {
// 		return false
// 	}

// 	return res
// }

// func GetProjectMember(ctx context.Context, c mdc.Client, projectID, tenantID string) (*mdcv1.ProjectMember, *mdcv1.Project, error) {
// 	getResp, err := c.Project().Get(ctx, &mdcv1.ProjectGetRequest{
// 		Id: projectID,
// 	})
// 	if err != nil {
// 		return nil, nil, connect.NewError(connect.CodeInternal, fmt.Errorf("no project found with id %q: %w", projectID, err))
// 	}

// 	memberships, err := c.ProjectMember().Find(ctx, &mdcv1.ProjectMemberFindRequest{
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

// func GetProject(ctx context.Context, c mdc.Client, projectID string) (*mdcv1.Project, error) {
// 	getResp, err := c.Project().Get(ctx, &mdcv1.ProjectGetRequest{
// 		Id: projectID,
// 	})
// 	if err != nil {
// 		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("no Tenant found with id %q: %w", projectID, err))
// 	}

// 	return getResp.Project, nil
// }

// type DefaultProjectRequirement bool

// const (
// 	DefaultProjectRequired    DefaultProjectRequirement = true
// 	DefaultProjectNotRequired DefaultProjectRequirement = false
// )

// type ProjectsAndTenants struct {
// 	Projects       []*apiv2.Project
// 	DefaultProject *apiv2.Project
// 	Tenants        []*apiv2.Tenant
// 	DefaultTenant  *apiv2.Tenant
// 	ProjectRoles   map[string]apiv2.ProjectRole
// 	TenantRoles    map[string]apiv2.TenantRole
// }

// // GetProjectsAndTenants returns all projects and tenants that the user is participating in
// func GetProjectsAndTenants(ctx context.Context, masterClient mdc.Client, userId string, defaultIsRequired DefaultProjectRequirement) (*ProjectsAndTenants, error) {
// 	var (
// 		projectRoles   = map[string]apiv2.ProjectRole{}
// 		projects       []*apiv2.Project
// 		defaultProject *apiv2.Project

// 		tenantRoles   = map[string]apiv2.TenantRole{}
// 		tenants       []*apiv2.Tenant
// 		defaultTenant *apiv2.Tenant
// 	)

// 	projectResp, err := masterClient.Tenant().FindParticipatingProjects(ctx, &mdcv1.FindParticipatingProjectsRequest{TenantId: userId, IncludeInherited: pointer.Pointer(true)})
// 	if err != nil {
// 		return nil, err
// 	}

// 	tenantResp, err := masterClient.Tenant().FindParticipatingTenants(ctx, &mdcv1.FindParticipatingTenantsRequest{TenantId: userId, IncludeInherited: pointer.Pointer(true)})
// 	if err != nil {
// 		return nil, err
// 	}

// 	for _, projectWithAnnotations := range projectResp.Projects {
// 		p := projectWithAnnotations.Project

// 		apip, err := ToProject(p)
// 		if err != nil {
// 			return nil, fmt.Errorf("unable to convert project %w", err)
// 		}

// 		if p.TenantId == userId && IsDefaultProject(p) {
// 			defaultProject = apip
// 		}

// 		projects = append(projects, apip)

// 		var (
// 			projectRole = ProjectRoleFromMap(projectWithAnnotations.ProjectAnnotations)
// 			tenantRole  = tutil.TenantRoleFromMap(projectWithAnnotations.TenantAnnotations)
// 		)

// 		switch {
// 		case projectRole == apiv2.ProjectRole_PROJECT_ROLE_OWNER, tenantRole == apiv2.TenantRole_TENANT_ROLE_OWNER:
// 			projectRole = apiv2.ProjectRole_PROJECT_ROLE_OWNER
// 		case projectRole == apiv2.ProjectRole_PROJECT_ROLE_EDITOR, tenantRole == apiv2.TenantRole_TENANT_ROLE_EDITOR:
// 			projectRole = apiv2.ProjectRole_PROJECT_ROLE_EDITOR
// 		case projectRole == apiv2.ProjectRole_PROJECT_ROLE_VIEWER, tenantRole == apiv2.TenantRole_TENANT_ROLE_VIEWER:
// 			projectRole = apiv2.ProjectRole_PROJECT_ROLE_VIEWER
// 		case tenantRole == apiv2.TenantRole_TENANT_ROLE_GUEST:
// 			// user has no access to this project, ignore
// 			continue
// 		default:
// 			// no roles associated with either tenant or project
// 			continue
// 		}

// 		projectRoles[p.Meta.GetId()] = projectRole
// 	}

// 	for _, tenantWithAnnotations := range tenantResp.Tenants {
// 		t := tenantWithAnnotations.Tenant

// 		apit := tutil.ConvertFromTenant(t)

// 		if t.Meta.Id == userId {
// 			defaultTenant = apit
// 		}

// 		tenants = append(tenants, apit)

// 		var (
// 			projectRole = ProjectRoleFromMap(tenantWithAnnotations.ProjectAnnotations)
// 			tenantRole  = tutil.TenantRoleFromMap(tenantWithAnnotations.TenantAnnotations)
// 		)

// 		if tenantRole == apiv2.TenantRole_TENANT_ROLE_UNSPECIFIED && projectRole > 0 {
// 			tenantRole = apiv2.TenantRole_TENANT_ROLE_GUEST
// 		}

// 		tenantRoles[t.Meta.GetId()] = tenantRole
// 	}

// 	if defaultIsRequired && defaultProject == nil {
// 		return nil, fmt.Errorf("unable to find a default project for user: %s", userId)
// 	}
// 	if defaultTenant == nil {
// 		return nil, fmt.Errorf("unable to find a default tenant for user: %s", userId)
// 	}

// 	return &ProjectsAndTenants{
// 		Tenants:        tenants,
// 		Projects:       projects,
// 		DefaultTenant:  defaultTenant,
// 		DefaultProject: defaultProject,
// 		ProjectRoles:   projectRoles,
// 		TenantRoles:    tenantRoles,
// 	}, nil
// }

// func EnsureProviderProject(ctx context.Context, masterClient mdc.Client, providerTenantID string) error {
// 	ensureMembership := func(projectId string) error {
// 		_, _, err := GetProjectMember(ctx, masterClient, projectId, providerTenantID)
// 		if err == nil {
// 			return nil
// 		}
// 		if connect.CodeOf(err) != connect.CodeNotFound {
// 			return err
// 		}

// 		_, err = masterClient.ProjectMember().Create(ctx, &mdcv1.ProjectMemberCreateRequest{
// 			ProjectMember: &mdcv1.ProjectMember{
// 				Meta: &mdcv1.Meta{
// 					Annotations: map[string]string{
// 						ProjectRoleAnnotation: apiv2.ProjectRole_PROJECT_ROLE_OWNER.String(),
// 					},
// 				},
// 				ProjectId: projectId,
// 				TenantId:  providerTenantID,
// 			},
// 		})

// 		return err
// 	}

// 	resp, err := masterClient.Project().Find(ctx, &mdcv1.ProjectFindRequest{
// 		TenantId: wrapperspb.String(providerTenantID),
// 		Annotations: map[string]string{
// 			DefaultProjectAnnotation: strconv.FormatBool(true),
// 		},
// 	})
// 	if err != nil {
// 		return fmt.Errorf("unable to get find project %q: %w", providerTenantID, err)
// 	}

// 	if len(resp.Projects) > 0 {
// 		return ensureMembership(resp.Projects[0].Meta.Id)
// 	}

// 	project, err := masterClient.Project().Create(ctx, &mdcv1.ProjectCreateRequest{
// 		Project: &mdcv1.Project{
// 			Meta: &mdcv1.Meta{
// 				Annotations: map[string]string{
// 					DefaultProjectAnnotation: strconv.FormatBool(true),
// 				},
// 			},
// 			Name:        "Default Project",
// 			TenantId:    providerTenantID,
// 			Description: "Default project of " + providerTenantID,
// 		},
// 	})
// 	if err != nil {
// 		return fmt.Errorf("unable to create project: %w", err)
// 	}

// 	return ensureMembership(project.Project.Meta.Id)
// }
