package tenant

import (
	"context"
	"errors"
	"log/slog"
	"sort"
	"time"

	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/api/go/metalstack/api/v2/apiv2connect"
	mdcv1 "github.com/metal-stack/masterdata-api/api/v1"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	"github.com/metal-stack/metal-apiserver/pkg/repository"
	msvc "github.com/metal-stack/metal-apiserver/pkg/service/method"
	"github.com/metal-stack/metal-lib/pkg/pointer"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/metal-stack/metal-apiserver/pkg/invite"
	"github.com/metal-stack/metal-apiserver/pkg/token"
)

type Config struct {
	Log         *slog.Logger
	Repo        *repository.Store
	InviteStore invite.TenantInviteStore
	TokenStore  token.TokenStore
}
type tenantServiceServer struct {
	log         *slog.Logger
	repo        *repository.Store
	inviteStore invite.TenantInviteStore
	tokenStore  token.TokenStore
}

type TenantService interface {
	apiv2connect.TenantServiceHandler
}

func New(c Config) TenantService {
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

		if req.Name != nil && tenant.Name != *req.Name {
			continue
		}
		if req.Id != nil && tenant.Login != *req.Id {
			continue
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
		if mdcv1.IsNotFound(err) {
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
	_, err = u.repo.Tenant().AdditionalMethods().Member(tenant.Login).Create(ctx, &repository.TenantMemberCreateRequest{
		MemberID: t.GetUser(),
		Role:     apiv2.TenantRole_TENANT_ROLE_OWNER,
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
		}, TenantMembers: nil}, nil
	case apiv2.TenantRole_TENANT_ROLE_UNSPECIFIED:
		if msvc.IsAdminToken(t) {
			break
		}
		fallthrough
	default:
		return nil, errorutil.Unauthenticated("tenant role insufficient")
	}

	members, err := u.repo.Tenant().AdditionalMethods().ListTenantMembers(ctx, req.Login, true)
	if err != nil {
		return nil, errorutil.Internal("unable to list tenant members: %w", err)
	}

	var tenantMembers []*apiv2.TenantMember
	for _, member := range members {
		tenantRole := repository.TenantRoleFromMap(member.TenantAnnotations)
		if tenantRole == apiv2.TenantRole_TENANT_ROLE_UNSPECIFIED {
			tenantRole = apiv2.TenantRole_TENANT_ROLE_GUEST
		}

		tenantMembers = append(tenantMembers, &apiv2.TenantMember{
			Id:        member.Tenant.Login,
			Role:      tenantRole,
			CreatedAt: member.Tenant.Meta.CreatedAt,
			Projects:  member.ProjectIds,
		})
	}

	sort.Slice(tenantMembers, func(i, j int) bool {
		return tenantMembers[i].Id < tenantMembers[j].Id
	})

	return &apiv2.TenantServiceGetResponse{Tenant: tenant, TenantMembers: tenantMembers}, nil
}

func (u *tenantServiceServer) Leave(ctx context.Context, req *apiv2.TenantServiceLeaveRequest) (*apiv2.TenantServiceLeaveResponse, error) {
	var (
		t, ok = token.TokenFromContext(ctx)
	)

	if !ok || t == nil {
		return nil, errorutil.Unauthenticated("no token found in request")
	}

	_, err := u.repo.Tenant().AdditionalMethods().Member(req.Login).Delete(ctx, t.User)
	if err != nil {
		return nil, err
	}

	return &apiv2.TenantServiceLeaveResponse{}, nil
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

	memberships, err := u.repo.Tenant().AdditionalMethods().Member(inv.TargetTenant).List(ctx, &repository.TenantMemberQuery{
		MemberId: &invitee.Login,
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

	_, err = u.repo.Tenant().AdditionalMethods().Member(inv.TargetTenant).Create(ctx, &repository.TenantMemberCreateRequest{
		MemberID: invitee.Login,
		Role:     inv.Role,
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

func (u *tenantServiceServer) RemoveMember(ctx context.Context, req *apiv2.TenantServiceRemoveMemberRequest) (*apiv2.TenantServiceRemoveMemberResponse, error) {
	_, err := u.repo.Tenant().AdditionalMethods().Member(req.Login).Delete(ctx, req.Member)
	if err != nil {
		return nil, err
	}

	return &apiv2.TenantServiceRemoveMemberResponse{}, nil
}

func (u *tenantServiceServer) UpdateMember(ctx context.Context, req *apiv2.TenantServiceUpdateMemberRequest) (*apiv2.TenantServiceUpdateMemberResponse, error) {
	updatedMember, err := u.repo.Tenant().AdditionalMethods().Member(req.Login).Update(ctx, req.Member, &repository.TenantMemberUpdateRequest{
		Role: req.Role,
	})
	if err != nil {
		return nil, err
	}

	return &apiv2.TenantServiceUpdateMemberResponse{TenantMember: &apiv2.TenantMember{
		Id:        req.Member,
		Role:      req.Role,
		CreatedAt: updatedMember.CreatedAt,
	}}, nil
}
