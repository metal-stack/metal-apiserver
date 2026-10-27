package auth

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"connectrpc.com/connect"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	infrav2 "github.com/metal-stack/api/go/metalstack/infra/v2"
	"github.com/metal-stack/api/go/metalstack/infra/v2/infrav2connect"
	"github.com/metal-stack/metal-apiserver/pkg/repository/api"
	"github.com/metal-stack/metal-apiserver/pkg/token"
	"github.com/stretchr/testify/require"
)

// Test_authorizeInterceptor_WrapStreamingHandler proves Finding #14.
//
// The authorizeInterceptor enforces the per-method RBAC rule only in WrapUnary.
// WrapStreamingHandler used to return next(ctx, conn) without ever calling the
// authorizer, so server-stream RPCs such as BMCService/WaitForBMCCommand and
// BootService/Wait were reachable with any authenticated token that carried no
// infra/machine role at all.
//
// The test wires a server-stream handler for those procedures through the real
// authorizeInterceptor (exactly as allInfraInterceptors does) and connects with
// (a) a plain user token lacking any infra/machine role, which must be denied
// before the handler runs, and (b) a token holding the required role, which must
// be let through.
func Test_authorizeInterceptor_WrapStreamingHandler(t *testing.T) {
	t.Parallel()

	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	newInterceptor := func() *authorizeInterceptor {
		return NewAuthorizeInterceptor(log, func(ctx context.Context, userID string) (*api.ProjectsAndTenants, error) {
			return &api.ProjectsAndTenants{}, nil
		})
	}

	tests := []struct {
		name    string
		token   *apiv2.Token
		wantErr bool
	}{
		{
			// The core claim of Finding #14: an ordinary USER token, which holds
			// no infra role, must not be able to open the BMC-credential stream.
			name: "infra stream denied for token without infra role",
			token: &apiv2.Token{
				User:      "john.doe@github.com",
				TokenType: apiv2.TokenType_TOKEN_TYPE_USER,
			},
			wantErr: true,
		},
		{
			name: "infra stream denied for api token without any permissions",
			token: &apiv2.Token{
				User:      "pixiecore",
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
			},
			wantErr: true,
		},
		{
			name: "infra stream allowed for token with infra role",
			token: &apiv2.Token{
				User:      "ansible",
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
				InfraRole: apiv2.InfraRole_INFRA_ROLE_EDITOR.Enum(),
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var called bool
			svc := &fakeBMCService{
				UnimplementedBMCServiceHandler: infrav2connect.UnimplementedBMCServiceHandler{},
				called:                         &called,
			}

			mux := http.NewServeMux()
			mux.Handle(infrav2connect.NewBMCServiceHandler(svc, connect.WithInterceptors(
				&streamTokenInjector{token: tt.token},
				newInterceptor(),
			)))
			server := httptest.NewServer(mux)
			defer server.Close()

			c := infrav2connect.NewBMCServiceClient(server.Client(), server.URL)
			stream, err := c.WaitForBMCCommand(t.Context(), &infrav2.WaitForBMCCommandRequest{Partition: "partition-a"})
			if err == nil {
				stream.Receive()
				err = stream.Err()
			}

			if tt.wantErr {
				require.Error(t, err, "expected the stream to be denied but it was accepted")
				require.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))
				require.False(t, called, "handler must not run for an unauthorized stream")
				return
			}

			require.True(t, called, "handler must run for an authorized stream")
		})
	}
}

type fakeBMCService struct {
	infrav2connect.UnimplementedBMCServiceHandler
	called *bool
}

func (f *fakeBMCService) WaitForBMCCommand(_ context.Context, _ *infrav2.WaitForBMCCommandRequest, _ *connect.ServerStream[infrav2.WaitForBMCCommandResponse]) error {
	*f.called = true
	return nil
}

// Test_authorizeInterceptor_WrapStreamingHandler_machineScope covers the second
// sink named by Finding #14, BootService/Wait. It is machine scoped, so it also
// proves the streaming authorization resolves the subject from the request
// message: a token for one machine must not open the Wait stream of another.
func Test_authorizeInterceptor_WrapStreamingHandler_machineScope(t *testing.T) {
	t.Parallel()

	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	const (
		ownMachine   = "de240964-ff9f-4e3d-95b2-8a96e43788f1"
		otherMachine = "fa350875-ee8f-4e3d-95b2-8a96e43788f2"
	)

	tests := []struct {
		name    string
		machine string
		wantErr bool
	}{
		{name: "machine stream allowed for own machine", machine: ownMachine, wantErr: false},
		{name: "machine stream denied for other machine", machine: otherMachine, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var called bool
			svc := &fakeBootService{
				UnimplementedBootServiceHandler: infrav2connect.UnimplementedBootServiceHandler{},
				called:                          &called,
			}

			mux := http.NewServeMux()
			mux.Handle(infrav2connect.NewBootServiceHandler(svc, connect.WithInterceptors(
				&streamTokenInjector{token: &apiv2.Token{
					User:      "pixie-core",
					TokenType: apiv2.TokenType_TOKEN_TYPE_API,
					MachineRoles: map[string]apiv2.MachineRole{
						ownMachine: apiv2.MachineRole_MACHINE_ROLE_EDITOR,
					},
				}},
				NewAuthorizeInterceptor(log, func(ctx context.Context, userID string) (*api.ProjectsAndTenants, error) {
					return &api.ProjectsAndTenants{}, nil
				}),
			)))
			server := httptest.NewServer(mux)
			defer server.Close()

			c := infrav2connect.NewBootServiceClient(server.Client(), server.URL)
			stream, err := c.Wait(t.Context(), &infrav2.BootServiceWaitRequest{Uuid: tt.machine})
			if err == nil {
				stream.Receive()
				err = stream.Err()
			}

			if tt.wantErr {
				require.Error(t, err, "expected the stream to be denied but it was accepted")
				require.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))
				require.False(t, called, "handler must not run for a machine the token has no role for")
				return
			}

			require.True(t, called, "handler must run for an authorized machine")
		})
	}
}

type fakeBootService struct {
	infrav2connect.UnimplementedBootServiceHandler
	called *bool
}

func (f *fakeBootService) Wait(_ context.Context, _ *infrav2.BootServiceWaitRequest, _ *connect.ServerStream[infrav2.BootServiceWaitResponse]) error {
	*f.called = true
	return nil
}

// streamTokenInjector stores the token in the context for streaming handlers,
// mimicking what auth.Authenticate does via the request gate before the
// interceptor chain runs.
type streamTokenInjector struct {
	token *apiv2.Token
}

func (t *streamTokenInjector) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc { return next }

func (t *streamTokenInjector) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return next
}

func (t *streamTokenInjector) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return func(ctx context.Context, conn connect.StreamingHandlerConn) error {
		if t.token != nil {
			ctx = token.ContextWithToken(ctx, t.token)
		}
		return next(ctx, conn)
	}
}
