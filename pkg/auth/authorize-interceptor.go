package auth

import (
	"context"
	"log/slog"

	"connectrpc.com/connect"
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

func (a *authorizeInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		t, _ := token.TokenFromContext(ctx)

		err := a.authorizer.Authorize(ctx, t, req)
		if err != nil {
			return nil, err
		}
		return next(ctx, req)
	}
}

func (a *authorizeInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return func(ctx context.Context, spec connect.Spec) connect.StreamingClientConn {
		return next(ctx, spec)
	}
}
func (a *authorizeInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return func(ctx context.Context, conn connect.StreamingHandlerConn) error {
		return next(ctx, &authorizingStreamingHandlerConn{
			StreamingHandlerConn: conn,
			ctx:                  ctx,
			authorizer:           a.authorizer,
		})
	}
}

// authorizingStreamingHandlerConn enforces the same per-method authorization
// rule as WrapUnary on the first message received on a stream.
//
// A connect streaming handler interceptor is invoked before the request message
// is read, so authorization can only be performed once the first message
// arrives. For server streaming RPCs (such as BMCService/WaitForBMCCommand and
// BootService/Wait) that first message is the single request, so the credential
// bearing responses are never produced for an unauthorized caller.
type authorizingStreamingHandlerConn struct {
	connect.StreamingHandlerConn

	ctx        context.Context
	authorizer request.Authorizer
	authorized bool
}

func (c *authorizingStreamingHandlerConn) Receive(msg any) error {
	// The message must be unmarshaled first: authorization resolves the subject
	// (e.g. the machine uuid) from the received request.
	if err := c.StreamingHandlerConn.Receive(msg); err != nil {
		return err
	}
	if !c.authorized {
		t, _ := token.TokenFromContext(c.ctx)
		if err := c.authorizer.Authorize(c.ctx, t, &streamRequest{spec: c.Spec(), msg: msg}); err != nil {
			return err
		}
		c.authorized = true
	}
	return nil
}

// streamRequest adapts a received streaming request message to connect.AnyRequest
// so the existing authorization logic can be reused unchanged.
//
// The embedded connect.AnyRequest is nil on purpose: connect.AnyRequest has
// unexported methods, so it cannot be implemented from outside the connect
// package, and only Spec and Any are ever consulted by the authorizer. Embedding
// a nil value is therefore enough and keeps the unary and streaming paths
// sharing one authorization implementation.
type streamRequest struct {
	connect.AnyRequest
	spec connect.Spec
	msg  any
}

func (r *streamRequest) Spec() connect.Spec { return r.spec }
func (r *streamRequest) Any() any           { return r.msg }
