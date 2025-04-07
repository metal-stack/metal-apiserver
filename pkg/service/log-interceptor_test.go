package service

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"connectrpc.com/connect"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"

	"github.com/metal-stack/api/go/client"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type interceptorTestFn func(string, []connect.Interceptor, func(context.Context)) *connect.Handler

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
				_, err := c.Apiv2().Health().Get(ctx, connect.NewRequest(&apiv2.HealthServiceGetRequest{}))
				return err
			},
			level:       slog.LevelInfo,
			method:      "/metalstack.api.v2.HealthService/Get",
			handler:     handler[apiv2.HealthServiceGetRequest, apiv2.HealthServiceGetResponse](),
			wantContain: `"level":"INFO","msg":"handling unary call","procedure":"/metalstack.api.v2.HealthService/Get"`,
		},
		{
			name: "log debug",
			reqFn: func(ctx context.Context, c client.Client) error {
				_, err := c.Apiv2().IP().Create(ctx, connect.NewRequest(&apiv2.IPServiceCreateRequest{
					Project: "project-a",
					Network: "network-b",
				}))
				return err
			},
			level:       slog.LevelDebug,
			method:      "/metalstack.api.v2.IPService/Create",
			handler:     handler[apiv2.IPServiceCreateRequest, apiv2.IPServiceCreateResponse](),
			wantContain: `"level":"INFO","msg":"handling unary call","procedure":"/metalstack.api.v2.IPService/Create","body":{"network":`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			var (
				buf            bytes.Buffer
				logger         = slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: tt.level}))
				logInterceptor = newLogRequestInterceptor(logger)
				called         = false

				interceptors = []connect.Interceptor{
					logInterceptor,
				}
			)

			mux := http.NewServeMux()
			mux.Handle(tt.method, tt.handler(tt.method, interceptors, func(ctx context.Context) {
				called = true
			}))

			server := httptest.NewServer(mux)
			defer server.Close()

			c := client.New(client.DialConfig{
				BaseURL: server.URL,
			})

			require.NotNil(t, tt.reqFn)
			err := tt.reqFn(context.Background(), c)
			require.NoError(t, err)

			assert.Contains(t, buf.String(), tt.wantContain)

			require.NoError(t, err)
			assert.True(t, called, "request was not forwarded to next")
		})
	}
}

func handler[Req, Resp any]() interceptorTestFn {
	return func(procedure string, interceptors []connect.Interceptor, test func(context.Context)) *connect.Handler {
		return connect.NewUnaryHandler(
			procedure,
			func(ctx context.Context, r *connect.Request[Req]) (*connect.Response[Resp], error) {
				test(ctx)
				var zero Resp
				return connect.NewResponse(&zero), nil
			},
			connect.WithInterceptors(interceptors...),
		)
	}
}
