package project

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/api/go/metalstack/api/v2/apiv2connect"
	v1 "github.com/metal-stack/masterdata-api/api/v1"
	mdc "github.com/metal-stack/masterdata-api/pkg/client"
	"github.com/metal-stack/metal-apiserver/pkg/invite"
	putil "github.com/metal-stack/metal-apiserver/pkg/project"
	"github.com/metal-stack/metal-apiserver/pkg/repository"
	tutil "github.com/metal-stack/metal-apiserver/pkg/tenant"
	"github.com/metal-stack/metal-apiserver/pkg/token"
	"github.com/metal-stack/metal-lib/pkg/pointer"
	"google.golang.org/protobuf/types/known/timestamppb"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

type Config struct {
	Log          *slog.Logger
	MasterClient mdc.Client
	Repo         *repository.Store
	InviteStore  invite.ProjectInviteStore
	TokenStore   token.TokenStore
}

// FIXME use repo where possible

type projectServiceServer struct {
	log          *slog.Logger
	masterClient mdc.Client
	repo         *repository.Store
	inviteStore  invite.ProjectInviteStore
	tokenStore   token.TokenStore
}

func New(c Config) apiv2connect.ProjectServiceHandler {
	return &projectServiceServer{
		log:          c.Log.WithGroup("projectService"),
		masterClient: c.MasterClient,
		inviteStore:  c.InviteStore,
		tokenStore:   c.TokenStore,
		repo:         c.Repo,
	}
}

func (p *projectServiceServer) Get(ctx context.Context, rq *connect.Request[apiv2.ProjectServiceGetRequest]) (*connect.Response[apiv2.ProjectServiceGetResponse], error) {
	var (
		t, ok                   = token.TokenFromContext(ctx)
		req                     = rq.Msg
		includeInheritedMembers bool
	)
	if !ok || t == nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("no token found in request"))
	}

	resp, err := p.masterClient.Project().Get(ctx, &v1.ProjectGetRequest{Id: req.Project})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	project, err := putil.ToProject(resp.Project)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	pmlr, err := p.masterClient.ProjectMember().Find(ctx, &v1.ProjectMemberFindRequest{
		ProjectId: &req.Project,
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("unable to list project members: %w", err))
	}

	memberMap := map[string]*apiv2.ProjectMember{}

	for _, pm := range pmlr.GetProjectMembers() {
		memberMap[pm.TenantId] = &apiv2.ProjectMember{
			Id:        pm.TenantId,
			Role:      putil.ProjectRoleFromMap(pm.Meta.Annotations),
			CreatedAt: pm.Meta.CreatedTime,
		}
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

		tmlr, err := p.masterClient.Tenant().ListTenantMembers(ctx, &v1.ListTenantMembersRequest{TenantId: project.Tenant, IncludeInherited: pointer.Pointer(true)})
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("unable to list project members: %w", err))
		}

		for _, tm := range tmlr.GetTenants() {
			var projectRole apiv2.ProjectRole
			switch tutil.TenantRoleFromMap(tm.TenantAnnotations) {
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

			member, ok := memberMap[tm.Tenant.Meta.Id]
			if !ok {
				memberMap[tm.Tenant.Meta.Id] = &apiv2.ProjectMember{
					Id:                  tm.Tenant.Meta.Id,
					Role:                projectRole,
					CreatedAt:           tm.Tenant.Meta.CreatedTime,
					InheritedMembership: true,
				}

				continue
			}

			if member.Role > projectRole {
				member.Role = projectRole
				memberMap[tm.Tenant.Meta.Id] = member
			}
		}
	}

	var projectMembers []*apiv2.ProjectMember
	for _, m := range memberMap {
		projectMembers = append(projectMembers, m)
	}

	sort.Slice(projectMembers, func(i, j int) bool {
		return projectMembers[i].Id < projectMembers[j].Id
	})

	return connect.NewResponse(&apiv2.ProjectServiceGetResponse{Project: project, ProjectMembers: projectMembers}), nil
}

func (p *projectServiceServer) List(ctx context.Context, rq *connect.Request[apiv2.ProjectServiceListRequest]) (*connect.Response[apiv2.ProjectServiceListResponse], error) {
	token, ok := token.TokenFromContext(ctx)
	if !ok || token == nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("no token found in request"))
	}

	var (
		req    = rq.Msg
		result []*apiv2.Project
	)

	projectsAndTenants, err := putil.GetProjectsAndTenants(ctx, p.masterClient, token.UserId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("error retrieving projects from backend: %w", err))
	}

	for _, project := range projectsAndTenants.Projects {
		// TODO: maybe we can pass the filter and not filter here

		if req.Name != nil && project.Name != *req.Name {
			continue
		}
		if req.Tenant != nil && project.Tenant != *req.Tenant {
			continue
		}

		result = append(result, project)
	}

	return connect.NewResponse(&apiv2.ProjectServiceListResponse{Projects: result}), nil
}

func (p *projectServiceServer) Create(ctx context.Context, rq *connect.Request[apiv2.ProjectServiceCreateRequest]) (*connect.Response[apiv2.ProjectServiceCreateResponse], error) {
	var (
		t, ok = token.TokenFromContext(ctx)
		req   = rq.Msg
	)

	if !ok || t == nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("no token found in request"))
	}

	findResp, err := p.masterClient.Project().Find(ctx, &v1.ProjectFindRequest{
		Name:     wrapperspb.String(req.Name),
		TenantId: wrapperspb.String(req.Login),
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("error retrieving projects from backend: %w", err))
	}

	if len(findResp.Projects) > 0 {
		return nil, connect.NewError(connect.CodeAlreadyExists, fmt.Errorf("a project with name %q already exists for this organization", req.Name))
	}

	createResp, err := p.masterClient.Project().Create(ctx, &v1.ProjectCreateRequest{
		Project: &v1.Project{
			Meta: &v1.Meta{
				Id: uuid.NewString(),
				Annotations: map[string]string{
					putil.AvatarURLAnnotation: pointer.SafeDeref(req.AvatarUrl),
				},
			},
			Name:        req.Name,
			Description: req.Description,
			TenantId:    req.Login,
		},
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("error creating project: %w", err))
	}

	project, err := putil.ToProject(createResp.Project)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	_, err = p.masterClient.ProjectMember().Create(ctx, &v1.ProjectMemberCreateRequest{
		ProjectMember: &v1.ProjectMember{
			Meta: &v1.Meta{
				Annotations: map[string]string{
					putil.ProjectRoleAnnotation: apiv2.ProjectRole_PROJECT_ROLE_OWNER.String(),
				},
			},
			ProjectId: project.Uuid,
			TenantId:  t.UserId,
		},
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("unable to store project member: %w", err))
	}

	return connect.NewResponse(&apiv2.ProjectServiceCreateResponse{Project: project}), nil
}

func (p *projectServiceServer) Delete(ctx context.Context, rq *connect.Request[apiv2.ProjectServiceDeleteRequest]) (*connect.Response[apiv2.ProjectServiceDeleteResponse], error) {
	var (
		t, ok = token.TokenFromContext(ctx)
		req   = rq.Msg
	)

	if !ok || t == nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("no token found in request"))
	}

	getResp, err := p.masterClient.Project().Get(ctx, &v1.ProjectGetRequest{
		Id: req.Project,
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("no project found with id %q: %w", req.Project, err))
	}

	if t.AdminRole != nil && *t.AdminRole == apiv2.AdminRole_ADMIN_ROLE_EDITOR {
		// we allow deleting default-projects with admins explicitly in order to allow an admin to fully delete a tenant on demand
	}

	// FIXME check for machines and networks first

	ips, err := p.repo.IP(req.Project).List(ctx, &apiv2.IPQuery{Project: &req.Project})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("error retrieving ips: %w", err))
	}

	if len(ips) > 0 {
		return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("there are still ips associated with this project, you need to delete them first"))
	}

	_, err = p.masterClient.Project().Delete(ctx, &v1.ProjectDeleteRequest{Id: req.Project})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("error deleting project: %w", err))
	}

	// TODO: ensure project tokens are revoked / cleaned up

	result, err := putil.ToProject(getResp.Project)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(&apiv2.ProjectServiceDeleteResponse{Project: result}), nil
}

func (p *projectServiceServer) Update(ctx context.Context, rq *connect.Request[apiv2.ProjectServiceUpdateRequest]) (*connect.Response[apiv2.ProjectServiceUpdateResponse], error) {
	var (
		req = rq.Msg
	)

	getResp, err := p.masterClient.Project().Get(ctx, &v1.ProjectGetRequest{
		Id: req.Project,
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("no project found with id %q: %w", req.Project, err))
	}

	project := getResp.Project

	if req.Name != nil {
		project.Name = *req.Name
	}

	if req.Description != nil {
		project.Description = *req.Description
	}

	if req.AvatarUrl != nil {
		project.Meta.Annotations[putil.AvatarURLAnnotation] = *req.AvatarUrl
	}

	updatedResp, err := p.masterClient.Project().Update(ctx, &v1.ProjectUpdateRequest{
		Project: project,
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("error updating project: %w", err))
	}

	result, err := putil.ToProject(updatedResp.Project)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(&apiv2.ProjectServiceUpdateResponse{Project: result}), nil
}

func (p *projectServiceServer) RemoveMember(ctx context.Context, rq *connect.Request[apiv2.ProjectServiceRemoveMemberRequest]) (*connect.Response[apiv2.ProjectServiceRemoveMemberResponse], error) {
	var (
		req   = rq.Msg
		t, ok = token.TokenFromContext(ctx)
	)

	if !ok || t == nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("no token found in request"))
	}

	membership, _, err := putil.GetProjectMember(ctx, p.masterClient, req.Project, req.MemberId)
	if err != nil {
		return nil, err
	}

	lastOwner, err := p.checkIfMemberIsLastOwner(ctx, membership)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if lastOwner {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("cannot remove last owner of a project"))
	}

	_, err = p.masterClient.ProjectMember().Delete(ctx, &v1.ProjectMemberDeleteRequest{
		Id: membership.Meta.Id,
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(&apiv2.ProjectServiceRemoveMemberResponse{}), nil
}

func (p *projectServiceServer) UpdateMember(ctx context.Context, rq *connect.Request[apiv2.ProjectServiceUpdateMemberRequest]) (*connect.Response[apiv2.ProjectServiceUpdateMemberResponse], error) {
	var (
		req = rq.Msg
	)

	membership, _, err := putil.GetProjectMember(ctx, p.masterClient, req.Project, req.MemberId)
	var connectErr *connect.Error

	if errors.As(err, &connectErr) {
		if connectErr.Code() == connect.CodeNotFound {
			// if there does not exist a direct membership for this user but the user belongs to the tenant already, we create a direct membership for the project
			projectGuest, projecterr := putil.GetProject(ctx, p.masterClient, req.Project)
			if projecterr != nil {
				return nil, err
			}
			partiTenants, err := p.masterClient.Tenant().FindParticipatingTenants(ctx, &v1.FindParticipatingTenantsRequest{TenantId: req.MemberId, IncludeInherited: pointer.Pointer(true)})
			if err != nil {
				return nil, err
			}
			found := false
			for _, tenantWrapper := range partiTenants.Tenants {
				tenantID := tenantWrapper.Tenant.Meta.Id
				if tenantID == projectGuest.TenantId {
					found = true
					break
				}
			}
			if !found {
				return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("tenant is not part of the project's tenants"))
			}
			// Create new project membership since the user is part of the tenant
			membership, err = p.createProjectMembership(ctx, req.MemberId, req.Project, req.Role)

			if err != nil {
				return nil, err
			}

			return connect.NewResponse(&apiv2.ProjectServiceUpdateMemberResponse{
				ProjectMember: &apiv2.ProjectMember{
					Id:                  req.MemberId,
					Role:                req.Role,
					InheritedMembership: false,
					CreatedAt:           membership.Meta.CreatedTime,
				},
			}), nil
		}

	}
	if err != nil {
		return nil, err
	}

	if req.Role != apiv2.ProjectRole_PROJECT_ROLE_UNSPECIFIED {
		// TODO: currently the API defines that only owners can update members so there is no possibility to elevate permissions
		// probably, we should still check that no elevation of permissions is possible in case we later change the API

		membership.Meta.Annotations[putil.ProjectRoleAnnotation] = req.Role.String()
	}
	lastOwner, err := p.checkIfMemberIsLastOwner(ctx, membership)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if lastOwner && req.Role != apiv2.ProjectRole_PROJECT_ROLE_OWNER {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("cannot demote last owner's permissions"))
	}

	updatedMember, err := p.masterClient.ProjectMember().Update(ctx, &v1.ProjectMemberUpdateRequest{ProjectMember: membership})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(&apiv2.ProjectServiceUpdateMemberResponse{ProjectMember: &apiv2.ProjectMember{
		Id:        req.MemberId,
		Role:      req.Role,
		CreatedAt: updatedMember.ProjectMember.Meta.CreatedTime,
	}}), nil
}

func (p *projectServiceServer) createProjectMembership(ctx context.Context, tenantID, projectID string, role apiv2.ProjectRole) (*v1.ProjectMember, error) {
	newMembership := &v1.ProjectMember{
		ProjectId: projectID,
		TenantId:  tenantID,
		Meta: &v1.Meta{
			Annotations: map[string]string{
				putil.ProjectRoleAnnotation: role.String(),
			},
		},
	}
	//If there is no role specified, give him Viewer. This can happen only in the CLI
	if role == apiv2.ProjectRole_PROJECT_ROLE_UNSPECIFIED {
		newMembership.Meta.Annotations[putil.ProjectRoleAnnotation] = apiv2.ProjectRole_PROJECT_ROLE_VIEWER.String()
	}
	// Attempt to create the new project membership
	createdMember, err := p.masterClient.ProjectMember().Create(ctx, &v1.ProjectMemberCreateRequest{ProjectMember: newMembership})
	if err != nil {
		return nil, err
	}
	return createdMember.ProjectMember, nil
}

func (p *projectServiceServer) InviteGet(ctx context.Context, rq *connect.Request[apiv2.ProjectServiceInviteGetRequest]) (*connect.Response[apiv2.ProjectServiceInviteGetResponse], error) {
	var (
		req = rq.Msg
	)

	inv, err := p.inviteStore.GetInvite(ctx, req.Secret)
	if err != nil {
		if errors.Is(err, invite.ErrInviteNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("the given invitation does not exist anymore"))
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(&apiv2.ProjectServiceInviteGetResponse{Invite: inv}), nil
}

func (p *projectServiceServer) Invite(ctx context.Context, rq *connect.Request[apiv2.ProjectServiceInviteRequest]) (*connect.Response[apiv2.ProjectServiceInviteResponse], error) {
	var (
		req = rq.Msg
	)
	pgr, err := p.masterClient.Project().Get(ctx, &v1.ProjectGetRequest{
		Id: req.Project,
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("no project found with id %q: %w", req.Project, err))
	}

	tgr, err := p.masterClient.Tenant().Get(ctx, &v1.TenantGetRequest{
		Id: pgr.Project.TenantId,
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("no account: %q found %w", pgr.Project.TenantId, err))
	}

	secret, err := invite.GenerateInviteSecret()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	var (
		expiresAt = time.Now().Add(7 * 24 * time.Hour)
	)

	if req.Role == apiv2.ProjectRole_PROJECT_ROLE_UNSPECIFIED {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("project role must be specified"))
	}

	invite := &apiv2.ProjectInvite{
		Secret:      secret,
		Project:     pgr.Project.Meta.Id,
		Role:        req.Role,
		Joined:      false,
		ProjectName: pgr.Project.Name,
		Tenant:      pgr.Project.TenantId,
		TenantName:  tgr.Tenant.Name,
		ExpiresAt:   timestamppb.New(expiresAt),
		JoinedAt:    &timestamppb.Timestamp{},
	}
	p.log.Info("project invitation created", "invitation", invite)

	err = p.inviteStore.SetInvite(ctx, invite)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(&apiv2.ProjectServiceInviteResponse{Invite: invite}), nil
}

func (p *projectServiceServer) InviteAccept(ctx context.Context, rq *connect.Request[apiv2.ProjectServiceInviteAcceptRequest]) (*connect.Response[apiv2.ProjectServiceInviteAcceptResponse], error) {
	var (
		t, ok = token.TokenFromContext(ctx)
		req   = rq.Msg
	)

	if !ok || t == nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("no token found in request"))
	}

	inv, err := p.inviteStore.GetInvite(ctx, req.Secret)
	if err != nil {
		if errors.Is(err, invite.ErrInviteNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("the given invitation does not exist anymore"))
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	tgr, err := p.masterClient.Tenant().Get(ctx, &v1.TenantGetRequest{
		Id: t.UserId,
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("no account: %q found %w", t.UserId, err))
	}

	invitee := tgr.Tenant

	pgr, err := p.masterClient.Project().Get(ctx, &v1.ProjectGetRequest{
		Id: inv.Project,
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("no project: %q for invite not found %w", inv.Project, err))
	}

	if pgr.Project.TenantId == invitee.Meta.Id {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("an owner cannot accept invitations to own projects"))
	}

	memberships, err := p.masterClient.ProjectMember().Find(ctx, &v1.ProjectMemberFindRequest{
		ProjectId: &inv.Project,
		TenantId:  &invitee.Meta.Id,
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	if len(memberships.GetProjectMembers()) > 0 {
		return nil, connect.NewError(connect.CodeAlreadyExists, fmt.Errorf("%s is already member of project %s", invitee.Meta.Id, inv.Project))
	}

	err = p.inviteStore.DeleteInvite(ctx, inv)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	_, err = p.masterClient.ProjectMember().Create(ctx, &v1.ProjectMemberCreateRequest{
		ProjectMember: &v1.ProjectMember{
			Meta: &v1.Meta{
				Annotations: map[string]string{
					putil.ProjectRoleAnnotation: inv.Role.String(),
				},
			},
			ProjectId: inv.Project,
			TenantId:  invitee.Meta.Id,
		},
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("unable to store project member: %w", err))
	}

	return connect.NewResponse(&apiv2.ProjectServiceInviteAcceptResponse{Project: inv.Project, ProjectName: inv.ProjectName}), nil
}

func (p *projectServiceServer) InviteDelete(ctx context.Context, rq *connect.Request[apiv2.ProjectServiceInviteDeleteRequest]) (*connect.Response[apiv2.ProjectServiceInviteDeleteResponse], error) {
	var (
		req = rq.Msg
	)

	err := p.inviteStore.DeleteInvite(ctx, &apiv2.ProjectInvite{Secret: req.Secret, Project: req.Project})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(&apiv2.ProjectServiceInviteDeleteResponse{}), nil
}

func (p *projectServiceServer) InvitesList(ctx context.Context, rq *connect.Request[apiv2.ProjectServiceInvitesListRequest]) (*connect.Response[apiv2.ProjectServiceInvitesListResponse], error) {
	var (
		req = rq.Msg
	)
	invites, err := p.inviteStore.ListInvites(ctx, req.Project)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(&apiv2.ProjectServiceInvitesListResponse{Invites: invites}), nil
}

func (p *projectServiceServer) checkIfMemberIsLastOwner(ctx context.Context, membership *v1.ProjectMember) (bool, error) {
	isOwner := membership.Meta.Annotations[putil.ProjectRoleAnnotation] == apiv2.ProjectRole_PROJECT_ROLE_OWNER.String()
	if !isOwner {
		return false, nil
	}

	resp, err := p.masterClient.ProjectMember().Find(ctx, &v1.ProjectMemberFindRequest{
		ProjectId: &membership.ProjectId,
		Annotations: map[string]string{
			putil.ProjectRoleAnnotation: apiv2.ProjectRole_PROJECT_ROLE_OWNER.String(),
		},
	})
	if err != nil {
		return false, err
	}

	return len(resp.ProjectMembers) < 2, nil
}
