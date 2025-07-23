package tenant

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"connectrpc.com/connect"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/api/go/metalstack/api/v2/apiv2connect"
	mdcv1 "github.com/metal-stack/masterdata-api/api/v1"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	"github.com/metal-stack/metal-apiserver/pkg/repository"
	msvc "github.com/metal-stack/metal-apiserver/pkg/service/method"
	"github.com/metal-stack/metal-lib/pkg/pointer"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/metal-stack/metal-apiserver/pkg/invite"
	tutil "github.com/metal-stack/metal-apiserver/pkg/tenant"
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

func (u *tenantServiceServer) List(ctx context.Context, rq *connect.Request[apiv2.TenantServiceListRequest]) (*connect.Response[apiv2.TenantServiceListResponse], error) {
	token, ok := token.TokenFromContext(ctx)
	if !ok || token == nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("no token found in request"))
	}

	var (
		req    = rq.Msg
		result []*apiv2.Tenant
	)

	projectsAndTenants, err := u.repo.UnscopedProject().GetProjectsAndTenants(ctx, token.UserId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("error retrieving tenants from backend: %w", err))
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

	return connect.NewResponse(&apiv2.TenantServiceListResponse{Tenants: result}), nil
}

func (u *tenantServiceServer) Create(ctx context.Context, rq *connect.Request[apiv2.TenantServiceCreateRequest]) (*connect.Response[apiv2.TenantServiceCreateResponse], error) {
	var (
		req   = rq.Msg
		t, ok = token.TokenFromContext(ctx)
	)

	if !ok || t == nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("no token found in request"))
	}

	ownTenant, err := u.repo.Tenant().Get(ctx, t.UserId)
	if err != nil {
		if mdcv1.IsNotFound(err) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("no tenant found with id %q: %w", t.UserId, err))
		}

		return nil, connect.NewError(connect.CodeInternal, err)
	}

	if pointer.SafeDeref(req.Email) == "" && ownTenant.Meta != nil && ownTenant.Meta.Annotations != nil {
		req.Email = pointer.Pointer(ownTenant.Meta.Annotations[tutil.TagEmail])

		if pointer.SafeDeref(req.Email) == "" {
			return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("email is required"))
		}
	}

	validated, err := u.repo.Tenant().ValidateCreate(ctx, req)
	if err != nil {
		return nil, err
	}

	created, err := u.repo.Tenant().Create(ctx, validated)
	if err != nil {
		return nil, err
	}

	converted, err := u.repo.Tenant().ConvertToProto(created)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	// make tenant owner and member of its own tenant

	validatedMember, err := u.repo.Tenant().Member(t.UserId).ValidateCreate(ctx, &repository.TenantMemberCreateRequest{
		MemberID: converted.Login,
		Role:     apiv2.TenantRole_TENANT_ROLE_OWNER,
	})
	if err != nil {
		return nil, err
	}

	_, err = u.repo.Tenant().Member(t.UserId).Create(ctx, validatedMember)
	if err != nil {
		return nil, err // TODO: give more instructions what to do now!
	}

	return connect.NewResponse(&apiv2.TenantServiceCreateResponse{Tenant: converted}), nil
}

func (u *tenantServiceServer) Get(ctx context.Context, rq *connect.Request[apiv2.TenantServiceGetRequest]) (*connect.Response[apiv2.TenantServiceGetResponse], error) {
	var (
		t, ok = token.TokenFromContext(ctx)
		req   = rq.Msg
	)
	if !ok || t == nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("no token found in request"))
	}

	tenant, err := u.repo.Tenant().Get(ctx, req.Login)
	if err != nil {
		if mdcv1.IsNotFound(err) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("no tenant found with id %q: %w", req.Login, err))
		}

		return nil, connect.NewError(connect.CodeInternal, err)
	}

	converted, err := u.repo.Tenant().ConvertToProto(tenant)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	role := t.TenantRoles[req.Login]
	switch role {
	case apiv2.TenantRole_TENANT_ROLE_OWNER, apiv2.TenantRole_TENANT_ROLE_EDITOR, apiv2.TenantRole_TENANT_ROLE_VIEWER:
	case apiv2.TenantRole_TENANT_ROLE_GUEST:
		// guests only see a minimal subset of the tenant information, a guest is not part of the tenant!

		return connect.NewResponse(&apiv2.TenantServiceGetResponse{Tenant: &apiv2.Tenant{
			Login:       converted.Login,
			Name:        converted.Name,
			Email:       "",
			Description: converted.Description,
			AvatarUrl:   converted.AvatarUrl,
			CreatedBy:   "",
			Meta: &apiv2.Meta{
				CreatedAt: converted.Meta.CreatedAt,
				UpdatedAt: converted.Meta.UpdatedAt,
			},
		}, TenantMembers: nil}), nil
	case apiv2.TenantRole_TENANT_ROLE_UNSPECIFIED:
		if msvc.IsAdminToken(t) {
			break
		}
		fallthrough
	default:
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("tenant role insufficient"))
	}

	members, err := u.repo.Tenant().ListTenantMembers(ctx, req.Login, true)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("unable to list tenant members: %w", err))
	}

	var tenantMembers []*apiv2.TenantMember
	for _, member := range members {
		tenantRole := tutil.TenantRoleFromMap(member.TenantAnnotations)
		if tenantRole == apiv2.TenantRole_TENANT_ROLE_UNSPECIFIED {
			tenantRole = apiv2.TenantRole_TENANT_ROLE_GUEST
		}

		tenantMembers = append(tenantMembers, &apiv2.TenantMember{
			Id:         member.Tenant.Meta.Id,
			Role:       tenantRole,
			CreatedAt:  member.Tenant.Meta.CreatedTime,
			ProjectIds: member.ProjectIds,
		})
	}

	sort.Slice(tenantMembers, func(i, j int) bool {
		return tenantMembers[i].Id < tenantMembers[j].Id
	})

	return connect.NewResponse(&apiv2.TenantServiceGetResponse{Tenant: converted, TenantMembers: tenantMembers}), nil
}

func (u *tenantServiceServer) Update(ctx context.Context, rq *connect.Request[apiv2.TenantServiceUpdateRequest]) (*connect.Response[apiv2.TenantServiceUpdateResponse], error) {
	req := rq.Msg

	validated, err := u.repo.Tenant().ValidateUpdate(ctx, req)
	if err != nil {
		return nil, err
	}

	updated, err := u.repo.Tenant().Update(ctx, validated)
	if err != nil {
		return nil, err
	}

	converted, err := u.repo.Tenant().ConvertToProto(updated)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return connect.NewResponse(&apiv2.TenantServiceUpdateResponse{Tenant: converted}), nil
}

func (u *tenantServiceServer) Delete(ctx context.Context, rq *connect.Request[apiv2.TenantServiceDeleteRequest]) (*connect.Response[apiv2.TenantServiceDeleteResponse], error) {
	var (
		t, ok = token.TokenFromContext(ctx)
		req   = rq.Msg
	)
	if !ok || t == nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("no token found in request"))
	}

	validated, err := u.repo.Tenant().ValidateDelete(ctx, &mdcv1.Tenant{
		Name: req.Login,
	})
	if err != nil {
		return nil, err
	}

	u.log.Debug("delete", "tenant", rq)

	if t.UserId == req.Login {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("the personal tenant (default-tenant) cannot be deleted"))
	}

	projects, err := u.repo.UnscopedProject().List(ctx, &apiv2.ProjectServiceListRequest{
		Tenant: &req.Login,
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("unable to lookup projects: %w", err))
	}

	if len(projects) > 0 {
		return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("there are still projects associated with this tenant, you need to delete them first"))
	}

	deleted, err := u.repo.Tenant().Delete(ctx, validated)
	if err != nil {
		return nil, err
	}

	converted, err := u.repo.Tenant().ConvertToProto(deleted)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return connect.NewResponse(&apiv2.TenantServiceDeleteResponse{Tenant: converted}), nil
}

func (u *tenantServiceServer) Invite(ctx context.Context, rq *connect.Request[apiv2.TenantServiceInviteRequest]) (*connect.Response[apiv2.TenantServiceInviteResponse], error) {
	var (
		t, ok = token.TokenFromContext(ctx)
		req   = rq.Msg
	)
	if !ok || t == nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("no token found in request"))
	}

	targetTenant, err := u.repo.Tenant().Get(ctx, req.Login)
	if err != nil {
		return nil, err
	}

	invitee, err := u.repo.Tenant().Get(ctx, t.UserId)
	if err != nil {
		return nil, err
	}

	secret, err := invite.GenerateInviteSecret()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	var (
		expiresAt = time.Now().Add(7 * 24 * time.Hour)
	)

	if req.Role == apiv2.TenantRole_TENANT_ROLE_UNSPECIFIED {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("tenant role must be specified"))
	}

	invite := &apiv2.TenantInvite{
		Secret:           secret,
		TargetTenant:     targetTenant.Meta.Id,
		Role:             req.Role,
		Joined:           false,
		TargetTenantName: targetTenant.Name,
		TenantName:       invitee.Name,
		Tenant:           invitee.Meta.Id,
		ExpiresAt:        timestamppb.New(expiresAt),
		JoinedAt:         &timestamppb.Timestamp{},
	}

	u.log.Info("tenant invitation created", "invitation", invite)

	err = u.inviteStore.SetInvite(ctx, invite)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(&apiv2.TenantServiceInviteResponse{Invite: invite}), nil
}

func (u *tenantServiceServer) InviteAccept(ctx context.Context, rq *connect.Request[apiv2.TenantServiceInviteAcceptRequest]) (*connect.Response[apiv2.TenantServiceInviteAcceptResponse], error) {
	var (
		t, ok = token.TokenFromContext(ctx)
		req   = rq.Msg
	)

	if !ok || t == nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("no token found in request"))
	}

	inv, err := u.inviteStore.GetInvite(ctx, req.Secret)
	if err != nil {
		if errors.Is(err, invite.ErrInviteNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("the given invitation does not exist anymore"))
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	invitee, err := u.repo.Tenant().Get(ctx, t.UserId)
	if err != nil {
		return nil, err
	}

	if invitee.Meta.Id == inv.TargetTenant {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("an owner cannot accept invitations to own tenants"))
	}

	memberships, err := u.repo.Tenant().Member(inv.TargetTenant).List(ctx, &repository.TenantMemberQuery{
		MemberId: &invitee.Meta.Id,
	})
	if err != nil {
		return nil, err
	}

	if len(memberships) > 0 {
		return nil, connect.NewError(connect.CodeAlreadyExists, fmt.Errorf("%s is already member of tenant %s", invitee.Meta.Id, inv.TargetTenant))
	}

	err = u.inviteStore.DeleteInvite(ctx, inv)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	validatedMember, err := u.repo.Tenant().Member(inv.TargetTenant).ValidateCreate(ctx, &repository.TenantMemberCreateRequest{
		MemberID: invitee.Meta.Id,
		Role:     inv.Role,
	})
	if err != nil {
		return nil, err
	}

	_, err = u.repo.Tenant().Member(inv.TargetTenant).Create(ctx, validatedMember)
	if err != nil {
		return nil, err
	}

	return connect.NewResponse(&apiv2.TenantServiceInviteAcceptResponse{Tenant: inv.TargetTenant, TenantName: inv.TargetTenantName}), nil
}

func (u *tenantServiceServer) InviteDelete(ctx context.Context, rq *connect.Request[apiv2.TenantServiceInviteDeleteRequest]) (*connect.Response[apiv2.TenantServiceInviteDeleteResponse], error) {
	var (
		req = rq.Msg
	)

	err := u.inviteStore.DeleteInvite(ctx, &apiv2.TenantInvite{Secret: req.Secret, TargetTenant: req.Login})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	u.log.Debug("tenant invite deleted")

	return connect.NewResponse(&apiv2.TenantServiceInviteDeleteResponse{}), nil
}

func (u *tenantServiceServer) InviteGet(ctx context.Context, rq *connect.Request[apiv2.TenantServiceInviteGetRequest]) (*connect.Response[apiv2.TenantServiceInviteGetResponse], error) {
	var (
		req = rq.Msg
	)

	inv, err := u.inviteStore.GetInvite(ctx, req.Secret)
	if err != nil {
		if errors.Is(err, invite.ErrInviteNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("the given invitation does not exist anymore"))
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(&apiv2.TenantServiceInviteGetResponse{Invite: inv}), nil
}

func (u *tenantServiceServer) InvitesList(ctx context.Context, rq *connect.Request[apiv2.TenantServiceInvitesListRequest]) (*connect.Response[apiv2.TenantServiceInvitesListResponse], error) {
	var (
		req = rq.Msg
	)
	invites, err := u.inviteStore.ListInvites(ctx, req.Login)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(&apiv2.TenantServiceInvitesListResponse{Invites: invites}), nil
}

func (u *tenantServiceServer) RemoveMember(ctx context.Context, rq *connect.Request[apiv2.TenantServiceRemoveMemberRequest]) (*connect.Response[apiv2.TenantServiceRemoveMemberResponse], error) {
	var (
		req = rq.Msg
	)

	membership, err := u.repo.Tenant().Member(req.Login).Get(ctx, req.MemberId)
	if err != nil {
		return nil, err
	}

	if membership.MemberId == membership.TenantId {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("cannot remove a member from their own default tenant"))
	}

	lastOwner, err := u.checkIfMemberIsLastOwner(ctx, membership)
	if err != nil {
		return nil, err
	}
	if lastOwner {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("cannot remove last owner of a tenant"))
	}

	validatedMember, err := u.repo.Tenant().Member(req.Login).ValidateDelete(ctx, membership)
	if err != nil {
		return nil, err
	}

	_, err = u.repo.Tenant().Member(req.Login).Delete(ctx, validatedMember)
	if err != nil {
		return nil, err
	}

	return connect.NewResponse(&apiv2.TenantServiceRemoveMemberResponse{}), nil
}

func (u *tenantServiceServer) UpdateMember(ctx context.Context, rq *connect.Request[apiv2.TenantServiceUpdateMemberRequest]) (*connect.Response[apiv2.TenantServiceUpdateMemberResponse], error) {
	var (
		req = rq.Msg
	)

	membership, err := u.repo.Tenant().Member(req.Login).Get(ctx, req.MemberId)
	if err != nil {
		return nil, err
	}

	if membership.MemberId == membership.TenantId && req.Role != apiv2.TenantRole_TENANT_ROLE_OWNER {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("cannot demote a user's role within their own default tenant"))
	}

	lastOwner, err := u.checkIfMemberIsLastOwner(ctx, membership)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	if lastOwner && req.Role != apiv2.TenantRole_TENANT_ROLE_OWNER {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("cannot demote last owner's permissions"))
	}

	if req.Role != apiv2.TenantRole_TENANT_ROLE_UNSPECIFIED {
		// TODO: currently the API defines that only owners can update members so there is no possibility to elevate permissions
		// probably, we should still check that no elevation of permissions is possible in case we later change the API

		membership.Meta.Annotations[tutil.TenantRoleAnnotation] = req.Role.String()
	}

	validatedMember, err := u.repo.Tenant().Member(req.Login).ValidateUpdate(ctx, &repository.TenantMemberUpdateRequest{Member: membership})
	if err != nil {
		return nil, err
	}

	updatedMember, err := u.repo.Tenant().Member(req.Login).Update(ctx, validatedMember)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(&apiv2.TenantServiceUpdateMemberResponse{TenantMember: &apiv2.TenantMember{
		Id:        req.MemberId,
		Role:      req.Role,
		CreatedAt: updatedMember.Meta.CreatedTime,
	}}), nil
}

func (u *tenantServiceServer) checkIfMemberIsLastOwner(ctx context.Context, membership *mdcv1.TenantMember) (bool, error) {
	isOwner := tutil.TenantRoleFromMap(membership.Meta.Annotations) == apiv2.TenantRole_TENANT_ROLE_OWNER
	if !isOwner {
		return false, nil
	}

	members, err := u.repo.Tenant().Member(membership.TenantId).List(ctx, &repository.TenantMemberQuery{
		Annotations: map[string]string{
			tutil.TenantRoleAnnotation: apiv2.TenantRole_TENANT_ROLE_OWNER.String(),
		},
	})
	if err != nil {
		return false, err
	}

	return len(members) < 2, nil
}
