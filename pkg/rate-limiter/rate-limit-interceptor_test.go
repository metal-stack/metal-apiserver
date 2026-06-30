package ratelimiter

import (
	"context"
	"log/slog"
	"testing"

	"connectrpc.com/connect"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/token"
)

type testMsg struct{}

func Test_ratelimitInterceptor_WrapUnary(t *testing.T) {
	s := miniredis.RunT(t)

	adminRole := apiv2.AdminRole_ADMIN_ROLE_EDITOR

	tests := []struct {
		name      string
		maxToken  int
		maxUnauth int
		setup     func() (context.Context, connect.AnyRequest)
		calls     int // total wrapped calls before the assertion on the last call
		wantErr   bool
		wantCode  connect.Code
		wantNext  bool
	}{
		{
			name:      "authenticated request within limit succeeds",
			maxToken:  5,
			maxUnauth: 3,
			setup: func() (context.Context, connect.AnyRequest) {
				ctx := token.ContextWithToken(t.Context(), &apiv2.Token{User: "u1", Uuid: "t1"})
				return ctx, connect.NewRequest(&testMsg{})
			},
			calls:    1,
			wantNext: true,
		},
		{
			name:      "authenticated request exceeding limit fails with ResourceExhausted",
			maxToken:  5,
			maxUnauth: 3,
			setup: func() (context.Context, connect.AnyRequest) {
				ctx := token.ContextWithToken(t.Context(), &apiv2.Token{User: "u1", Uuid: "t1"})
				return ctx, connect.NewRequest(&testMsg{})
			},
			calls:    7, // maxToken=5 allows 6 calls before rejection
			wantErr:  true,
			wantCode: connect.CodeResourceExhausted,
			wantNext: false,
		},
		{
			name:      "admin token bypasses rate limit",
			maxToken:  5,
			maxUnauth: 3,
			setup: func() (context.Context, connect.AnyRequest) {
				ctx := token.ContextWithToken(t.Context(), &apiv2.Token{User: "admin", Uuid: "t-admin", AdminRole: &adminRole})
				return ctx, connect.NewRequest(&testMsg{})
			},
			calls:    10,
			wantNext: true,
		},
		{
			name:      "unauthenticated request with X-Forwarded-For within limit succeeds",
			maxToken:  5,
			maxUnauth: 3,
			setup: func() (context.Context, connect.AnyRequest) {
				req := connect.NewRequest(&testMsg{})
				req.Header().Set("X-Forwarded-For", "10.0.0.1")
				return t.Context(), req
			},
			calls:    1,
			wantNext: true,
		},
		{
			name:      "unauthenticated request with X-Forwarded-For exceeding limit fails with ResourceExhausted",
			maxToken:  5,
			maxUnauth: 3,
			setup: func() (context.Context, connect.AnyRequest) {
				req := connect.NewRequest(&testMsg{})
				req.Header().Set("X-Forwarded-For", "10.0.0.1")
				return t.Context(), req
			},
			calls:    5, // maxUnauth=3 allows 4 calls before rejection
			wantErr:  true,
			wantCode: connect.CodeResourceExhausted,
			wantNext: false,
		},
		{
			name:      "unauthenticated request without client IP skips rate limiter",
			maxToken:  5,
			maxUnauth: 3,
			setup: func() (context.Context, connect.AnyRequest) {
				return t.Context(), connect.NewRequest(&testMsg{})
			},
			calls:    10,
			wantNext: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s.FlushAll()

			c := redis.NewClient(&redis.Options{Addr: s.Addr()})

			interceptor := &ratelimitInterceptor{
				ratelimiter:                         New(c),
				maxRequestsPerMinuteToken:           tt.maxToken,
				maxRequestsPerMinuteUnauthenticated: tt.maxUnauth,
				log:                                 slog.Default(),
			}

			var nextCallCount int
			wrapped := interceptor.WrapUnary(func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
				nextCallCount++
				return connect.NewResponse(&testMsg{}), nil
			})

			ctx, req := tt.setup()

			for i := range tt.calls - 1 {
				_, err := wrapped(ctx, req)
				require.NoError(t, err, "unexpected error on call %d during exhaustion", i)
			}

			nextBefore := nextCallCount
			_, err := wrapped(ctx, req)

			if tt.wantErr {
				require.Error(t, err)
				var connectErr *connect.Error
				require.ErrorAs(t, err, &connectErr)
				require.Equal(t, tt.wantCode, connectErr.Code())
			} else {
				require.NoError(t, err)
			}

			if tt.wantNext {
				require.Equal(t, nextBefore+1, nextCallCount, "next should have been called")
			} else {
				require.Equal(t, nextBefore, nextCallCount, "next should not have been called")
			}
		})
	}
}
