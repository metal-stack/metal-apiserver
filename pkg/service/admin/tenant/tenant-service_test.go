package admin

import (
	"log/slog"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/metal-stack/api/go/errorutil"
	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/api/go/tag"
	"github.com/metal-stack/metal-apiserver/pkg/repository/api"
	"github.com/metal-stack/metal-apiserver/pkg/test"
	"github.com/metal-stack/metal-apiserver/pkg/token"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
		rq      *apiv2.TenantQuery
		want    *adminv2.TenantServiceListResponse
		wantErr error
	}{
		{
			name: "list the tenants",
			rq:   &apiv2.TenantQuery{},
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
			rq: &apiv2.TenantQuery{
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
			rq: &apiv2.TenantQuery{
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
		{
			name: "request first page",
			rq: &apiv2.TenantQuery{
				Paging: &apiv2.Paging{
					Page:  new(uint64(1)),
					Count: new(uint64(1)),
				},
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
			got, err := u.List(reqCtx, &adminv2.TenantServiceListRequest{
				Query: tt.rq,
			})
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

func Test_tenantServiceServer_AddMember(t *testing.T) {
	t.Parallel()

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	testStore, closer := test.StartRepositoryWithCleanup(t, log, test.WithPostgres(true))
	defer closer()

	test.CreateTenants(t, testStore, []*apiv2.TenantServiceCreateRequest{
		{Name: "john.doe@github"},
		{Name: "jane.roe@github"},
		{Name: "sam.sane@github"},
	})
	// CreateTenants creates tenants directly without OWNER memberships, so we add one explicitly.
	test.CreateTenantMemberships(t, testStore, "john.doe@github", []*api.TenantMemberCreateRequest{
		{MemberID: "john.doe@github", Role: apiv2.TenantRole_TENANT_ROLE_OWNER},
	})

	tests := []struct {
		name    string
		rq      *adminv2.TenantServiceAddMemberRequest
		wantErr error
	}{
		{
			name: "add a member",
			rq: &adminv2.TenantServiceAddMemberRequest{
				Tenant: "john.doe@github",
				Member: "sam.sane@github",
				Role:   apiv2.TenantRole_TENANT_ROLE_EDITOR,
			},
		},
		{
			name: "add already existing member",
			rq: &adminv2.TenantServiceAddMemberRequest{
				Tenant: "john.doe@github",
				Member: "john.doe@github",
				Role:   apiv2.TenantRole_TENANT_ROLE_EDITOR,
			},
			wantErr: errorutil.Conflict(`tenant with id "john.doe@github" already is member in tenant: "john.doe@github"`),
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
			_, err := u.AddMember(reqCtx, tt.rq)
			if diff := cmp.Diff(err, tt.wantErr, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("diff = %s", diff)
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

func Test_EnsureProviderTenant(t *testing.T) {
	t.Parallel()

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	testStore, closer := test.StartRepositoryWithCleanup(t, log, test.WithPostgres(true))
	defer closer()

	tests := []struct {
		name             string
		providerTenantID string
		existingTenants  []*apiv2.TenantServiceCreateRequest
		wantErr          error
	}{
		{
			name:             "ensure provider tenant on fresh database",
			providerTenantID: "metal-stack",
			wantErr:          nil,
		},
		{
			name: "ensure provider tenant next to existing tenants",
			existingTenants: []*apiv2.TenantServiceCreateRequest{
				{
					Name: "tenant-a",
				},
				{
					Name: "tenant-b",
				},
			},
			providerTenantID: "metal-stack",
			wantErr:          nil,
		},
		{
			name:             "ensure label added on existing tenant",
			providerTenantID: "metal-stack",
			existingTenants: []*apiv2.TenantServiceCreateRequest{
				{
					Name: "metal-stack",
				},
			},
			wantErr: nil,
		},
		{
			name:             "provider tenant already present",
			providerTenantID: "metal-stack",
			existingTenants: []*apiv2.TenantServiceCreateRequest{
				{
					Name: "metal-stack",
					Labels: &apiv2.Labels{
						Labels: map[string]string{
							tag.ProviderTenant: strconv.FormatBool(true),
						},
					},
				},
			},
			wantErr: nil,
		},
		{
			name:             "provider tenant already present, but want to create another one",
			providerTenantID: "i-want-to-get-admin",
			existingTenants: []*apiv2.TenantServiceCreateRequest{
				{
					Name: "metal-stack",
					Labels: &apiv2.Labels{
						Labels: map[string]string{
							tag.ProviderTenant: strconv.FormatBool(true),
						},
					},
				},
			},
			wantErr: errorutil.InvalidArgument(`provider tenant "metal-stack" already exists, refusing to create another one with id "i-want-to-get-admin"`),
		},
		{
			name:             "two provider tenants present for some reason, results into an error",
			providerTenantID: "metal-stack",
			existingTenants: []*apiv2.TenantServiceCreateRequest{
				{
					Name: "metal-stack",
					Labels: &apiv2.Labels{
						Labels: map[string]string{
							tag.ProviderTenant: strconv.FormatBool(true),
						},
					},
				},
				{
					Name: "metal-stack-2",
					Labels: &apiv2.Labels{
						Labels: map[string]string{
							tag.ProviderTenant: strconv.FormatBool(true),
						},
					},
				},
			},
			wantErr: errorutil.Internal(`unable to find unique provider tenant "metal-stack": more than one tenant exists`),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(innerT *testing.T) {
			ctx := innerT.Context()
			defer testStore.Cleanup(t)

			_ = test.CreateTenants(innerT, testStore, tt.existingTenants)

			err := testStore.Tenant().AdditionalMethods().EnsureProviderTenant(ctx, tt.providerTenantID)
			if diff := cmp.Diff(err, tt.wantErr, errorutil.ConnectErrorComparer()); diff != "" {
				innerT.Errorf("diff = %s", diff)
			}

			if tt.wantErr != nil {
				return
			}

			tenant, err := testStore.Tenant().Get(ctx, tt.providerTenantID)
			require.NoError(innerT, err)

			assert.Equal(innerT, &apiv2.Labels{
				Labels: map[string]string{tag.ProviderTenant: "true"},
			}, tenant.Meta.Labels, "provider tenant missing provider tenant label")

			member, err := testStore.Tenant().AdditionalMethods().Member(tenant.Login).Get(ctx, tenant.Login)
			require.NoError(innerT, err)

			assert.Equal(innerT, apiv2.TenantRole_TENANT_ROLE_OWNER, member.Role)
		})
	}
}
