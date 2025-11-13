package project

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sort"
	"time"

	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/api/go/metalstack/api/v2/apiv2connect"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	"github.com/metal-stack/metal-apiserver/pkg/invite"
	"github.com/metal-stack/metal-apiserver/pkg/repository"
	"github.com/metal-stack/metal-apiserver/pkg/token"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type Config struct {
	Log         *slog.Logger
	Repo        *repository.Store
	InviteStore invite.ProjectInviteStore
	TokenStore  token.TokenStore
}

type projectServiceServer struct {
	log         *slog.Logger
	repo        *repository.Store
	inviteStore invite.ProjectInviteStore
	tokenStore  token.TokenStore
}

func New(c Config) apiv2connect.ProjectServiceHandler {
	return &projectServiceServer{
		log:         c.Log.WithGroup("projectService"),
		inviteStore: c.InviteStore,
		tokenStore:  c.TokenStore,
		repo:        c.Repo,
	}
}

func (p *projectServiceServer) Get(ctx context.Context, req *apiv2.ProjectServiceGetRequest) (*apiv2.ProjectServiceGetResponse, error) {
	var (
		t, ok                   = token.TokenFromContext(ctx)
		includeInheritedMembers bool
	)
	if !ok || t == nil {
		return nil, errorutil.Unauthenticated("no token found in request")
	}

	project, err := p.repo.Project(req.Project).Get(ctx, req.Project)
	if err != nil {
		return nil, err
	}

	// TODO: maybe we should shadow some fields of the project when a tenant guest accesses this endpoint
	// e.g. project annotations should not be completely visible?

	projectMembers, err := p.repo.Project(req.Project).AdditionalMethods().Member().List(ctx, &repository.ProjectMemberQuery{})
	if err != nil {
		return nil, err
	}

	memberMap := map[string]*apiv2.ProjectMember{}

	for _, pm := range projectMembers {
		memberMap[pm.Id] = pm
	}

	role := t.TenantRoles[project.Tenant]

	if role != apiv2.TenantRole_TENANT_ROLE_UNSPECIFIED && role < apiv2.TenantRole_TENANT_ROLE_GUEST {
		includeInheritedMembers = true
	}
	if t.AdminRole != nil {
		includeInheritedMembers = true
	}

	if includeInheritedMembers {
		// we are at least viewer for this tenant, we should also be able to see all indirect members of this project

		tenantMembers, err := p.repo.Tenant().AdditionalMethods().ListTenantMembers(ctx, project.Tenant, includeInheritedMembers)
		if err != nil {
			return nil, fmt.Errorf("unable to list project members: %w", err)
		}

		for _, tm := range tenantMembers {
			var projectRole apiv2.ProjectRole
			switch repository.TenantRoleFromMap(tm.TenantAnnotations) {
			case apiv2.TenantRole_TENANT_ROLE_OWNER:
				projectRole = apiv2.ProjectRole_PROJECT_ROLE_OWNER
			case apiv2.TenantRole_TENANT_ROLE_EDITOR:
				projectRole = apiv2.ProjectRole_PROJECT_ROLE_EDITOR
			case apiv2.TenantRole_TENANT_ROLE_VIEWER:
				projectRole = apiv2.ProjectRole_PROJECT_ROLE_VIEWER
			case apiv2.TenantRole_TENANT_ROLE_GUEST, apiv2.TenantRole_TENANT_ROLE_UNSPECIFIED:
				continue
			default:
				continue
			}

			member, ok := memberMap[tm.Tenant.Login]
			if !ok {
				memberMap[tm.Tenant.Login] = &apiv2.ProjectMember{
					Id:                  tm.Tenant.Login,
					Role:                projectRole,
					CreatedAt:           tm.Tenant.Meta.CreatedAt,
					InheritedMembership: true,
				}

				continue
			}

			if member.Role > projectRole {
				member.Role = projectRole
				memberMap[tm.Tenant.Login] = member
			}
		}
	}

	var memberResult []*apiv2.ProjectMember
	for _, m := range memberMap {
		memberResult = append(memberResult, m)
	}

	sort.Slice(memberResult, func(i, j int) bool {
		return memberResult[i].Id < memberResult[j].Id
	})

	return &apiv2.ProjectServiceGetResponse{Project: project, ProjectMembers: memberResult}, nil
}

func (p *projectServiceServer) List(ctx context.Context, req *apiv2.ProjectServiceListRequest) (*apiv2.ProjectServiceListResponse, error) {
	token, ok := token.TokenFromContext(ctx)
	if !ok || token == nil {
		return nil, errorutil.Unauthenticated("no token found in request")
	}

	var (
		result []*apiv2.Project
	)

	projectsAndTenants, err := p.repo.UnscopedProject().AdditionalMethods().GetProjectsAndTenants(ctx, token.User)
	if err != nil {
		return nil, errorutil.Internal("error retrieving projects from backend: %w", err)
	}

	for _, project := range projectsAndTenants.Projects {
		// TODO: maybe we can pass the filter and not filter here

		if req.Id != nil && project.Uuid != *req.Id {
			continue
		}
		if req.Name != nil && project.Name != *req.Name {
			continue
		}
		if req.Tenant != nil && project.Tenant != *req.Tenant {
			continue
		}

		result = append(result, project)
	}

	sort.SliceStable(result, func(i, j int) bool {
		return result[i].Uuid < result[j].Uuid
	})

	return &apiv2.ProjectServiceListResponse{Projects: result}, nil
}

func (p *projectServiceServer) Create(ctx context.Context, req *apiv2.ProjectServiceCreateRequest) (*apiv2.ProjectServiceCreateResponse, error) {
	var (
		t, ok = token.TokenFromContext(ctx)
	)

	if !ok || t == nil {
		return nil, errorutil.Unauthenticated("no token found in request")
	}

	project, err := p.repo.UnscopedProject().Create(ctx, req)
	if err != nil {
		return nil, err
	}

	_, err = p.repo.Project(project.Uuid).AdditionalMethods().Member().Create(ctx, &repository.ProjectMemberCreateRequest{
		TenantId: req.Login,
		Role:     apiv2.ProjectRole_PROJECT_ROLE_OWNER,
	})
	if err != nil {
		return nil, err
	}

	return &apiv2.ProjectServiceCreateResponse{Project: project}, nil
}

func (p *projectServiceServer) Delete(ctx context.Context, req *apiv2.ProjectServiceDeleteRequest) (*apiv2.ProjectServiceDeleteResponse, error) {
	var (
		t, ok = token.TokenFromContext(ctx)
	)

	if !ok || t == nil {
		return nil, errorutil.Unauthenticated("no token found in request")
	}

	project, err := p.repo.Project(req.Project).Delete(ctx, req.Project)
	if err != nil {
		return nil, err
	}

	return &apiv2.ProjectServiceDeleteResponse{Project: project}, nil
}

func (p *projectServiceServer) Update(ctx context.Context, req *apiv2.ProjectServiceUpdateRequest) (*apiv2.ProjectServiceUpdateResponse, error) {
	project, err := p.repo.Project(req.Project).Update(ctx, req.Project, req)
	if err != nil {
		return nil, err
	}

	return &apiv2.ProjectServiceUpdateResponse{Project: project}, nil
}

func (p *projectServiceServer) RemoveMember(ctx context.Context, req *apiv2.ProjectServiceRemoveMemberRequest) (*apiv2.ProjectServiceRemoveMemberResponse, error) {
	var (
		t, ok = token.TokenFromContext(ctx)
	)

	if !ok || t == nil {
		return nil, errorutil.Unauthenticated("no token found in request")
	}

	_, err := p.repo.Project(req.Project).AdditionalMethods().Member().Delete(ctx, req.Member)
	if err != nil {
		return nil, err
	}

	return &apiv2.ProjectServiceRemoveMemberResponse{}, nil
}

func (p *projectServiceServer) Leave(ctx context.Context, req *apiv2.ProjectServiceLeaveRequest) (*apiv2.ProjectServiceLeaveResponse, error) {
	var (
		t, ok = token.TokenFromContext(ctx)
	)

	if !ok || t == nil {
		return nil, errorutil.Unauthenticated("no token found in request")
	}

	_, err := p.repo.Project(req.Project).AdditionalMethods().Member().Delete(ctx, t.User)
	if err != nil {
		return nil, err
	}

	return &apiv2.ProjectServiceLeaveResponse{}, nil
}

func (p *projectServiceServer) UpdateMember(ctx context.Context, req *apiv2.ProjectServiceUpdateMemberRequest) (*apiv2.ProjectServiceUpdateMemberResponse, error) {
	pm, err := p.repo.Project(req.Project).AdditionalMethods().Member().Update(ctx, req.Member, &repository.ProjectMemberUpdateRequest{
		Role: req.Role,
	})

	if errorutil.IsNotFound(err) {
		// if there does not exist a direct membership for this user but the user belongs to the tenant already, we create a direct membership for the project
		projectGuest, projecterr := p.repo.Project(req.Project).Get(ctx, req.Project)
		if projecterr != nil {
			return nil, err
		}

		partiTenants, err := p.repo.Tenant().AdditionalMethods().FindParticipatingTenants(ctx, req.Member, true)
		if err != nil {
			return nil, err
		}

		if !slices.ContainsFunc(partiTenants, func(t *repository.TenantWithMembershipAnnotations) bool {
			return t.Tenant.Login == projectGuest.Tenant
		}) {
			return nil, errorutil.InvalidArgument("tenant is not part of the project's tenants")
		}

		// Create new project membership since the user is part of the tenant
		membership, err := p.createProjectMembership(ctx, req.Member, req.Project, req.Role)

		if err != nil {
			return nil, err
		}

		return &apiv2.ProjectServiceUpdateMemberResponse{
			ProjectMember: &apiv2.ProjectMember{
				Id:                  req.Member,
				Role:                req.Role,
				InheritedMembership: false,
				CreatedAt:           membership.CreatedAt,
			},
		}, nil
	}

	if err != nil {
		return nil, err
	}

	return &apiv2.ProjectServiceUpdateMemberResponse{ProjectMember: pm}, nil
}

func (p *projectServiceServer) createProjectMembership(ctx context.Context, tenantID, projectID string, role apiv2.ProjectRole) (*apiv2.ProjectMember, error) {
	if role == apiv2.ProjectRole_PROJECT_ROLE_UNSPECIFIED {
		role = apiv2.ProjectRole_PROJECT_ROLE_VIEWER
	}

	created, err := p.repo.Project(projectID).AdditionalMethods().Member().Create(ctx, &repository.ProjectMemberCreateRequest{
		TenantId: tenantID,
		Role:     role,
	})
	if err != nil {
		return nil, err
	}

	return created, nil
}

func (p *projectServiceServer) InviteGet(ctx context.Context, req *apiv2.ProjectServiceInviteGetRequest) (*apiv2.ProjectServiceInviteGetResponse, error) {
	inv, err := p.inviteStore.GetInvite(ctx, req.Secret)
	if err != nil {
		if errors.Is(err, invite.ErrInviteNotFound) {
			return nil, errorutil.NotFound("the given invitation does not exist anymore")
		}
		return nil, errorutil.NewInternal(err)
	}

	return &apiv2.ProjectServiceInviteGetResponse{Invite: inv}, nil
}

func (p *projectServiceServer) Invite(ctx context.Context, req *apiv2.ProjectServiceInviteRequest) (*apiv2.ProjectServiceInviteResponse, error) {
	project, err := p.repo.Project(req.Project).Get(ctx, req.Project)
	if err != nil {
		return nil, err
	}

	tenant, err := p.repo.Tenant().Get(ctx, project.Tenant)
	if err != nil {
		return nil, err
	}

	secret, err := invite.GenerateInviteSecret()
	if err != nil {
		return nil, errorutil.NewInternal(err)
	}

	var (
		expiresAt = time.Now().Add(7 * 24 * time.Hour)
	)

	if req.Role == apiv2.ProjectRole_PROJECT_ROLE_UNSPECIFIED {
		return nil, errorutil.InvalidArgument("project role must be specified")
	}

	invite := &apiv2.ProjectInvite{
		Secret:      secret,
		Project:     project.Uuid,
		Role:        req.Role,
		Joined:      false,
		ProjectName: project.Name,
		Tenant:      project.Tenant,
		TenantName:  tenant.Name,
		ExpiresAt:   timestamppb.New(expiresAt),
		JoinedAt:    nil,
	}

	p.log.Info("project invitation created", "invitation", invite)

	err = p.inviteStore.SetInvite(ctx, invite)
	if err != nil {
		return nil, errorutil.NewInternal(err)
	}

	return &apiv2.ProjectServiceInviteResponse{Invite: invite}, nil
}

func (p *projectServiceServer) InviteAccept(ctx context.Context, req *apiv2.ProjectServiceInviteAcceptRequest) (*apiv2.ProjectServiceInviteAcceptResponse, error) {
	var (
		t, ok = token.TokenFromContext(ctx)
	)

	if !ok || t == nil {
		return nil, errorutil.Unauthenticated("no token found in request")
	}

	inv, err := p.inviteStore.GetInvite(ctx, req.Secret)
	if err != nil {
		if errors.Is(err, invite.ErrInviteNotFound) {
			return nil, errorutil.NotFound("the given invitation does not exist anymore")
		}
		return nil, errorutil.NewInternal(err)
	}

	invitee, err := p.repo.Tenant().Get(ctx, t.User)
	if err != nil {
		return nil, err
	}

	project, err := p.repo.UnscopedProject().Get(ctx, inv.Project)
	if err != nil {
		return nil, errorutil.NotFound("no project: %q for invite not found %w", inv.Project, err)
	}

	if project.Tenant == invitee.Login {
		return nil, errorutil.InvalidArgument("an owner cannot accept invitations to own projects")
	}

	memberships, err := p.repo.Project(inv.Project).AdditionalMethods().Member().List(ctx, &repository.ProjectMemberQuery{
		TenantId: &invitee.Login,
	})
	if err != nil {
		return nil, err
	}

	if len(memberships) > 0 {
		return nil, errorutil.Conflict("%s is already member of project %s", invitee.Login, inv.Project)
	}

	err = p.inviteStore.DeleteInvite(ctx, inv)
	if err != nil {
		return nil, errorutil.NewInternal(err)
	}

	_, err = p.repo.Project(inv.Project).AdditionalMethods().Member().Create(ctx, &repository.ProjectMemberCreateRequest{
		Role:     inv.Role,
		TenantId: invitee.Login,
	})
	if err != nil {
		return nil, err
	}

	return &apiv2.ProjectServiceInviteAcceptResponse{Project: inv.Project, ProjectName: inv.ProjectName}, nil
}

func (p *projectServiceServer) InviteDelete(ctx context.Context, req *apiv2.ProjectServiceInviteDeleteRequest) (*apiv2.ProjectServiceInviteDeleteResponse, error) {
	err := p.inviteStore.DeleteInvite(ctx, &apiv2.ProjectInvite{Secret: req.Secret, Project: req.Project})
	if err != nil {
		return nil, errorutil.NewInternal(err)
	}

	return &apiv2.ProjectServiceInviteDeleteResponse{}, nil
}

func (p *projectServiceServer) InvitesList(ctx context.Context, req *apiv2.ProjectServiceInvitesListRequest) (*apiv2.ProjectServiceInvitesListResponse, error) {
	invites, err := p.inviteStore.ListInvites(ctx, req.Project)
	if err != nil {
		return nil, errorutil.NewInternal(err)
	}

	return &apiv2.ProjectServiceInvitesListResponse{Invites: invites}, nil
}
