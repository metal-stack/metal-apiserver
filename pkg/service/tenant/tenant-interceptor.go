package tenant

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"connectrpc.com/connect"
	tutil "github.com/metal-stack/api-server/pkg/tenant"
	"github.com/metal-stack/api-server/pkg/token"
	"github.com/metal-stack/api/go/permissions"
	mdcv1 "github.com/metal-stack/masterdata-api/api/v1"
	mdc "github.com/metal-stack/masterdata-api/pkg/client"
	"github.com/metal-stack/metal-lib/pkg/cache"
	"github.com/metal-stack/metal-lib/pkg/pointer"
	"github.com/metal-stack/security"
)

type (
	tenantInterceptor struct {
		projectCache *cache.Cache[string, *mdcv1.Project]
		log          *slog.Logger
		masterClient mdc.Client
	}
)

func NewInterceptor(log *slog.Logger, masterClient mdc.Client) *tenantInterceptor {
	return &tenantInterceptor{
		projectCache: cache.New(1*time.Hour, func(ctx context.Context, id string) (*mdcv1.Project, error) {
			pgr, err := masterClient.Project().Get(ctx, &mdcv1.ProjectGetRequest{Id: id})
			if err != nil {
				return nil, fmt.Errorf("unable to get project: %w", err)
			}
			return pgr.GetProject(), nil
		}),
		log:          log,
		masterClient: masterClient,
	}
}

// TenantUnaryInterceptor will check if the request targets a project, if yes, checks if tenant of this project
// already exists, if not an error is returned.
func (i *tenantInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return connect.UnaryFunc(func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		var (
			tok, tokenInCtx = token.TokenFromContext(ctx)
			user            = &security.User{
				EMail:   "",
				Name:    "",
				Tenant:  "",
				Groups:  []security.ResourceAccess{},
				Issuer:  "",
				Subject: pointer.SafeDeref(tok).UserId,
			}

			setUserFieldsByTenantLookup = func(tenantID string) error {
				tgr, err := i.masterClient.Tenant().Get(ctx, &mdcv1.TenantGetRequest{Id: tenantID})
				if mdcv1.IsNotFound(err) {
					return connect.NewError(connect.CodeNotFound, err)
				}
				if err != nil {
					return connect.NewError(connect.CodeInternal, err)
				}

				user.Tenant = tgr.Tenant.Meta.Id
				user.EMail = tgr.Tenant.Meta.Annotations[tutil.TagEmail]

				// update the context with the user information BEFORE calling next
				ctx = security.PutUserInContext(ctx, user)

				return nil
			}
		)

		if getPublicScope(req) {
			i.log.Debug("tenant interceptor", "request-scope", "public")

			if tokenInCtx {
				err := setUserFieldsByTenantLookup(tok.UserId)
				if err != nil {
					return nil, err
				}

				user.Tenant = "" // public methods do not operate on a tenant, therefore erase again
			} else {
				ctx = security.PutUserInContext(ctx, user)
			}

			return next(ctx, req)
		}

		if !tokenInCtx {
			return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("token must be present when requesting non-public scope method"))
		}

		if getSelfScope(req) {
			i.log.Debug("tenant interceptor", "request-scope", "self")

			err := setUserFieldsByTenantLookup(tok.UserId)
			if err != nil {
				return nil, err
			}

			return next(ctx, req)
		}

		if getAdminScope(req) {
			i.log.Debug("tenant interceptor", "request-scope", "admin")

			err := setUserFieldsByTenantLookup(tok.UserId)
			if err != nil {
				return nil, err
			}

			user.Tenant = "" // public methods do not operate on a tenant, therefore erase again

			return next(ctx, req)
		}

		if tenantID, ok := getTenantScope(req); ok {
			i.log.Debug("tenant interceptor", "request-scope", "tenant", "id", tenantID)

			err := setUserFieldsByTenantLookup(tenantID)
			if err != nil {
				return nil, err
			}

			return next(ctx, req)
		}

		if projectID, ok := getProjectScope(req); ok {
			i.log.Debug("tenant interceptor", "request-scope", "project", "id", projectID)

			project, err := i.projectCache.Get(ctx, projectID)
			if mdcv1.IsNotFound(err) {
				return nil, connect.NewError(connect.CodeNotFound, err)
			}
			if err != nil {
				return nil, connect.NewError(connect.CodeInternal, err)
			}

			user.Project = projectID

			err = setUserFieldsByTenantLookup(project.TenantId)
			if err != nil {
				return nil, err
			}

			return next(ctx, req)
		}

		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("unable to determine request scope: %q", req.Spec().Procedure))
	})
}

func (i *tenantInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return connect.StreamingClientFunc(func(ctx context.Context, spec connect.Spec) connect.StreamingClientConn {
		return next(ctx, spec)
	})
}
func (i *tenantInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return connect.StreamingHandlerFunc(func(ctx context.Context, conn connect.StreamingHandlerConn) error {
		return next(ctx, conn)
	})
}

func getPublicScope(req connect.AnyRequest) bool {
	servicePermissions := permissions.GetServicePermissions()

	_, ok := servicePermissions.Visibility.Public[req.Spec().Procedure]
	return ok
}

func getSelfScope(req connect.AnyRequest) bool {
	servicePermissions := permissions.GetServicePermissions()

	_, ok := servicePermissions.Visibility.Self[req.Spec().Procedure]
	return ok
}

func getAdminScope(req connect.AnyRequest) bool {
	servicePermissions := permissions.GetServicePermissions()

	for _, proc := range servicePermissions.Roles.Admin {
		if slices.Contains(proc, req.Spec().Procedure) {
			return true
		}
	}

	return false
}

func getProjectScope(req connect.AnyRequest) (string, bool) {
	servicePermissions := permissions.GetServicePermissions()

	for _, proc := range servicePermissions.Roles.Project {
		if !slices.Contains(proc, req.Spec().Procedure) {
			continue
		}

		switch rq := req.Any().(type) {
		case interface{ GetProject() string }:
			return rq.GetProject(), true
		}
	}

	return "", false
}

func getTenantScope(req connect.AnyRequest) (string, bool) {
	servicePermissions := permissions.GetServicePermissions()

	for _, proc := range servicePermissions.Roles.Tenant {
		if !slices.Contains(proc, req.Spec().Procedure) {
			continue
		}

		switch rq := req.Any().(type) {
		case interface{ GetLogin() string }:
			return rq.GetLogin(), true
		}
	}

	return "", false
}
