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

// func Test_service_Get(t *testing.T) {
// 	tests := []struct {
// 		name                    string
// 		req                     *apiv1.TenantServiceGetRequest
// 		tenantRole              apiv1.TenantRole
// 		tenantServiceMock       func(mock *tmock.Mock)
// 		tenantMemberServiceMock func(mock *tmock.Mock)
// 		want                    *apiv1.TenantServiceGetResponse
// 		wantErr                 *connect.Error
// 	}{
// 		{
// 			name: "no members apart from self",
// 			req: &apiv1.TenantServiceGetRequest{
// 				Login: "me",
// 			},
// 			tenantRole: apiv1.TenantRole_TENANT_ROLE_OWNER,
// 			tenantServiceMock: func(mock *tmock.Mock) {
// 				mock.On("Get", tmock.Anything, &mdmv1.TenantGetRequest{Id: "me"}).Return(&mdmv1.TenantResponse{Tenant: &mdmv1.Tenant{
// 					Meta: &mdmv1.Meta{Id: "me"},
// 				}}, nil)

// 				mock.On("ListTenantMembers", tmock.Anything, &mdmv1.ListTenantMembersRequest{TenantId: "me"}).Return(&mdmv1.ListTenantMembersResponse{
// 					Tenants: []*mdmv1.TenantWithMembershipAnnotations{
// 						{
// 							Tenant: &mdmv1.Tenant{
// 								Meta: &mdmv1.Meta{Id: "me"},
// 							},
// 							TenantAnnotations: map[string]string{
// 								repository.TenantRoleAnnotation: apiv1.ProjectRole_PROJECT_ROLE_OWNER.String(),
// 							},
// 						},
// 					},
// 				}, nil)
// 			},
// 			want: &apiv1.TenantServiceGetResponse{Tenant: &apiv1.Tenant{
// 				Login: "me",
// 				Meta:  &apiv1.Meta{},
// 			},
// 				TenantMembers: []*apiv1.TenantMember{
// 					{
// 						Id:   "me",
// 						Role: 1,
// 					},
// 				},
// 			},
// 			wantErr: nil,
// 		},
// 		{
// 			name: "one direct member",
// 			req: &apiv1.TenantServiceGetRequest{
// 				Login: "me",
// 			},
// 			tenantRole: apiv1.TenantRole_TENANT_ROLE_OWNER,
// 			tenantServiceMock: func(mock *tmock.Mock) {
// 				mock.On("Get", tmock.Anything, &mdmv1.TenantGetRequest{Id: "me"}).Return(&mdmv1.TenantResponse{Tenant: &mdmv1.Tenant{
// 					Meta: &mdmv1.Meta{Id: "me"},
// 				}}, nil)

// 				mock.On("ListTenantMembers", tmock.Anything, &mdmv1.ListTenantMembersRequest{TenantId: "me"}).Return(&mdmv1.ListTenantMembersResponse{
// 					Tenants: []*mdmv1.TenantWithMembershipAnnotations{
// 						{
// 							Tenant: &mdmv1.Tenant{
// 								Meta: &mdmv1.Meta{Id: "me"},
// 							},
// 							TenantAnnotations: map[string]string{
// 								repository.TenantRoleAnnotation: apiv1.ProjectRole_PROJECT_ROLE_OWNER.String(),
// 							},
// 						},
// 						{
// 							Tenant: &mdmv1.Tenant{
// 								Meta: &mdmv1.Meta{Id: "viewer"},
// 							},
// 							TenantAnnotations: map[string]string{
// 								repository.TenantRoleAnnotation: apiv1.ProjectRole_PROJECT_ROLE_VIEWER.String(),
// 							},
// 						},
// 					},
// 				}, nil)
// 			},
// 			want: &apiv1.TenantServiceGetResponse{Tenant: &apiv1.Tenant{
// 				Login: "me",
// 				Meta:  &apiv1.Meta{},
// 			},
// 				TenantMembers: []*apiv1.TenantMember{
// 					{
// 						Id:   "me",
// 						Role: 1,
// 					},
// 					{
// 						Id:   "viewer",
// 						Role: 3,
// 					},
// 				},
// 			},
// 			wantErr: nil,
// 		},
// 		{
// 			name: "one guest member",
// 			req: &apiv1.TenantServiceGetRequest{
// 				Login: "me",
// 			},
// 			tenantRole: apiv1.TenantRole_TENANT_ROLE_OWNER,
// 			tenantServiceMock: func(mock *tmock.Mock) {
// 				mock.On("Get", tmock.Anything, &mdmv1.TenantGetRequest{Id: "me"}).Return(&mdmv1.TenantResponse{Tenant: &mdmv1.Tenant{
// 					Meta: &mdmv1.Meta{Id: "me"},
// 				}}, nil)

// 				mock.On("ListTenantMembers", tmock.Anything, &mdmv1.ListTenantMembersRequest{TenantId: "me"}).Return(&mdmv1.ListTenantMembersResponse{
// 					Tenants: []*mdmv1.TenantWithMembershipAnnotations{
// 						{
// 							Tenant: &mdmv1.Tenant{
// 								Meta: &mdmv1.Meta{Id: "me"},
// 							},
// 							TenantAnnotations: map[string]string{
// 								repository.TenantRoleAnnotation: apiv1.TenantRole_TENANT_ROLE_OWNER.String(),
// 							},
// 							ProjectIds: []string{
// 								"1",
// 							},
// 						},
// 						{
// 							Tenant: &mdmv1.Tenant{
// 								Meta: &mdmv1.Meta{Id: "guest"},
// 							},
// 							TenantAnnotations: map[string]string{
// 								repository.TenantRoleAnnotation: apiv1.ProjectRole_PROJECT_ROLE_UNSPECIFIED.String(),
// 							},
// 							ProjectIds: []string{
// 								"1",
// 							},
// 						},
// 					},
// 				}, nil)
// 			},
// 			want: &apiv1.TenantServiceGetResponse{Tenant: &apiv1.Tenant{
// 				Login: "me",
// 				Meta:  &apiv1.Meta{},
// 			},
// 				TenantMembers: []*apiv1.TenantMember{
// 					{
// 						Id:   "me",
// 						Role: 1,
// 						Projects: []string{
// 							"1",
// 						},
// 					},
// 					{
// 						Id:   "guest",
// 						Role: 4,
// 						Projects: []string{
// 							"1",
// 						},
// 					},
// 				},
// 			},
// 			wantErr: nil,
// 		},
// 		{
// 			name: "tenant viewer sends get request",
// 			req: &apiv1.TenantServiceGetRequest{
// 				Login: "me",
// 			},
// 			tenantRole: apiv1.TenantRole_TENANT_ROLE_VIEWER,
// 			tenantServiceMock: func(mock *tmock.Mock) {
// 				mock.On("Get", tmock.Anything, &mdmv1.TenantGetRequest{Id: "me"}).Return(&mdmv1.TenantResponse{Tenant: &mdmv1.Tenant{
// 					Meta: &mdmv1.Meta{Id: "me"},
// 				}}, nil)

// 				mock.On("ListTenantMembers", tmock.Anything, &mdmv1.ListTenantMembersRequest{TenantId: "me"}).Return(&mdmv1.ListTenantMembersResponse{
// 					Tenants: []*mdmv1.TenantWithMembershipAnnotations{
// 						{
// 							Tenant: &mdmv1.Tenant{
// 								Meta: &mdmv1.Meta{Id: "me"},
// 							},
// 							TenantAnnotations: map[string]string{
// 								repository.TenantRoleAnnotation: apiv1.ProjectRole_PROJECT_ROLE_OWNER.String(),
// 							},
// 						},
// 						{
// 							Tenant: &mdmv1.Tenant{
// 								Meta: &mdmv1.Meta{Id: "viewer"},
// 							},
// 							TenantAnnotations: map[string]string{
// 								repository.TenantRoleAnnotation: apiv1.ProjectRole_PROJECT_ROLE_VIEWER.String(),
// 							},
// 						},
// 					},
// 				}, nil)
// 			},
// 			want: &apiv1.TenantServiceGetResponse{Tenant: &apiv1.Tenant{
// 				Login: "me",
// 				Meta:  &apiv1.Meta{},
// 			},
// 				TenantMembers: []*apiv1.TenantMember{
// 					{
// 						Id:   "me",
// 						Role: 1,
// 					},
// 					{
// 						Id:   "viewer",
// 						Role: 3,
// 					},
// 				},
// 			},
// 			wantErr: nil,
// 		},
// 		{
// 			name: "tenant guest sends get request",
// 			req: &apiv1.TenantServiceGetRequest{
// 				Login: "me",
// 			},
// 			tenantRole: apiv1.TenantRole_TENANT_ROLE_GUEST,
// 			tenantServiceMock: func(mock *tmock.Mock) {
// 				mock.On("Get", tmock.Anything, &mdmv1.TenantGetRequest{Id: "me"}).Return(&mdmv1.TenantResponse{Tenant: &mdmv1.Tenant{
// 					Meta: &mdmv1.Meta{
// 						Id: "me",
// 						Annotations: map[string]string{
// 							repository.TenantTagEmail: "mail@me.com",
// 						},
// 					},
// 					Name:        "name",
// 					Description: "description",
// 				}}, nil)
// 			},
// 			want: &apiv1.TenantServiceGetResponse{Tenant: &apiv1.Tenant{
// 				Login:       "me",
// 				Meta:        &apiv1.Meta{},
// 				Name:        "name",
// 				Description: "description",
// 				Email:       "",
// 			},
// 			},
// 			wantErr: nil,
// 		},
// 	}
// 	for _, tt := range tests {
// 		tt := tt
// 		t.Run(tt.name, func(t *testing.T) {
// 			m := miniredis.RunT(t)
// 			defer m.Close()
// 			c := redis.NewClient(&redis.Options{Addr: m.Addr()})

// 			tokenStore := token.NewRedisStore(c)

// 			ctx := token.ContextWithToken(t.Context(), &apiv1.Token{
// 				TenantRoles: map[string]apiv1.TenantRole{
// 					tt.req.Login: tt.tenantRole,
// 				},
// 			})

// 			s := &tenantServiceServer{
// 				log: slog.Default(),
// 				// masterClient: newMasterdataMockClient(t, tt.tenantServiceMock, tt.tenantMemberServiceMock, nil, nil),
// 				inviteStore: nil,
// 				tokenStore:  tokenStore,
// 			}

// 			result, err := s.Get(ctx, connect.NewRequest(tt.req))
// 			require.NoError(t, err)
// 			assert.Equal(t, result.Msg.Tenant, tt.want.Tenant)
// 		})
// 	}
// }

// func Test_service_InviteAccept(t *testing.T) {
// 	ctx := t.Context()
// 	secret, err := invite.GenerateInviteSecret()
// 	require.NoError(t, err)

// 	tests := []struct {
// 		name                    string
// 		tenant                  *apiv1.TenantServiceInviteAcceptRequest
// 		token                   *apiv1.Token
// 		tenantServiceMock       func(mock *tmock.Mock)
// 		tenantMemberServiceMock func(mock *tmock.Mock)
// 		inviteStorePrepare      func(store invite.TenantInviteStore)
// 		want                    *apiv1.TenantServiceInviteAcceptResponse
// 		wantErr                 *connect.Error
// 	}{
// 		{
// 			name: "accept an invite",
// 			tenant: &apiv1.TenantServiceInviteAcceptRequest{
// 				Secret: secret,
// 			},
// 			token: &apiv1.Token{
// 				Uuid: "123",
// 				User: "new-member",
// 			},
// 			tenantServiceMock: func(mock *tmock.Mock) {
// 				mock.On("Get", tmock.Anything, &mdmv1.TenantGetRequest{Id: "new-member"}).Return(&mdmv1.TenantResponse{Tenant: &mdmv1.Tenant{
// 					Meta: &mdmv1.Meta{Id: "new-member"},
// 				}}, nil)
// 			},
// 			tenantMemberServiceMock: func(mock *tmock.Mock) {
// 				mock.On("Find", tmock.Anything, &mdmv1.TenantMemberFindRequest{TenantId: pointer.Pointer("a"), MemberId: pointer.Pointer("new-member")}).Return(&mdmv1.TenantMemberListResponse{TenantMembers: nil}, nil)
// 				mock.On("Create", tmock.Anything, &mdmv1.TenantMemberCreateRequest{
// 					TenantMember: &mdmv1.TenantMember{
// 						Meta: &mdmv1.Meta{
// 							Annotations: map[string]string{
// 								repository.TenantRoleAnnotation: apiv1.TenantRole_TENANT_ROLE_EDITOR.String(),
// 							},
// 						},
// 						TenantId: "a",
// 						MemberId: "new-member",
// 					},
// 				}).Return(&mdmv1.TenantMemberResponse{
// 					TenantMember: &mdmv1.TenantMember{
// 						Meta: &mdmv1.Meta{
// 							Id: "a-random-uuid",
// 						},
// 						TenantId: "a",
// 						MemberId: "new-member",
// 					},
// 				}, nil)
// 			},
// 			inviteStorePrepare: func(store invite.TenantInviteStore) {
// 				err := store.SetInvite(ctx, &apiv1.TenantInvite{
// 					Secret:           secret,
// 					TargetTenant:     "a",
// 					Role:             apiv1.TenantRole_TENANT_ROLE_EDITOR,
// 					Joined:           false,
// 					TargetTenantName: "name of a",
// 					Tenant:           "user a",
// 					TenantName:       "name of user a",
// 					ExpiresAt:        timestamppb.New(time.Now().Add(10 * time.Minute)),
// 					JoinedAt:         nil,
// 				})
// 				require.NoError(t, err)
// 			},
// 			want: &apiv1.TenantServiceInviteAcceptResponse{
// 				TenantName: "name of a",
// 				Tenant:     "a",
// 			},
// 			wantErr: nil,
// 		},
// 	}
// 	for _, tt := range tests {
// 		tt := tt
// 		t.Run(tt.name, func(t *testing.T) {
// 			m := miniredis.RunT(t)
// 			defer m.Close()

// 			var (
// 				c = redis.NewClient(&redis.Options{Addr: m.Addr()})

// 				inviteStore = invite.NewTenantRedisStore(c)
// 			)

// 			ctx := token.ContextWithToken(ctx, tt.token)

// 			if tt.inviteStorePrepare != nil {
// 				tt.inviteStorePrepare(inviteStore)
// 			}

// 			s := &tenantServiceServer{
// 				log: slog.Default(),
// 				// masterClient: newMasterdataMockClient(t, tt.tenantServiceMock, tt.tenantMemberServiceMock, nil, nil),
// 				inviteStore: inviteStore,
// 			}

// 			result, err := s.InviteAccept(ctx, connect.NewRequest(tt.tenant))
// 			require.NoError(t, err)

// 			assert.Equal(t, result.Msg.TenantName, tt.want.TenantName)
// 			assert.Equal(t, result.Msg.Tenant, tt.want.Tenant)
// 		})
// 	}
// }

func Test_tenantServiceServer_Get(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	testStore, closer := test.StartRepositoryWithCleanup(t, log, test.WithCockroach(true))
	defer closer()
	repo := testStore.Store

	test.CreateTenants(t, testStore, []*apiv2.TenantServiceCreateRequest{{Name: "john.doe@github"}})

	tests := []struct {
		name    string
		rq      *apiv2.TenantServiceGetRequest
		want    *apiv2.TenantServiceGetResponse
		wantErr error
	}{
		{
			name: "get a tenant",
			rq: &apiv2.TenantServiceGetRequest{
				Login: "john.doe@github",
			},
			want: &apiv2.TenantServiceGetResponse{
				Tenant: &apiv2.Tenant{
					Meta:  &apiv2.Meta{},
					Name:  "john.doe@github",
					Login: "john.doe@github",
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

			tok := testStore.GetToken("john.doe@github", &apiv2.TokenServiceCreateRequest{
				Expires: durationpb.New(time.Hour),
				TenantRoles: map[string]apiv2.TenantRole{
					"john.doe@github": apiv2.TenantRole_TENANT_ROLE_OWNER,
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
