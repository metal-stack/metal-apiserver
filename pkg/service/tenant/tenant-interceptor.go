package tenant

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"connectrpc.com/connect"
	"github.com/metal-stack/api/go/permissions"
	mdcv1 "github.com/metal-stack/masterdata-api/api/v1"
	mdc "github.com/metal-stack/masterdata-api/pkg/client"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	"github.com/metal-stack/metal-apiserver/pkg/repository"
	"github.com/metal-stack/metal-apiserver/pkg/token"
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
				Subject: pointer.SafeDeref(tok).User,
			}

			setUserFieldsByTenantLookup = func(tenantID string) error {
				tgr, err := i.masterClient.Tenant().Get(ctx, &mdcv1.TenantGetRequest{Id: tenantID})
				if mdcv1.IsNotFound(err) {
					return errorutil.NewNotFound(err)
				}
				if err != nil {
					return errorutil.NewInternal(err)
				}

				user.Tenant = tgr.Tenant.Meta.Id
				user.EMail = tgr.Tenant.Meta.Annotations[repository.TenantTagEmail]

				// update the context with the user information BEFORE calling next
				ctx = security.PutUserInContext(ctx, user)

				return nil
			}
		)

		if permissions.IsPublicScope(req) {
			i.log.Debug("tenant interceptor", "request-scope", "public")

			if tokenInCtx {
				err := setUserFieldsByTenantLookup(tok.User)
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
			return nil, errorutil.Unauthenticated("token must be present when requesting non-public scope method")
		}

		if permissions.IsSelfScope(req) {
			i.log.Debug("tenant interceptor", "request-scope", "self")

			err := setUserFieldsByTenantLookup(tok.User)
			if err != nil {
				return nil, err
			}

			return next(ctx, req)
		}

		if permissions.IsAdminScope(req) {
			i.log.Debug("tenant interceptor", "request-scope", "admin")

			err := setUserFieldsByTenantLookup(tok.User)
			if err != nil {
				return nil, err
			}

			user.Tenant = "" // admin methods do not operate on a tenant, therefore erase again

			return next(ctx, req)
		}

		if permissions.IsMachineScope(req) {
			i.log.Debug("tenant interceptor", "request-scope", "machine")

			user := &security.User{
				Name: tok.User,
			}

			machineId, ok := permissions.GetMachineIdFromRequest(req)
			if ok {
				user.Subject = machineId
			}

			// machine methods do not operate on a tenant, therefore create the security user from the machine id
			ctx = security.PutUserInContext(ctx, user)

			return next(ctx, req)
		}

		if permissions.IsInfraScope(req) {
			i.log.Debug("tenant interceptor", "request-scope", "infra")

			// infra methods do not operate on a tenant, therefore create the security user from the token id
			ctx = security.PutUserInContext(ctx, &security.User{
				Name:    tok.User,
				Subject: tok.User,
			})

			return next(ctx, req)
		}

		if tenantID, ok := permissions.GetTenantFromRequest(req); ok {
			i.log.Debug("tenant interceptor", "request-scope", "tenant", "id", tenantID)

			err := setUserFieldsByTenantLookup(tenantID)
			if err != nil {
				return nil, err
			}

			return next(ctx, req)
		}

		if projectID, ok := permissions.GetProjectFromRequest(req); ok {
			i.log.Debug("tenant interceptor", "request-scope", "project", "id", projectID)

			project, err := i.projectCache.Get(ctx, projectID)
			if mdcv1.IsNotFound(err) {
				return nil, errorutil.NewNotFound(err)
			}
			if err != nil {
				return nil, errorutil.NewInternal(err)
			}

			user.Project = projectID

			err = setUserFieldsByTenantLookup(project.TenantId)
			if err != nil {
				return nil, err
			}

			return next(ctx, req)
		}

		return nil, errorutil.Internal("unable to determine request scope: %q", req.Spec().Procedure)
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
