package repository

import (
	"context"
	"maps"

	"github.com/metal-stack/api/go/errorutil"
	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/repository/api"
	"github.com/metal-stack/metal-apiserver/pkg/request"
	"github.com/metal-stack/metal-apiserver/pkg/token"
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

func (t *tokenRepository) update(ctx context.Context, tok *api.TokenWithSecret, req *apiv2.TokenServiceUpdateRequest) (*api.TokenWithSecret, error) {
	panic("unimplemented")
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
