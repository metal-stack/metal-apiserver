package test

// import (
// 	"context"
// 	"fmt"
// 	"net/http"
// 	"net/http/httptest"
// 	"testing"

// 	"connectrpc.com/connect"
// 	"connectrpc.com/validate"

// 	validatepb "buf.build/gen/go/bufbuild/protovalidate/protocolbuffers/go/buf/validate"

// 	apiv2 "github.com/metal-stack-cloud/api/go/api/v1"
// 	"github.com/metal-stack-cloud/api/go/api/v1/apiv2connect"
// 	"github.com/metal-stack/metal-lib/pkg/pointer"
// 	"github.com/stretchr/testify/assert"
// 	"github.com/stretchr/testify/require"
// )

// func TestValidationInterceptorUnary(t *testing.T) {
// 	t.Parallel()
// 	tests := []struct {
// 		name        string
// 		svc         func(context.Context, *apiv2.TenantServiceInviteRequest) (*apiv2.TenantServiceInviteResponse, error)
// 		req         *apiv2.TenantServiceInviteRequest
// 		wantCode    connect.Code
// 		wantPath    *string // field path, from error details
// 		wantMessage *string // message from error details
// 	}{
// 		{
// 			name: "valid",
// 			svc:  createInvite,
// 			req: &apiv2.TenantServiceInviteRequest{
// 				Login: "some@example.com",
// 				Role:  apiv2.TenantRole_TENANT_ROLE_GUEST,
// 			},
// 		},
// 		{
// 			name: "invalid",
// 			svc:  createInvite,
// 			req: &apiv2.TenantServiceInviteRequest{
// 				Login: "some@example.com",
// 				Role:  7,
// 			},
// 			wantCode:    connect.CodeInvalidArgument,
// 			wantPath:    pointer.Pointer("role"),
// 			wantMessage: pointer.Pointer("value must be one of the defined enum values"),
// 		},
// 		{
// 			name: "underlying_error",
// 			svc:  createInviteWithError,
// 			req: &apiv2.TenantServiceInviteRequest{
// 				Login: "some@example.com",
// 				Role:  apiv2.TenantRole_TENANT_ROLE_GUEST,
// 			},
// 			wantCode: connect.CodeInternal,
// 		},
// 	}
// 	for _, test := range tests {
// 		test := test
// 		t.Run(test.name, func(t *testing.T) {
// 			t.Parallel()

// 			validator, err := validate.NewInterceptor()
// 			require.NoError(t, err)

// 			mux := http.NewServeMux()
// 			mux.Handle(apiv2connect.TenantServiceInviteProcedure, connect.NewUnaryHandler(
// 				apiv2connect.TenantServiceInviteProcedure,
// 				test.svc,
// 				connect.WithInterceptors(validator),
// 			))
// 			srv := startHTTPServer(t, mux)

// 			tenantService := apiv2connect.NewTenantServiceClient(srv.Client(), srv.URL)
// 			got, err := tenantService.Invite(context.Background(), connect.NewRequest(test.req))

// 			if test.wantCode > 0 {
// 				require.Error(t, err)
// 				var connectErr *connect.Error
// 				require.ErrorAs(t, err, &connectErr)
// 				assert.Equal(t, test.wantCode, connectErr.Code())
// 				if test.wantPath != nil {
// 					details := connectErr.Details()
// 					require.Len(t, details, 1)
// 					detail, err := details[0].Value()
// 					require.NoError(t, err)
// 					violations, ok := detail.(*validatepb.Violations)
// 					require.True(t, ok)
// 					require.Len(t, violations.Violations, 1)
// 					require.EqualValues(t, test.wantPath, violations.Violations[0].FieldPath)
// 					require.EqualValues(t, test.wantMessage, violations.Violations[0].Message)
// 				}
// 			} else {
// 				require.NoError(t, err)
// 				assert.NotZero(t, got)
// 			}
// 		})
// 	}
// }

// func startHTTPServer(tb testing.TB, h http.Handler) *httptest.Server {
// 	tb.Helper()
// 	srv := httptest.NewUnstartedServer(h)
// 	srv.EnableHTTP2 = true
// 	srv.Start()
// 	tb.Cleanup(srv.Close)
// 	return srv
// }

// func createInvite(_ context.Context, req *apiv2.TenantServiceInviteRequest) (*apiv2.TenantServiceInviteResponse, error) {
// 	return &apiv2.TenantServiceInviteResponse{Invite: &apiv2.TenantInvite{Secret: "geheim"}}), nil
// }

// func createInviteWithError(_ context.Context, req *apiv2.TenantServiceInviteRequest) (*apiv2.TenantServiceInviteResponse, error) {
// 	return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("something internal was bad"))
// }
