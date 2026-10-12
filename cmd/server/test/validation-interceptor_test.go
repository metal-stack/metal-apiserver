package test

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"connectrpc.com/connect/v2"
	"connectrpc.com/connect/v2/connecthttp"
	"connectrpc.com/validate"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/api/go/metalstack/api/v2/apiv2connect"
	"github.com/metal-stack/metal-apiserver/pkg/service/api/token"
	"github.com/stretchr/testify/require"
)

func TestWithValidator(t *testing.T) {
	t.Parallel()
	interceptor := validate.NewServerInterceptor()

	server := connect.NewServer(interceptor)
	apiv2connect.RegisterTokenServiceHandler(server, token.New(token.Config{
		Log: slog.Default(),
	}))

	mux := http.NewServeMux()
	connecthttp.Mount(mux, server)
	srv := startHTTPServer(t, mux)

	req := &apiv2.TokenServiceCreateRequest{
		Description: `
aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
`,
	}
	client := apiv2connect.NewTokenServiceClient(connect.NewClient(connecthttp.NewTransport(srv.Client(), srv.URL)))
	_, err := client.Create(t.Context(), req)
	require.Error(t, err)
	require.EqualError(t, err, "invalid_argument: validation error: description: must be shorter than 256 characters")
	require.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

func startHTTPServer(tb testing.TB, h http.Handler) *httptest.Server {
	tb.Helper()
	srv := httptest.NewUnstartedServer(h)
	srv.EnableHTTP2 = true
	srv.Start()
	tb.Cleanup(srv.Close)
	return srv
}
