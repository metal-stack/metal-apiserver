package service

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"connectrpc.com/connect/v2"
	"connectrpc.com/connect/v2/connecthttp"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"

	"github.com/metal-stack/api/go/client"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type interceptorTestFn func(string, func(context.Context)) connect.Method

func Test_logInterceptor_AuditingCtx(t *testing.T) {
	tests := []struct {
		name        string
		method      string
		level       slog.Level
		reqFn       func(ctx context.Context, c client.Client) error
		handler     interceptorTestFn
		wantContain string
	}{
		{
			name: "log a request",
			reqFn: func(ctx context.Context, c client.Client) error {
				_, err := c.Apiv2().Health().Get(ctx, &apiv2.HealthServiceGetRequest{})
				return err
			},
			level:       slog.LevelInfo,
			method:      "/metalstack.api.v2.HealthService/Get",
			handler:     handler[apiv2.HealthServiceGetRequest, apiv2.HealthServiceGetResponse](),
			wantContain: `"level":"INFO","msg":"handled unary call","procedure":"/metalstack.api.v2.HealthService/Get"`,
		},
		{
			name: "log debug",
			reqFn: func(ctx context.Context, c client.Client) error {
				_, err := c.Apiv2().IP().Create(ctx, &apiv2.IPServiceCreateRequest{
					Project: "project-a",
					Network: "network-b",
				})
				return err
			},
			level:       slog.LevelDebug,
			method:      "/metalstack.api.v2.IPService/Create",
			handler:     handler[apiv2.IPServiceCreateRequest, apiv2.IPServiceCreateResponse](),
			wantContain: `"level":"INFO","msg":"handled unary call","procedure":"/metalstack.api.v2.IPService/Create","request":{"network":`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			var (
				buf            bytes.Buffer
				logger         = slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: tt.level}))
				logInterceptor = newLogRequestInterceptor(logger)
				called         = false
			)

			server := connect.NewServer(logInterceptor.Interceptor())
			server.Register(tt.handler(tt.method, func(ctx context.Context) {
				called = true
			}))

			mux := http.NewServeMux()
			connecthttp.Mount(mux, server)

			httpServer := httptest.NewServer(mux)
			defer httpServer.Close()

			c, err := client.New(&client.DialConfig{
				BaseURL: httpServer.URL,
				Log:     logger,
			})
			require.NoError(t, err)

			require.NotNil(t, tt.reqFn)
			err = tt.reqFn(t.Context(), c)
			require.NoError(t, err)

			assert.Contains(t, buf.String(), tt.wantContain)

			require.NoError(t, err)
			assert.True(t, called, "request was not forwarded to next")
		})
	}
}

func handler[Req, Resp any]() interceptorTestFn {
	return func(procedure string, test func(context.Context)) connect.Method {
		return connect.Method{
			Spec: connect.Spec{
				Procedure:  procedure,
				StreamType: connect.StreamTypeUnary,
			},
			Handler: func(ctx context.Context, _ connect.Spec, stream connect.ServerStream) error {
				var req Req
				if err := stream.Receive(&req); err != nil {
					return err
				}
				test(ctx)
				var zero Resp
				return stream.Send(&zero)
			},
		}
	}
}
