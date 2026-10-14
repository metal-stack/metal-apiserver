package auth

import (
	"context"
	"log/slog"

	"connectrpc.com/connect/v2"
	"github.com/metal-stack/metal-apiserver/pkg/connectutil"
	"github.com/metal-stack/metal-apiserver/pkg/repository/api"
	"github.com/metal-stack/metal-apiserver/pkg/request"
	"github.com/metal-stack/metal-apiserver/pkg/token"
)

type (
	authorizeInterceptor struct {
		log        *slog.Logger
		authorizer request.Authorizer
	}
)

func NewAuthorizeInterceptor(log *slog.Logger, patg api.ProjectsAndTenantsGetter) *authorizeInterceptor {
	return &authorizeInterceptor{
		log:        log,
		authorizer: request.NewAuthorizer(log, patg),
	}
}

// Interceptor returns a connect server interceptor that authorizes the request
// against the permissions of the token found in the context.
func (a *authorizeInterceptor) Interceptor() connect.ServerInterceptor {
	return func(next connect.ServerFunc) connect.ServerFunc {
		return func(ctx context.Context, spec connect.Spec, stream connect.ServerStream) error {
			req, ok, err := connectutil.ReceiveFirstMessage(stream, spec)
			if err != nil {
				return err
			}
			if !ok {
				return next(ctx, spec, stream)
			}

			t, _ := token.TokenFromContext(ctx)

			err = a.authorizer.Authorize(ctx, t, spec, req)
			if err != nil {
				return err
			}

			return next(ctx, spec, connectutil.ReplayStream(stream, req))
		}
	}
}
