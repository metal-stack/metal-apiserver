package auth

import (
	"context"
	"log/slog"
	"time"

	"connectrpc.com/connect"
	"github.com/metal-stack/metal-apiserver/pkg/repository"
	"github.com/metal-stack/metal-apiserver/pkg/request"
	"github.com/metal-stack/metal-apiserver/pkg/token"
	"github.com/metal-stack/metal-lib/pkg/cache"
)

type (
	authorizeInterceptor struct {
		log        *slog.Logger
		authorizer request.Authorizer
	}
)

func NewAuthorizeInterceptor(log *slog.Logger, repo *repository.Store) *authorizeInterceptor {
	// FIXME decide what is a proper cache timeout
	projectAndTenantCache := cache.New(10*time.Millisecond, func(ctx context.Context, id string) (*repository.ProjectsAndTenants, error) {
		pat, err := repo.UnscopedProject().AdditionalMethods().GetProjectsAndTenants(ctx, id)
		if err != nil {
			return nil, err
		}
		return pat, nil
	})

	patg := func(ctx context.Context, userId string) (*repository.ProjectsAndTenants, error) {
		return projectAndTenantCache.Get(ctx, userId)
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
	return connect.StreamingClientFunc(func(ctx context.Context, spec connect.Spec) connect.StreamingClientConn {
		return next(ctx, spec)
	})
}
func (a *authorizeInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return connect.StreamingHandlerFunc(func(ctx context.Context, conn connect.StreamingHandlerConn) error {
		return next(ctx, conn)
	})
}
