package tenant

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"connectrpc.com/connect"
	"github.com/davecgh/go-spew/spew"
	"github.com/google/go-cmp/cmp"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/api/go/metalstack/api/v2/apiv2connect"

	"github.com/metal-stack/api-server/pkg/repository"
	ipservice "github.com/metal-stack/api-server/pkg/service/ip"
	tutil "github.com/metal-stack/api-server/pkg/tenant"
	"github.com/metal-stack/api-server/pkg/test"
	"github.com/metal-stack/api-server/pkg/token"

	mdmv1 "github.com/metal-stack/masterdata-api/api/v1"
	"github.com/metal-stack/metal-lib/pkg/testcommon"
	"github.com/metal-stack/security"

	"github.com/stretchr/testify/assert"
	tmock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func Test_tenantInterceptor_WrapUnary(t *testing.T) {
	logger := slog.Default()
	ipam := test.StartIpam(t)
	tests := []struct {
		name               string
		ip                 *apiv2.IPServiceCreateRequest
		projectServiceMock func(mock *tmock.Mock)
		tenantServiceMock  func(mock *tmock.Mock)
		want               *apiv2.IPServiceCreateResponse
		wantErr            *connect.Error
	}{
		{
			name: "create ip with existing project",
			ip: &apiv2.IPServiceCreateRequest{
				Network: "n1",
				Project: "p1",
			},
			projectServiceMock: func(mock *tmock.Mock) {
				mock.On("Get", tmock.Anything, &mdmv1.ProjectGetRequest{
					Id: "p1",
				}).Return(&mdmv1.ProjectResponse{Project: &mdmv1.Project{Meta: &mdmv1.Meta{Id: "p1"}, Name: "Project 1", TenantId: "t1"}}, nil)
			},
			tenantServiceMock: func(mock *tmock.Mock) {
				mock.On("Get", tmock.Anything, &mdmv1.TenantGetRequest{
					Id: "t1",
				}).Return(&mdmv1.TenantResponse{Tenant: &mdmv1.Tenant{Meta: &mdmv1.Meta{Id: "t1"}}}, nil)
			},
			want:    &apiv2.IPServiceCreateResponse{},
			wantErr: nil,
		},
		{
			name: "create ip with non-existing project",
			ip: &apiv2.IPServiceCreateRequest{
				Project: "p2",
			},
			projectServiceMock: func(mock *tmock.Mock) {
				mock.On("Get", tmock.Anything, &mdmv1.ProjectGetRequest{
					Id: "p2",
				}).Return(nil, fmt.Errorf("project p2 not found"))
			},
			want:    nil,
			wantErr: connect.NewError(connect.CodeInternal, fmt.Errorf("error fetching cache entry: unable to get project: project p2 not found")),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mc := newMasterdataMockClient(t, tt.tenantServiceMock, nil, tt.projectServiceMock, nil)
			interceptor := NewInterceptor(logger, mc)

			ipService := ipservice.New(ipservice.Config{
				Repo: repository.New(logger, mc, nil, ipam), // FIXME datastore mock is missing
				Log:  logger,
			})

			mux := http.NewServeMux()
			mux.Handle(apiv2connect.NewIPServiceHandler(ipService, connect.WithInterceptors(interceptor)))

			server := httptest.NewUnstartedServer(mux)
			server.EnableHTTP2 = true
			server.StartTLS()
			defer server.Close()

			connectClient := apiv2connect.NewIPServiceClient(
				server.Client(),
				server.URL,
			)
			grpcClient := apiv2connect.NewIPServiceClient(
				server.Client(),
				server.URL,
				connect.WithGRPC(),
			)
			clients := []apiv2connect.IPServiceClient{connectClient, grpcClient}

			for _, client := range clients {
				ctx := token.ContextWithToken(context.Background(), &apiv2.Token{
					UserId: "t1",
				})

				got, err := client.Create(ctx, connect.NewRequest(tt.ip))
				spew.Dump(got)
				spew.Dump(err)

				if err != nil {
					if diff := cmp.Diff(tt.wantErr, err, testcommon.ErrorStringComparer()); diff != "" {
						t.Errorf("error diff (+got -want):\n %s", diff)
					}
				} else {
					require.Equal(t, got.Msg.Ip.Project, tt.ip.Project)
					// require.EqualValues(t, got.Msg.Ip.Name, *tt.ip.Name)
					require.NotEmpty(t, got.Msg.Ip.Ip)
				}
			}
		})
	}
}

func Test_tenantInterceptor_AuditingCtx(t *testing.T) {
	tests := []struct {
		name               string
		req                connect.AnyRequest
		token              *apiv2.Token
		projectServiceMock func(mock *tmock.Mock)
		tenantServiceMock  func(mock *tmock.Mock)
		wantUser           *security.User
		wantErr            error
	}{
		{
			name: "anonymous request",
			req:  connect.NewRequest(&apiv2.HealthServiceGetRequest{}),
			wantUser: &security.User{
				EMail:   "",
				Name:    "",
				Groups:  []security.ResourceAccess{},
				Tenant:  "",
				Issuer:  "",
				Subject: "",
			},
			wantErr: nil,
		},
		{
			name: "self request is best effort",
			req:  connect.NewRequest(&apiv2.ProjectServiceListRequest{}),
			token: &apiv2.Token{
				Uuid:   "a-uuid",
				UserId: "user@github",
			},
			tenantServiceMock: func(mock *tmock.Mock) {
				mock.On("Get", tmock.Anything, &mdmv1.TenantGetRequest{
					Id: "user@github",
				}).Return(&mdmv1.TenantResponse{Tenant: &mdmv1.Tenant{Meta: &mdmv1.Meta{Id: "user@github", Annotations: map[string]string{tutil.TagEmail: "mail@user"}}}}, nil)
			},
			wantUser: &security.User{
				EMail:   "mail@user",
				Name:    "",
				Groups:  []security.ResourceAccess{},
				Tenant:  "user@github",
				Issuer:  "",
				Subject: "user@github",
			},
			wantErr: nil,
		},
		{
			name: "project request",
			req: connect.NewRequest(&apiv2.IPServiceGetRequest{
				Project: "a-project",
			}),
			token: &apiv2.Token{
				Uuid:   "a-uuid",
				UserId: "user@github",
			},
			projectServiceMock: func(mock *tmock.Mock) {
				mock.On("Get", tmock.Anything, &mdmv1.ProjectGetRequest{
					Id: "a-project",
				}).Return(&mdmv1.ProjectResponse{Project: &mdmv1.Project{Meta: &mdmv1.Meta{Id: "a-project"}, Name: "Project A", TenantId: "t1"}}, nil)
			},
			tenantServiceMock: func(mock *tmock.Mock) {
				mock.On("Get", tmock.Anything, &mdmv1.TenantGetRequest{
					Id: "t1",
				}).Return(&mdmv1.TenantResponse{Tenant: &mdmv1.Tenant{Meta: &mdmv1.Meta{Id: "t1", Annotations: map[string]string{tutil.TagEmail: "mail@t1"}}}}, nil)
			},
			wantUser: &security.User{
				EMail:   "mail@t1",
				Name:    "",
				Groups:  []security.ResourceAccess{},
				Tenant:  "t1",
				Issuer:  "",
				Subject: "user@github",
			},
			wantErr: nil,
		},
		{
			name: "tenant request",
			req: connect.NewRequest(&apiv2.TenantServiceGetRequest{
				Login: "a-tenant",
			}),
			token: &apiv2.Token{
				Uuid:   "a-uuid",
				UserId: "user@github",
			},
			tenantServiceMock: func(mock *tmock.Mock) {
				mock.On("Get", tmock.Anything, &mdmv1.TenantGetRequest{
					Id: "a-tenant",
				}).Return(&mdmv1.TenantResponse{Tenant: &mdmv1.Tenant{Meta: &mdmv1.Meta{Id: "a-tenant", Annotations: map[string]string{tutil.TagEmail: "mail@tenant-a"}}}}, nil)
			},
			wantUser: &security.User{
				EMail:   "mail@tenant-a",
				Name:    "",
				Groups:  []security.ResourceAccess{},
				Tenant:  "a-tenant",
				Issuer:  "",
				Subject: "user@github",
			},
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				ctx = context.Background()
				mc  = newMasterdataMockClient(t, tt.tenantServiceMock, nil, tt.projectServiceMock, nil)
				ti  = NewInterceptor(slog.Default(), mc)

				called = false
				noopFn = func(ctx context.Context, ar connect.AnyRequest) (connect.AnyResponse, error) {
					called = true

					user := security.GetUserFromContext(ctx)
					assert.Equal(t, tt.wantUser, user)

					return nil, nil
				}
			)

			if tt.token != nil {
				ctx = token.ContextWithToken(ctx, tt.token)
			}

			_, err := ti.WrapUnary(noopFn)(ctx, tt.req)
			require.NoError(t, err)

			assert.True(t, called, "request was not forwarded to next")
		})
	}
}
