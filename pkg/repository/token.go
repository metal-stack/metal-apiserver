package repository

import (
	"context"
	"slices"
	"time"

	"github.com/google/go-cmp/cmp"
	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	"github.com/metal-stack/metal-apiserver/pkg/repository/api"
	"github.com/metal-stack/metal-apiserver/pkg/token"
)

type (
	tokenRepository struct {
		s     *Store
		scope *UserScope
		patg  api.ProjectsAndTenantsGetter
	}
)

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

	secret, tok, err := token.NewJWT(apiv2.TokenType_TOKEN_TYPE_API, user, t.s.issuer, c.TokenCreateRequest.Expires.AsDuration(), privateKey)
	if err != nil {
		return nil, errorutil.NewInternal(err)
	}

	req := c.TokenCreateRequest

	tok.Description = req.Description
	tok.Permissions = req.Permissions
	tok.ProjectRoles = req.ProjectRoles
	tok.TenantRoles = req.TenantRoles
	tok.AdminRole = req.AdminRole
	tok.InfraRole = req.InfraRole
	tok.Meta = &apiv2.Meta{
		Labels:     req.Labels,
		CreatedAt:  tok.IssuedAt,
		Generation: 0,
	}

	err = t.s.tokens.Set(ctx, tok)
	if err != nil {
		return nil, errorutil.NewInternal(err)
	}

	return &api.TokenWithSecret{
		Token:  tok,
		Secret: secret,
	}, nil
}

func (t *tokenRepository) delete(ctx context.Context, e *api.TokenWithSecret) error {
	if t.scope == nil {
		return errorutil.FailedPrecondition("tokens cannot be revoked unscoped")
	}

	err := t.s.tokens.Revoke(ctx, t.scope.user, e.Token.Uuid)
	if err != nil {
		return err
	}

	return nil
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

		match := true

		if query.Description != nil {
			match = match && *query.Description == tok.Description
		}
		if query.TokenType != nil {
			match = match && *query.TokenType == tok.TokenType
		}
		if query.User != nil {
			match = match && *query.User == tok.User
		}
		if query.Uuid != nil {
			match = match && *query.Uuid == tok.Uuid
		}
		if query.Labels != nil {
			if tok.Meta == nil || tok.Meta.Labels == nil {
				continue
			}

			match = match && cmp.Equal(query.Labels.Labels, tok.Meta.Labels.Labels)
		}

		if match {
			result = append(result, entity)
		}
	}

	return result, nil
}

func (t *tokenRepository) update(ctx context.Context, tok *api.TokenWithSecret, req *apiv2.TokenServiceUpdateRequest) (*api.TokenWithSecret, error) {
	tokenToUpdate, err := t.s.tokens.Get(ctx, tok.Token.GetUser(), req.Uuid)
	if err != nil {
		return nil, err
	}

	if req.Description != nil {
		tokenToUpdate.Description = *req.Description
	}
	if req.AdminRole != nil {
		if *req.AdminRole == apiv2.AdminRole_ADMIN_ROLE_UNSPECIFIED {
			tokenToUpdate.AdminRole = nil
		} else {
			tokenToUpdate.AdminRole = req.AdminRole
		}
	}
	if req.Labels != nil {
		if tokenToUpdate.Meta == nil {
			tokenToUpdate.Meta = &apiv2.Meta{}
		}
		if tokenToUpdate.Meta.Labels == nil {
			tokenToUpdate.Meta.Labels = &apiv2.Labels{}
		}

		tokenToUpdate.Meta.Labels.Labels = updateLabelsOnMap(req.Labels, tok.Token.Meta.Labels.Labels)
	}

	tokenToUpdate.Permissions = req.Permissions
	tokenToUpdate.ProjectRoles = req.ProjectRoles
	tokenToUpdate.TenantRoles = req.TenantRoles

	err = t.s.tokens.Set(ctx, tokenToUpdate)
	if err != nil {
		return nil, err
	}

	return &api.TokenWithSecret{
		Token: tokenToUpdate,
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

func (t *tokenRepository) Refresh(ctx context.Context) (*api.TokenWithSecret, error) {
	tok, ok := token.TokenFromContext(ctx)
	if !ok || tok == nil {
		return nil, errorutil.Unauthenticated("no token found in request")
	}

	if _, err := t.s.Token(tok.User).Get(ctx, tok.Uuid); err != nil {
		return nil, err
	}

	// we first copy the token permission from the old token
	createRequest := &apiv2.TokenServiceCreateRequest{
		Permissions:  tok.Permissions,
		ProjectRoles: tok.ProjectRoles,
		TenantRoles:  tok.TenantRoles,
		AdminRole:    tok.AdminRole,
		InfraRole:    tok.InfraRole,
	}

	err := t.validateTokenRequest(ctx, tok, createRequest)
	if err != nil {
		return nil, errorutil.NewPermissionDenied(err)
	}

	// now, we validate if the user is still permitted to refresh the token
	// doing this check is not strictly necessary because the resulting token would fail in the auther when being compared
	// to the actual user permissions, but it's nicer for the user to already prevent token update immediately in this place

	projectsAndTenants, err := t.patg(ctx, tok.GetUser())
	if err != nil {
		return nil, errorutil.NewInternal(err)
	}
	fullUserToken := &apiv2.Token{
		User:         tok.User,
		ProjectRoles: projectsAndTenants.ProjectRoles,
		TenantRoles:  projectsAndTenants.TenantRoles,
		AdminRole:    nil,
		InfraRole:    tok.InfraRole,
	}
	if slices.Contains(t.s.adminSubjects, tok.User) {
		fullUserToken.AdminRole = apiv2.AdminRole_ADMIN_ROLE_EDITOR.Enum()
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
	exp := tok.Expires.AsTime().Sub(tok.IssuedAt.AsTime())

	secret, newToken, err := token.NewJWT(apiv2.TokenType_TOKEN_TYPE_API, tok.GetUser(), t.s.issuer, exp, privateKey)
	if err != nil {
		return nil, errorutil.NewInternal(err)
	}

	newToken.Description = tok.Description
	newToken.Permissions = tok.Permissions
	newToken.ProjectRoles = tok.ProjectRoles
	newToken.TenantRoles = tok.TenantRoles
	newToken.AdminRole = tok.AdminRole
	newToken.InfraRole = tok.InfraRole

	err = t.s.tokens.Set(ctx, newToken)
	if err != nil {
		return nil, err
	}

	return &api.TokenWithSecret{
		Token:  newToken,
		Secret: secret,
	}, nil
}

// CreateUserTokenWithoutPermissionCheck is only called from the auth service during login through console
// No validation against requested roles and permissions is required and implemented here
func (t *tokenRepository) CreateUserTokenWithoutPermissionCheck(ctx context.Context, subject string, expiration *time.Duration) (*api.TokenWithSecret, error) {
	if t.scope != nil {
		return nil, errorutil.FailedPrecondition("creating tokens without permission check must be called unscoped")
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
		return nil, errorutil.NewInternal(err)
	}

	return &api.TokenWithSecret{
		Token:  token,
		Secret: secret,
	}, nil
}

// CreateApiTokenWithoutPermissionCheck is only called from the api-server command line interface
// No validation against requested roles and permissions is required and implemented here
func (t *tokenRepository) CreateApiTokenWithoutPermissionCheck(ctx context.Context, subject string, req *apiv2.TokenServiceCreateRequest) (*api.TokenWithSecret, error) {
	if t.scope != nil {
		return nil, errorutil.FailedPrecondition("creating tokens without permission check must be called unscoped")
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

	err = t.s.tokens.Set(ctx, token)
	if err != nil {
		return nil, errorutil.NewInternal(err)
	}

	return &api.TokenWithSecret{
		Token:  token,
		Secret: secret,
	}, nil
}
