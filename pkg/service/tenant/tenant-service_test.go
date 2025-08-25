package tenant

import (
	"log/slog"
	"os"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/go-cmp/cmp"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	"github.com/metal-stack/metal-apiserver/pkg/repository"
	"github.com/metal-stack/metal-apiserver/pkg/test"
	"github.com/metal-stack/metal-apiserver/pkg/token"
	"github.com/metal-stack/metal-lib/pkg/pointer"
	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/durationpb"
)

func Test_tenantServiceServer_Get(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	testStore, closer := test.StartRepositoryWithCleanup(t, log, test.WithCockroach(true))
	defer closer()
	repo := testStore.Store

	test.CreateTenants(t, testStore, []*apiv2.TenantServiceCreateRequest{
		{
			Name:        "john.doe@github",
			Email:       pointer.Pointer("john.doe@github.com"),
			Description: pointer.Pointer("a description"),
			AvatarUrl:   pointer.Pointer("http://test"),
			Labels: &apiv2.Labels{
				Labels: map[string]string{
					"a": "b",
				},
			},
		},
	})

	tests := []struct {
		name    string
		rq      *apiv2.TenantServiceGetRequest
		want    *apiv2.TenantServiceGetResponse
		as      *apiv2.TenantRole
		wantErr error
	}{
		{
			name: "get a tenant",
			rq: &apiv2.TenantServiceGetRequest{
				Login: "john.doe@github",
			},
			want: &apiv2.TenantServiceGetResponse{
				Tenant: &apiv2.Tenant{
					Meta: &apiv2.Meta{
						Labels: &apiv2.Labels{
							Labels: map[string]string{
								"a": "b",
							},
						},
					},
					Name:        "john.doe@github",
					Login:       "john.doe@github",
					Email:       "john.doe@github.com",
					Description: "a description",
					AvatarUrl:   "http://test",
					CreatedBy:   "john.doe@github",
				},
			},
			wantErr: nil,
		},
		{
			name: "get a tenant as guest member",
			rq: &apiv2.TenantServiceGetRequest{
				Login: "john.doe@github",
			},
			as: pointer.Pointer(apiv2.TenantRole_TENANT_ROLE_GUEST),
			want: &apiv2.TenantServiceGetResponse{
				Tenant: &apiv2.Tenant{
					Meta:        &apiv2.Meta{},
					Name:        "john.doe@github",
					Login:       "john.doe@github",
					Description: "a description",
					AvatarUrl:   "http://test",
				},
			},
			wantErr: nil,
		},
		{
			name: "get a tenant that does not exist",
			rq: &apiv2.TenantServiceGetRequest{
				Login: "will.smith@github",
			},
			want:    nil,
			wantErr: errorutil.NotFound("tenant with id:will.smith@github not found sql: no rows in result set"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := &tenantServiceServer{
				log:         log,
				repo:        repo,
				inviteStore: testStore.GetTenantInviteStore(),
				tokenStore:  testStore.GetTokenStore(),
			}

			as := apiv2.TenantRole_TENANT_ROLE_OWNER
			if tt.as != nil {
				as = *tt.as
			}

			tok := testStore.GetToken("john.doe@github", &apiv2.TokenServiceCreateRequest{
				Expires: durationpb.New(time.Hour),
				TenantRoles: map[string]apiv2.TenantRole{
					"john.doe@github": as,
				},
			})

			reqCtx := token.ContextWithToken(t.Context(), tok)

			got, err := u.Get(reqCtx, connect.NewRequest(tt.rq))
			if diff := cmp.Diff(err, tt.wantErr, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("diff = %s", diff)
			}

			if diff := cmp.Diff(
				tt.want, pointer.SafeDeref(got).Msg,
				protocmp.Transform(),
				protocmp.IgnoreFields(
					&apiv2.Meta{}, "created_at", "updated_at",
				),
			); diff != "" {
				t.Errorf("%v, want %v diff: %s", got.Msg, tt.want, diff)
			}
		})
	}
}

func Test_tenantServiceServer_List(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	testStore, closer := test.StartRepositoryWithCleanup(t, log, test.WithCockroach(true))
	defer closer()
	repo := testStore.Store

	test.CreateTenants(t, testStore, []*apiv2.TenantServiceCreateRequest{
		{Name: "john.doe@github"},
		{Name: "will.smith@github"},
		{Name: "b950f4f5-d8b8-4252-aa02-ae08a1d2b044"},
	})

	// create some memberships
	test.CreateTenantMemberships(t, testStore, "john.doe@github", []*repository.TenantMemberCreateRequest{
		{MemberID: "john.doe@github", Role: apiv2.TenantRole_TENANT_ROLE_OWNER},
	})
	test.CreateTenantMemberships(t, testStore, "will.smith@github", []*repository.TenantMemberCreateRequest{
		{MemberID: "will.smith@github", Role: apiv2.TenantRole_TENANT_ROLE_OWNER},
	})
	test.CreateTenantMemberships(t, testStore, "b950f4f5-d8b8-4252-aa02-ae08a1d2b044", []*repository.TenantMemberCreateRequest{
		{MemberID: "john.doe@github", Role: apiv2.TenantRole_TENANT_ROLE_OWNER},
	})

	tests := []struct {
		name    string
		rq      *apiv2.TenantServiceListRequest
		want    *apiv2.TenantServiceListResponse
		wantErr error
	}{
		{
			name: "list the tenants",
			rq:   &apiv2.TenantServiceListRequest{},
			want: &apiv2.TenantServiceListResponse{
				Tenants: []*apiv2.Tenant{
					{
						Meta:  &apiv2.Meta{},
						Name:  "b950f4f5-d8b8-4252-aa02-ae08a1d2b044",
						Login: "b950f4f5-d8b8-4252-aa02-ae08a1d2b044",
					},
					{
						Meta:  &apiv2.Meta{},
						Name:  "john.doe@github",
						Login: "john.doe@github",
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "list the tenants filtered by id",
			rq: &apiv2.TenantServiceListRequest{
				Id: pointer.Pointer("john.doe@github"),
			},
			want: &apiv2.TenantServiceListResponse{
				Tenants: []*apiv2.Tenant{
					{
						Meta:  &apiv2.Meta{},
						Name:  "john.doe@github",
						Login: "john.doe@github",
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "list the tenants filtered by name",
			rq: &apiv2.TenantServiceListRequest{
				Name: pointer.Pointer("b950f4f5-d8b8-4252-aa02-ae08a1d2b044"),
			},
			want: &apiv2.TenantServiceListResponse{
				Tenants: []*apiv2.Tenant{
					{
						Meta:  &apiv2.Meta{},
						Name:  "b950f4f5-d8b8-4252-aa02-ae08a1d2b044",
						Login: "b950f4f5-d8b8-4252-aa02-ae08a1d2b044",
					},
				},
			},
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := &tenantServiceServer{
				log:         log,
				repo:        repo,
				inviteStore: testStore.GetTenantInviteStore(),
				tokenStore:  testStore.GetTokenStore(),
			}

			tok := testStore.GetToken("john.doe@github", &apiv2.TokenServiceCreateRequest{
				Expires: durationpb.New(time.Hour),
				TenantRoles: map[string]apiv2.TenantRole{
					"john.doe@github": apiv2.TenantRole_TENANT_ROLE_OWNER,
				},
			})

			reqCtx := token.ContextWithToken(t.Context(), tok)

			got, err := u.List(reqCtx, connect.NewRequest(tt.rq))
			if diff := cmp.Diff(err, tt.wantErr, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("diff = %s", diff)
			}

			if diff := cmp.Diff(
				tt.want, pointer.SafeDeref(got).Msg,
				protocmp.Transform(),
				protocmp.IgnoreFields(
					&apiv2.Meta{}, "created_at", "updated_at",
				),
			); diff != "" {
				t.Errorf("%v, want %v diff: %s", got.Msg, tt.want, diff)
			}
		})
	}
}

func Test_tenantServiceServer_Create(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	testStore, closer := test.StartRepositoryWithCleanup(t, log, test.WithCockroach(true))
	defer closer()
	repo := testStore.Store

	test.CreateTenants(t, testStore, []*apiv2.TenantServiceCreateRequest{{Name: "john.doe@github"}})

	tests := []struct {
		name        string
		rq          *apiv2.TenantServiceCreateRequest
		want        *apiv2.TenantServiceCreateResponse
		wantMembers []*apiv2.TenantMember
		wantErr     error
	}{
		{
			name: "create a tenant",
			rq: &apiv2.TenantServiceCreateRequest{
				Name:        "My New Org Tenant",
				Description: pointer.Pointer("tenant desc"),
				Email:       pointer.Pointer("tenant@github"),
				AvatarUrl:   pointer.Pointer("http://test"),
				Labels: &apiv2.Labels{
					Labels: map[string]string{
						"a": "b",
					},
				},
			},
			want: &apiv2.TenantServiceCreateResponse{
				Tenant: &apiv2.Tenant{
					Login: "a-uuid",
					Meta: &apiv2.Meta{
						Labels: &apiv2.Labels{
							Labels: map[string]string{
								"a": "b",
							},
						},
					},
					Name:        "My New Org Tenant",
					Email:       "tenant@github",
					Description: "tenant desc",
					AvatarUrl:   "http://test",
					CreatedBy:   "john.doe@github",
				},
			},
			wantMembers: []*apiv2.TenantMember{
				{
					Id:       "john.doe@github",
					Role:     apiv2.TenantRole_TENANT_ROLE_OWNER,
					Projects: []string{},
				},
			},
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := &tenantServiceServer{
				log:         log,
				repo:        repo,
				inviteStore: testStore.GetTenantInviteStore(),
				tokenStore:  testStore.GetTokenStore(),
			}

			tok := testStore.GetToken("john.doe@github", &apiv2.TokenServiceCreateRequest{
				Expires: durationpb.New(time.Hour),
				TenantRoles: map[string]apiv2.TenantRole{
					"john.doe@github": apiv2.TenantRole_TENANT_ROLE_OWNER,
				},
			})

			reqCtx := token.ContextWithToken(t.Context(), tok)

			got, err := u.Create(reqCtx, connect.NewRequest(tt.rq))
			if diff := cmp.Diff(err, tt.wantErr, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("diff = %s", diff)
			}

			assert.NotEmpty(t, got.Msg.Tenant.Login)

			if diff := cmp.Diff(
				tt.want, pointer.SafeDeref(got).Msg,
				protocmp.Transform(),
				protocmp.IgnoreFields(
					&apiv2.Meta{}, "created_at", "updated_at",
				),
				protocmp.IgnoreFields(
					&apiv2.Tenant{}, "login",
				),
			); diff != "" {
				t.Errorf("%v, want %v diff: %s", got.Msg, tt.want, diff)
			}

			// to check whether the owner membership was created, we need to get the tenant as well
			// as we do not request through opa auther, we also need to extend the token

			tok.TenantRoles[got.Msg.Tenant.Login] = apiv2.TenantRole_TENANT_ROLE_OWNER

			reqCtx = token.ContextWithToken(t.Context(), tok)

			getResp, err := u.Get(reqCtx, connect.NewRequest(&apiv2.TenantServiceGetRequest{
				Login: got.Msg.Tenant.Login,
			}))
			if diff := cmp.Diff(err, tt.wantErr, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("diff = %s", diff)
			}

			tt.want.Tenant.Login = got.Msg.Tenant.Login

			if diff := cmp.Diff(
				tt.want.Tenant, pointer.SafeDeref(getResp).Msg.Tenant,
				protocmp.Transform(),
				protocmp.IgnoreFields(
					&apiv2.Meta{}, "created_at", "updated_at",
				),
			); diff != "" {
				t.Errorf("%v, want %v diff: %s", getResp.Msg.Tenant, tt.want.Tenant, diff)
			}
			if diff := cmp.Diff(
				tt.wantMembers, pointer.SafeDeref(getResp).Msg.TenantMembers,
				protocmp.Transform(),
				protocmp.IgnoreFields(
					&apiv2.TenantMember{}, "created_at",
				),
			); diff != "" {
				t.Errorf("%v, want %v diff: %s", getResp.Msg.TenantMembers, tt.wantMembers, diff)
			}
		})
	}
}

func Test_tenantServiceServer_Update(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	testStore, closer := test.StartRepositoryWithCleanup(t, log, test.WithCockroach(true))
	defer closer()
	repo := testStore.Store

	test.CreateTenants(t, testStore, []*apiv2.TenantServiceCreateRequest{{
		Name:        "john.doe@github",
		Description: pointer.Pointer("old desc"),
		Email:       pointer.Pointer("old mail"),
		AvatarUrl:   pointer.Pointer("http://old"),
		Labels: &apiv2.Labels{
			Labels: map[string]string{
				"a": "b",
			},
		},
	}})

	tests := []struct {
		name    string
		rq      *apiv2.TenantServiceUpdateRequest
		want    *apiv2.TenantServiceUpdateResponse
		wantErr error
	}{
		{
			name: "create a tenant",
			rq: &apiv2.TenantServiceUpdateRequest{
				Login:       "john.doe@github",
				Name:        pointer.Pointer("new name"),
				Description: pointer.Pointer("new desc"),
				Email:       pointer.Pointer("new mail"),
				AvatarUrl:   pointer.Pointer("http://new"),
				Labels: &apiv2.UpdateLabels{
					Update: &apiv2.Labels{
						Labels: map[string]string{
							"c": "d",
						},
					},
				},
			},
			want: &apiv2.TenantServiceUpdateResponse{
				Tenant: &apiv2.Tenant{
					Login: "john.doe@github",
					Meta: &apiv2.Meta{
						Labels: &apiv2.Labels{
							Labels: map[string]string{
								"a": "b",
								"c": "d",
							},
						},
					},
					Name:        "new name",
					Email:       "new mail",
					Description: "new desc",
					AvatarUrl:   "http://new",
					CreatedBy:   "john.doe@github",
				},
			},
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := &tenantServiceServer{
				log:         log,
				repo:        repo,
				inviteStore: testStore.GetTenantInviteStore(),
				tokenStore:  testStore.GetTokenStore(),
			}

			tok := testStore.GetToken("john.doe@github", &apiv2.TokenServiceCreateRequest{
				Expires: durationpb.New(time.Hour),
				TenantRoles: map[string]apiv2.TenantRole{
					"john.doe@github": apiv2.TenantRole_TENANT_ROLE_OWNER,
				},
			})

			reqCtx := token.ContextWithToken(t.Context(), tok)

			got, err := u.Update(reqCtx, connect.NewRequest(tt.rq))
			if diff := cmp.Diff(err, tt.wantErr, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("diff = %s", diff)
			}

			assert.NotEmpty(t, got.Msg.Tenant.Login)

			if diff := cmp.Diff(
				tt.want, pointer.SafeDeref(got).Msg,
				protocmp.Transform(),
				protocmp.IgnoreFields(
					&apiv2.Meta{}, "created_at", "updated_at",
				),
				protocmp.IgnoreFields(
					&apiv2.Tenant{}, "login",
				),
			); diff != "" {
				t.Errorf("%v, want %v diff: %s", got.Msg, tt.want, diff)
			}
		})
	}
}
