package project

import (
	"context"
	"errors"
	"log/slog"
	"sort"
	"time"

	"github.com/metal-stack/api/go/errorutil"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/api/go/metalstack/api/v2/apiv2connect"
	"github.com/metal-stack/metal-apiserver/pkg/invite"
	"github.com/metal-stack/metal-apiserver/pkg/repository"
	"github.com/metal-stack/metal-apiserver/pkg/tags"
	"github.com/metal-stack/metal-apiserver/pkg/token"
	"github.com/metal-stack/metal-lib/pkg/pointer"
	"github.com/samber/lo"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type Config struct {
	Log         *slog.Logger
	Repo        *repository.Store
	InviteStore invite.ProjectInviteStore
}

type projectServiceServer struct {
	log         *slog.Logger
	repo        *repository.Store
	inviteStore invite.ProjectInviteStore
}

func New(c Config) apiv2connect.ProjectServiceHandler {
	return &projectServiceServer{
		log:         c.Log.WithGroup("projectService"),
		inviteStore: c.InviteStore,
		repo:        c.Repo,
	}
}

func (p *projectServiceServer) Get(ctx context.Context, req *apiv2.ProjectServiceGetRequest) (*apiv2.ProjectServiceGetResponse, error) {
	var (
		t, ok = token.TokenFromContext(ctx)
	)
	if !ok || t == nil {
		return nil, errorutil.Unauthenticated("no token found in request")
	}

	project, err := p.repo.Project(req.Project).Get(ctx, req.Project)
	if err != nil {
		return nil, err
	}

	return &apiv2.ProjectServiceGetResponse{Project: project}, nil
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

		if req.Query != nil {
			q := req.Query
			if q.Uuid != nil && project.Uuid != *q.Uuid {
				continue
			}
			if q.Name != nil && project.Name != *q.Name {
				continue
			}
			if q.Tenant != nil && project.Tenant != *q.Tenant {
				continue
			}
			if req.Query.Labels != nil {
				var (
					queryTags   = tags.ToTags(req.Query.Labels.Labels)
					projectTags = tags.ToTags(pointer.SafeDeref(pointer.SafeDeref(project.Meta).Labels).Labels)
				)

				noMatch, _ := lo.Difference(queryTags, projectTags)
				if len(noMatch) > 0 {
					continue
				}
			}
		}

		result = append(result, project)
	}

	sort.SliceStable(result, func(i, j int) bool {
		return result[i].Uuid < result[j].Uuid
	})

	return &apiv2.ProjectServiceListResponse{Projects: result}, nil
}

func (p *projectServiceServer) Create(ctx context.Context, req *apiv2.ProjectServiceCreateRequest) (*apiv2.ProjectServiceCreateResponse, error) {
	project, err := p.repo.UnscopedProject().Create(ctx, req)
	if err != nil {
		return nil, err
	}

	_, err = p.repo.ProjectMember(project.Uuid).Create(ctx, &apiv2.ProjectMemberServiceCreateRequest{
		Member: req.Login,
		Role:   apiv2.ProjectRole_PROJECT_ROLE_OWNER,
	})
	if err != nil {
		return nil, err
	}

	return &apiv2.ProjectServiceCreateResponse{Project: project}, nil
}

func (p *projectServiceServer) Delete(ctx context.Context, req *apiv2.ProjectServiceDeleteRequest) (*apiv2.ProjectServiceDeleteResponse, error) {
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
	token, ok := token.TokenFromContext(ctx)
	if !ok || token == nil {
		return nil, errorutil.Unauthenticated("no token found in request")
	}

	inv, err := p.inviteStore.GetInvite(ctx, req.Secret)
	if err != nil {
		if errors.Is(err, invite.ErrInviteNotFound) {
			return nil, errorutil.NotFound("the given invitation does not exist anymore")
		}
		return nil, errorutil.NewInternal(err)
	}

	invitee, err := p.repo.Tenant().Get(ctx, token.User)
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

	memberships, err := p.repo.ProjectMember(inv.Project).List(ctx, &apiv2.ProjectMemberQuery{
		Member: &invitee.Login,
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

	_, err = p.repo.ProjectMember(inv.Project).Create(ctx, &apiv2.ProjectMemberServiceCreateRequest{
		Role:   inv.Role,
		Member: invitee.Login,
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
