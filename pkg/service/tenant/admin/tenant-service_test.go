package admin

import (
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	"github.com/metal-stack/metal-apiserver/pkg/test"
	"github.com/metal-stack/metal-apiserver/pkg/token"
	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/durationpb"
)

func Test_tenantServiceServer_List(t *testing.T) {
	t.Parallel()

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	testStore, closer := test.StartRepositoryWithCleanup(t, log, test.WithPostgres(true))
	defer closer()

	test.CreateTenants(t, testStore, []*apiv2.TenantServiceCreateRequest{
		{Name: "john.doe@github"},
		{Name: "jane.roe@github"},
	})

	tests := []struct {
		name    string
		rq      *adminv2.TenantServiceListRequest
		want    *adminv2.TenantServiceListResponse
		wantErr error
	}{
		{
			name: "list the tenants",
			rq:   &adminv2.TenantServiceListRequest{},
			want: &adminv2.TenantServiceListResponse{
				Tenants: []*apiv2.Tenant{
					{
						Meta:        &apiv2.Meta{},
						Login:       "jane.roe@github",
						Name:        "jane.roe@github",
						Email:       "",
						Description: "",
						AvatarUrl:   "",
						CreatedBy:   "jane.roe@github",
					},
					{
						Meta:        &apiv2.Meta{},
						Login:       "john.doe@github",
						Name:        "john.doe@github",
						Email:       "",
						Description: "",
						AvatarUrl:   "",
						CreatedBy:   "john.doe@github",
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "filter by name",
			rq: &adminv2.TenantServiceListRequest{
				Name: new("jane.roe@github"),
			},
			want: &adminv2.TenantServiceListResponse{
				Tenants: []*apiv2.Tenant{
					{
						Meta:        &apiv2.Meta{},
						Login:       "jane.roe@github",
						Name:        "jane.roe@github",
						Email:       "",
						Description: "",
						AvatarUrl:   "",
						CreatedBy:   "jane.roe@github",
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "filter by login",
			rq: &adminv2.TenantServiceListRequest{
				Login: new("john.doe@github"),
			},
			want: &adminv2.TenantServiceListResponse{
				Tenants: []*apiv2.Tenant{
					{
						Meta:        &apiv2.Meta{},
						Login:       "john.doe@github",
						Name:        "john.doe@github",
						Email:       "",
						Description: "",
						AvatarUrl:   "",
						CreatedBy:   "john.doe@github",
					},
				},
			},
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := &tenantServiceServer{
				log:  log,
				repo: testStore.Store,
			}
			role := apiv2.AdminRole_ADMIN_ROLE_EDITOR
			tok := testStore.GetToken("john.doe@github", &apiv2.TokenServiceCreateRequest{
				Expires:   durationpb.New(time.Hour),
				AdminRole: &role,
			})

			reqCtx := token.ContextWithToken(t.Context(), tok)
			if tt.wantErr == nil {
				// Execute proto based validation
				test.Validate(t, tt.rq)
			}
			got, err := u.List(reqCtx, tt.rq)
			if diff := cmp.Diff(err, tt.wantErr, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("diff = %s", diff)
			}

			if diff := cmp.Diff(
				tt.want, got,
				protocmp.Transform(),
				protocmp.IgnoreFields(
					&apiv2.Meta{}, "created_at", "updated_at",
				),
			); diff != "" {
				t.Errorf("%v, want %v diff: %s", got, tt.want, diff)
			}
		})
	}
}

func Test_tenantServiceServer_Create(t *testing.T) {
	t.Parallel()

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	testStore, closer := test.StartRepositoryWithCleanup(t, log, test.WithPostgres(true))
	defer closer()

	test.CreateTenants(t, testStore, []*apiv2.TenantServiceCreateRequest{
		{Name: "john.doe@github"},
	})

	tests := []struct {
		name    string
		rq      *adminv2.TenantServiceCreateRequest
		want    *adminv2.TenantServiceCreateResponse
		wantErr error
	}{
		{
			name: "create a tenant",
			rq: &adminv2.TenantServiceCreateRequest{
				Name:        "jane.roe@github",
				Description: new("desc"),
				Email:       new("mail@test.com"),
				AvatarUrl:   new("http://avatar-url.com"),
				Labels: &apiv2.Labels{
					Labels: map[string]string{
						"a": "b",
					},
				},
			},
			want: &adminv2.TenantServiceCreateResponse{
				Tenant: &apiv2.Tenant{
					Meta: &apiv2.Meta{
						Labels: &apiv2.Labels{
							Labels: map[string]string{
								"a": "b",
							},
						},
					},
					Login:       "<some-uuid>",
					Name:        "jane.roe@github",
					Email:       "mail@test.com",
					Description: "desc",
					AvatarUrl:   "http://avatar-url.com",
					CreatedBy:   "john.doe@github",
				},
			},
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := &tenantServiceServer{
				log:  log,
				repo: testStore.Store,
			}
			role := apiv2.AdminRole_ADMIN_ROLE_EDITOR
			tok := testStore.GetToken("john.doe@github", &apiv2.TokenServiceCreateRequest{
				Expires:   durationpb.New(time.Hour),
				AdminRole: &role,
			})

			reqCtx := token.ContextWithToken(t.Context(), tok)
			if tt.wantErr == nil {
				// Execute proto based validation
				test.Validate(t, tt.rq)
			}
			got, err := u.Create(reqCtx, tt.rq)
			if diff := cmp.Diff(err, tt.wantErr, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("diff = %s", diff)
			}

			if diff := cmp.Diff(
				tt.want, got,
				protocmp.Transform(),
				protocmp.IgnoreFields(
					&apiv2.Meta{}, "created_at", "updated_at",
				),
				protocmp.IgnoreFields(
					&apiv2.Tenant{}, "login",
				),
			); diff != "" {
				t.Errorf("%v, want %v diff: %s", got, tt.want, diff)
			}

			assert.NotEmpty(t, got.Tenant.Login)
		})
	}
}
