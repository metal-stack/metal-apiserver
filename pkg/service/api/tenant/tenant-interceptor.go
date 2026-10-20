package tenant

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"connectrpc.com/connect/v2"
	"github.com/metal-stack/api/go/errorutil"
	"github.com/metal-stack/api/go/permissions"
	"github.com/metal-stack/metal-apiserver/pkg/connectutil"
	"github.com/metal-stack/metal-apiserver/pkg/repository/api"
	"github.com/metal-stack/metal-apiserver/pkg/token"
	"github.com/metal-stack/metal-lib/pkg/cache"
	"github.com/metal-stack/metal-lib/pkg/pointer"
	tenant "github.com/metal-stack/tenant-api/go/client"
	tenantv1 "github.com/metal-stack/tenant-api/go/tenant/api/v1"

	"github.com/metal-stack/metal-lib/auditing"
	"google.golang.org/protobuf/proto"
)

type (
	tenantInterceptor struct {
		projectCache *cache.Cache[string, *tenantv1.Project]
		log          *slog.Logger
		tenantClient tenant.Client
	}
)

func NewInterceptor(log *slog.Logger, tenantClient tenant.Client) *tenantInterceptor {
	return &tenantInterceptor{
		projectCache: cache.New(1*time.Hour, func(ctx context.Context, id string) (*tenantv1.Project, error) {
			pgr, err := tenantClient.Apiv1().Project().Get(ctx, &tenantv1.ProjectServiceGetRequest{Id: id})
			if err != nil {
				return nil, fmt.Errorf("unable to get project: %w", err)
			}
			return pgr.GetProject(), nil
		}),
		log:          log,
		tenantClient: tenantClient,
	}
}

// Interceptor determines the tenant/project/machine scope of an RPC, checks
// that the request is allowed to target it and enriches the context with the
// auditing user derived from the request.
func (i *tenantInterceptor) Interceptor() connect.ServerInterceptor {
	return func(next connect.ServerFunc) connect.ServerFunc {
		return func(ctx context.Context, spec connect.Spec, stream connect.ServerStream) error {
			req, ok, err := connectutil.ReceiveFirstMessage(stream, spec)
			if err != nil {
				return err
			}
			if !ok {
				return next(ctx, spec, stream)
			}

			ctx, err = i.prepare(ctx, spec, req)
			if err != nil {
				return err
			}

			return next(ctx, spec, connectutil.ReplayStream(stream, req))
		}
	}
}

// prepare checks the scope of the given request and returns a context enriched
// with the auditing user. If the request targets a project, it also checks if
// the tenant of this project already exists, if not an error is returned.
func (i *tenantInterceptor) prepare(ctx context.Context, spec connect.Spec, req proto.Message) (context.Context, error) {
	var (
		tok, tokenInCtx = token.TokenFromContext(ctx)
		user            = &auditing.User{
			EMail:   "",
			Name:    "",
			Tenant:  "",
			Subject: pointer.SafeDeref(tok).User,
		}

		setUserFieldsByTenantLookup = func(tenantID string) error {
			tgr, err := i.tenantClient.Apiv1().Tenant().Get(ctx, &tenantv1.TenantServiceGetRequest{Id: tenantID})
			if errorutil.IsNotFound(err) {
				return errorutil.NewNotFound(err)
			}
			if err != nil {
				return errorutil.NewInternal(err)
			}

			user.Tenant = tgr.Tenant.Meta.Id
			user.EMail = tgr.Tenant.Meta.Annotations[api.TenantTagEmail]

			// update the context with the user information BEFORE calling next
			ctx = auditing.PutUserInContext(ctx, user)

			return nil
		}
	)

	if permissions.IsPublicScope(spec) {
		i.log.Debug("tenant interceptor", "request-scope", "public")

		if tokenInCtx {
			err := setUserFieldsByTenantLookup(tok.User)
			if err != nil {
				return ctx, err
			}

			user.Tenant = "" // public methods do not operate on a tenant, therefore erase again
		} else {
			ctx = auditing.PutUserInContext(ctx, user)
		}

		return ctx, nil
	}

	if !tokenInCtx {
		return ctx, errorutil.Unauthenticated("token must be present when requesting non-public scope method")
	}

	if permissions.IsSelfScope(spec) {
		i.log.Debug("tenant interceptor", "request-scope", "self")

		err := setUserFieldsByTenantLookup(tok.User)
		if err != nil {
			return ctx, err
		}

		return ctx, nil
	}

	if permissions.IsAdminScope(spec) {
		i.log.Debug("tenant interceptor", "request-scope", "admin")

		err := setUserFieldsByTenantLookup(tok.User)
		if err != nil {
			return ctx, err
		}

		user.Tenant = "" // admin methods do not operate on a tenant, therefore erase again

		return ctx, nil
	}

	if permissions.IsMachineScope(spec) {
		i.log.Debug("tenant interceptor", "request-scope", "machine")

		user := &auditing.User{
			Name: tok.User,
		}

		machineId, ok := permissions.GetMachineIdFromRequest(spec, req)
		if ok {
			user.Subject = machineId
		}

		// machine methods do not operate on a tenant, therefore create the security user from the machine id
		ctx = auditing.PutUserInContext(ctx, user)

		return ctx, nil
	}

	if permissions.IsInfraScope(spec) {
		i.log.Debug("tenant interceptor", "request-scope", "infra")

		// infra methods do not operate on a tenant, therefore create the security user from the token id
		ctx = auditing.PutUserInContext(ctx, &auditing.User{
			Name:    tok.User,
			Subject: tok.User,
		})

		return ctx, nil
	}

	if tenantID, ok := permissions.GetTenantFromRequest(spec, req); ok {
		i.log.Debug("tenant interceptor", "request-scope", "tenant", "id", tenantID)

		err := setUserFieldsByTenantLookup(tenantID)
		if err != nil {
			return ctx, err
		}

		return ctx, nil
	}

	if projectID, ok := permissions.GetProjectFromRequest(spec, req); ok {
		i.log.Debug("tenant interceptor", "request-scope", "project", "id", projectID)

		project, err := i.projectCache.Get(ctx, projectID)
		if errorutil.IsNotFound(err) {
			return ctx, errorutil.NewNotFound(err)
		}
		if err != nil {
			return ctx, errorutil.NewInternal(err)
		}

		user.Project = projectID

		err = setUserFieldsByTenantLookup(project.TenantId)
		if err != nil {
			return ctx, err
		}

		return ctx, nil
	}

	return ctx, errorutil.Internal("unable to determine request scope: %q", spec.Procedure)
}
