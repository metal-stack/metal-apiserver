package project

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

func Test_projectServiceServer_Get(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	testStore, closer := test.StartRepositoryWithCleanup(t, log, test.WithCockroach(true))
	defer closer()
	repo := testStore.Store

	test.CreateTenants(t, testStore, []*apiv2.TenantServiceCreateRequest{
		{Name: "john.doe@github"},
		{Name: "will.smith@github"},  // direct project membership
		{Name: "tina.turner@github"}, // inherited project membership
	})
	test.CreateProjects(t, repo, []*apiv2.ProjectServiceCreateRequest{
		{
			Name:        "john.doe@github",
			Description: "a description",
			AvatarUrl:   pointer.Pointer("http://test"),
			Labels: &apiv2.Labels{
				Labels: map[string]string{
					"a": "b",
				},
			},
			Login: "john.doe@github",
		},
	})

	test.CreateProjectMemberships(t, testStore, "john.doe@github", []*repository.ProjectMemberCreateRequest{
		{MemberID: "john.doe@github", Role: apiv2.ProjectRole_PROJECT_ROLE_OWNER},
		{MemberID: "will.smith@github", Role: apiv2.ProjectRole_PROJECT_ROLE_EDITOR},
	})
	test.CreateTenantMemberships(t, testStore, "john.doe@github", []*repository.TenantMemberCreateRequest{
		{MemberID: "john.doe@github", Role: apiv2.TenantRole_TENANT_ROLE_OWNER},
		{MemberID: "tina.turner@github", Role: apiv2.TenantRole_TENANT_ROLE_OWNER},
	})

	tests := []struct {
		name    string
		rq      *apiv2.ProjectServiceGetRequest
		want    *apiv2.ProjectServiceGetResponse
		as      *apiv2.TenantRole
		wantErr error
	}{
		{
			name: "get a project",
			rq: &apiv2.ProjectServiceGetRequest{
				Project: "john.doe@github",
			},
			want: &apiv2.ProjectServiceGetResponse{
				Project: &apiv2.Project{
					Meta: &apiv2.Meta{
						Labels: &apiv2.Labels{
							Labels: map[string]string{
								"a": "b",
							},
						},
					},
					Uuid:        "john.doe@github",
					Tenant:      "john.doe@github",
					Name:        "john.doe@github",
					Description: "a description",
					AvatarUrl:   pointer.Pointer("http://test"),
				},
				ProjectMembers: []*apiv2.ProjectMember{
					{
						Id:   "john.doe@github",
						Role: apiv2.ProjectRole_PROJECT_ROLE_OWNER,
					},
					{
						Id:                  "tina.turner@github",
						InheritedMembership: true,
						Role:                apiv2.ProjectRole_PROJECT_ROLE_OWNER,
					},
					{
						Id:   "will.smith@github",
						Role: apiv2.ProjectRole_PROJECT_ROLE_EDITOR,
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "get a project as guest member",
			rq: &apiv2.ProjectServiceGetRequest{
				Project: "john.doe@github",
			},
			as: pointer.Pointer(apiv2.TenantRole_TENANT_ROLE_GUEST),
			want: &apiv2.ProjectServiceGetResponse{
				Project: &apiv2.Project{
					Meta: &apiv2.Meta{
						Labels: &apiv2.Labels{
							Labels: map[string]string{
								"a": "b",
							},
						},
					},
					Name:        "john.doe@github",
					Description: "a description",
					AvatarUrl:   pointer.Pointer("http://test"),
					Uuid:        "john.doe@github",
					Tenant:      "john.doe@github",
				},
				ProjectMembers: []*apiv2.ProjectMember{
					{
						Id:   "john.doe@github",
						Role: apiv2.ProjectRole_PROJECT_ROLE_OWNER,
					},
					{
						Id:   "will.smith@github",
						Role: apiv2.ProjectRole_PROJECT_ROLE_EDITOR,
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "get a project that does not exist",
			rq: &apiv2.ProjectServiceGetRequest{
				Project: "no.one@github",
			},
			want:    nil,
			wantErr: errorutil.NotFound("project with id:no.one@github not found sql: no rows in result set"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := &projectServiceServer{
				log:         log,
				repo:        repo,
				inviteStore: testStore.GetProjectInviteStore(),
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
				ProjectRoles: map[string]apiv2.ProjectRole{
					"john.doe@github": apiv2.ProjectRole_PROJECT_ROLE_OWNER,
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
					&apiv2.ProjectMember{}, "created_at",
				),
			); diff != "" {
				t.Errorf("%v, want %v diff: %s", got.Msg, tt.want, diff)
			}
		})
	}
}

// func Test_projectServiceServer_List(t *testing.T) {
// 	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

// 	testStore, closer := test.StartRepositoryWithCleanup(t, log, test.WithCockroach(true))
// 	defer closer()
// 	repo := testStore.Store

// 	test.CreateProjects(t, testStore, []*apiv2.ProjectServiceCreateRequest{
// 		{Name: "john.doe@github"},
// 		{Name: "will.smith@github"},
// 		{Name: "b950f4f5-d8b8-4252-aa02-ae08a1d2b044"},
// 	})

// 	// create some memberships
// 	test.CreateProjectMemberships(t, testStore, "john.doe@github", []*repository.ProjectMemberCreateRequest{
// 		{MemberID: "john.doe@github", Role: apiv2.ProjectRole_TENANT_ROLE_OWNER},
// 	})
// 	test.CreateProjectMemberships(t, testStore, "will.smith@github", []*repository.ProjectMemberCreateRequest{
// 		{MemberID: "will.smith@github", Role: apiv2.ProjectRole_TENANT_ROLE_OWNER},
// 	})
// 	test.CreateProjectMemberships(t, testStore, "b950f4f5-d8b8-4252-aa02-ae08a1d2b044", []*repository.ProjectMemberCreateRequest{
// 		{MemberID: "john.doe@github", Role: apiv2.ProjectRole_TENANT_ROLE_OWNER},
// 	})

// 	tests := []struct {
// 		name    string
// 		rq      *apiv2.ProjectServiceListRequest
// 		want    *apiv2.ProjectServiceListResponse
// 		wantErr error
// 	}{
// 		{
// 			name: "list the projects",
// 			rq:   &apiv2.ProjectServiceListRequest{},
// 			want: &apiv2.ProjectServiceListResponse{
// 				Projects: []*apiv2.Project{
// 					{
// 						Meta:      &apiv2.Meta{},
// 						Name:      "b950f4f5-d8b8-4252-aa02-ae08a1d2b044",
// 						Login:     "b950f4f5-d8b8-4252-aa02-ae08a1d2b044",
// 						CreatedBy: "b950f4f5-d8b8-4252-aa02-ae08a1d2b044",
// 					},
// 					{
// 						Meta:      &apiv2.Meta{},
// 						Name:      "john.doe@github",
// 						Login:     "john.doe@github",
// 						CreatedBy: "john.doe@github",
// 					},
// 				},
// 			},
// 			wantErr: nil,
// 		},
// 		{
// 			name: "list the projects filtered by id",
// 			rq: &apiv2.ProjectServiceListRequest{
// 				Id: pointer.Pointer("john.doe@github"),
// 			},
// 			want: &apiv2.ProjectServiceListResponse{
// 				Projects: []*apiv2.Project{
// 					{
// 						Meta:      &apiv2.Meta{},
// 						Name:      "john.doe@github",
// 						Login:     "john.doe@github",
// 						CreatedBy: "john.doe@github",
// 					},
// 				},
// 			},
// 			wantErr: nil,
// 		},
// 		{
// 			name: "list the projects filtered by name",
// 			rq: &apiv2.ProjectServiceListRequest{
// 				Name: pointer.Pointer("b950f4f5-d8b8-4252-aa02-ae08a1d2b044"),
// 			},
// 			want: &apiv2.ProjectServiceListResponse{
// 				Projects: []*apiv2.Project{
// 					{
// 						Meta:      &apiv2.Meta{},
// 						Name:      "b950f4f5-d8b8-4252-aa02-ae08a1d2b044",
// 						Login:     "b950f4f5-d8b8-4252-aa02-ae08a1d2b044",
// 						CreatedBy: "b950f4f5-d8b8-4252-aa02-ae08a1d2b044",
// 					},
// 				},
// 			},
// 			wantErr: nil,
// 		},
// 	}
// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			u := &projectServiceServer{
// 				log:         log,
// 				repo:        repo,
// 				inviteStore: testStore.GetProjectInviteStore(),
// 				tokenStore:  testStore.GetTokenStore(),
// 			}

// 			tok := testStore.GetToken("john.doe@github", &apiv2.TokenServiceCreateRequest{
// 				Expires: durationpb.New(time.Hour),
// 				ProjectRoles: map[string]apiv2.ProjectRole{
// 					"john.doe@github": apiv2.ProjectRole_TENANT_ROLE_OWNER,
// 				},
// 			})

// 			reqCtx := token.ContextWithToken(t.Context(), tok)

// 			got, err := u.List(reqCtx, connect.NewRequest(tt.rq))
// 			if diff := cmp.Diff(err, tt.wantErr, errorutil.ConnectErrorComparer()); diff != "" {
// 				t.Errorf("diff = %s", diff)
// 			}

// 			if diff := cmp.Diff(
// 				tt.want, pointer.SafeDeref(got).Msg,
// 				protocmp.Transform(),
// 				protocmp.IgnoreFields(
// 					&apiv2.Meta{}, "created_at", "updated_at",
// 				),
// 			); diff != "" {
// 				t.Errorf("%v, want %v diff: %s", got.Msg, tt.want, diff)
// 			}
// 		})
// 	}
// }

// func Test_projectServiceServer_Create(t *testing.T) {
// 	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

// 	testStore, closer := test.StartRepositoryWithCleanup(t, log, test.WithCockroach(true))
// 	defer closer()
// 	repo := testStore.Store

// 	test.CreateProjects(t, testStore, []*apiv2.ProjectServiceCreateRequest{{Name: "john.doe@github"}})

// 	tests := []struct {
// 		name        string
// 		rq          *apiv2.ProjectServiceCreateRequest
// 		want        *apiv2.ProjectServiceCreateResponse
// 		wantMembers []*apiv2.ProjectMember
// 		wantErr     error
// 	}{
// 		{
// 			name: "create a project",
// 			rq: &apiv2.ProjectServiceCreateRequest{
// 				Name:        "My New Org Project",
// 				Description: pointer.Pointer("project desc"),
// 				Email:       pointer.Pointer("project@github"),
// 				AvatarUrl:   pointer.Pointer("http://test"),
// 				Labels: &apiv2.Labels{
// 					Labels: map[string]string{
// 						"a": "b",
// 					},
// 				},
// 			},
// 			want: &apiv2.ProjectServiceCreateResponse{
// 				Project: &apiv2.Project{
// 					Login: "a-uuid",
// 					Meta: &apiv2.Meta{
// 						Labels: &apiv2.Labels{
// 							Labels: map[string]string{
// 								"a": "b",
// 							},
// 						},
// 					},
// 					Name:        "My New Org Project",
// 					Email:       "project@github",
// 					Description: "project desc",
// 					AvatarUrl:   "http://test",
// 					CreatedBy:   "john.doe@github",
// 				},
// 			},
// 			wantMembers: []*apiv2.ProjectMember{
// 				{
// 					Id:       "john.doe@github",
// 					Role:     apiv2.ProjectRole_TENANT_ROLE_OWNER,
// 					Projects: []string{},
// 				},
// 			},
// 			wantErr: nil,
// 		},
// 	}
// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			u := &projectServiceServer{
// 				log:         log,
// 				repo:        repo,
// 				inviteStore: testStore.GetProjectInviteStore(),
// 				tokenStore:  testStore.GetTokenStore(),
// 			}

// 			tok := testStore.GetToken("john.doe@github", &apiv2.TokenServiceCreateRequest{
// 				Expires: durationpb.New(time.Hour),
// 				ProjectRoles: map[string]apiv2.ProjectRole{
// 					"john.doe@github": apiv2.ProjectRole_TENANT_ROLE_OWNER,
// 				},
// 			})

// 			reqCtx := token.ContextWithToken(t.Context(), tok)

// 			got, err := u.Create(reqCtx, connect.NewRequest(tt.rq))
// 			if diff := cmp.Diff(err, tt.wantErr, errorutil.ConnectErrorComparer()); diff != "" {
// 				t.Errorf("diff = %s", diff)
// 			}

// 			assert.NotEmpty(t, got.Msg.Project.Login)

// 			if diff := cmp.Diff(
// 				tt.want, pointer.SafeDeref(got).Msg,
// 				protocmp.Transform(),
// 				protocmp.IgnoreFields(
// 					&apiv2.Meta{}, "created_at", "updated_at",
// 				),
// 				protocmp.IgnoreFields(
// 					&apiv2.Project{}, "login",
// 				),
// 			); diff != "" {
// 				t.Errorf("%v, want %v diff: %s", got.Msg, tt.want, diff)
// 			}

// 			// to check whether the owner membership was created, we need to get the project as well
// 			// as we do not request through opa auther, we also need to extend the token

// 			tok.ProjectRoles[got.Msg.Project.Login] = apiv2.ProjectRole_TENANT_ROLE_OWNER

// 			reqCtx = token.ContextWithToken(t.Context(), tok)

// 			getResp, err := u.Get(reqCtx, connect.NewRequest(&apiv2.ProjectServiceGetRequest{
// 				Login: got.Msg.Project.Login,
// 			}))
// 			if diff := cmp.Diff(err, tt.wantErr, errorutil.ConnectErrorComparer()); diff != "" {
// 				t.Errorf("diff = %s", diff)
// 			}

// 			tt.want.Project.Login = got.Msg.Project.Login

// 			if diff := cmp.Diff(
// 				tt.want.Project, pointer.SafeDeref(getResp).Msg.Project,
// 				protocmp.Transform(),
// 				protocmp.IgnoreFields(
// 					&apiv2.Meta{}, "created_at", "updated_at",
// 				),
// 			); diff != "" {
// 				t.Errorf("%v, want %v diff: %s", getResp.Msg.Project, tt.want.Project, diff)
// 			}
// 			if diff := cmp.Diff(
// 				tt.wantMembers, pointer.SafeDeref(getResp).Msg.ProjectMembers,
// 				protocmp.Transform(),
// 				protocmp.IgnoreFields(
// 					&apiv2.ProjectMember{}, "created_at",
// 				),
// 			); diff != "" {
// 				t.Errorf("%v, want %v diff: %s", getResp.Msg.ProjectMembers, tt.wantMembers, diff)
// 			}
// 		})
// 	}
// }

// func Test_projectServiceServer_Update(t *testing.T) {
// 	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

// 	testStore, closer := test.StartRepositoryWithCleanup(t, log, test.WithCockroach(true))
// 	defer closer()
// 	repo := testStore.Store

// 	test.CreateProjects(t, testStore, []*apiv2.ProjectServiceCreateRequest{{
// 		Name:        "john.doe@github",
// 		Description: pointer.Pointer("old desc"),
// 		Email:       pointer.Pointer("old mail"),
// 		AvatarUrl:   pointer.Pointer("http://old"),
// 		Labels: &apiv2.Labels{
// 			Labels: map[string]string{
// 				"a": "b",
// 			},
// 		},
// 	}})

// 	tests := []struct {
// 		name    string
// 		rq      *apiv2.ProjectServiceUpdateRequest
// 		want    *apiv2.ProjectServiceUpdateResponse
// 		wantErr error
// 	}{
// 		{
// 			name: "create a project",
// 			rq: &apiv2.ProjectServiceUpdateRequest{
// 				Login:       "john.doe@github",
// 				Name:        pointer.Pointer("new name"),
// 				Description: pointer.Pointer("new desc"),
// 				Email:       pointer.Pointer("new mail"),
// 				AvatarUrl:   pointer.Pointer("http://new"),
// 				Labels: &apiv2.UpdateLabels{
// 					Update: &apiv2.Labels{
// 						Labels: map[string]string{
// 							"c": "d",
// 						},
// 					},
// 				},
// 			},
// 			want: &apiv2.ProjectServiceUpdateResponse{
// 				Project: &apiv2.Project{
// 					Login: "john.doe@github",
// 					Meta: &apiv2.Meta{
// 						Labels: &apiv2.Labels{
// 							Labels: map[string]string{
// 								"a": "b",
// 								"c": "d",
// 							},
// 						},
// 					},
// 					Name:        "new name",
// 					Email:       "new mail",
// 					Description: "new desc",
// 					AvatarUrl:   "http://new",
// 					CreatedBy:   "john.doe@github",
// 				},
// 			},
// 			wantErr: nil,
// 		},
// 	}
// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			u := &projectServiceServer{
// 				log:         log,
// 				repo:        repo,
// 				inviteStore: testStore.GetProjectInviteStore(),
// 				tokenStore:  testStore.GetTokenStore(),
// 			}

// 			tok := testStore.GetToken("john.doe@github", &apiv2.TokenServiceCreateRequest{
// 				Expires: durationpb.New(time.Hour),
// 				ProjectRoles: map[string]apiv2.ProjectRole{
// 					"john.doe@github": apiv2.ProjectRole_TENANT_ROLE_OWNER,
// 				},
// 			})

// 			reqCtx := token.ContextWithToken(t.Context(), tok)

// 			got, err := u.Update(reqCtx, connect.NewRequest(tt.rq))
// 			if diff := cmp.Diff(err, tt.wantErr, errorutil.ConnectErrorComparer()); diff != "" {
// 				t.Errorf("diff = %s", diff)
// 			}

// 			assert.NotEmpty(t, got.Msg.Project.Login)

// 			if diff := cmp.Diff(
// 				tt.want, pointer.SafeDeref(got).Msg,
// 				protocmp.Transform(),
// 				protocmp.IgnoreFields(
// 					&apiv2.Meta{}, "created_at", "updated_at",
// 				),
// 				protocmp.IgnoreFields(
// 					&apiv2.Project{}, "login",
// 				),
// 			); diff != "" {
// 				t.Errorf("%v, want %v diff: %s", got.Msg, tt.want, diff)
// 			}
// 		})
// 	}
// }

// func Test_projectServiceServer_InviteFlow(t *testing.T) {
// 	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

// 	testStore, closer := test.StartRepositoryWithCleanup(t, log, test.WithCockroach(true))
// 	defer closer()
// 	repo := testStore.Store

// 	test.CreateProjects(t, testStore, []*apiv2.ProjectServiceCreateRequest{
// 		{Name: "john.doe@github"},
// 		{Name: "b950f4f5-d8b8-4252-aa02-ae08a1d2b044"},
// 		{Name: "will.smith@github"},
// 	})

// 	test.CreateProjectMemberships(t, testStore, "b950f4f5-d8b8-4252-aa02-ae08a1d2b044", []*repository.ProjectMemberCreateRequest{
// 		{MemberID: "john.doe@github", Role: apiv2.ProjectRole_TENANT_ROLE_OWNER},
// 	})

// 	u := &projectServiceServer{
// 		log:         log,
// 		repo:        repo,
// 		inviteStore: testStore.GetProjectInviteStore(),
// 		tokenStore:  testStore.GetTokenStore(),
// 	}

// 	var inviteSecret string

// 	t.Run("create an invite", func(t *testing.T) {
// 		tok := testStore.GetToken("john.doe@github", &apiv2.TokenServiceCreateRequest{
// 			Expires: durationpb.New(time.Hour),
// 			ProjectRoles: map[string]apiv2.ProjectRole{
// 				"john.doe@github":                      apiv2.ProjectRole_TENANT_ROLE_OWNER,
// 				"b950f4f5-d8b8-4252-aa02-ae08a1d2b044": apiv2.ProjectRole_TENANT_ROLE_OWNER,
// 			},
// 		})

// 		reqCtx := token.ContextWithToken(t.Context(), tok)

// 		got, err := u.Invite(reqCtx, connect.NewRequest(&apiv2.ProjectServiceInviteRequest{
// 			Login: "b950f4f5-d8b8-4252-aa02-ae08a1d2b044",
// 			Role:  apiv2.ProjectRole_TENANT_ROLE_EDITOR,
// 		}))
// 		require.NoError(t, err)

// 		assert.Len(t, got.Msg.Invite.Secret, 32)
// 		assert.WithinDuration(t, time.Now().Add(7*24*time.Hour), got.Msg.Invite.ExpiresAt.AsTime(), 1*time.Minute)

// 		inviteSecret = got.Msg.Invite.Secret

// 		if diff := cmp.Diff(
// 			&apiv2.ProjectServiceInviteResponse{
// 				Invite: &apiv2.ProjectInvite{
// 					Secret:            inviteSecret,
// 					TargetProject:     "b950f4f5-d8b8-4252-aa02-ae08a1d2b044",
// 					Role:              apiv2.ProjectRole_TENANT_ROLE_EDITOR,
// 					Joined:            false,
// 					TargetProjectName: "b950f4f5-d8b8-4252-aa02-ae08a1d2b044",
// 					Project:           "john.doe@github",
// 					ProjectName:       "john.doe@github",
// 					ExpiresAt:         &timestamppb.Timestamp{},
// 					JoinedAt:          &timestamppb.Timestamp{},
// 				},
// 			},
// 			pointer.SafeDeref(got).Msg,
// 			protocmp.Transform(),
// 			protocmp.IgnoreFields(
// 				&apiv2.ProjectInvite{}, "expires_at",
// 			),
// 		); diff != "" {
// 			t.Errorf("diff: %s", diff)
// 		}
// 	})

// 	t.Run("listing and getting the invites works", func(t *testing.T) {
// 		tok := testStore.GetToken("john.doe@github", &apiv2.TokenServiceCreateRequest{
// 			Expires: durationpb.New(time.Hour),
// 			ProjectRoles: map[string]apiv2.ProjectRole{
// 				"john.doe@github":                      apiv2.ProjectRole_TENANT_ROLE_OWNER,
// 				"b950f4f5-d8b8-4252-aa02-ae08a1d2b044": apiv2.ProjectRole_TENANT_ROLE_OWNER,
// 			},
// 		})

// 		reqCtx := token.ContextWithToken(t.Context(), tok)

// 		got, err := u.InvitesList(reqCtx, connect.NewRequest(&apiv2.ProjectServiceInvitesListRequest{
// 			Login: "b950f4f5-d8b8-4252-aa02-ae08a1d2b044",
// 		}))
// 		require.NoError(t, err)

// 		require.Len(t, got.Msg.Invites, 1)

// 		if diff := cmp.Diff(
// 			&apiv2.ProjectServiceInvitesListResponse{
// 				Invites: []*apiv2.ProjectInvite{
// 					{
// 						Secret:            inviteSecret,
// 						TargetProject:     "b950f4f5-d8b8-4252-aa02-ae08a1d2b044",
// 						Role:              apiv2.ProjectRole_TENANT_ROLE_EDITOR,
// 						Joined:            false,
// 						TargetProjectName: "b950f4f5-d8b8-4252-aa02-ae08a1d2b044",
// 						Project:           "john.doe@github",
// 						ProjectName:       "john.doe@github",
// 						ExpiresAt:         &timestamppb.Timestamp{},
// 						JoinedAt:          &timestamppb.Timestamp{},
// 					},
// 				},
// 			},
// 			pointer.SafeDeref(got).Msg,
// 			protocmp.Transform(),
// 			protocmp.IgnoreFields(
// 				&apiv2.ProjectInvite{}, "expires_at",
// 			),
// 		); diff != "" {
// 			t.Errorf("diff: %s", diff)
// 		}

// 		getResp, err := u.InviteGet(reqCtx, connect.NewRequest(&apiv2.ProjectServiceInviteGetRequest{
// 			Secret: inviteSecret,
// 		}))
// 		require.NoError(t, err)

// 		if diff := cmp.Diff(
// 			&apiv2.ProjectServiceInviteGetResponse{
// 				Invite: &apiv2.ProjectInvite{
// 					Secret:            inviteSecret,
// 					TargetProject:     "b950f4f5-d8b8-4252-aa02-ae08a1d2b044",
// 					Role:              apiv2.ProjectRole_TENANT_ROLE_EDITOR,
// 					Joined:            false,
// 					TargetProjectName: "b950f4f5-d8b8-4252-aa02-ae08a1d2b044",
// 					Project:           "john.doe@github",
// 					ProjectName:       "john.doe@github",
// 					ExpiresAt:         &timestamppb.Timestamp{},
// 					JoinedAt:          &timestamppb.Timestamp{},
// 				},
// 			},
// 			pointer.SafeDeref(getResp).Msg,
// 			protocmp.Transform(),
// 			protocmp.IgnoreFields(
// 				&apiv2.ProjectInvite{}, "expires_at",
// 			),
// 		); diff != "" {
// 			t.Errorf("diff: %s", diff)
// 		}
// 	})

// 	t.Run("create and delete another invite", func(t *testing.T) {
// 		tok := testStore.GetToken("john.doe@github", &apiv2.TokenServiceCreateRequest{
// 			Expires: durationpb.New(time.Hour),
// 			ProjectRoles: map[string]apiv2.ProjectRole{
// 				"john.doe@github":                      apiv2.ProjectRole_TENANT_ROLE_OWNER,
// 				"b950f4f5-d8b8-4252-aa02-ae08a1d2b044": apiv2.ProjectRole_TENANT_ROLE_OWNER,
// 			},
// 		})

// 		reqCtx := token.ContextWithToken(t.Context(), tok)

// 		got, err := u.Invite(reqCtx, connect.NewRequest(&apiv2.ProjectServiceInviteRequest{
// 			Login: "b950f4f5-d8b8-4252-aa02-ae08a1d2b044",
// 			Role:  apiv2.ProjectRole_TENANT_ROLE_OWNER,
// 		}))
// 		require.NoError(t, err)

// 		assert.Len(t, got.Msg.Invite.Secret, 32)
// 		assert.WithinDuration(t, time.Now().Add(7*24*time.Hour), got.Msg.Invite.ExpiresAt.AsTime(), 1*time.Minute)

// 		secondInviteSecret := got.Msg.Invite.Secret

// 		if diff := cmp.Diff(
// 			&apiv2.ProjectServiceInviteResponse{
// 				Invite: &apiv2.ProjectInvite{
// 					Secret:            secondInviteSecret,
// 					TargetProject:     "b950f4f5-d8b8-4252-aa02-ae08a1d2b044",
// 					Role:              apiv2.ProjectRole_TENANT_ROLE_OWNER,
// 					Joined:            false,
// 					TargetProjectName: "b950f4f5-d8b8-4252-aa02-ae08a1d2b044",
// 					Project:           "john.doe@github",
// 					ProjectName:       "john.doe@github",
// 					ExpiresAt:         &timestamppb.Timestamp{},
// 					JoinedAt:          &timestamppb.Timestamp{},
// 				},
// 			},
// 			pointer.SafeDeref(got).Msg,
// 			protocmp.Transform(),
// 			protocmp.IgnoreFields(
// 				&apiv2.ProjectInvite{}, "expires_at",
// 			),
// 		); diff != "" {
// 			t.Errorf("diff: %s", diff)
// 		}

// 		deleteResp, err := u.InviteDelete(reqCtx, connect.NewRequest(&apiv2.ProjectServiceInviteDeleteRequest{
// 			Login:  "b950f4f5-d8b8-4252-aa02-ae08a1d2b044",
// 			Secret: secondInviteSecret,
// 		}))
// 		require.NoError(t, err)

// 		if diff := cmp.Diff(
// 			&apiv2.ProjectServiceInviteDeleteResponse{},
// 			pointer.SafeDeref(deleteResp).Msg,
// 			protocmp.Transform(),
// 		); diff != "" {
// 			t.Errorf("diff: %s", diff)
// 		}
// 	})

// 	t.Run("invite gets accepted", func(t *testing.T) {
// 		tok := testStore.GetToken("will.smith@github", &apiv2.TokenServiceCreateRequest{
// 			Expires: durationpb.New(time.Hour),
// 			ProjectRoles: map[string]apiv2.ProjectRole{
// 				"will.smith@github": apiv2.ProjectRole_TENANT_ROLE_OWNER,
// 			},
// 		})

// 		reqCtx := token.ContextWithToken(t.Context(), tok)

// 		got, err := u.InviteAccept(reqCtx, connect.NewRequest(&apiv2.ProjectServiceInviteAcceptRequest{
// 			Secret: inviteSecret,
// 		}))
// 		require.NoError(t, err)

// 		if diff := cmp.Diff(
// 			&apiv2.ProjectServiceInviteAcceptResponse{
// 				Project:     "b950f4f5-d8b8-4252-aa02-ae08a1d2b044",
// 				ProjectName: "b950f4f5-d8b8-4252-aa02-ae08a1d2b044",
// 			},
// 			pointer.SafeDeref(got).Msg,
// 			protocmp.Transform(),
// 		); diff != "" {
// 			t.Errorf("diff: %s", diff)
// 		}
// 	})

// 	t.Run("invite was removed and membership created", func(t *testing.T) {
// 		tok := testStore.GetToken("john.doe@github", &apiv2.TokenServiceCreateRequest{
// 			Expires: durationpb.New(time.Hour),
// 			ProjectRoles: map[string]apiv2.ProjectRole{
// 				"john.doe@github":                      apiv2.ProjectRole_TENANT_ROLE_OWNER,
// 				"b950f4f5-d8b8-4252-aa02-ae08a1d2b044": apiv2.ProjectRole_TENANT_ROLE_OWNER,
// 			},
// 		})

// 		reqCtx := token.ContextWithToken(t.Context(), tok)

// 		got, err := u.InvitesList(reqCtx, connect.NewRequest(&apiv2.ProjectServiceInvitesListRequest{
// 			Login: "b950f4f5-d8b8-4252-aa02-ae08a1d2b044",
// 		}))
// 		require.NoError(t, err)

// 		require.Empty(t, got.Msg.Invites)

// 		getResp, err := u.Get(reqCtx, connect.NewRequest(&apiv2.ProjectServiceGetRequest{
// 			Login: "b950f4f5-d8b8-4252-aa02-ae08a1d2b044",
// 		}))
// 		require.NoError(t, err)

// 		if diff := cmp.Diff(
// 			[]*apiv2.ProjectMember{
// 				{
// 					Id:   "john.doe@github",
// 					Role: apiv2.ProjectRole_TENANT_ROLE_OWNER,
// 				},
// 				{
// 					Id:   "will.smith@github",
// 					Role: apiv2.ProjectRole_TENANT_ROLE_EDITOR,
// 				},
// 			},
// 			pointer.SafeDeref(getResp).Msg.ProjectMembers,
// 			protocmp.Transform(),
// 			protocmp.IgnoreFields(
// 				&apiv2.ProjectMember{}, "created_at",
// 			),
// 		); diff != "" {
// 			t.Errorf("diff: %s", diff)
// 		}
// 	})
// }

// func Test_projectServiceServer_MemberUpdate(t *testing.T) {
// 	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

// 	testStore, closer := test.StartRepositoryWithCleanup(t, log, test.WithCockroach(true))
// 	defer closer()
// 	repo := testStore.Store

// 	tests := []struct {
// 		name                   string
// 		rq                     *apiv2.ProjectServiceUpdateMemberRequest
// 		existingProjects       []*apiv2.ProjectServiceCreateRequest
// 		existingProjectMembers map[string][]*repository.ProjectMemberCreateRequest
// 		want                   *apiv2.ProjectServiceUpdateMemberResponse
// 		wantErr                error
// 	}{
// 		{
// 			name: "update a member",
// 			rq: &apiv2.ProjectServiceUpdateMemberRequest{
// 				Login:  "b950f4f5-d8b8-4252-aa02-ae08a1d2b044",
// 				Member: "will.smith@github",
// 				Role:   apiv2.ProjectRole_TENANT_ROLE_EDITOR,
// 			},
// 			existingProjects: []*apiv2.ProjectServiceCreateRequest{
// 				{Name: "john.doe@github"},
// 				{Name: "b950f4f5-d8b8-4252-aa02-ae08a1d2b044"},
// 				{Name: "will.smith@github"},
// 			},
// 			existingProjectMembers: map[string][]*repository.ProjectMemberCreateRequest{
// 				"b950f4f5-d8b8-4252-aa02-ae08a1d2b044": {
// 					{MemberID: "john.doe@github", Role: apiv2.ProjectRole_TENANT_ROLE_OWNER},
// 					{MemberID: "will.smith@github", Role: apiv2.ProjectRole_TENANT_ROLE_OWNER},
// 				},
// 			},
// 			want: &apiv2.ProjectServiceUpdateMemberResponse{
// 				ProjectMember: &apiv2.ProjectMember{
// 					Id:   "will.smith@github",
// 					Role: apiv2.ProjectRole_TENANT_ROLE_EDITOR,
// 				},
// 			},
// 			wantErr: nil,
// 		},
// 		{
// 			name: "unable to demote own default project",
// 			rq: &apiv2.ProjectServiceUpdateMemberRequest{
// 				Login:  "john.doe@github",
// 				Member: "john.doe@github",
// 				Role:   apiv2.ProjectRole_TENANT_ROLE_EDITOR,
// 			},
// 			existingProjects: []*apiv2.ProjectServiceCreateRequest{
// 				{Name: "john.doe@github"},
// 			},
// 			existingProjectMembers: map[string][]*repository.ProjectMemberCreateRequest{
// 				"john.doe@github": {
// 					{MemberID: "john.doe@github", Role: apiv2.ProjectRole_TENANT_ROLE_OWNER},
// 				},
// 			},
// 			wantErr: errorutil.InvalidArgument("cannot demote a user's role within their own default project"),
// 		},
// 		{
// 			name: "unable to demote last owner",
// 			rq: &apiv2.ProjectServiceUpdateMemberRequest{
// 				Login:  "b950f4f5-d8b8-4252-aa02-ae08a1d2b044",
// 				Member: "john.doe@github",
// 				Role:   apiv2.ProjectRole_TENANT_ROLE_EDITOR,
// 			},
// 			existingProjects: []*apiv2.ProjectServiceCreateRequest{
// 				{Name: "john.doe@github"},
// 				{Name: "b950f4f5-d8b8-4252-aa02-ae08a1d2b044"},
// 			},
// 			existingProjectMembers: map[string][]*repository.ProjectMemberCreateRequest{
// 				"b950f4f5-d8b8-4252-aa02-ae08a1d2b044": {
// 					{MemberID: "john.doe@github", Role: apiv2.ProjectRole_TENANT_ROLE_OWNER},
// 				},
// 			},
// 			wantErr: errorutil.InvalidArgument("cannot demote last owner's permissions"),
// 		},
// 	}
// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			u := &projectServiceServer{
// 				log:         log,
// 				repo:        repo,
// 				inviteStore: testStore.GetProjectInviteStore(),
// 				tokenStore:  testStore.GetTokenStore(),
// 			}

// 			test.CreateProjects(t, testStore, tt.existingProjects)
// 			if tt.existingProjectMembers != nil {
// 				for project, members := range tt.existingProjectMembers {
// 					test.CreateProjectMemberships(t, testStore, project, members)
// 				}
// 			}
// 			defer func() {
// 				testStore.DeleteProjects()
// 			}()

// 			tok := testStore.GetToken("john.doe@github", &apiv2.TokenServiceCreateRequest{
// 				Expires: durationpb.New(time.Hour),
// 				ProjectRoles: map[string]apiv2.ProjectRole{
// 					"john.doe@github": apiv2.ProjectRole_TENANT_ROLE_OWNER,
// 				},
// 			})

// 			reqCtx := token.ContextWithToken(t.Context(), tok)

// 			got, err := u.UpdateMember(reqCtx, connect.NewRequest(tt.rq))
// 			if diff := cmp.Diff(err, tt.wantErr, errorutil.ConnectErrorComparer()); diff != "" {
// 				t.Errorf("diff = %s", diff)
// 			}

// 			if diff := cmp.Diff(
// 				tt.want, pointer.SafeDeref(got).Msg,
// 				protocmp.Transform(),
// 				protocmp.IgnoreFields(
// 					&apiv2.ProjectMember{}, "created_at",
// 				),
// 			); diff != "" {
// 				t.Errorf("%v, want %v diff: %s", got.Msg, tt.want, diff)
// 			}
// 		})
// 	}
// }

// func Test_projectServiceServer_MemberRemove(t *testing.T) {
// 	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

// 	testStore, closer := test.StartRepositoryWithCleanup(t, log, test.WithCockroach(true))
// 	defer closer()
// 	repo := testStore.Store

// 	tests := []struct {
// 		name                   string
// 		existingProjects       []*apiv2.ProjectServiceCreateRequest
// 		existingProjectMembers map[string][]*repository.ProjectMemberCreateRequest
// 		rq                     *apiv2.ProjectServiceRemoveMemberRequest
// 		wantErr                error
// 	}{
// 		{
// 			name: "remove a member",
// 			rq: &apiv2.ProjectServiceRemoveMemberRequest{
// 				Login:  "b950f4f5-d8b8-4252-aa02-ae08a1d2b044",
// 				Member: "will.smith@github",
// 			},
// 			existingProjects: []*apiv2.ProjectServiceCreateRequest{
// 				{Name: "john.doe@github"},
// 				{Name: "b950f4f5-d8b8-4252-aa02-ae08a1d2b044"},
// 				{Name: "will.smith@github"},
// 			},
// 			existingProjectMembers: map[string][]*repository.ProjectMemberCreateRequest{
// 				"b950f4f5-d8b8-4252-aa02-ae08a1d2b044": {
// 					{MemberID: "john.doe@github", Role: apiv2.ProjectRole_TENANT_ROLE_OWNER},
// 					{MemberID: "will.smith@github", Role: apiv2.ProjectRole_TENANT_ROLE_OWNER},
// 				},
// 			},
// 			wantErr: nil,
// 		},
// 		{
// 			name: "unable to remove own default project",
// 			rq: &apiv2.ProjectServiceRemoveMemberRequest{
// 				Login:  "john.doe@github",
// 				Member: "john.doe@github",
// 			},
// 			existingProjects: []*apiv2.ProjectServiceCreateRequest{
// 				{Name: "john.doe@github"},
// 				{Name: "will.smith@github"},
// 			},
// 			existingProjectMembers: map[string][]*repository.ProjectMemberCreateRequest{
// 				"john.doe@github": {
// 					{MemberID: "john.doe@github", Role: apiv2.ProjectRole_TENANT_ROLE_OWNER},
// 					{MemberID: "will.smith@github", Role: apiv2.ProjectRole_TENANT_ROLE_OWNER},
// 				},
// 			},
// 			wantErr: errorutil.InvalidArgument("cannot remove a member from their own default project"),
// 		},
// 		{
// 			name: "unable to remove last owner",
// 			rq: &apiv2.ProjectServiceRemoveMemberRequest{
// 				Login:  "b950f4f5-d8b8-4252-aa02-ae08a1d2b044",
// 				Member: "john.doe@github",
// 			},
// 			existingProjects: []*apiv2.ProjectServiceCreateRequest{
// 				{Name: "john.doe@github"},
// 				{Name: "b950f4f5-d8b8-4252-aa02-ae08a1d2b044"},
// 			},
// 			existingProjectMembers: map[string][]*repository.ProjectMemberCreateRequest{
// 				"b950f4f5-d8b8-4252-aa02-ae08a1d2b044": {
// 					{MemberID: "john.doe@github", Role: apiv2.ProjectRole_TENANT_ROLE_OWNER},
// 				},
// 			},
// 			wantErr: errorutil.InvalidArgument("cannot remove last owner of a project"),
// 		},
// 	}
// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			u := &projectServiceServer{
// 				log:         log,
// 				repo:        repo,
// 				inviteStore: testStore.GetProjectInviteStore(),
// 				tokenStore:  testStore.GetTokenStore(),
// 			}

// 			test.CreateProjects(t, testStore, tt.existingProjects)
// 			if tt.existingProjectMembers != nil {
// 				for project, members := range tt.existingProjectMembers {
// 					test.CreateProjectMemberships(t, testStore, project, members)
// 				}
// 			}
// 			defer func() {
// 				testStore.DeleteProjects()
// 			}()

// 			tok := testStore.GetToken("john.doe@github", &apiv2.TokenServiceCreateRequest{
// 				Expires: durationpb.New(time.Hour),
// 				ProjectRoles: map[string]apiv2.ProjectRole{
// 					"john.doe@github": apiv2.ProjectRole_TENANT_ROLE_OWNER,
// 				},
// 			})

// 			reqCtx := token.ContextWithToken(t.Context(), tok)

// 			_, err := u.RemoveMember(reqCtx, connect.NewRequest(tt.rq))
// 			if diff := cmp.Diff(err, tt.wantErr, errorutil.ConnectErrorComparer()); diff != "" {
// 				t.Errorf("diff = %s", diff)
// 			}
// 		})
// 	}
// }

// func Test_projectServiceServer_Get(t *testing.T) {
// 	tests := []struct {
// 		name                     string
// 		req                      *apiv2.ProjectServiceGetRequest
// 		tenantRole               apiv2.TenantRole
// 		projectServiceMock       func(mock *tmock.Mock)
// 		tenantServiceMock        func(mock *tmock.Mock)
// 		projectMemberServiceMock func(mock *tmock.Mock)
// 		want                     *apiv2.ProjectServiceGetResponse
// 		wantErr                  bool
// 	}{
// 		{
// 			name: "no members except one",
// 			req: &apiv2.ProjectServiceGetRequest{
// 				Project: "project",
// 			},
// 			tenantRole: apiv2.TenantRole_TENANT_ROLE_OWNER,
// 			projectServiceMock: func(mock *tmock.Mock) {
// 				mock.On("Get", tmock.Anything, &mdmv1.ProjectGetRequest{Id: "project"}).Return(&mdmv1.ProjectResponse{
// 					Project: &mdmv1.Project{
// 						Meta:     &mdmv1.Meta{Id: "project"},
// 						TenantId: "me",
// 					},
// 				}, nil)
// 			},
// 			tenantServiceMock: func(mock *tmock.Mock) {
// 				mock.On("ListTenantMembers", tmock.Anything, &mdmv1.ListTenantMembersRequest{
// 					TenantId: "me", IncludeInherited: pointer.Pointer(true),
// 				}).Return(&mdmv1.ListTenantMembersResponse{
// 					Tenants: []*mdmv1.TenantWithMembershipAnnotations{
// 						{
// 							Tenant: &mdmv1.Tenant{
// 								Meta: &mdmv1.Meta{Id: "me"},
// 							},
// 							ProjectAnnotations: map[string]string{
// 								repository.ProjectRoleAnnotation: apiv2.ProjectRole_PROJECT_ROLE_OWNER.String(),
// 							},
// 							TenantAnnotations: map[string]string{
// 								repository.TenantRoleAnnotation: apiv2.TenantRole_TENANT_ROLE_OWNER.String(),
// 							},
// 						},
// 					},
// 				}, nil)
// 			},
// 			projectMemberServiceMock: func(mock *tmock.Mock) {
// 				mock.On("Find", tmock.Anything, &mdmv1.ProjectMemberFindRequest{
// 					ProjectId: pointer.Pointer("project"),
// 				}).Return(&mdmv1.ProjectMemberListResponse{
// 					ProjectMembers: []*mdmv1.ProjectMember{
// 						{
// 							Meta: &mdmv1.Meta{
// 								Annotations: map[string]string{
// 									repository.ProjectRoleAnnotation: apiv2.ProjectRole_PROJECT_ROLE_OWNER.String(),
// 								},
// 							},
// 							ProjectId: "project",
// 							TenantId:  "me",
// 						},
// 					},
// 				}, nil)
// 			},
// 			want: &apiv2.ProjectServiceGetResponse{
// 				Project: &apiv2.Project{
// 					Uuid:      "project",
// 					Meta:      &apiv2.Meta{},
// 					Tenant:    "me",
// 					AvatarUrl: pointer.Pointer(""),
// 				},
// 				ProjectMembers: []*apiv2.ProjectMember{
// 					{
// 						Id:   "me",
// 						Role: apiv2.ProjectRole_PROJECT_ROLE_OWNER,
// 					},
// 				},
// 			},
// 			wantErr: false,
// 		},
// 		{
// 			name: "one direct member with tenant role guest",
// 			req: &apiv2.ProjectServiceGetRequest{
// 				Project: "project",
// 			},
// 			tenantRole: apiv2.TenantRole_TENANT_ROLE_OWNER,
// 			projectServiceMock: func(mock *tmock.Mock) {
// 				mock.On("Get", tmock.Anything, &mdmv1.ProjectGetRequest{Id: "project"}).Return(&mdmv1.ProjectResponse{
// 					Project: &mdmv1.Project{
// 						Meta:     &mdmv1.Meta{Id: "project"},
// 						TenantId: "me",
// 					},
// 				}, nil)
// 			},
// 			tenantServiceMock: func(mock *tmock.Mock) {
// 				mock.On("ListTenantMembers", tmock.Anything, &mdmv1.ListTenantMembersRequest{
// 					TenantId: "me", IncludeInherited: pointer.Pointer(true),
// 				}).Return(&mdmv1.ListTenantMembersResponse{
// 					Tenants: []*mdmv1.TenantWithMembershipAnnotations{
// 						{
// 							Tenant: &mdmv1.Tenant{
// 								Meta: &mdmv1.Meta{Id: "me"},
// 							},
// 							TenantAnnotations: map[string]string{
// 								repository.TenantRoleAnnotation: apiv2.TenantRole_TENANT_ROLE_OWNER.String(),
// 							},
// 						},
// 						{
// 							Tenant: &mdmv1.Tenant{
// 								Meta: &mdmv1.Meta{Id: "guest"},
// 							},
// 							TenantAnnotations: map[string]string{
// 								repository.TenantRoleAnnotation: apiv2.TenantRole_TENANT_ROLE_GUEST.String(),
// 							},
// 						},
// 					},
// 				}, nil)
// 			},
// 			projectMemberServiceMock: func(mock *tmock.Mock) {
// 				mock.On("Find", tmock.Anything, &mdmv1.ProjectMemberFindRequest{
// 					ProjectId: pointer.Pointer("project"),
// 				}).Return(&mdmv1.ProjectMemberListResponse{
// 					ProjectMembers: []*mdmv1.ProjectMember{
// 						{
// 							Meta: &mdmv1.Meta{
// 								Annotations: map[string]string{
// 									repository.ProjectRoleAnnotation: apiv2.ProjectRole_PROJECT_ROLE_OWNER.String(),
// 								},
// 							},
// 							ProjectId: "project",
// 							TenantId:  "me",
// 						},
// 						{
// 							Meta: &mdmv1.Meta{
// 								Annotations: map[string]string{
// 									repository.ProjectRoleAnnotation: apiv2.ProjectRole_PROJECT_ROLE_VIEWER.String(),
// 								},
// 							},
// 							ProjectId: "project",
// 							TenantId:  "guest",
// 						},
// 					},
// 				}, nil)
// 			},
// 			want: &apiv2.ProjectServiceGetResponse{
// 				Project: &apiv2.Project{
// 					Uuid:      "project",
// 					Meta:      &apiv2.Meta{},
// 					Tenant:    "me",
// 					AvatarUrl: pointer.Pointer(""),
// 				},
// 				ProjectMembers: []*apiv2.ProjectMember{
// 					{
// 						Id:   "guest",
// 						Role: apiv2.ProjectRole_PROJECT_ROLE_VIEWER,
// 					},
// 					{
// 						Id:   "me",
// 						Role: apiv2.ProjectRole_PROJECT_ROLE_OWNER,
// 					},
// 				},
// 			},
// 			wantErr: false,
// 		},
// 		{
// 			name: "editor member with tenant role viewer",
// 			req: &apiv2.ProjectServiceGetRequest{
// 				Project: "project",
// 			},
// 			tenantRole: apiv2.TenantRole_TENANT_ROLE_OWNER,
// 			projectServiceMock: func(mock *tmock.Mock) {
// 				mock.On("Get", tmock.Anything, &mdmv1.ProjectGetRequest{Id: "project"}).Return(&mdmv1.ProjectResponse{
// 					Project: &mdmv1.Project{
// 						Meta:     &mdmv1.Meta{Id: "project"},
// 						TenantId: "me",
// 					},
// 				}, nil)
// 			},
// 			tenantServiceMock: func(mock *tmock.Mock) {
// 				mock.On("ListTenantMembers", tmock.Anything, &mdmv1.ListTenantMembersRequest{
// 					TenantId: "me", IncludeInherited: pointer.Pointer(true),
// 				}).Return(&mdmv1.ListTenantMembersResponse{
// 					Tenants: []*mdmv1.TenantWithMembershipAnnotations{
// 						{
// 							Tenant: &mdmv1.Tenant{
// 								Meta: &mdmv1.Meta{Id: "me"},
// 							},
// 							TenantAnnotations: map[string]string{
// 								repository.TenantRoleAnnotation: apiv2.TenantRole_TENANT_ROLE_OWNER.String(),
// 							},
// 						},
// 						{
// 							Tenant: &mdmv1.Tenant{
// 								Meta: &mdmv1.Meta{Id: "editor"},
// 							},
// 							TenantAnnotations: map[string]string{
// 								repository.TenantRoleAnnotation: apiv2.TenantRole_TENANT_ROLE_VIEWER.String(),
// 							},
// 						},
// 					},
// 				}, nil)
// 			},
// 			projectMemberServiceMock: func(mock *tmock.Mock) {
// 				mock.On("Find", tmock.Anything, &mdmv1.ProjectMemberFindRequest{
// 					ProjectId: pointer.Pointer("project"),
// 				}).Return(&mdmv1.ProjectMemberListResponse{
// 					ProjectMembers: []*mdmv1.ProjectMember{
// 						{
// 							Meta: &mdmv1.Meta{
// 								Annotations: map[string]string{
// 									repository.ProjectRoleAnnotation: apiv2.ProjectRole_PROJECT_ROLE_OWNER.String(),
// 								},
// 							},
// 							ProjectId: "project",
// 							TenantId:  "me",
// 						},
// 						{
// 							Meta: &mdmv1.Meta{
// 								Annotations: map[string]string{
// 									repository.ProjectRoleAnnotation: apiv2.ProjectRole_PROJECT_ROLE_EDITOR.String(),
// 								},
// 							},
// 							ProjectId: "project",
// 							TenantId:  "editor",
// 						},
// 					},
// 				}, nil)
// 			},
// 			want: &apiv2.ProjectServiceGetResponse{
// 				Project: &apiv2.Project{
// 					Uuid:      "project",
// 					Meta:      &apiv2.Meta{},
// 					Tenant:    "me",
// 					AvatarUrl: pointer.Pointer(""),
// 				},
// 				ProjectMembers: []*apiv2.ProjectMember{
// 					{
// 						Id:   "editor",
// 						Role: apiv2.ProjectRole_PROJECT_ROLE_EDITOR,
// 					},
// 					{
// 						Id:   "me",
// 						Role: apiv2.ProjectRole_PROJECT_ROLE_OWNER,
// 					},
// 				},
// 			},
// 			wantErr: false,
// 		},
// 		{
// 			name: "viewer member with tenant role owner",
// 			req: &apiv2.ProjectServiceGetRequest{
// 				Project: "project",
// 			},
// 			tenantRole: apiv2.TenantRole_TENANT_ROLE_OWNER,
// 			projectServiceMock: func(mock *tmock.Mock) {
// 				mock.On("Get", tmock.Anything, &mdmv1.ProjectGetRequest{Id: "project"}).Return(&mdmv1.ProjectResponse{
// 					Project: &mdmv1.Project{
// 						Meta:     &mdmv1.Meta{Id: "project"},
// 						TenantId: "me",
// 					},
// 				}, nil)
// 			},
// 			tenantServiceMock: func(mock *tmock.Mock) {
// 				mock.On("ListTenantMembers", tmock.Anything, &mdmv1.ListTenantMembersRequest{
// 					TenantId: "me", IncludeInherited: pointer.Pointer(true),
// 				}).Return(&mdmv1.ListTenantMembersResponse{
// 					Tenants: []*mdmv1.TenantWithMembershipAnnotations{
// 						{
// 							Tenant: &mdmv1.Tenant{
// 								Meta: &mdmv1.Meta{Id: "me"},
// 							},
// 							TenantAnnotations: map[string]string{
// 								repository.TenantRoleAnnotation: apiv2.TenantRole_TENANT_ROLE_OWNER.String(),
// 							},
// 						},
// 						{
// 							Tenant: &mdmv1.Tenant{
// 								Meta: &mdmv1.Meta{Id: "owner"},
// 							},
// 							TenantAnnotations: map[string]string{
// 								repository.TenantRoleAnnotation: apiv2.TenantRole_TENANT_ROLE_OWNER.String(),
// 							},
// 						},
// 					},
// 				}, nil)
// 			},
// 			projectMemberServiceMock: func(mock *tmock.Mock) {
// 				mock.On("Find", tmock.Anything, &mdmv1.ProjectMemberFindRequest{
// 					ProjectId: pointer.Pointer("project"),
// 				}).Return(&mdmv1.ProjectMemberListResponse{
// 					ProjectMembers: []*mdmv1.ProjectMember{
// 						{
// 							Meta: &mdmv1.Meta{
// 								Annotations: map[string]string{
// 									repository.ProjectRoleAnnotation: apiv2.ProjectRole_PROJECT_ROLE_OWNER.String(),
// 								},
// 							},
// 							ProjectId: "project",
// 							TenantId:  "me",
// 						},
// 						{
// 							Meta: &mdmv1.Meta{
// 								Annotations: map[string]string{
// 									repository.ProjectRoleAnnotation: apiv2.ProjectRole_PROJECT_ROLE_VIEWER.String(),
// 								},
// 							},
// 							ProjectId: "project",
// 							TenantId:  "owner",
// 						},
// 					},
// 				}, nil)
// 			},
// 			want: &apiv2.ProjectServiceGetResponse{
// 				Project: &apiv2.Project{
// 					Uuid:      "project",
// 					Meta:      &apiv2.Meta{},
// 					Tenant:    "me",
// 					AvatarUrl: pointer.Pointer(""),
// 				},
// 				ProjectMembers: []*apiv2.ProjectMember{
// 					{
// 						Id:   "me",
// 						Role: apiv2.ProjectRole_PROJECT_ROLE_OWNER,
// 					},
// 					{
// 						Id:   "owner",
// 						Role: apiv2.ProjectRole_PROJECT_ROLE_OWNER,
// 					},
// 				},
// 			},
// 			wantErr: false,
// 		},
// 		{
// 			name: "inherited member",
// 			req: &apiv2.ProjectServiceGetRequest{
// 				Project: "project",
// 			},
// 			tenantRole: apiv2.TenantRole_TENANT_ROLE_VIEWER,
// 			projectServiceMock: func(mock *tmock.Mock) {
// 				mock.On("Get", tmock.Anything, &mdmv1.ProjectGetRequest{Id: "project"}).Return(&mdmv1.ProjectResponse{
// 					Project: &mdmv1.Project{
// 						Meta:     &mdmv1.Meta{Id: "project"},
// 						TenantId: "me",
// 					},
// 				}, nil)
// 			},
// 			tenantServiceMock: func(mock *tmock.Mock) {
// 				mock.On("ListTenantMembers", tmock.Anything, &mdmv1.ListTenantMembersRequest{
// 					TenantId: "me", IncludeInherited: pointer.Pointer(true),
// 				}).Return(&mdmv1.ListTenantMembersResponse{
// 					Tenants: []*mdmv1.TenantWithMembershipAnnotations{
// 						{
// 							Tenant: &mdmv1.Tenant{
// 								Meta: &mdmv1.Meta{Id: "me"},
// 							},
// 							TenantAnnotations: map[string]string{
// 								repository.TenantRoleAnnotation: apiv2.TenantRole_TENANT_ROLE_OWNER.String(),
// 							},
// 						},
// 						{
// 							Tenant: &mdmv1.Tenant{
// 								Meta: &mdmv1.Meta{Id: "viewer"},
// 							},
// 							TenantAnnotations: map[string]string{
// 								repository.TenantRoleAnnotation: apiv2.TenantRole_TENANT_ROLE_VIEWER.String(),
// 							},
// 						},
// 					},
// 				}, nil)
// 			},
// 			projectMemberServiceMock: func(mock *tmock.Mock) {
// 				mock.On("Find", tmock.Anything, &mdmv1.ProjectMemberFindRequest{
// 					ProjectId: pointer.Pointer("project"),
// 				}).Return(&mdmv1.ProjectMemberListResponse{
// 					ProjectMembers: []*mdmv1.ProjectMember{
// 						{
// 							Meta: &mdmv1.Meta{
// 								Annotations: map[string]string{
// 									repository.ProjectRoleAnnotation: apiv2.ProjectRole_PROJECT_ROLE_OWNER.String(),
// 								},
// 							},
// 							ProjectId: "project",
// 							TenantId:  "me",
// 						},
// 					},
// 				}, nil)
// 			},
// 			want: &apiv2.ProjectServiceGetResponse{
// 				Project: &apiv2.Project{
// 					Uuid:      "project",
// 					Meta:      &apiv2.Meta{},
// 					Tenant:    "me",
// 					AvatarUrl: pointer.Pointer(""),
// 				},
// 				ProjectMembers: []*apiv2.ProjectMember{
// 					{
// 						Id:   "me",
// 						Role: apiv2.ProjectRole_PROJECT_ROLE_OWNER,
// 					},
// 					{
// 						Id:                  "viewer",
// 						Role:                apiv2.ProjectRole_PROJECT_ROLE_VIEWER,
// 						InheritedMembership: true,
// 					},
// 				},
// 			},
// 			wantErr: false,
// 		},
// 		{
// 			name: "do not list inherited member for guests",
// 			req: &apiv2.ProjectServiceGetRequest{
// 				Project: "project",
// 			},
// 			tenantRole: apiv2.TenantRole_TENANT_ROLE_GUEST,
// 			projectServiceMock: func(mock *tmock.Mock) {
// 				mock.On("Get", tmock.Anything, &mdmv1.ProjectGetRequest{Id: "project"}).Return(&mdmv1.ProjectResponse{
// 					Project: &mdmv1.Project{
// 						Meta:     &mdmv1.Meta{Id: "project"},
// 						TenantId: "me",
// 					},
// 				}, nil)
// 			},
// 			projectMemberServiceMock: func(mock *tmock.Mock) {
// 				mock.On("Find", tmock.Anything, &mdmv1.ProjectMemberFindRequest{
// 					ProjectId: pointer.Pointer("project"),
// 				}).Return(&mdmv1.ProjectMemberListResponse{
// 					ProjectMembers: []*mdmv1.ProjectMember{
// 						{
// 							Meta: &mdmv1.Meta{
// 								Annotations: map[string]string{
// 									repository.ProjectRoleAnnotation: apiv2.ProjectRole_PROJECT_ROLE_OWNER.String(),
// 								},
// 							},
// 							ProjectId: "project",
// 							TenantId:  "me",
// 						},
// 						{
// 							Meta: &mdmv1.Meta{
// 								Annotations: map[string]string{
// 									repository.ProjectRoleAnnotation: apiv2.ProjectRole_PROJECT_ROLE_EDITOR.String(),
// 								},
// 							},
// 							ProjectId: "project",
// 							TenantId:  "guest",
// 						},
// 					},
// 				}, nil)
// 			},
// 			want: &apiv2.ProjectServiceGetResponse{
// 				Project: &apiv2.Project{
// 					Uuid:      "project",
// 					Meta:      &apiv2.Meta{},
// 					Tenant:    "me",
// 					AvatarUrl: pointer.Pointer(""),
// 				},
// 				ProjectMembers: []*apiv2.ProjectMember{
// 					{
// 						Id:   "guest",
// 						Role: apiv2.ProjectRole_PROJECT_ROLE_EDITOR,
// 					},
// 					{
// 						Id:   "me",
// 						Role: apiv2.ProjectRole_PROJECT_ROLE_OWNER,
// 					},
// 				},
// 			},
// 			wantErr: false,
// 		},
// 	}
// 	for _, tt := range tests {
// 		tt := tt
// 		t.Run(tt.name, func(t *testing.T) {
// 			m := miniredis.RunT(t)
// 			defer m.Close()
// 			c := redis.NewClient(&redis.Options{Addr: m.Addr()})

// 			tokenStore := token.NewRedisStore(c)

// 			ctx := token.ContextWithToken(t.Context(), &apiv2.Token{
// 				TenantRoles: map[string]apiv2.TenantRole{
// 					tt.want.Project.Tenant: tt.tenantRole,
// 				},
// 			})

// 			p := &projectServiceServer{
// 				log:        slog.Default(),
// 				repo:       nil, // FIXME
// 				tokenStore: tokenStore,
// 			}

// 			result, err := p.Get(ctx, connect.NewRequest(tt.req))
// 			require.NoError(t, err)
// 			assert.Equal(t, tt.want, result.Msg)
// 		})
// 	}
// }

// func Test_service_InviteAccept(t *testing.T) {
// 	ctx := t.Context()
// 	secret, err := invite.GenerateInviteSecret()
// 	require.NoError(t, err)

// 	tests := []struct {
// 		name                     string
// 		tenant                   *apiv2.ProjectServiceInviteAcceptRequest
// 		token                    *apiv2.Token
// 		projectServiceMock       func(mock *tmock.Mock)
// 		tenantServiceMock        func(mock *tmock.Mock)
// 		projectMemberServiceMock func(mock *tmock.Mock)
// 		inviteStorePrepare       func(store invite.ProjectInviteStore)
// 		want                     *apiv2.ProjectServiceInviteAcceptResponse
// 		wantErr                  *connect.Error
// 	}{
// 		{
// 			name: "accept an invite",
// 			tenant: &apiv2.ProjectServiceInviteAcceptRequest{
// 				Secret: secret,
// 			},
// 			token: &apiv2.Token{
// 				Uuid: "123",
// 				User: "new-member",
// 			},
// 			tenantServiceMock: func(mock *tmock.Mock) {
// 				mock.On("Get", tmock.Anything, &mdmv1.TenantGetRequest{Id: "new-member"}).Return(&mdmv1.TenantResponse{Tenant: &mdmv1.Tenant{
// 					Meta: &mdmv1.Meta{Id: "new-member"},
// 				}}, nil)
// 			},
// 			projectServiceMock: func(mock *tmock.Mock) {
// 				mock.On("Get", tmock.Anything, &mdmv1.ProjectGetRequest{Id: "1"}).Return(&mdmv1.ProjectResponse{Project: &mdmv1.Project{
// 					Meta:     &mdmv1.Meta{Id: "1"},
// 					TenantId: "a",
// 				}}, nil)
// 			},
// 			projectMemberServiceMock: func(mock *tmock.Mock) {
// 				mock.On("Find", tmock.Anything, &mdmv1.ProjectMemberFindRequest{TenantId: pointer.Pointer("new-member"), ProjectId: pointer.Pointer("1")}).Return(&mdmv1.ProjectMemberListResponse{ProjectMembers: nil}, nil)
// 				mock.On("Create", tmock.Anything, &mdmv1.ProjectMemberCreateRequest{
// 					ProjectMember: &mdmv1.ProjectMember{
// 						Meta: &mdmv1.Meta{
// 							Annotations: map[string]string{
// 								repository.ProjectRoleAnnotation: apiv2.ProjectRole_PROJECT_ROLE_EDITOR.String(),
// 							},
// 						},
// 						TenantId:  "new-member",
// 						ProjectId: "1",
// 					},
// 				}).Return(&mdmv1.ProjectMemberResponse{
// 					ProjectMember: &mdmv1.ProjectMember{
// 						Meta: &mdmv1.Meta{
// 							Id: "a-random-uuid",
// 						},
// 						TenantId:  "new-member",
// 						ProjectId: "1",
// 					},
// 				}, nil)
// 			},
// 			inviteStorePrepare: func(store invite.ProjectInviteStore) {
// 				err := store.SetInvite(ctx, &apiv2.ProjectInvite{
// 					Secret:      secret,
// 					Project:     "1",
// 					Role:        apiv2.ProjectRole_PROJECT_ROLE_EDITOR,
// 					Joined:      false,
// 					ProjectName: "name of 1",
// 					Tenant:      "user a",
// 					TenantName:  "name of user a",
// 					ExpiresAt:   timestamppb.New(time.Now().Add(10 * time.Minute)),
// 					JoinedAt:    nil,
// 				})
// 				require.NoError(t, err)
// 			},
// 			want: &apiv2.ProjectServiceInviteAcceptResponse{
// 				ProjectName: "name of 1",
// 				Project:     "1",
// 			},
// 			wantErr: nil,
// 		},
// 	}
// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			m := miniredis.RunT(t)
// 			defer m.Close()

// 			var (
// 				c = redis.NewClient(&redis.Options{Addr: m.Addr()})

// 				inviteStore = invite.NewProjectRedisStore(c)
// 			)

// 			ctx := token.ContextWithToken(ctx, tt.token)

// 			if tt.inviteStorePrepare != nil {
// 				tt.inviteStorePrepare(inviteStore)
// 			}

// 			p := &projectServiceServer{
// 				log:         slog.Default(),
// 				repo:        nil, // FIXME
// 				inviteStore: inviteStore,
// 			}

// 			result, err := p.InviteAccept(ctx, connect.NewRequest(tt.tenant))
// 			require.NoError(t, err)

// 			assert.Equal(t, result.Msg.ProjectName, tt.want.ProjectName)
// 			assert.Equal(t, result.Msg.Project, tt.want.Project)
// 		})
// 	}
// }

// // FIXME test delete which traverses all assets with project reference
