package auth

import (
	"context"
	"log/slog"

	"connectrpc.com/connect"
	"github.com/metal-stack/metal-apiserver/pkg/repository"
	"github.com/metal-stack/metal-apiserver/pkg/request"
	"github.com/metal-stack/metal-apiserver/pkg/token"
)

type (
	authorizeInterceptor struct {
		log        *slog.Logger
		authorizer request.Authorizer
	}
)

func NewAuthorizeInterceptor(log *slog.Logger, repo *repository.Store) *authorizeInterceptor {
	// We fetch projects and tenants on every request, if this hurts performance we can
	// put the result into the context, and reuse the result in subsequent queries
	// or we introduce a cache with a short timeout.
	patg := func(ctx context.Context, userId string) (*repository.ProjectsAndTenants, error) {
		return repo.UnscopedProject().AdditionalMethods().GetProjectsAndTenants(ctx, userId)
	}

	return &authorizeInterceptor{
		log:        log,
		authorizer: request.NewAuthorizer(log, patg),
	}
}

func (a *authorizeInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return connect.UnaryFunc(func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		t, _ := token.TokenFromContext(ctx)

		err := a.authorizer.Authorize(ctx, t, req)
		if err != nil {
			return nil, err
		}
		return next(ctx, req)
	})
}

func (a *authorizeInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return func(ctx context.Context, spec connect.Spec) connect.StreamingClientConn {
		return next(ctx, spec)
	}
}
func (a *authorizeInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return func(ctx context.Context, conn connect.StreamingHandlerConn) error {
		return next(ctx, conn)
	}
}
