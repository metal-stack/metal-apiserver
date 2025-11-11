package auth

import (
	"context"
	"log/slog"
	"time"

	"connectrpc.com/connect"
	"github.com/metal-stack/api/go/request"
	"github.com/metal-stack/metal-apiserver/pkg/repository"
	"github.com/metal-stack/metal-apiserver/pkg/token"
	"github.com/metal-stack/metal-lib/pkg/cache"
)

type (
	authInterceptor struct {
		authorizer request.Authorizer
	}
)

func NewInterceptor(log *slog.Logger, repo *repository.Store) *authInterceptor {
	projectAndTenantCache := cache.New(1*time.Hour, func(ctx context.Context, id string) (*request.ProjectsAndTenants, error) {
		pat, err := repo.UnscopedProject().AdditionalMethods().GetProjectsAndTenants(ctx, id)
		if err != nil {
			return nil, err
		}
		return &request.ProjectsAndTenants{
			Projects:      pat.Projects,
			Tenants:       pat.Tenants,
			DefaultTenant: pat.DefaultTenant,
			ProjectRoles:  pat.ProjectRoles,
			TenantRoles:   pat.TenantRoles,
		}, nil
	})

	patg := func(ctx context.Context, userId string) (*request.ProjectsAndTenants, error) {
		return projectAndTenantCache.Get(ctx, userId)
	}

	return &authInterceptor{
		authorizer: request.NewAuthorizer(log, patg),
	}
}

func (a *authInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return connect.UnaryFunc(func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		t, _ := token.TokenFromContext(ctx)

		err := a.authorizer.Authorize(ctx, t, req)
		if err != nil {
			return nil, err
		}
		return next(ctx, req)
	})
}

func (a *authInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return connect.StreamingClientFunc(func(ctx context.Context, spec connect.Spec) connect.StreamingClientConn {
		return next(ctx, spec)
	})
}
func (a *authInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return connect.StreamingHandlerFunc(func(ctx context.Context, conn connect.StreamingHandlerConn) error {
		return next(ctx, conn)
	})
}
