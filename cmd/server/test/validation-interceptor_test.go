package test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"connectrpc.com/connect"
	"connectrpc.com/validate"
	apiv1 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/api/go/metalstack/api/v2/apiv2connect"
	"github.com/stretchr/testify/require"
)

func TestWithValidator(t *testing.T) {
	t.Parallel()
	interceptor := validate.NewInterceptor()

	mux := http.NewServeMux()
	mux.Handle(apiv2connect.TokenServiceCreateProcedure, connect.NewUnaryHandler(
		apiv2connect.TokenServiceCreateProcedure,
		createToken,
		connect.WithInterceptors(interceptor),
	))
	srv := startHTTPServer(t, mux)

	req := connect.NewRequest(&apiv1.TokenServiceCreateRequest{
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
	})
	_, err := apiv2connect.NewTokenServiceClient(srv.Client(), srv.URL).Create(t.Context(), req)
	require.Error(t, err)
	require.EqualError(t, err, "invalid_argument: validation error:\n - description: must be shorter than 256 characters [string.is_description]")
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

func createToken(_ context.Context, req *connect.Request[apiv1.TokenServiceCreateRequest]) (*connect.Response[apiv1.TokenServiceCreateResponse], error) {
	return connect.NewResponse(&apiv1.TokenServiceCreateResponse{Token: &apiv1.Token{Uuid: "abc"}}), nil
}
