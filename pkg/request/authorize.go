package request

import (
	"context"
	"log/slog"
	"slices"

	"connectrpc.com/connect"
	"github.com/metal-stack/api/go/errorutil"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/api/go/permissions"
	"github.com/metal-stack/metal-apiserver/pkg/repository/api"
)

type (
	authorizer struct {
		log                      *slog.Logger
		projectsAndTenantsGetter api.ProjectsAndTenantsGetter
	}

	// Authorizer provides methods to authorize requests with a given token
	Authorizer interface {
		// Authorize checks if with the given token the request is allowed.
		// If the access is not allowed, a PermissionDenied Error is returned with a proper error message.
		// req is only fully populated after a interceptor call.
		Authorize(ctx context.Context, token *apiv2.Token, req connect.AnyRequest) error
		// TokenPermissions returns the permissions based on the given token
		TokenPermissions(ctx context.Context, token *apiv2.Token) (tokenPermissions, error)
	}
)

func NewAuthorizer(log *slog.Logger, patg api.ProjectsAndTenantsGetter) Authorizer {
	return &authorizer{
		log:                      log,
		projectsAndTenantsGetter: patg,
	}
}

func (a *authorizer) Authorize(ctx context.Context, token *apiv2.Token, req connect.AnyRequest) error {
	var (
		method  = req.Spec().Procedure
		subject string
	)
	if req == nil {
		return errorutil.Internal("request is nil")
	}

	if permissions.IsProjectScope(req) {
		project, ok := permissions.GetProjectFromRequest(req)
		if ok {
			subject = project
		} else {
			return errorutil.InvalidArgument("no project found in project scoped request")
		}
	}

	if permissions.IsTenantScope(req) {
		tenant, ok := permissions.GetTenantFromRequest(req)
		if ok {
			subject = tenant
		} else {
			return errorutil.InvalidArgument("no tenant found in tenant scoped request")
		}
	}

	if permissions.IsMachineScope(req) {
		machineId, ok := permissions.GetMachineIdFromRequest(req)
		if ok {
			subject = machineId
		} else {
			return errorutil.InvalidArgument("no machine uuid found in machine scoped request")
		}
	}

	return a.authorize(ctx, token, method, subject)
}

func (a *authorizer) authorize(ctx context.Context, token *apiv2.Token, method string, subject string) error {
	a.log.Debug("authorize", "token", token, "method", method, "subject", subject)

	if _, ok := permissions.GetServicePermissions().Methods[method]; !ok {
		return errorutil.PermissionDenied("requested procedure %q is not known", method)
	}

	permissions, err := a.getTokenPermissions(ctx, token)
	if err != nil {
		return errorutil.NewInternal(err)
	}
	if permissions == nil {
		return errorutil.PermissionDenied("no permissions found in token")
	}

	a.log.Debug("authorize", "permissions", permissions, "method", method, "subject", subject)

	subjects, ok := permissions[method]
	if !ok {
		return errorutil.PermissionDenied("access to:%q is not allowed because it is not part of the token permissions", method)
	}

	if _, allSubjectsAllowed := subjects[AnySubject]; allSubjectsAllowed {
		// This token contains permissions to access this method regardless of subject
		return nil
	}

	if _, subjectAllowed := subjects[subject]; !subjectAllowed {
		var allowedSubjects []string
		for s := range subjects {
			allowedSubjects = append(allowedSubjects, s)
		}
		slices.Sort(allowedSubjects)
		return errorutil.PermissionDenied("access to:%q with subject:%q is not allowed because it is not part of the token permissions, allowed subjects are:%q", method, subject, allowedSubjects)
	}

	return nil
}
