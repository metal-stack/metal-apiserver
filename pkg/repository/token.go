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
	tok.Permissions = flattenPermissions(req.Permissions)
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
		tok.Permissions = flattenPermissions(req.Permissions)
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
	token.Permissions = flattenPermissions(req.Permissions)
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

	return &apiv2.TokenServiceRefreshResponse{
		Token:  newToken,
		Secret: secret,
	}, nil
}

func flattenPermissions(perms []*apiv2.PermissionsByVisibility) []*apiv2.MethodPermission {
	res := make([]*apiv2.MethodPermission, len(perms))

	for i, p := range perms {
		switch p.Visibility.(type) {
		case *apiv2.PermissionsByVisibility_Admin:
			res[i] = &apiv2.MethodPermission{
				Subject: "",
				Methods: p.GetAdmin().GetMethods(),
			}
		case *apiv2.PermissionsByVisibility_Infra:
			res[i] = &apiv2.MethodPermission{
				Subject: "",
				Methods: p.GetInfra().GetMethods(),
			}
		case *apiv2.PermissionsByVisibility_Machine:
			res[i] = &apiv2.MethodPermission{
				Subject: p.GetMachine().GetUuid(),
				Methods: p.GetMachine().GetMethods(),
			}
		case *apiv2.PermissionsByVisibility_Project:
			res[i] = &apiv2.MethodPermission{
				Subject: p.GetProject().GetProject(),
				Methods: p.GetProject().GetMethods(),
			}
		case *apiv2.PermissionsByVisibility_Public:
			res[i] = &apiv2.MethodPermission{
				Subject: "",
				Methods: p.GetPublic().GetMethods(),
			}
		case *apiv2.PermissionsByVisibility_Self:
			res[i] = &apiv2.MethodPermission{
				Subject: "",
				Methods: p.GetSelf().GetMethods(),
			}
		case *apiv2.PermissionsByVisibility_Tenant:
			res[i] = &apiv2.MethodPermission{
				Subject: p.GetTenant().GetLogin(),
				Methods: p.GetTenant().GetMethods(),
			}
		}
	}

	return res
}

func compactPermissions(perms []*apiv2.PermissionsByVisibility) []*apiv2.PermissionsByVisibility {
	var res []*apiv2.PermissionsByVisibility

	for _, p := range perms {
		switch v := p.Visibility.(type) {
		case *apiv2.PermissionsByVisibility_Admin:
			idx := slices.IndexFunc(res, func(perm *apiv2.PermissionsByVisibility) bool {
				return perm.GetAdmin() != nil
			})
			if idx < 0 {
				res = append(res, &apiv2.PermissionsByVisibility{
					Visibility: &apiv2.PermissionsByVisibility_Admin{
						Admin: &apiv2.AdminPermissions{
							Methods: sortUniq(v.Admin.Methods),
						},
					},
				})
				continue
			}
			res[idx].GetAdmin().Methods = sortUniq(append(res[idx].GetAdmin().Methods, v.Admin.Methods...))

		case *apiv2.PermissionsByVisibility_Infra:
			idx := slices.IndexFunc(res, func(perm *apiv2.PermissionsByVisibility) bool {
				return perm.GetInfra() != nil
			})
			if idx < 0 {
				res = append(res, &apiv2.PermissionsByVisibility{
					Visibility: &apiv2.PermissionsByVisibility_Infra{
						Infra: &apiv2.InfraPermissions{
							Methods: sortUniq(v.Infra.Methods),
						},
					},
				})
				continue
			}
			res[idx].GetInfra().Methods = sortUniq(append(res[idx].GetInfra().Methods, v.Infra.Methods...))

		case *apiv2.PermissionsByVisibility_Machine:
			idx := slices.IndexFunc(res, func(perm *apiv2.PermissionsByVisibility) bool {
				m := perm.GetMachine()
				return m != nil && m.Uuid == v.Machine.Uuid
			})
			if idx < 0 {
				res = append(res, &apiv2.PermissionsByVisibility{
					Visibility: &apiv2.PermissionsByVisibility_Machine{
						Machine: &apiv2.MachinePermissions{
							Uuid:    v.Machine.Uuid,
							Methods: sortUniq(v.Machine.Methods),
						},
					},
				})
				continue
			}
			res[idx].GetMachine().Methods = sortUniq(append(res[idx].GetMachine().Methods, v.Machine.Methods...))

		case *apiv2.PermissionsByVisibility_Project:
			idx := slices.IndexFunc(res, func(perm *apiv2.PermissionsByVisibility) bool {
				m := perm.GetProject()
				return m != nil && m.Project == v.Project.Project
			})
			if idx < 0 {
				res = append(res, &apiv2.PermissionsByVisibility{
					Visibility: &apiv2.PermissionsByVisibility_Project{
						Project: &apiv2.ProjectPermissions{
							Project: v.Project.Project,
							Methods: sortUniq(v.Project.Methods),
						},
					},
				})
				continue
			}
			res[idx].GetProject().Methods = sortUniq(append(res[idx].GetProject().Methods, v.Project.Methods...))

		case *apiv2.PermissionsByVisibility_Public:
			idx := slices.IndexFunc(res, func(perm *apiv2.PermissionsByVisibility) bool {
				return perm.GetPublic() != nil
			})
			if idx < 0 {
				res = append(res, &apiv2.PermissionsByVisibility{
					Visibility: &apiv2.PermissionsByVisibility_Public{
						Public: &apiv2.PublicPermissions{
							Methods: sortUniq(v.Public.Methods),
						},
					},
				})
				continue
			}
			res[idx].GetPublic().Methods = sortUniq(append(res[idx].GetPublic().Methods, v.Public.Methods...))

		case *apiv2.PermissionsByVisibility_Self:
			idx := slices.IndexFunc(res, func(perm *apiv2.PermissionsByVisibility) bool {
				return perm.GetSelf() != nil
			})
			if idx < 0 {
				res = append(res, &apiv2.PermissionsByVisibility{
					Visibility: &apiv2.PermissionsByVisibility_Self{
						Self: &apiv2.SelfPermissions{
							Methods: sortUniq(v.Self.Methods),
						},
					},
				})
				continue
			}
			res[idx].GetSelf().Methods = sortUniq(append(res[idx].GetSelf().Methods, v.Self.Methods...))

		case *apiv2.PermissionsByVisibility_Tenant:
			idx := slices.IndexFunc(res, func(perm *apiv2.PermissionsByVisibility) bool {
				m := perm.GetTenant()
				return m != nil && m.Login == v.Tenant.Login
			})
			if idx < 0 {
				res = append(res, &apiv2.PermissionsByVisibility{
					Visibility: &apiv2.PermissionsByVisibility_Tenant{
						Tenant: &apiv2.TenantPermissions{
							Login:   v.Tenant.Login,
							Methods: sortUniq(v.Tenant.Methods),
						},
					},
				})
				continue
			}
			res[idx].GetTenant().Methods = sortUniq(append(res[idx].GetTenant().Methods, v.Tenant.Methods...))
		}
	}

	return res
}

func sortUniq(s []string) []string {
	slices.Sort(s)
	return lo.Uniq(s)
}
