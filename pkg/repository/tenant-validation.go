package repository

import (
	"context"

	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	"github.com/metal-stack/metal-apiserver/pkg/token"
)

func (t *tenantRepository) validateCreate(ctx context.Context, create *apiv2.TenantServiceCreateRequest) error {
	return nil
}

func (t *tenantRepository) validateDelete(ctx context.Context, e *tenant) error {
	tok, ok := token.TokenFromContext(ctx)
	if !ok || t == nil {
		return errorutil.Unauthenticated("no token found in request")
	}

	if tok.User == e.Meta.Id {
		return errorutil.InvalidArgument("the personal tenant (default-tenant) cannot be deleted")
	}

	projects, err := t.s.UnscopedProject().List(ctx, &apiv2.ProjectServiceListRequest{
		Tenant: &e.Meta.Id,
	})
	if err != nil {
		return errorutil.Convert(err)
	}

	if len(projects) > 0 {
		return errorutil.FailedPrecondition("there are still projects associated with this tenant, you need to delete them first")
	}

	return nil
}

func (t *tenantRepository) validateUpdate(ctx context.Context, msg *apiv2.TenantServiceUpdateRequest, _ *tenant) error {
	return nil
}
