package tenant

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/metal-stack/api/go/errorutil"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/api/go/metalstack/api/v2/apiv2connect"
	"github.com/metal-stack/metal-apiserver/pkg/repository"
	"github.com/metal-stack/metal-apiserver/pkg/tags"
	"github.com/metal-stack/metal-lib/pkg/pointer"
	"github.com/samber/lo"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/metal-stack/metal-apiserver/pkg/invite"
	"github.com/metal-stack/metal-apiserver/pkg/token"
)

type (
	Config struct {
		Log         *slog.Logger
		Repo        *repository.Store
		InviteStore invite.TenantInviteStore
		TokenStore  token.TokenStore
	}

	tenantServiceServer struct {
		log         *slog.Logger
		repo        *repository.Store
		inviteStore invite.TenantInviteStore
		tokenStore  token.TokenStore
	}
)

func New(c Config) apiv2connect.TenantServiceHandler {
	return &tenantServiceServer{
		log:         c.Log.WithGroup("tenantService"),
		inviteStore: c.InviteStore,
		tokenStore:  c.TokenStore,
		repo:        c.Repo,
	}
}

func (u *tenantServiceServer) List(ctx context.Context, req *apiv2.TenantServiceListRequest) (*apiv2.TenantServiceListResponse, error) {
	token, ok := token.TokenFromContext(ctx)
	if !ok || token == nil {
		return nil, errorutil.Unauthenticated("no token found in request")
	}

	var (
		result []*apiv2.Tenant
	)

	projectsAndTenants, err := u.repo.UnscopedProject().AdditionalMethods().GetProjectsAndTenants(ctx, token.User)
	if err != nil {
		return nil, errorutil.Internal("error retrieving tenants from backend: %w", err)
	}

	for _, tenant := range projectsAndTenants.Tenants {
		// TODO: maybe we can pass the filter and not filter here
		if req.Query != nil {
			if req.Query.Paging != nil {
				return nil, errorutil.InvalidArgument("paging is currently not supported on tenant list request")
			}

			if req.Query.Name != nil && tenant.Name != *req.Query.Name {
				continue
			}
			if req.Query.Login != nil && tenant.Login != *req.Query.Login {
				continue
			}
			if req.Query.Labels != nil {
				var (
					queryTags  = tags.ToTags(req.Query.Labels.Labels)
					tenantTags = tags.ToTags(pointer.SafeDeref(pointer.SafeDeref(tenant.Meta).Labels).Labels)
				)

				noMatch, _ := lo.Difference(queryTags, tenantTags)
				if len(noMatch) > 0 {
					continue
				}
			}
		}

		result = append(result, tenant)
	}

	return &apiv2.TenantServiceListResponse{Tenants: result}, nil
}

func (u *tenantServiceServer) Create(ctx context.Context, req *apiv2.TenantServiceCreateRequest) (*apiv2.TenantServiceCreateResponse, error) {
	var (
		t, ok = token.TokenFromContext(ctx)
	)

	if !ok || t == nil {
		return nil, errorutil.Unauthenticated("no token found in request")
	}

	ownTenant, err := u.repo.Tenant().Get(ctx, t.User)
	if err != nil {
		if errorutil.IsNotFound(err) {
			return nil, errorutil.NotFound("no tenant found with id %q: %w", t.User, err)
		}

		return nil, errorutil.NewInternal(err)
	}

	if pointer.SafeDeref(req.Email) == "" && ownTenant.Email != "" {
		req.Email = new(ownTenant.Email)

		if pointer.SafeDeref(req.Email) == "" {
			return nil, errorutil.FailedPrecondition("email is required")
		}
	}

	tenant, err := u.repo.Tenant().Create(ctx, req)
	if err != nil {
		return nil, err
	}

	// make tenant owner and member of its own tenant
	_, err = u.repo.TenantMember(tenant.Login).Create(ctx, &apiv2.TenantMemberServiceCreateRequest{
		Member: t.GetUser(),
		Role:   apiv2.TenantRole_TENANT_ROLE_OWNER,
	})
	if err != nil {
		return nil, err // TODO: give more instructions what to do now!
	}

	return &apiv2.TenantServiceCreateResponse{Tenant: tenant}, nil
}

func (u *tenantServiceServer) Get(ctx context.Context, req *apiv2.TenantServiceGetRequest) (*apiv2.TenantServiceGetResponse, error) {
	var (
		t, ok = token.TokenFromContext(ctx)
	)
	if !ok || t == nil {
		return nil, errorutil.Unauthenticated("no token found in request")
	}

	tenant, err := u.repo.Tenant().Get(ctx, req.Login)
	if err != nil {
		return nil, err
	}

	role := t.TenantRoles[req.Login]
	switch role {
	case apiv2.TenantRole_TENANT_ROLE_OWNER, apiv2.TenantRole_TENANT_ROLE_EDITOR, apiv2.TenantRole_TENANT_ROLE_VIEWER:
	case apiv2.TenantRole_TENANT_ROLE_GUEST:
		// guests only see a minimal subset of the tenant information, a guest is not part of the tenant!

		return &apiv2.TenantServiceGetResponse{Tenant: &apiv2.Tenant{
			Login:       tenant.Login,
			Name:        tenant.Name,
			Email:       "",
			Description: tenant.Description,
			AvatarUrl:   tenant.AvatarUrl,
			CreatedBy:   "",
			Meta: &apiv2.Meta{
				CreatedAt: tenant.Meta.CreatedAt,
				UpdatedAt: tenant.Meta.UpdatedAt,
			},
		}}, nil
	case apiv2.TenantRole_TENANT_ROLE_UNSPECIFIED:
		if token.IsAdminToken(t) {
			break
		}
		fallthrough
	default:
		return nil, errorutil.Unauthenticated("tenant role insufficient")
	}

	return &apiv2.TenantServiceGetResponse{Tenant: tenant}, nil
}

func (u *tenantServiceServer) Update(ctx context.Context, req *apiv2.TenantServiceUpdateRequest) (*apiv2.TenantServiceUpdateResponse, error) {
	tenant, err := u.repo.Tenant().Update(ctx, req.Login, req)
	if err != nil {
		return nil, err
	}

	return &apiv2.TenantServiceUpdateResponse{Tenant: tenant}, nil
}

func (u *tenantServiceServer) Delete(ctx context.Context, req *apiv2.TenantServiceDeleteRequest) (*apiv2.TenantServiceDeleteResponse, error) {
	tenant, err := u.repo.Tenant().Delete(ctx, req.Login)
	if err != nil {
		return nil, err
	}

	return &apiv2.TenantServiceDeleteResponse{Tenant: tenant}, nil
}

func (u *tenantServiceServer) Invite(ctx context.Context, req *apiv2.TenantServiceInviteRequest) (*apiv2.TenantServiceInviteResponse, error) {
	var (
		t, ok = token.TokenFromContext(ctx)
	)
	if !ok || t == nil {
		return nil, errorutil.Unauthenticated("no token found in request")
	}

	targetTenant, err := u.repo.Tenant().Get(ctx, req.Login)
	if err != nil {
		return nil, err
	}

	invitee, err := u.repo.Tenant().Get(ctx, t.User)
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

	if req.Role == apiv2.TenantRole_TENANT_ROLE_UNSPECIFIED {
		return nil, errorutil.InvalidArgument("tenant role must be specified")
	}

	invite := &apiv2.TenantInvite{
		Secret:           secret,
		TargetTenant:     targetTenant.Login,
		Role:             req.Role,
		Joined:           false,
		TargetTenantName: targetTenant.Name,
		TenantName:       invitee.Name,
		Tenant:           invitee.Login,
		ExpiresAt:        timestamppb.New(expiresAt),
		JoinedAt:         nil,
	}

	u.log.Info("tenant invitation created", "invitation", invite)

	err = u.inviteStore.SetInvite(ctx, invite)
	if err != nil {
		return nil, errorutil.NewInternal(err)
	}

	return &apiv2.TenantServiceInviteResponse{Invite: invite}, nil
}

func (u *tenantServiceServer) InviteAccept(ctx context.Context, req *apiv2.TenantServiceInviteAcceptRequest) (*apiv2.TenantServiceInviteAcceptResponse, error) {
	var (
		t, ok = token.TokenFromContext(ctx)
	)

	if !ok || t == nil {
		return nil, errorutil.Unauthenticated("no token found in request")
	}

	inv, err := u.inviteStore.GetInvite(ctx, req.Secret)
	if err != nil {
		if errors.Is(err, invite.ErrInviteNotFound) {
			return nil, errorutil.NotFound("the given invitation does not exist anymore")
		}
		return nil, errorutil.NewInternal(err)
	}

	invitee, err := u.repo.Tenant().Get(ctx, t.User)
	if err != nil {
		return nil, err
	}

	if invitee.Login == inv.TargetTenant {
		return nil, errorutil.InvalidArgument("an owner cannot accept invitations to own tenants")
	}

	memberships, err := u.repo.TenantMember(inv.TargetTenant).List(ctx, &apiv2.TenantMemberQuery{
		Member: &invitee.Login,
	})
	if err != nil {
		return nil, err
	}

	if len(memberships) > 0 {
		return nil, errorutil.Conflict("%s is already member of tenant %s", invitee.Login, inv.TargetTenant)
	}

	err = u.inviteStore.DeleteInvite(ctx, inv)
	if err != nil {
		return nil, errorutil.NewInternal(err)
	}

	_, err = u.repo.TenantMember(inv.TargetTenant).Create(ctx, &apiv2.TenantMemberServiceCreateRequest{
		Member: invitee.Login,
		Role:   inv.Role,
	})
	if err != nil {
		return nil, err
	}

	return &apiv2.TenantServiceInviteAcceptResponse{Tenant: inv.TargetTenant, TenantName: inv.TargetTenantName}, nil
}

func (u *tenantServiceServer) InviteDelete(ctx context.Context, req *apiv2.TenantServiceInviteDeleteRequest) (*apiv2.TenantServiceInviteDeleteResponse, error) {
	err := u.inviteStore.DeleteInvite(ctx, &apiv2.TenantInvite{Secret: req.Secret, TargetTenant: req.Login})
	if err != nil {
		return nil, errorutil.NewInternal(err)
	}

	u.log.Debug("tenant invite deleted")

	return &apiv2.TenantServiceInviteDeleteResponse{}, nil
}

func (u *tenantServiceServer) InviteGet(ctx context.Context, req *apiv2.TenantServiceInviteGetRequest) (*apiv2.TenantServiceInviteGetResponse, error) {
	inv, err := u.inviteStore.GetInvite(ctx, req.Secret)
	if err != nil {
		if errors.Is(err, invite.ErrInviteNotFound) {
			return nil, errorutil.NotFound("the given invitation does not exist anymore")
		}
		return nil, errorutil.NewInternal(err)
	}

	return &apiv2.TenantServiceInviteGetResponse{Invite: inv}, nil
}

func (u *tenantServiceServer) InvitesList(ctx context.Context, req *apiv2.TenantServiceInvitesListRequest) (*apiv2.TenantServiceInvitesListResponse, error) {
	invites, err := u.inviteStore.ListInvites(ctx, req.Login)
	if err != nil {
		return nil, errorutil.NewInternal(err)
	}

	return &apiv2.TenantServiceInvitesListResponse{Invites: invites}, nil
}
