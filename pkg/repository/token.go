package repository

import (
	"context"
	"maps"
	"slices"
	"time"

	"github.com/metal-stack/api/go/errorutil"
	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/repository/api"
	"github.com/metal-stack/metal-apiserver/pkg/request"
	"github.com/metal-stack/metal-apiserver/pkg/token"
	"github.com/samber/lo"
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
	tok.Permissions = flattenTypedTokenPermissions(req.Permissions)
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
		tok.Permissions = flattenTypedTokenPermissions(req.Permissions)
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
	token.Permissions = flattenTypedTokenPermissions(req.Permissions)
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

	currentToken, err := t.s.tokens.Get(ctx, t.scope.user, uuid)
	if err != nil {
		return nil, err
	}

	tok, ok := token.TokenFromContext(ctx)
	if !ok || tok == nil {
		return nil, errorutil.Unauthenticated("no token found in request")
	}

	err = t.validateTokenRequest(ctx, tok, currentToken)
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
		Permissions:  tok.Permissions,
		ProjectRoles: projectsAndTenants.ProjectRoles,
		TenantRoles:  projectsAndTenants.TenantRoles,
		AdminRole:    nil,
		InfraRole:    tok.InfraRole,
		MachineRoles: tok.MachineRoles,
	}
	if role, ok := t.hasAdminRole(projectsAndTenants); ok {
		fullUserToken.AdminRole = role
	}
	err = t.validateTokenRequest(ctx, fullUserToken, currentToken)
	if err != nil {
		return nil, errorutil.NewPermissionDenied(err)
	}

	// now follows the refresh, aka create a new token

	privateKey, err := t.s.certs.LatestPrivate(ctx)
	if err != nil {
		return nil, errorutil.NewInternal(err)
	}

	// New duration is calculated from the old token
	exp := currentToken.Expires.AsTime().Sub(currentToken.IssuedAt.AsTime())

	secret, newToken, err := token.NewJWT(apiv2.TokenType_TOKEN_TYPE_API, t.scope.user, t.s.issuer, exp, privateKey)
	if err != nil {
		return nil, errorutil.NewInternal(err)
	}

	newToken.Description = currentToken.Description
	newToken.Permissions = currentToken.Permissions
	newToken.ProjectRoles = currentToken.ProjectRoles
	newToken.TenantRoles = currentToken.TenantRoles
	newToken.AdminRole = currentToken.AdminRole
	newToken.InfraRole = currentToken.InfraRole
	newToken.MachineRoles = currentToken.MachineRoles
	if newToken.Meta == nil {
		newToken.Meta = &apiv2.Meta{}
	}
	newToken.Meta.Generation = 1
	newToken.Meta.CreatedAt = newToken.IssuedAt
	if currentToken.Meta != nil {
		newToken.Meta.Labels = currentToken.Meta.Labels
	}

	err = t.s.tokens.Set(ctx, newToken)
	if err != nil {
		return nil, err
	}

	// TODO, should we delete the old token now ?

	return &apiv2.TokenServiceRefreshResponse{
		Token:  newToken,
		Secret: secret,
	}, nil
}

func flattenTypedTokenPermissions(typed []*apiv2.TypedMethodPermission) []*apiv2.MethodPermission {
	res := make([]*apiv2.MethodPermission, len(typed))

	for i, p := range typed {
		switch p.Permissiontype.(type) {
		case *apiv2.TypedMethodPermission_Admin:
			res[i] = &apiv2.MethodPermission{
				Subject: "",
				Methods: p.GetAdmin().GetMethods(),
			}
		case *apiv2.TypedMethodPermission_Infra:
			res[i] = &apiv2.MethodPermission{
				Subject: "",
				Methods: p.GetInfra().GetMethods(),
			}
		case *apiv2.TypedMethodPermission_Machine:
			res[i] = &apiv2.MethodPermission{
				Subject: p.GetMachine().GetUuid(),
				Methods: p.GetMachine().GetMethods(),
			}
		case *apiv2.TypedMethodPermission_Project:
			res[i] = &apiv2.MethodPermission{
				Subject: p.GetProject().GetProject(),
				Methods: p.GetProject().GetMethods(),
			}
		case *apiv2.TypedMethodPermission_Public:
			res[i] = &apiv2.MethodPermission{
				Subject: "",
				Methods: p.GetPublic().GetMethods(),
			}
		case *apiv2.TypedMethodPermission_Self:
			res[i] = &apiv2.MethodPermission{
				Subject: "",
				Methods: p.GetSelf().GetMethods(),
			}
		case *apiv2.TypedMethodPermission_Tenant:
			res[i] = &apiv2.MethodPermission{
				Subject: p.GetTenant().GetLogin(),
				Methods: p.GetTenant().GetMethods(),
			}
		}
	}

	return res
}

func CompactTypedMethodPermissions(perms []*apiv2.TypedMethodPermission) []*apiv2.TypedMethodPermission {
	var res []*apiv2.TypedMethodPermission

	for _, p := range perms {
		switch p.Permissiontype.(type) {
		case *apiv2.TypedMethodPermission_Admin:
			var (
				idx = slices.IndexFunc(res, func(perm *apiv2.TypedMethodPermission) bool {
					return perm.GetAdmin() != nil
				})
				methods = p.GetAdmin().Methods
			)

			if idx < 0 {
				slices.Sort(methods)
				methods = lo.Uniq(methods)

				res = append(res, &apiv2.TypedMethodPermission{
					Permissiontype: &apiv2.TypedMethodPermission_Admin{
						Admin: &apiv2.AdminPermissions{
							Methods: methods,
						},
					},
				})

				continue
			}

			res[idx].GetAdmin().Methods = append(res[idx].GetAdmin().Methods, methods...)

			slices.Sort(res[idx].GetAdmin().Methods)
			res[idx].GetAdmin().Methods = lo.Uniq(res[idx].GetAdmin().Methods)

		case *apiv2.TypedMethodPermission_Infra:
			var (
				idx = slices.IndexFunc(res, func(perm *apiv2.TypedMethodPermission) bool {
					return perm.GetInfra() != nil
				})
				methods = p.GetInfra().Methods
			)

			if idx < 0 {
				slices.Sort(methods)
				methods = lo.Uniq(methods)

				res = append(res, &apiv2.TypedMethodPermission{
					Permissiontype: &apiv2.TypedMethodPermission_Infra{
						Infra: &apiv2.InfraPermissions{
							Methods: methods,
						},
					},
				})

				continue
			}

			res[idx].GetInfra().Methods = append(res[idx].GetInfra().Methods, methods...)

			slices.Sort(res[idx].GetInfra().Methods)
			res[idx].GetInfra().Methods = lo.Uniq(res[idx].GetInfra().Methods)

		case *apiv2.TypedMethodPermission_Machine:
			var (
				idx = slices.IndexFunc(res, func(perm *apiv2.TypedMethodPermission) bool {
					return perm.GetMachine() != nil
				})
				methods = p.GetMachine().Methods
			)

			if idx < 0 {
				slices.Sort(methods)
				methods = lo.Uniq(methods)

				res = append(res, &apiv2.TypedMethodPermission{
					Permissiontype: &apiv2.TypedMethodPermission_Machine{
						Machine: &apiv2.MachinePermissions{
							Uuid:    p.GetMachine().GetUuid(),
							Methods: methods,
						},
					},
				})

				continue
			}

			res[idx].GetMachine().Methods = append(res[idx].GetMachine().Methods, methods...)

			slices.Sort(res[idx].GetMachine().Methods)
			res[idx].GetMachine().Methods = lo.Uniq(res[idx].GetMachine().Methods)

		case *apiv2.TypedMethodPermission_Project:
			var (
				idx = slices.IndexFunc(res, func(perm *apiv2.TypedMethodPermission) bool {
					return perm.GetProject() != nil
				})
				methods = p.GetProject().Methods
			)

			if idx < 0 {
				slices.Sort(methods)
				methods = lo.Uniq(methods)

				res = append(res, &apiv2.TypedMethodPermission{
					Permissiontype: &apiv2.TypedMethodPermission_Project{
						Project: &apiv2.ProjectPermissions{
							Project: p.GetProject().GetProject(),
							Methods: methods,
						},
					},
				})

				continue
			}

			res[idx].GetProject().Methods = append(res[idx].GetProject().Methods, methods...)

			slices.Sort(res[idx].GetProject().Methods)
			res[idx].GetProject().Methods = lo.Uniq(res[idx].GetProject().Methods)

		case *apiv2.TypedMethodPermission_Public:
			var (
				idx = slices.IndexFunc(res, func(perm *apiv2.TypedMethodPermission) bool {
					return perm.GetPublic() != nil
				})
				methods = p.GetPublic().Methods
			)

			if idx < 0 {
				slices.Sort(methods)
				methods = lo.Uniq(methods)

				res = append(res, &apiv2.TypedMethodPermission{
					Permissiontype: &apiv2.TypedMethodPermission_Public{
						Public: &apiv2.PublicPermissions{
							Methods: methods,
						},
					},
				})

				continue
			}

			res[idx].GetPublic().Methods = append(res[idx].GetPublic().Methods, methods...)

			slices.Sort(res[idx].GetPublic().Methods)
			res[idx].GetPublic().Methods = lo.Uniq(res[idx].GetPublic().Methods)

		case *apiv2.TypedMethodPermission_Self:
			var (
				idx = slices.IndexFunc(res, func(perm *apiv2.TypedMethodPermission) bool {
					return perm.GetSelf() != nil
				})
				methods = p.GetSelf().Methods
			)

			if idx < 0 {
				slices.Sort(methods)
				methods = lo.Uniq(methods)

				res = append(res, &apiv2.TypedMethodPermission{
					Permissiontype: &apiv2.TypedMethodPermission_Self{
						Self: &apiv2.SelfPermissions{
							Methods: methods,
						},
					},
				})

				continue
			}

			res[idx].GetSelf().Methods = append(res[idx].GetSelf().Methods, methods...)

			slices.Sort(res[idx].GetSelf().Methods)
			res[idx].GetSelf().Methods = lo.Uniq(res[idx].GetSelf().Methods)

		case *apiv2.TypedMethodPermission_Tenant:
			var (
				idx = slices.IndexFunc(res, func(perm *apiv2.TypedMethodPermission) bool {
					return perm.GetTenant() != nil
				})
				methods = p.GetTenant().Methods
			)

			if idx < 0 {
				slices.Sort(methods)
				methods = lo.Uniq(methods)

				res = append(res, &apiv2.TypedMethodPermission{
					Permissiontype: &apiv2.TypedMethodPermission_Tenant{
						Tenant: &apiv2.TenantPermissions{
							Login:   p.GetTenant().GetLogin(),
							Methods: methods,
						},
					},
				})

				continue
			}

			res[idx].GetTenant().Methods = append(res[idx].GetTenant().Methods, methods...)

			slices.Sort(res[idx].GetTenant().Methods)
			res[idx].GetTenant().Methods = lo.Uniq(res[idx].GetTenant().Methods)

		}
	}

	return res
}
