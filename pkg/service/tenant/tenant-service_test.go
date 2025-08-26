package tenant

import (
	"fmt"
	"log/slog"
	"os"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/go-cmp/cmp"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	"github.com/metal-stack/metal-apiserver/pkg/invite"
	"github.com/metal-stack/metal-apiserver/pkg/repository"
	"github.com/metal-stack/metal-apiserver/pkg/test"
	"github.com/metal-stack/metal-apiserver/pkg/token"
	"github.com/metal-stack/metal-lib/pkg/pointer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
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
		{Name: "will.smith@github"},  // direct tenant member
		{Name: "tina.turner@github"}, // inherited tenant member
	})
	test.CreateProjects(t, repo, []*apiv2.ProjectServiceCreateRequest{
		{
			Name:  "project-a",
			Login: "john.doe@github",
		},
	})

	test.CreateTenantMemberships(t, testStore, "john.doe@github", []*repository.TenantMemberCreateRequest{
		{MemberID: "john.doe@github", Role: apiv2.TenantRole_TENANT_ROLE_OWNER},
		{MemberID: "will.smith@github", Role: apiv2.TenantRole_TENANT_ROLE_EDITOR},
	})
	test.CreateProjectMemberships(t, testStore, "project-a", []*repository.ProjectMemberCreateRequest{
		{TenantId: "john.doe@github", Role: apiv2.ProjectRole_PROJECT_ROLE_OWNER},
		{TenantId: "tina.turner@github", Role: apiv2.ProjectRole_PROJECT_ROLE_VIEWER},
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
				TenantMembers: []*apiv2.TenantMember{
					{
						Id:       "john.doe@github",
						Projects: []string{"project-a"},
						Role:     apiv2.TenantRole_TENANT_ROLE_OWNER,
					},
					{
						Id:       "tina.turner@github",
						Projects: []string{"project-a"},
						Role:     apiv2.TenantRole_TENANT_ROLE_GUEST,
					},
					{
						Id:   "will.smith@github",
						Role: apiv2.TenantRole_TENANT_ROLE_EDITOR,
					},
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
				Login: "no.one@github",
			},
			want:    nil,
			wantErr: errorutil.NotFound("tenant with id:no.one@github not found sql: no rows in result set"),
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
				protocmp.IgnoreFields(
					&apiv2.TenantMember{}, "created_at",
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
						Meta:      &apiv2.Meta{},
						Name:      "b950f4f5-d8b8-4252-aa02-ae08a1d2b044",
						Login:     "b950f4f5-d8b8-4252-aa02-ae08a1d2b044",
						CreatedBy: "b950f4f5-d8b8-4252-aa02-ae08a1d2b044",
					},
					{
						Meta:      &apiv2.Meta{},
						Name:      "john.doe@github",
						Login:     "john.doe@github",
						CreatedBy: "john.doe@github",
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
						Meta:      &apiv2.Meta{},
						Name:      "john.doe@github",
						Login:     "john.doe@github",
						CreatedBy: "john.doe@github",
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
						Meta:      &apiv2.Meta{},
						Name:      "b950f4f5-d8b8-4252-aa02-ae08a1d2b044",
						Login:     "b950f4f5-d8b8-4252-aa02-ae08a1d2b044",
						CreatedBy: "b950f4f5-d8b8-4252-aa02-ae08a1d2b044",
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

func Test_tenantServiceServer_Delete(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	testStore, closer := test.StartRepositoryWithCleanup(t, log, test.WithCockroach(true))
	defer closer()
	repo := testStore.Store

	tests := []struct {
		name             string
		rq               *apiv2.TenantServiceDeleteRequest
		want             *apiv2.TenantServiceDeleteResponse
		existingTenants  []*apiv2.TenantServiceCreateRequest
		existingProjects []*apiv2.ProjectServiceCreateRequest
		wantErr          error
	}{
		{
			name: "delete a tenant",
			rq: &apiv2.TenantServiceDeleteRequest{
				Login: "tenant-a",
			},
			existingTenants: []*apiv2.TenantServiceCreateRequest{
				{Name: "john.doe@github"},
				{Name: "tenant-a"},
			},
			want: &apiv2.TenantServiceDeleteResponse{
				Tenant: &apiv2.Tenant{
					Meta:      &apiv2.Meta{},
					Name:      "tenant-a",
					Login:     "tenant-a",
					CreatedBy: "tenant-a",
				},
			},
			wantErr: nil,
		},
		{
			name: "delete the own tenant is not possible",
			rq: &apiv2.TenantServiceDeleteRequest{
				Login: "john.doe@github",
			},
			existingTenants: []*apiv2.TenantServiceCreateRequest{
				{Name: "john.doe@github"},
			},
			wantErr: errorutil.InvalidArgument("the personal tenant (default-tenant) cannot be deleted"),
		},
		{
			name: "delete non-existing tenant",
			rq: &apiv2.TenantServiceDeleteRequest{
				Login: "tenant-a",
			},
			existingTenants: []*apiv2.TenantServiceCreateRequest{
				{Name: "john.doe@github"},
			},
			wantErr: errorutil.NotFound("tenant with id:tenant-a not found sql: no rows in result set"),
		},
		{
			name: "cannot delete tenant when projects are still present",
			rq: &apiv2.TenantServiceDeleteRequest{
				Login: "tenant-a",
			},
			existingTenants: []*apiv2.TenantServiceCreateRequest{
				{Name: "john.doe@github"},
				{Name: "tenant-a"},
			},
			existingProjects: []*apiv2.ProjectServiceCreateRequest{
				{Name: "project-a", Login: "tenant-a"},
			},
			wantErr: connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("there are still projects associated with this tenant, you need to delete them first")),
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

			test.CreateTenants(t, testStore, tt.existingTenants)
			test.CreateProjects(t, repo, tt.existingProjects)
			defer func() {
				testStore.DeleteProjects()
				testStore.DeleteTenants()
			}()

			tok := testStore.GetToken("john.doe@github", &apiv2.TokenServiceCreateRequest{
				Expires: durationpb.New(time.Hour),
				TenantRoles: map[string]apiv2.TenantRole{
					"john.doe@github": apiv2.TenantRole_TENANT_ROLE_OWNER,
					"tenant-a":        apiv2.TenantRole_TENANT_ROLE_OWNER,
				},
			})

			reqCtx := token.ContextWithToken(t.Context(), tok)

			got, err := u.Delete(reqCtx, connect.NewRequest(tt.rq))
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

func Test_tenantServiceServer_MemberUpdate(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	testStore, closer := test.StartRepositoryWithCleanup(t, log, test.WithCockroach(true))
	defer closer()
	repo := testStore.Store

	tests := []struct {
		name                  string
		rq                    *apiv2.TenantServiceUpdateMemberRequest
		existingTenants       []*apiv2.TenantServiceCreateRequest
		existingTenantMembers map[string][]*repository.TenantMemberCreateRequest
		want                  *apiv2.TenantServiceUpdateMemberResponse
		wantErr               error
	}{
		{
			name: "update a member",
			rq: &apiv2.TenantServiceUpdateMemberRequest{
				Login:  "b950f4f5-d8b8-4252-aa02-ae08a1d2b044",
				Member: "will.smith@github",
				Role:   apiv2.TenantRole_TENANT_ROLE_EDITOR,
			},
			existingTenants: []*apiv2.TenantServiceCreateRequest{
				{Name: "john.doe@github"},
				{Name: "b950f4f5-d8b8-4252-aa02-ae08a1d2b044"},
				{Name: "will.smith@github"},
			},
			existingTenantMembers: map[string][]*repository.TenantMemberCreateRequest{
				"b950f4f5-d8b8-4252-aa02-ae08a1d2b044": {
					{MemberID: "john.doe@github", Role: apiv2.TenantRole_TENANT_ROLE_OWNER},
					{MemberID: "will.smith@github", Role: apiv2.TenantRole_TENANT_ROLE_OWNER},
				},
			},
			want: &apiv2.TenantServiceUpdateMemberResponse{
				TenantMember: &apiv2.TenantMember{
					Id:   "will.smith@github",
					Role: apiv2.TenantRole_TENANT_ROLE_EDITOR,
				},
			},
			wantErr: nil,
		},
		{
			name: "unable to demote own default tenant",
			rq: &apiv2.TenantServiceUpdateMemberRequest{
				Login:  "john.doe@github",
				Member: "john.doe@github",
				Role:   apiv2.TenantRole_TENANT_ROLE_EDITOR,
			},
			existingTenants: []*apiv2.TenantServiceCreateRequest{
				{Name: "john.doe@github"},
			},
			existingTenantMembers: map[string][]*repository.TenantMemberCreateRequest{
				"john.doe@github": {
					{MemberID: "john.doe@github", Role: apiv2.TenantRole_TENANT_ROLE_OWNER},
				},
			},
			wantErr: errorutil.InvalidArgument("cannot demote a user's role within their own default tenant"),
		},
		{
			name: "unable to demote last owner",
			rq: &apiv2.TenantServiceUpdateMemberRequest{
				Login:  "b950f4f5-d8b8-4252-aa02-ae08a1d2b044",
				Member: "john.doe@github",
				Role:   apiv2.TenantRole_TENANT_ROLE_EDITOR,
			},
			existingTenants: []*apiv2.TenantServiceCreateRequest{
				{Name: "john.doe@github"},
				{Name: "b950f4f5-d8b8-4252-aa02-ae08a1d2b044"},
			},
			existingTenantMembers: map[string][]*repository.TenantMemberCreateRequest{
				"b950f4f5-d8b8-4252-aa02-ae08a1d2b044": {
					{MemberID: "john.doe@github", Role: apiv2.TenantRole_TENANT_ROLE_OWNER},
				},
			},
			wantErr: errorutil.InvalidArgument("cannot demote last owner's permissions"),
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

			test.CreateTenants(t, testStore, tt.existingTenants)
			if tt.existingTenantMembers != nil {
				for tenant, members := range tt.existingTenantMembers {
					test.CreateTenantMemberships(t, testStore, tenant, members)
				}
			}
			defer func() {
				testStore.DeleteTenants()
			}()

			tok := testStore.GetToken("john.doe@github", &apiv2.TokenServiceCreateRequest{
				Expires: durationpb.New(time.Hour),
				TenantRoles: map[string]apiv2.TenantRole{
					"john.doe@github": apiv2.TenantRole_TENANT_ROLE_OWNER,
				},
			})

			reqCtx := token.ContextWithToken(t.Context(), tok)

			got, err := u.UpdateMember(reqCtx, connect.NewRequest(tt.rq))
			if diff := cmp.Diff(err, tt.wantErr, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("diff = %s", diff)
			}

			if diff := cmp.Diff(
				tt.want, pointer.SafeDeref(got).Msg,
				protocmp.Transform(),
				protocmp.IgnoreFields(
					&apiv2.TenantMember{}, "created_at",
				),
			); diff != "" {
				t.Errorf("%v, want %v diff: %s", got.Msg, tt.want, diff)
			}
		})
	}
}

func Test_tenantServiceServer_MemberRemove(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	testStore, closer := test.StartRepositoryWithCleanup(t, log, test.WithCockroach(true))
	defer closer()
	repo := testStore.Store

	tests := []struct {
		name                  string
		existingTenants       []*apiv2.TenantServiceCreateRequest
		existingTenantMembers map[string][]*repository.TenantMemberCreateRequest
		rq                    *apiv2.TenantServiceRemoveMemberRequest
		wantErr               error
	}{
		{
			name: "remove a member",
			rq: &apiv2.TenantServiceRemoveMemberRequest{
				Login:  "b950f4f5-d8b8-4252-aa02-ae08a1d2b044",
				Member: "will.smith@github",
			},
			existingTenants: []*apiv2.TenantServiceCreateRequest{
				{Name: "john.doe@github"},
				{Name: "b950f4f5-d8b8-4252-aa02-ae08a1d2b044"},
				{Name: "will.smith@github"},
			},
			existingTenantMembers: map[string][]*repository.TenantMemberCreateRequest{
				"b950f4f5-d8b8-4252-aa02-ae08a1d2b044": {
					{MemberID: "john.doe@github", Role: apiv2.TenantRole_TENANT_ROLE_OWNER},
					{MemberID: "will.smith@github", Role: apiv2.TenantRole_TENANT_ROLE_OWNER},
				},
			},
			wantErr: nil,
		},
		{
			name: "unable to remove own default tenant",
			rq: &apiv2.TenantServiceRemoveMemberRequest{
				Login:  "john.doe@github",
				Member: "john.doe@github",
			},
			existingTenants: []*apiv2.TenantServiceCreateRequest{
				{Name: "john.doe@github"},
				{Name: "will.smith@github"},
			},
			existingTenantMembers: map[string][]*repository.TenantMemberCreateRequest{
				"john.doe@github": {
					{MemberID: "john.doe@github", Role: apiv2.TenantRole_TENANT_ROLE_OWNER},
					{MemberID: "will.smith@github", Role: apiv2.TenantRole_TENANT_ROLE_OWNER},
				},
			},
			wantErr: errorutil.InvalidArgument("cannot remove a member from their own default tenant"),
		},
		{
			name: "unable to remove last owner",
			rq: &apiv2.TenantServiceRemoveMemberRequest{
				Login:  "b950f4f5-d8b8-4252-aa02-ae08a1d2b044",
				Member: "john.doe@github",
			},
			existingTenants: []*apiv2.TenantServiceCreateRequest{
				{Name: "john.doe@github"},
				{Name: "b950f4f5-d8b8-4252-aa02-ae08a1d2b044"},
			},
			existingTenantMembers: map[string][]*repository.TenantMemberCreateRequest{
				"b950f4f5-d8b8-4252-aa02-ae08a1d2b044": {
					{MemberID: "john.doe@github", Role: apiv2.TenantRole_TENANT_ROLE_OWNER},
				},
			},
			wantErr: errorutil.InvalidArgument("cannot remove last owner of a tenant"),
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

			test.CreateTenants(t, testStore, tt.existingTenants)
			if tt.existingTenantMembers != nil {
				for tenant, members := range tt.existingTenantMembers {
					test.CreateTenantMemberships(t, testStore, tenant, members)
				}
			}
			defer func() {
				testStore.DeleteTenants()
			}()

			tok := testStore.GetToken("john.doe@github", &apiv2.TokenServiceCreateRequest{
				Expires: durationpb.New(time.Hour),
				TenantRoles: map[string]apiv2.TenantRole{
					"john.doe@github": apiv2.TenantRole_TENANT_ROLE_OWNER,
				},
			})

			reqCtx := token.ContextWithToken(t.Context(), tok)

			_, err := u.RemoveMember(reqCtx, connect.NewRequest(tt.rq))
			if diff := cmp.Diff(err, tt.wantErr, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("diff = %s", diff)
			}
		})
	}
}

func Test_tenantServiceServer_Invite(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	testStore, closer := test.StartRepositoryWithCleanup(t, log, test.WithCockroach(true))
	defer closer()
	repo := testStore.Store

	test.CreateTenants(t, testStore, []*apiv2.TenantServiceCreateRequest{
		{Name: "john.doe@github"},
		{Name: "b950f4f5-d8b8-4252-aa02-ae08a1d2b044"},
	})

	tests := []struct {
		name    string
		rq      *apiv2.TenantServiceInviteRequest
		want    *apiv2.TenantServiceInviteResponse
		wantErr error
	}{
		{
			name: "create a tenant invite",
			rq: &apiv2.TenantServiceInviteRequest{
				Login: "john.doe@github",
				Role:  apiv2.TenantRole_TENANT_ROLE_EDITOR,
			},
			want: &apiv2.TenantServiceInviteResponse{
				Invite: &apiv2.TenantInvite{
					TargetTenant:     "john.doe@github",
					Role:             apiv2.TenantRole_TENANT_ROLE_EDITOR,
					Joined:           false,
					TargetTenantName: "john.doe@github",
					Tenant:           "john.doe@github",
					TenantName:       "john.doe@github",
				},
			},
			wantErr: nil,
		},
		{
			name: "create an invite for another tenant",
			rq: &apiv2.TenantServiceInviteRequest{
				Login: "b950f4f5-d8b8-4252-aa02-ae08a1d2b044",
				Role:  apiv2.TenantRole_TENANT_ROLE_VIEWER,
			},
			want: &apiv2.TenantServiceInviteResponse{
				Invite: &apiv2.TenantInvite{
					TargetTenant:     "b950f4f5-d8b8-4252-aa02-ae08a1d2b044",
					Role:             apiv2.TenantRole_TENANT_ROLE_VIEWER,
					Joined:           false,
					TargetTenantName: "b950f4f5-d8b8-4252-aa02-ae08a1d2b044",
					Tenant:           "john.doe@github",
					TenantName:       "john.doe@github",
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

			got, err := u.Invite(reqCtx, connect.NewRequest(tt.rq))
			require.NoError(t, err)

			assert.Len(t, got.Msg.Invite.Secret, 32)
			assert.WithinDuration(t, time.Now().Add(7*24*time.Hour), got.Msg.Invite.ExpiresAt.AsTime(), 1*time.Minute)
			tt.want.Invite.Secret = got.Msg.Invite.GetSecret()
			tt.want.Invite.ExpiresAt = got.Msg.Invite.GetExpiresAt()

			if diff := cmp.Diff(
				tt.want,
				pointer.SafeDeref(got).Msg,
				protocmp.Transform(),
			); diff != "" {
				t.Errorf("diff: %s", diff)
			}
		})
	}
}

func Test_tenantServiceServer_InviteGet(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	testStore, closer := test.StartRepositoryWithCleanup(t, log, test.WithCockroach(true))
	defer closer()
	repo := testStore.Store

	now := timestamppb.Now()

	test.CreateTenantInvites(t, testStore, []*apiv2.TenantInvite{
		{
			Secret:           "abcdefghijklmnopqrstuvwxyz123456",
			TargetTenant:     "john.doe@github",
			Role:             apiv2.TenantRole_TENANT_ROLE_EDITOR,
			Joined:           false,
			TargetTenantName: "john.doe@github",
			Tenant:           "john.doe@github",
			TenantName:       "john.doe@github",
			ExpiresAt:        now,
			JoinedAt:         nil,
		},
	})

	tests := []struct {
		name    string
		rq      *apiv2.TenantServiceInviteGetRequest
		want    *apiv2.TenantServiceInviteGetResponse
		wantErr error
	}{
		{
			name: "get an invite",
			rq: &apiv2.TenantServiceInviteGetRequest{
				Secret: "abcdefghijklmnopqrstuvwxyz123456",
			},
			want: &apiv2.TenantServiceInviteGetResponse{
				Invite: &apiv2.TenantInvite{
					TargetTenant:     "john.doe@github",
					Role:             apiv2.TenantRole_TENANT_ROLE_EDITOR,
					Joined:           false,
					TargetTenantName: "john.doe@github",
					Tenant:           "john.doe@github",
					TenantName:       "john.doe@github",
					Secret:           "abcdefghijklmnopqrstuvwxyz123456",
					ExpiresAt:        now,
					JoinedAt:         nil,
				},
			},
			wantErr: nil,
		},
		{
			name: "get an invite that does not exist",
			rq: &apiv2.TenantServiceInviteGetRequest{
				Secret: "abcdefghijklmnopqrstuvwxyz987654",
			},
			wantErr: errorutil.NotFound("the given invitation does not exist anymore"),
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

			got, err := u.InviteGet(reqCtx, connect.NewRequest(tt.rq))
			if diff := cmp.Diff(err, tt.wantErr, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("diff = %s", diff)
			}

			if diff := cmp.Diff(
				tt.want,
				pointer.SafeDeref(got).Msg,
				protocmp.Transform(),
			); diff != "" {
				t.Errorf("diff: %s", diff)
			}
		})
	}
}

func Test_tenantServiceServer_InvitesList(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	testStore, closer := test.StartRepositoryWithCleanup(t, log, test.WithCockroach(true))
	defer closer()
	repo := testStore.Store

	now := timestamppb.Now()

	test.CreateTenantInvites(t, testStore, []*apiv2.TenantInvite{
		{
			Secret:           "abcdefghijklmnopqrstuvwxyz000000",
			TargetTenant:     "john.doe@github",
			Role:             apiv2.TenantRole_TENANT_ROLE_EDITOR,
			Joined:           false,
			TargetTenantName: "john.doe@github",
			Tenant:           "john.doe@github",
			TenantName:       "john.doe@github",
			ExpiresAt:        now,
			JoinedAt:         nil,
		},
		{
			Secret:           "abcdefghijklmnopqrstuvwxyz000001",
			TargetTenant:     "will.smith@github",
			Role:             apiv2.TenantRole_TENANT_ROLE_VIEWER,
			Joined:           false,
			TargetTenantName: "will.smith@github",
			Tenant:           "john.doe@github",
			TenantName:       "john.doe@github",
			ExpiresAt:        now,
			JoinedAt:         nil,
		},
	})

	tests := []struct {
		name    string
		rq      *apiv2.TenantServiceInvitesListRequest
		want    *apiv2.TenantServiceInvitesListResponse
		wantErr error
	}{
		{
			name: "list invites",
			rq: &apiv2.TenantServiceInvitesListRequest{
				Login: "john.doe@github",
			},
			want: &apiv2.TenantServiceInvitesListResponse{
				Invites: []*apiv2.TenantInvite{
					{
						Secret:           "abcdefghijklmnopqrstuvwxyz000000",
						TargetTenant:     "john.doe@github",
						Role:             apiv2.TenantRole_TENANT_ROLE_EDITOR,
						Joined:           false,
						TargetTenantName: "john.doe@github",
						Tenant:           "john.doe@github",
						TenantName:       "john.doe@github",
						ExpiresAt:        now,
						JoinedAt:         nil,
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

			got, err := u.InvitesList(reqCtx, connect.NewRequest(tt.rq))
			if diff := cmp.Diff(err, tt.wantErr, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("diff = %s", diff)
			}

			if diff := cmp.Diff(
				tt.want,
				pointer.SafeDeref(got).Msg,
				protocmp.Transform(),
			); diff != "" {
				t.Errorf("diff: %s", diff)
			}
		})
	}
}

func Test_tenantServiceServer_InviteDelete(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	testStore, closer := test.StartRepositoryWithCleanup(t, log, test.WithCockroach(true))
	defer closer()
	repo := testStore.Store

	now := timestamppb.Now()

	test.CreateTenantInvites(t, testStore, []*apiv2.TenantInvite{
		{
			Secret:           "abcdefghijklmnopqrstuvwxyz123456",
			TargetTenant:     "john.doe@github",
			Role:             apiv2.TenantRole_TENANT_ROLE_EDITOR,
			Joined:           false,
			TargetTenantName: "john.doe@github",
			Tenant:           "john.doe@github",
			TenantName:       "john.doe@github",
			ExpiresAt:        now,
			JoinedAt:         nil,
		},
	})

	tests := []struct {
		name    string
		rq      *apiv2.TenantServiceInviteDeleteRequest
		wantErr error
	}{
		{
			name: "delete invite",
			rq: &apiv2.TenantServiceInviteDeleteRequest{
				Login:  "john.doe@github",
				Secret: "abcdefghijklmnopqrstuvwxyz123456",
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

			_, err := u.InviteDelete(reqCtx, connect.NewRequest(tt.rq))
			if diff := cmp.Diff(err, tt.wantErr, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("diff = %s", diff)
			}

			_, err = testStore.GetTenantInviteStore().GetInvite(t.Context(), tt.rq.Secret)
			assert.ErrorIs(t, err, invite.ErrInviteNotFound)
		})
	}
}

func Test_tenantServiceServer_InviteAccept(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	testStore, closer := test.StartRepositoryWithCleanup(t, log, test.WithCockroach(true))
	defer closer()
	repo := testStore.Store

	anHour := timestamppb.New(time.Now().Add(time.Hour))

	tests := []struct {
		name                  string
		existingInvites       []*apiv2.TenantInvite
		existingTenants       []*apiv2.TenantServiceCreateRequest
		existingTenantMembers map[string][]*repository.TenantMemberCreateRequest
		rq                    *apiv2.TenantServiceInviteAcceptRequest
		want                  *apiv2.TenantServiceInviteAcceptResponse
		wantMembers           []*apiv2.TenantMember
		wantErr               error
	}{
		{
			name: "accept an invite",
			rq: &apiv2.TenantServiceInviteAcceptRequest{
				Secret: "abcdefghijklmnopqrstuvwxyz123456",
			},
			existingTenants: []*apiv2.TenantServiceCreateRequest{
				{Name: "john.doe@github"},
				{Name: "will.smith@github"},
				{Name: "b950f4f5-d8b8-4252-aa02-ae08a1d2b044"},
			},
			existingTenantMembers: map[string][]*repository.TenantMemberCreateRequest{
				"john.doe@github":                      {{MemberID: "john.doe@github", Role: apiv2.TenantRole_TENANT_ROLE_OWNER}},
				"will.smith@github":                    {{MemberID: "will.smith@github", Role: apiv2.TenantRole_TENANT_ROLE_OWNER}},
				"b950f4f5-d8b8-4252-aa02-ae08a1d2b044": {{MemberID: "john.doe@github", Role: apiv2.TenantRole_TENANT_ROLE_OWNER}},
			},
			existingInvites: []*apiv2.TenantInvite{
				{
					Secret:           "abcdefghijklmnopqrstuvwxyz123456",
					TargetTenant:     "b950f4f5-d8b8-4252-aa02-ae08a1d2b044",
					Role:             apiv2.TenantRole_TENANT_ROLE_EDITOR,
					Joined:           false,
					TargetTenantName: "b950f4f5-d8b8-4252-aa02-ae08a1d2b044",
					Tenant:           "john.doe@github",
					TenantName:       "john.doe@github",
					ExpiresAt:        anHour,
					JoinedAt:         nil,
				},
			},
			want: &apiv2.TenantServiceInviteAcceptResponse{
				Tenant:     "b950f4f5-d8b8-4252-aa02-ae08a1d2b044",
				TenantName: "b950f4f5-d8b8-4252-aa02-ae08a1d2b044",
			},
			wantMembers: []*apiv2.TenantMember{
				{
					Id:   "john.doe@github",
					Role: apiv2.TenantRole_TENANT_ROLE_OWNER,
				},
				{
					Id:   "will.smith@github",
					Role: apiv2.TenantRole_TENANT_ROLE_EDITOR,
				},
			},
			wantErr: nil,
		},
		{
			name: "cannot join twice",
			rq: &apiv2.TenantServiceInviteAcceptRequest{
				Secret: "abcdefghijklmnopqrstuvwxyz123456",
			},
			existingTenants: []*apiv2.TenantServiceCreateRequest{
				{Name: "john.doe@github"},
				{Name: "will.smith@github"},
				{Name: "b950f4f5-d8b8-4252-aa02-ae08a1d2b044"},
			},
			existingTenantMembers: map[string][]*repository.TenantMemberCreateRequest{
				"john.doe@github":   {{MemberID: "john.doe@github", Role: apiv2.TenantRole_TENANT_ROLE_OWNER}},
				"will.smith@github": {{MemberID: "will.smith@github", Role: apiv2.TenantRole_TENANT_ROLE_OWNER}},
				"b950f4f5-d8b8-4252-aa02-ae08a1d2b044": {
					{MemberID: "john.doe@github", Role: apiv2.TenantRole_TENANT_ROLE_OWNER},
					{MemberID: "will.smith@github", Role: apiv2.TenantRole_TENANT_ROLE_EDITOR},
				},
			},
			existingInvites: []*apiv2.TenantInvite{
				{
					Secret:           "abcdefghijklmnopqrstuvwxyz123456",
					TargetTenant:     "b950f4f5-d8b8-4252-aa02-ae08a1d2b044",
					Role:             apiv2.TenantRole_TENANT_ROLE_EDITOR,
					Joined:           false,
					TargetTenantName: "b950f4f5-d8b8-4252-aa02-ae08a1d2b044",
					Tenant:           "john.doe@github",
					TenantName:       "john.doe@github",
					ExpiresAt:        anHour,
					JoinedAt:         nil,
				},
			},
			wantErr: errorutil.Conflict("will.smith@github is already member of tenant b950f4f5-d8b8-4252-aa02-ae08a1d2b044"),
		},
		{
			name: "no self-joining",
			rq: &apiv2.TenantServiceInviteAcceptRequest{
				Secret: "abcdefghijklmnopqrstuvwxyz123456",
			},
			existingTenants: []*apiv2.TenantServiceCreateRequest{
				{Name: "will.smith@github"},
			},
			existingTenantMembers: map[string][]*repository.TenantMemberCreateRequest{
				"will.smith@github": {{MemberID: "will.smith@github", Role: apiv2.TenantRole_TENANT_ROLE_OWNER}},
			},
			existingInvites: []*apiv2.TenantInvite{
				{
					Secret:           "abcdefghijklmnopqrstuvwxyz123456",
					TargetTenant:     "will.smith@github",
					Role:             apiv2.TenantRole_TENANT_ROLE_EDITOR,
					Joined:           false,
					TargetTenantName: "will.smith@github",
					Tenant:           "will.smith@github",
					TenantName:       "will.smith@github",
					ExpiresAt:        anHour,
					JoinedAt:         nil,
				},
			},
			wantErr: errorutil.InvalidArgument("an owner cannot accept invitations to own tenants"),
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

			test.CreateTenants(t, testStore, tt.existingTenants)
			if tt.existingTenantMembers != nil {
				for tenant, members := range tt.existingTenantMembers {
					test.CreateTenantMemberships(t, testStore, tenant, members)
				}
			}
			test.CreateTenantInvites(t, testStore, tt.existingInvites)
			defer func() {
				testStore.DeleteTenants()
				testStore.DeleteTenantInvites()
			}()

			tok := testStore.GetToken("will.smith@github", &apiv2.TokenServiceCreateRequest{
				Expires: durationpb.New(time.Hour),
				TenantRoles: map[string]apiv2.TenantRole{
					"will.smith@github": apiv2.TenantRole_TENANT_ROLE_OWNER,
				},
			})

			reqCtx := token.ContextWithToken(t.Context(), tok)

			acceptResp, err := u.InviteAccept(reqCtx, connect.NewRequest(tt.rq))
			if diff := cmp.Diff(err, tt.wantErr, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("diff = %s", diff)
			}
			if err != nil {
				return
			}
			if diff := cmp.Diff(
				tt.want,
				pointer.SafeDeref(acceptResp).Msg,
				protocmp.Transform(),
			); diff != "" {
				t.Errorf("diff: %s", diff)
			}

			_, err = testStore.GetTenantInviteStore().GetInvite(t.Context(), tt.rq.Secret)
			require.ErrorIs(t, err, invite.ErrInviteNotFound)

			tok = testStore.GetToken("will.smith@github", &apiv2.TokenServiceCreateRequest{
				Expires: durationpb.New(time.Hour),
				TenantRoles: map[string]apiv2.TenantRole{
					"will.smith@github":   apiv2.TenantRole_TENANT_ROLE_OWNER,
					acceptResp.Msg.Tenant: apiv2.TenantRole_TENANT_ROLE_EDITOR,
				},
			})

			reqCtx = token.ContextWithToken(t.Context(), tok)

			getResp, err := u.Get(reqCtx, connect.NewRequest(&apiv2.TenantServiceGetRequest{
				Login: acceptResp.Msg.Tenant,
			}))
			require.NoError(t, err)

			if diff := cmp.Diff(
				tt.wantMembers,
				pointer.SafeDeref(getResp).Msg.TenantMembers,
				protocmp.Transform(),
				protocmp.IgnoreFields(
					&apiv2.TenantMember{}, "created_at",
				),
			); diff != "" {
				t.Errorf("diff: %s", diff)
			}
		})
	}
}

func Test_tenantServiceServer_InviteFlow(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	testStore, closer := test.StartRepositoryWithCleanup(t, log, test.WithCockroach(true))
	defer closer()
	repo := testStore.Store

	test.CreateTenants(t, testStore, []*apiv2.TenantServiceCreateRequest{
		{Name: "john.doe@github"},
		{Name: "b950f4f5-d8b8-4252-aa02-ae08a1d2b044"},
		{Name: "will.smith@github"},
	})

	test.CreateTenantMemberships(t, testStore, "b950f4f5-d8b8-4252-aa02-ae08a1d2b044", []*repository.TenantMemberCreateRequest{
		{MemberID: "john.doe@github", Role: apiv2.TenantRole_TENANT_ROLE_OWNER},
	})

	u := &tenantServiceServer{
		log:         log,
		repo:        repo,
		inviteStore: testStore.GetTenantInviteStore(),
		tokenStore:  testStore.GetTokenStore(),
	}

	var inviteSecret string

	t.Run("create an invite", func(t *testing.T) {
		tok := testStore.GetToken("john.doe@github", &apiv2.TokenServiceCreateRequest{
			Expires: durationpb.New(time.Hour),
			TenantRoles: map[string]apiv2.TenantRole{
				"john.doe@github":                      apiv2.TenantRole_TENANT_ROLE_OWNER,
				"b950f4f5-d8b8-4252-aa02-ae08a1d2b044": apiv2.TenantRole_TENANT_ROLE_OWNER,
			},
		})

		reqCtx := token.ContextWithToken(t.Context(), tok)

		got, err := u.Invite(reqCtx, connect.NewRequest(&apiv2.TenantServiceInviteRequest{
			Login: "b950f4f5-d8b8-4252-aa02-ae08a1d2b044",
			Role:  apiv2.TenantRole_TENANT_ROLE_EDITOR,
		}))
		require.NoError(t, err)

		assert.Len(t, got.Msg.Invite.Secret, 32)
		assert.WithinDuration(t, time.Now().Add(7*24*time.Hour), got.Msg.Invite.ExpiresAt.AsTime(), 1*time.Minute)

		inviteSecret = got.Msg.Invite.Secret

		if diff := cmp.Diff(
			&apiv2.TenantServiceInviteResponse{
				Invite: &apiv2.TenantInvite{
					Secret:           inviteSecret,
					TargetTenant:     "b950f4f5-d8b8-4252-aa02-ae08a1d2b044",
					Role:             apiv2.TenantRole_TENANT_ROLE_EDITOR,
					Joined:           false,
					TargetTenantName: "b950f4f5-d8b8-4252-aa02-ae08a1d2b044",
					Tenant:           "john.doe@github",
					TenantName:       "john.doe@github",
					ExpiresAt:        &timestamppb.Timestamp{},
				},
			},
			pointer.SafeDeref(got).Msg,
			protocmp.Transform(),
			protocmp.IgnoreFields(
				&apiv2.TenantInvite{}, "expires_at",
			),
		); diff != "" {
			t.Errorf("diff: %s", diff)
		}
	})

	t.Run("listing and getting the invites works", func(t *testing.T) {
		tok := testStore.GetToken("john.doe@github", &apiv2.TokenServiceCreateRequest{
			Expires: durationpb.New(time.Hour),
			TenantRoles: map[string]apiv2.TenantRole{
				"john.doe@github":                      apiv2.TenantRole_TENANT_ROLE_OWNER,
				"b950f4f5-d8b8-4252-aa02-ae08a1d2b044": apiv2.TenantRole_TENANT_ROLE_OWNER,
			},
		})

		reqCtx := token.ContextWithToken(t.Context(), tok)

		got, err := u.InvitesList(reqCtx, connect.NewRequest(&apiv2.TenantServiceInvitesListRequest{
			Login: "b950f4f5-d8b8-4252-aa02-ae08a1d2b044",
		}))
		require.NoError(t, err)

		require.Len(t, got.Msg.Invites, 1)

		if diff := cmp.Diff(
			&apiv2.TenantServiceInvitesListResponse{
				Invites: []*apiv2.TenantInvite{
					{
						Secret:           inviteSecret,
						TargetTenant:     "b950f4f5-d8b8-4252-aa02-ae08a1d2b044",
						Role:             apiv2.TenantRole_TENANT_ROLE_EDITOR,
						Joined:           false,
						TargetTenantName: "b950f4f5-d8b8-4252-aa02-ae08a1d2b044",
						Tenant:           "john.doe@github",
						TenantName:       "john.doe@github",
						ExpiresAt:        &timestamppb.Timestamp{},
					},
				},
			},
			pointer.SafeDeref(got).Msg,
			protocmp.Transform(),
			protocmp.IgnoreFields(
				&apiv2.TenantInvite{}, "expires_at",
			),
		); diff != "" {
			t.Errorf("diff: %s", diff)
		}

		getResp, err := u.InviteGet(reqCtx, connect.NewRequest(&apiv2.TenantServiceInviteGetRequest{
			Secret: inviteSecret,
		}))
		require.NoError(t, err)

		if diff := cmp.Diff(
			&apiv2.TenantServiceInviteGetResponse{
				Invite: &apiv2.TenantInvite{
					Secret:           inviteSecret,
					TargetTenant:     "b950f4f5-d8b8-4252-aa02-ae08a1d2b044",
					Role:             apiv2.TenantRole_TENANT_ROLE_EDITOR,
					Joined:           false,
					TargetTenantName: "b950f4f5-d8b8-4252-aa02-ae08a1d2b044",
					Tenant:           "john.doe@github",
					TenantName:       "john.doe@github",
					ExpiresAt:        &timestamppb.Timestamp{},
				},
			},
			pointer.SafeDeref(getResp).Msg,
			protocmp.Transform(),
			protocmp.IgnoreFields(
				&apiv2.TenantInvite{}, "expires_at",
			),
		); diff != "" {
			t.Errorf("diff: %s", diff)
		}
	})

	t.Run("create and delete another invite", func(t *testing.T) {
		tok := testStore.GetToken("john.doe@github", &apiv2.TokenServiceCreateRequest{
			Expires: durationpb.New(time.Hour),
			TenantRoles: map[string]apiv2.TenantRole{
				"john.doe@github":                      apiv2.TenantRole_TENANT_ROLE_OWNER,
				"b950f4f5-d8b8-4252-aa02-ae08a1d2b044": apiv2.TenantRole_TENANT_ROLE_OWNER,
			},
		})

		reqCtx := token.ContextWithToken(t.Context(), tok)

		got, err := u.Invite(reqCtx, connect.NewRequest(&apiv2.TenantServiceInviteRequest{
			Login: "b950f4f5-d8b8-4252-aa02-ae08a1d2b044",
			Role:  apiv2.TenantRole_TENANT_ROLE_OWNER,
		}))
		require.NoError(t, err)

		assert.Len(t, got.Msg.Invite.Secret, 32)
		assert.WithinDuration(t, time.Now().Add(7*24*time.Hour), got.Msg.Invite.ExpiresAt.AsTime(), 1*time.Minute)

		secondInviteSecret := got.Msg.Invite.Secret

		if diff := cmp.Diff(
			&apiv2.TenantServiceInviteResponse{
				Invite: &apiv2.TenantInvite{
					Secret:           secondInviteSecret,
					TargetTenant:     "b950f4f5-d8b8-4252-aa02-ae08a1d2b044",
					Role:             apiv2.TenantRole_TENANT_ROLE_OWNER,
					Joined:           false,
					TargetTenantName: "b950f4f5-d8b8-4252-aa02-ae08a1d2b044",
					Tenant:           "john.doe@github",
					TenantName:       "john.doe@github",
					ExpiresAt:        &timestamppb.Timestamp{},
				},
			},
			pointer.SafeDeref(got).Msg,
			protocmp.Transform(),
			protocmp.IgnoreFields(
				&apiv2.TenantInvite{}, "expires_at",
			),
		); diff != "" {
			t.Errorf("diff: %s", diff)
		}

		deleteResp, err := u.InviteDelete(reqCtx, connect.NewRequest(&apiv2.TenantServiceInviteDeleteRequest{
			Login:  "b950f4f5-d8b8-4252-aa02-ae08a1d2b044",
			Secret: secondInviteSecret,
		}))
		require.NoError(t, err)

		if diff := cmp.Diff(
			&apiv2.TenantServiceInviteDeleteResponse{},
			pointer.SafeDeref(deleteResp).Msg,
			protocmp.Transform(),
		); diff != "" {
			t.Errorf("diff: %s", diff)
		}
	})

	t.Run("invite gets accepted", func(t *testing.T) {
		tok := testStore.GetToken("will.smith@github", &apiv2.TokenServiceCreateRequest{
			Expires: durationpb.New(time.Hour),
			TenantRoles: map[string]apiv2.TenantRole{
				"will.smith@github": apiv2.TenantRole_TENANT_ROLE_OWNER,
			},
		})

		reqCtx := token.ContextWithToken(t.Context(), tok)

		got, err := u.InviteAccept(reqCtx, connect.NewRequest(&apiv2.TenantServiceInviteAcceptRequest{
			Secret: inviteSecret,
		}))
		require.NoError(t, err)

		if diff := cmp.Diff(
			&apiv2.TenantServiceInviteAcceptResponse{
				Tenant:     "b950f4f5-d8b8-4252-aa02-ae08a1d2b044",
				TenantName: "b950f4f5-d8b8-4252-aa02-ae08a1d2b044",
			},
			pointer.SafeDeref(got).Msg,
			protocmp.Transform(),
		); diff != "" {
			t.Errorf("diff: %s", diff)
		}
	})

	t.Run("invite was removed and membership created", func(t *testing.T) {
		tok := testStore.GetToken("john.doe@github", &apiv2.TokenServiceCreateRequest{
			Expires: durationpb.New(time.Hour),
			TenantRoles: map[string]apiv2.TenantRole{
				"john.doe@github":                      apiv2.TenantRole_TENANT_ROLE_OWNER,
				"b950f4f5-d8b8-4252-aa02-ae08a1d2b044": apiv2.TenantRole_TENANT_ROLE_OWNER,
			},
		})

		reqCtx := token.ContextWithToken(t.Context(), tok)

		got, err := u.InvitesList(reqCtx, connect.NewRequest(&apiv2.TenantServiceInvitesListRequest{
			Login: "b950f4f5-d8b8-4252-aa02-ae08a1d2b044",
		}))
		require.NoError(t, err)

		require.Empty(t, got.Msg.Invites)

		getResp, err := u.Get(reqCtx, connect.NewRequest(&apiv2.TenantServiceGetRequest{
			Login: "b950f4f5-d8b8-4252-aa02-ae08a1d2b044",
		}))
		require.NoError(t, err)

		if diff := cmp.Diff(
			[]*apiv2.TenantMember{
				{
					Id:   "john.doe@github",
					Role: apiv2.TenantRole_TENANT_ROLE_OWNER,
				},
				{
					Id:   "will.smith@github",
					Role: apiv2.TenantRole_TENANT_ROLE_EDITOR,
				},
			},
			pointer.SafeDeref(getResp).Msg.TenantMembers,
			protocmp.Transform(),
			protocmp.IgnoreFields(
				&apiv2.TenantMember{}, "created_at",
			),
		); diff != "" {
			t.Errorf("diff: %s", diff)
		}
	})
}
