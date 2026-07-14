package repository

import (
	"context"
	"maps"
	"time"

	"github.com/metal-stack/api/go/errorutil"
	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/repository/api"
	"github.com/metal-stack/metal-apiserver/pkg/request"
	"github.com/metal-stack/metal-apiserver/pkg/token"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type (
	tokenRepository struct {
		s          *Store
		scope      *UserScope
		patg       api.ProjectsAndTenantsGetter
		authorizer request.Authorizer
	}
)

func (t *tokenRepository) get(ctx context.Context, id string) (*api.TokenWithSecret, error) {
	if t.scope == nil {
		return nil, errorutil.FailedPrecondition("tokens cannot be retrieved unscoped")
	}

	res, err := t.s.tokens.Get(ctx, t.scope.user, id)
	if err != nil {
		return nil, err
	}

	return &api.TokenWithSecret{
		Token: res,
	}, nil
}

func (t *tokenRepository) list(ctx context.Context, query *apiv2.TokenQuery) ([]*api.TokenWithSecret, error) {
	var tokens []*apiv2.Token

	if t.scope == nil {
		var err error

		tokens, err = t.s.tokens.AdminList(ctx)
		if err != nil {
			return nil, err
		}
	} else {
		var err error

		tokens, err = t.s.tokens.List(ctx, t.scope.user)
		if err != nil {
			return nil, err
		}
	}

	var result []*api.TokenWithSecret

	for _, tok := range tokens {
		entity := &api.TokenWithSecret{
			Token: tok,
		}

		if !t.matchScope(entity) {
			// due to the list and admin list this is theoretically not necessary, but just for safety
			continue
		}

		if query == nil {
			result = append(result, entity)

			continue
		}

		if query.Description != nil && *query.Description != tok.Description {
			continue
		}
		if query.TokenType != nil && *query.TokenType != tok.TokenType {
			continue
		}
		if query.User != nil && *query.User != tok.User {
			continue
		}
		if query.Uuid != nil && *query.Uuid != tok.Uuid {
			continue
		}
		if query.Labels != nil {
			if tok.Meta == nil || tok.Meta.Labels == nil {
				continue
			}

			if !maps.Equal(query.Labels.Labels, tok.Meta.Labels.Labels) {
				continue
			}
		}

		result = append(result, entity)
	}

	return result, nil
}

func (t *tokenRepository) find(ctx context.Context, query *apiv2.TokenQuery) (*api.TokenWithSecret, error) {
	if query == nil {
		return nil, errorutil.InvalidArgument("query must be specified")
	}

	toks, err := t.list(ctx, query)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	switch len(toks) {
	case 0:
		return nil, errorutil.NotFound("unable to find a token by the specified query")
	case 1:
		// noop
	default:
		return nil, errorutil.Internal("found multiple tokens by the specified query")
	}

	return toks[0], nil
}

func (t *tokenRepository) create(ctx context.Context, c *adminv2.TokenServiceCreateRequest) (*api.TokenWithSecret, error) {
	if t.scope == nil {
		return nil, errorutil.FailedPrecondition("tokens cannot be created unscoped")
	}

	user := t.scope.user
	if c.User != nil {
		user = *c.User
	}

	privateKey, err := t.s.certs.LatestPrivate(ctx)
	if err != nil {
		return nil, errorutil.NewInternal(err)
	}

	req := c.TokenCreateRequest

	secret, tok, err := token.NewJWT(apiv2.TokenType_TOKEN_TYPE_API, user, t.s.issuer, req.Expires.AsDuration(), privateKey)
	if err != nil {
		return nil, errorutil.NewInternal(err)
	}

	tok.Description = req.Description
	tok.Permissions = req.Permissions
	tok.ProjectRoles = req.ProjectRoles
	tok.TenantRoles = req.TenantRoles
	tok.AdminRole = req.AdminRole
	tok.InfraRole = req.InfraRole
	tok.MachineRoles = req.MachineRoles

	if tok.Meta == nil {
		tok.Meta = &apiv2.Meta{}
	}
	tok.Meta.Generation = 1
	tok.Meta.CreatedAt = tok.IssuedAt
	tok.Meta.Labels = req.Labels

	err = t.s.tokens.Set(ctx, tok)
	if err != nil {
		return nil, err
	}

	return &api.TokenWithSecret{
		Token:  tok,
		Secret: secret,
	}, nil
}

func (t *tokenRepository) delete(ctx context.Context, e *api.TokenWithSecret) (*deleteInfo, error) {
	if t.scope == nil {
		return nil, errorutil.FailedPrecondition("tokens cannot be revoked unscoped")
	}

	err := t.s.tokens.Revoke(ctx, t.scope.user, e.Token.Uuid)
	if err != nil {
		return nil, err
	}

	return nil, nil
}

func (t *tokenRepository) update(ctx context.Context, tokenToUpdate *api.TokenWithSecret, req *apiv2.TokenServiceUpdateRequest) (*api.TokenWithSecret, error) {
	if t.scope == nil {
		return nil, errorutil.FailedPrecondition("tokens cannot be updated unscoped")
	}

	tok := tokenToUpdate.Token

	if tok.TokenType != apiv2.TokenType_TOKEN_TYPE_API {
		return nil, errorutil.FailedPrecondition("only updating API tokens is currently supported")
	}

	if req.Description != nil {
		tok.Description = *req.Description
	}

	if req.AdminRole != nil {
		if *req.AdminRole == apiv2.AdminRole_ADMIN_ROLE_UNSPECIFIED {
			tok.AdminRole = nil
		} else {
			tok.AdminRole = req.AdminRole
		}
	}
	if req.Permissions != nil {
		tok.Permissions = req.Permissions
	}
	if req.ProjectRoles != nil {
		tok.ProjectRoles = req.ProjectRoles
	}
	if req.TenantRoles != nil {
		tok.TenantRoles = req.TenantRoles
	}
	if req.InfraRole != nil {
		tok.InfraRole = req.InfraRole
	}
	if req.MachineRoles != nil {
		tok.MachineRoles = req.MachineRoles
	}

	if tok.Meta == nil {
		tok.Meta = &apiv2.Meta{}
	}
	tok.Meta.Generation++
	tok.Meta.UpdatedAt = timestamppb.Now()

	if req.Labels != nil {
		if tok.Meta.Labels == nil {
			tok.Meta.Labels = &apiv2.Labels{}
		}

		tok.Meta.Labels.Labels = updateLabelsOnMap(req.Labels, tok.Meta.Labels.Labels)
	}

	err := t.s.tokens.Set(ctx, tok)
	if err != nil {
		return nil, err
	}

	return &api.TokenWithSecret{
		Token: tok,
	}, nil
}

func (t *tokenRepository) matchScope(e *api.TokenWithSecret) bool {
	if t.scope == nil {
		return true
	}

	return e.Token.User == t.scope.user
}

func (t *tokenRepository) convertToInternal(ctx context.Context, msg *api.TokenWithSecret) (*api.TokenWithSecret, error) {
	return msg, nil
}

func (t *tokenRepository) convertToProto(ctx context.Context, e *api.TokenWithSecret) (*api.TokenWithSecret, error) {
	return e, nil
}

func (t *tokenRepository) CreateUserTokenWithoutPermissionCheck(ctx context.Context, subject string, expiration *time.Duration) (*apiv2.TokenServiceCreateResponse, error) {
	if t.scope != nil {
		return nil, errorutil.FailedPrecondition("tokens without permission check can only be created unscoped")
	}

	expires := token.DefaultExpiration
	if expiration != nil {
		expires = *expiration
	}

	privateKey, err := t.s.certs.LatestPrivate(ctx)
	if err != nil {
		return nil, errorutil.Internal("unable to fetch signing certificate: %w", err)
	}

	secret, token, err := token.NewJWT(apiv2.TokenType_TOKEN_TYPE_USER, subject, t.s.issuer, expires, privateKey)
	if err != nil {
		return nil, errorutil.Internal("unable to create console token: %w", err)
	}

	err = t.s.tokens.Set(ctx, token)
	if err != nil {
		return nil, err
	}

	return &apiv2.TokenServiceCreateResponse{
		Token:  token,
		Secret: secret,
	}, nil
}

func (t *tokenRepository) CreateApiTokenWithoutPermissionCheck(ctx context.Context, subject string, req *apiv2.TokenServiceCreateRequest) (*apiv2.TokenServiceCreateResponse, error) {
	if t.scope != nil {
		return nil, errorutil.FailedPrecondition("tokens without permission check can only be created unscoped")
	}

	expires := token.DefaultExpiration
	if req.Expires != nil {
		expires = req.Expires.AsDuration()
	}

	privateKey, err := t.s.certs.LatestPrivate(ctx)
	if err != nil {
		return nil, errorutil.NewInternal(err)
	}

	secret, token, err := token.NewJWT(apiv2.TokenType_TOKEN_TYPE_API, subject, t.s.issuer, expires, privateKey)
	if err != nil {
		return nil, errorutil.NewInternal(err)
	}

	token.Description = req.Description
	token.Permissions = req.Permissions
	token.ProjectRoles = req.ProjectRoles
	token.TenantRoles = req.TenantRoles
	token.AdminRole = req.AdminRole
	token.InfraRole = req.InfraRole
	token.MachineRoles = req.MachineRoles

	err = t.s.tokens.Set(ctx, token)
	if err != nil {
		return nil, err
	}

	return &apiv2.TokenServiceCreateResponse{
		Token:  token,
		Secret: secret,
	}, nil
}

func (t *tokenRepository) Refresh(ctx context.Context, uuid string) (*apiv2.TokenServiceRefreshResponse, error) {
	if t.scope == nil {
		return nil, errorutil.FailedPrecondition("tokens cannot be refreshed unscoped")
	}

	oldtoken, err := t.s.tokens.Get(ctx, t.scope.user, uuid)
	if err != nil {
		return nil, err
	}

	// we first copy the token permission from the old token
	createRequest := &apiv2.TokenServiceCreateRequest{
		Permissions:  oldtoken.Permissions,
		ProjectRoles: oldtoken.ProjectRoles,
		TenantRoles:  oldtoken.TenantRoles,
		AdminRole:    oldtoken.AdminRole,
		InfraRole:    oldtoken.InfraRole,
		MachineRoles: oldtoken.MachineRoles,
	}

	tok, ok := token.TokenFromContext(ctx)
	if !ok || tok == nil {
		return nil, errorutil.Unauthenticated("no token found in request")
	}

	err = t.validateTokenRequest(ctx, tok, createRequest)
	if err != nil {
		return nil, errorutil.NewPermissionDenied(err)
	}

	// now, we validate if the user is still permitted to refresh the token
	// doing this check is not strictly necessary because the resulting token would fail in the auther when being compared
	// to the actual user permissions, but it's nicer for the user to already prevent token update immediately in this place

	projectsAndTenants, err := t.patg(ctx, t.scope.user)
	if err != nil {
		return nil, errorutil.NewInternal(err)
	}
	fullUserToken := &apiv2.Token{
		User:         t.scope.user,
		ProjectRoles: projectsAndTenants.ProjectRoles,
		TenantRoles:  projectsAndTenants.TenantRoles,
		AdminRole:    nil,
		InfraRole:    tok.InfraRole,
		MachineRoles: tok.MachineRoles,
	}
	if role, ok := t.hasAdminRole(projectsAndTenants); ok {
		fullUserToken.AdminRole = role
	}
	err = t.validateTokenRequest(ctx, fullUserToken, createRequest)
	if err != nil {
		return nil, errorutil.NewPermissionDenied(err)
	}

	// now follows the refresh, aka create a new token

	privateKey, err := t.s.certs.LatestPrivate(ctx)
	if err != nil {
		return nil, errorutil.NewInternal(err)
	}

	// New duration is calculated from the old token
	exp := oldtoken.Expires.AsTime().Sub(oldtoken.IssuedAt.AsTime())

	secret, newToken, err := token.NewJWT(apiv2.TokenType_TOKEN_TYPE_API, t.scope.user, t.s.issuer, exp, privateKey)
	if err != nil {
		return nil, errorutil.NewInternal(err)
	}

	newToken.Description = oldtoken.Description
	newToken.Permissions = oldtoken.Permissions
	newToken.ProjectRoles = oldtoken.ProjectRoles
	newToken.TenantRoles = oldtoken.TenantRoles
	newToken.AdminRole = oldtoken.AdminRole
	newToken.InfraRole = oldtoken.InfraRole
	newToken.MachineRoles = oldtoken.MachineRoles
	if newToken.Meta == nil {
		newToken.Meta = &apiv2.Meta{}
	}
	newToken.Meta.Generation = 1
	newToken.Meta.CreatedAt = newToken.IssuedAt
	if oldtoken.Meta != nil {
		newToken.Meta.Labels = oldtoken.Meta.Labels
	}

	err = t.s.tokens.Set(ctx, newToken)
	if err != nil {
		return nil, err
	}

	return &apiv2.TokenServiceRefreshResponse{
		Token:  newToken,
		Secret: secret,
	}, nil
}
