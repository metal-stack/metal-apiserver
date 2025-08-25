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
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/durationpb"
)

// func Test_service_Create(t *testing.T) {
// 	tests := []struct {
// 		name                    string
// 		tenant                  *apiv1.TenantServiceCreateRequest
// 		tenantServiceMock       func(mock *tmock.Mock)
// 		tenantMemberServiceMock func(mock *tmock.Mock)
// 		want                    *apiv1.TenantServiceCreateResponse
// 		wantErr                 *connect.Error
// 	}{
// 		{
// 			name: "create user",
// 			tenant: &apiv1.TenantServiceCreateRequest{
// 				Name:        "test",
// 				Description: pointer.Pointer("test tenant"),
// 				Email:       pointer.Pointer("foo@a.b"),
// 				AvatarUrl:   pointer.Pointer("https://example.jpg"),
// 				PhoneNumber: pointer.Pointer("1023"),
// 			},
// 			tenantServiceMock: func(mock *tmock.Mock) {
// 				matcher := testcommon.MatchByCmpDiff(t, &mdmv1.TenantCreateRequest{
// 					Tenant: &mdmv1.Tenant{
// 						Meta: &mdmv1.Meta{
// 							Annotations: map[string]string{
// 								"metal-stack.io/avatarurl": "https://example.jpg",
// 								"metal-stack.io/email":     "foo@a.b",
// 								"metal-stack.io/phone":     "1023",
// 								"metal-stack.io/creator":   "original-owner",
// 							},
// 						},
// 						Name:        "test",
// 						Description: "test tenant",
// 					},
// 				}, cmpopts.IgnoreTypes(protoimpl.MessageState{}))

// 				mock.On("Get", tmock.Anything, &mdmv1.TenantGetRequest{Id: "original-owner"}).Return(&mdmv1.TenantResponse{Tenant: &mdmv1.Tenant{}}, nil)
// 				mock.On("Create", tmock.Anything, matcher).Return(&mdmv1.TenantResponse{Tenant: &mdmv1.Tenant{Meta: &mdmv1.Meta{Id: "e7938bfa-9e47-4fa4-af8c-c059f938f467"}, Name: "test"}}, nil)
// 			},
// 			tenantMemberServiceMock: func(mock *tmock.Mock) {
// 				member := &mdmv1.TenantMember{
// 					Meta: &mdmv1.Meta{
// 						Annotations: map[string]string{
// 							"metal-stack.io/tenant-role": "TENANT_ROLE_OWNER",
// 						},
// 					},
// 					TenantId: "<generated-at-runtime>",
// 					MemberId: "original-owner",
// 				}
// 				matcher := testcommon.MatchByCmpDiff(t, &mdmv1.TenantMemberCreateRequest{
// 					TenantMember: member,
// 				}, cmpopts.IgnoreTypes(protoimpl.MessageState{}), cmpopts.IgnoreFields(mdmv1.TenantMember{}, "TenantId"))

// 				mock.On("Create", tmock.Anything, matcher).Return(&mdmv1.TenantMemberResponse{TenantMember: member}, nil)
// 			},
// 			want: &apiv1.TenantServiceCreateResponse{Tenant: &apiv1.Tenant{
// 				Login: "e7938bfa-9e47-4fa4-af8c-c059f938f467",
// 				Name:  "test",
// 			}},
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
// 				User: "original-owner",
// 			})

// 			s := &tenantServiceServer{
// 				log: slog.Default(),
// 				// masterClient: newMasterdataMockClient(t, tt.tenantServiceMock, tt.tenantMemberServiceMock, nil, nil),
// 				inviteStore: nil,
// 				tokenStore:  tokenStore,
// 			}

// 			result, err := s.Create(ctx, connect.NewRequest(tt.tenant))
// 			require.NoError(t, err)
// 			assert.Equal(t, result.Msg.Tenant.Login, tt.want.Tenant.Login)
// 			assert.Equal(t, result.Msg.Tenant.Name, tt.want.Tenant.Name)
// 		})
// 	}
// }

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
					&apiv2.Image{}, "expires_at",
				),
				protocmp.IgnoreFields(
					&apiv2.Meta{}, "created_at", "updated_at",
				),
				protocmp.IgnoreFields(
					&apiv2.MachineProvisioningEvent{}, "time",
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
					&apiv2.Image{}, "expires_at",
				),
				protocmp.IgnoreFields(
					&apiv2.Meta{}, "created_at", "updated_at",
				),
				protocmp.IgnoreFields(
					&apiv2.MachineProvisioningEvent{}, "time",
				),
			); diff != "" {
				t.Errorf("%v, want %v diff: %s", got.Msg, tt.want, diff)
			}
		})
	}
}
