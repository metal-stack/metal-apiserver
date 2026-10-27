package admin

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/metal-stack/api/go/errorutil"
	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/test"
	"github.com/metal-stack/metal-apiserver/pkg/token"
	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/durationpb"
)

const (
	p0 = "00000000-0000-0000-0000-000000000000"
	p1 = "00000000-0000-0000-0000-000000000001"
)

func Test_projectServiceServer_List(t *testing.T) {
	t.Parallel()

	testStore, closer := test.StartRepositoryWithCleanup(t, test.WithPostgres(true))
	log := testStore.GetLogger()
	defer closer()

	test.CreateTenants(t, testStore, []*apiv2.TenantServiceCreateRequest{
		{Name: "john.doe@github"},
		{Name: "will.smith@github"},
	})
	test.CreateProjects(t, testStore, []*apiv2.ProjectServiceCreateRequest{
		{Name: p0, Login: "john.doe@github"},
		{Name: "will.smith@github", Login: "will.smith@github"},
		{Name: p1, Login: "john.doe@github"},
	})

	test.CreateProjectMemberships(t, testStore, p0, []*apiv2.ProjectMemberServiceCreateRequest{
		{Member: "john.doe@github", Role: apiv2.ProjectRole_PROJECT_ROLE_OWNER},
	})
	test.CreateProjectMemberships(t, testStore, p1, []*apiv2.ProjectMemberServiceCreateRequest{
		{Member: "john.doe@github", Role: apiv2.ProjectRole_PROJECT_ROLE_EDITOR},
	})
	test.CreateProjectMemberships(t, testStore, "will.smith@github", []*apiv2.ProjectMemberServiceCreateRequest{
		{Member: "will.smith@github", Role: apiv2.ProjectRole_PROJECT_ROLE_OWNER},
	})

	tests := []struct {
		name    string
		rq      *adminv2.ProjectServiceListRequest
		want    *adminv2.ProjectServiceListResponse
		wantErr error
	}{
		{
			name: "list the projects",
			rq:   &adminv2.ProjectServiceListRequest{},
			want: &adminv2.ProjectServiceListResponse{
				Projects: []*apiv2.Project{
					{
						Meta:   &apiv2.Meta{},
						Name:   p0,
						Uuid:   p0,
						Tenant: "john.doe@github",
					},
					{
						Meta:   &apiv2.Meta{},
						Name:   p1,
						Uuid:   p1,
						Tenant: "john.doe@github",
					},
					{
						Meta:   &apiv2.Meta{},
						Name:   "will.smith@github",
						Uuid:   "will.smith@github",
						Tenant: "will.smith@github",
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "list the projects filtered by john doe",
			rq: &adminv2.ProjectServiceListRequest{
				Query: &apiv2.ProjectQuery{
					Tenant: new("john.doe@github"),
				},
			},
			want: &adminv2.ProjectServiceListResponse{
				Projects: []*apiv2.Project{
					{
						Meta:   &apiv2.Meta{},
						Name:   p0,
						Uuid:   p0,
						Tenant: "john.doe@github",
					},
					{
						Meta:   &apiv2.Meta{},
						Name:   p1,
						Uuid:   p1,
						Tenant: "john.doe@github",
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "list the projects filtered by will smith",
			rq: &adminv2.ProjectServiceListRequest{
				Query: &apiv2.ProjectQuery{
					Tenant: new("will.smith@github"),
				},
			},
			want: &adminv2.ProjectServiceListResponse{
				Projects: []*apiv2.Project{
					{
						Meta:   &apiv2.Meta{},
						Name:   "will.smith@github",
						Uuid:   "will.smith@github",
						Tenant: "will.smith@github",
					},
				},
			},
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := &projectServiceServer{
				log:  log,
				repo: testStore.Store,
			}

			tok := testStore.GetToken("john.doe@github", &apiv2.TokenServiceCreateRequest{
				Expires:   durationpb.New(time.Hour),
				AdminRole: new(apiv2.AdminRole_ADMIN_ROLE_EDITOR),
			})

			reqCtx := token.ContextWithToken(t.Context(), tok)
			if tt.wantErr == nil {
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

func Test_projectServiceServer_Create(t *testing.T) {
	t.Parallel()

	testStore, closer := test.StartRepositoryWithCleanup(t, test.WithPostgres(true))
	log := testStore.GetLogger()
	defer closer()

	test.CreateTenants(t, testStore, []*apiv2.TenantServiceCreateRequest{
		{Name: "john.doe@github"},
	})

	tests := []struct {
		name    string
		rq      *adminv2.ProjectServiceCreateRequest
		want    *adminv2.ProjectServiceCreateResponse
		wantErr error
	}{
		{
			name: "create project with static id",
			rq: &adminv2.ProjectServiceCreateRequest{
				Id: new(p0),
				Project: &apiv2.ProjectServiceCreateRequest{
					Name:  "my-project",
					Login: "john.doe@github",
					Labels: &apiv2.Labels{
						Labels: map[string]string{
							"a": "b",
						},
					},
					Description: "my project",
					AvatarUrl:   new("http://url"),
				},
			},
			want: &adminv2.ProjectServiceCreateResponse{
				Project: &apiv2.Project{
					Meta: &apiv2.Meta{
						Labels: &apiv2.Labels{
							Labels: map[string]string{
								"a": "b",
							},
						},
					},
					Name:        "my-project",
					Uuid:        p0,
					Tenant:      "john.doe@github",
					Description: "my project",
					AvatarUrl:   new("http://url"),
				},
			},
			wantErr: nil,
		},
		{
			name: "create project twice",
			rq: &adminv2.ProjectServiceCreateRequest{
				Id: new(p0),
				Project: &apiv2.ProjectServiceCreateRequest{
					Name:  "duplicate-project",
					Login: "john.doe@github",
				},
			},
			want:    nil,
			wantErr: errorutil.Conflict("an entity of type:project with the id:%s already exists", p0),
		},
		{
			name: "create project with auto-generated id",
			rq: &adminv2.ProjectServiceCreateRequest{
				Project: &apiv2.ProjectServiceCreateRequest{
					Name:  "another-project",
					Login: "john.doe@github",
				},
			},
			want: &adminv2.ProjectServiceCreateResponse{
				Project: &apiv2.Project{
					Meta:   &apiv2.Meta{},
					Name:   "another-project",
					Tenant: "john.doe@github",
				},
			},
			wantErr: nil,
		},
		{
			name: "create without tenant",
			rq: &adminv2.ProjectServiceCreateRequest{
				Id: new(p1),
				Project: &apiv2.ProjectServiceCreateRequest{
					Name: "no tenant though",
				},
			},
			want:    nil,
			wantErr: errorutil.NotFound("unable to find tenant: for project"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := &projectServiceServer{
				log:  log,
				repo: testStore.Store,
			}

			tok := testStore.GetToken("john.doe@github", &apiv2.TokenServiceCreateRequest{
				Expires:   durationpb.New(time.Hour),
				AdminRole: new(apiv2.AdminRole_ADMIN_ROLE_EDITOR),
			})

			reqCtx := token.ContextWithToken(t.Context(), tok)
			if tt.wantErr == nil {
				test.Validate(t, tt.rq)
			}
			got, err := u.Create(reqCtx, tt.rq)
			if diff := cmp.Diff(err, tt.wantErr, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("diff = %s", diff)
			}

			if tt.wantErr == nil {
				if diff := cmp.Diff(
					tt.want, got,
					protocmp.Transform(),
					protocmp.IgnoreFields(
						&apiv2.Meta{}, "created_at", "updated_at",
					),
					protocmp.IgnoreFields(
						&apiv2.Project{}, "uuid",
					),
				); diff != "" {
					t.Errorf("%v, want %v diff: %s", got, tt.want, diff)
				}

				assert.NotEmpty(t, got.Project.Uuid)
			}
		})
	}
}

func Test_projectServiceServer_AddMember(t *testing.T) {
	t.Parallel()

	testStore, closer := test.StartRepositoryWithCleanup(t, test.WithPostgres(true))
	log := testStore.GetLogger()
	defer closer()

	test.CreateTenants(t, testStore, []*apiv2.TenantServiceCreateRequest{
		{Name: "john.doe@github"},
		{Name: "will.smith@github"},
		{Name: "tina.turner@github"},
	})
	test.CreateProjects(t, testStore, []*apiv2.ProjectServiceCreateRequest{
		{Name: p0, Login: "john.doe@github"},
	})
	test.CreateProjectMemberships(t, testStore, p0, []*apiv2.ProjectMemberServiceCreateRequest{
		{Project: p0, Member: "john.doe@github", Role: apiv2.ProjectRole_PROJECT_ROLE_OWNER},
		{Project: p0, Member: "will.smith@github", Role: apiv2.ProjectRole_PROJECT_ROLE_EDITOR},
	})

	tests := []struct {
		name    string
		rq      *adminv2.ProjectServiceAddMemberRequest
		want    *adminv2.ProjectServiceAddMemberResponse
		wantErr error
	}{
		{
			name: "add member to project",
			rq: &adminv2.ProjectServiceAddMemberRequest{
				Project: p0,
				Member:  "tina.turner@github",
				Role:    apiv2.ProjectRole_PROJECT_ROLE_VIEWER,
				Labels: &apiv2.Labels{
					Labels: map[string]string{
						"a": "b",
					},
				},
			},
			want: &adminv2.ProjectServiceAddMemberResponse{
				Member: &apiv2.ProjectMember{
					Member:  "tina.turner@github",
					Role:    apiv2.ProjectRole_PROJECT_ROLE_VIEWER,
					Project: p0,
					Meta: &apiv2.Meta{
						Labels: &apiv2.Labels{
							Labels: map[string]string{
								"a": "b",
							},
						},
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "add member that already exists",
			rq: &adminv2.ProjectServiceAddMemberRequest{
				Project: p0,
				Member:  "will.smith@github",
				Role:    apiv2.ProjectRole_PROJECT_ROLE_OWNER,
			},
			want:    nil,
			wantErr: errorutil.Conflict(`"will.smith@github" is already member of project %q`, p0),
		},
		{
			name: "add member to non-existent project",
			rq: &adminv2.ProjectServiceAddMemberRequest{
				Project: "non-existent",
				Member:  "tina.turner@github",
				Role:    apiv2.ProjectRole_PROJECT_ROLE_EDITOR,
			},
			want:    nil,
			wantErr: errorutil.NotFound("unable to find project:non-existent for projectMember"),
		},
		{
			name: "non-specified role",
			rq: &adminv2.ProjectServiceAddMemberRequest{
				Project: p0,
				Member:  "tina.turner@github",
				Role:    apiv2.ProjectRole_PROJECT_ROLE_UNSPECIFIED,
			},
			want:    nil,
			wantErr: errorutil.InvalidArgument("role must not be PROJECT_ROLE_UNSPECIFIED"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := &projectServiceServer{
				log:  log,
				repo: testStore.Store,
			}

			tok := testStore.GetToken("john.doe@github", &apiv2.TokenServiceCreateRequest{
				Expires:   durationpb.New(time.Hour),
				AdminRole: new(apiv2.AdminRole_ADMIN_ROLE_EDITOR),
			})

			reqCtx := token.ContextWithToken(t.Context(), tok)
			if tt.wantErr == nil {
				test.Validate(t, tt.rq)
			}
			got, err := u.AddMember(reqCtx, tt.rq)
			if diff := cmp.Diff(err, tt.wantErr, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("diff = %s", diff)
			}

			if tt.wantErr == nil {
				if diff := cmp.Diff(
					tt.want, got,
					protocmp.Transform(),
					protocmp.IgnoreFields(
						&apiv2.Meta{}, "created_at", "updated_at",
					),
				); diff != "" {
					t.Errorf("%v, want %v diff: %s", got, tt.want, diff)
				}
			}
		})
	}
}

func Test_projectServiceServer_RemoveMember(t *testing.T) {
	t.Parallel()

	testStore, closer := test.StartRepositoryWithCleanup(t, test.WithPostgres(true))
	log := testStore.GetLogger()
	defer closer()

	test.CreateTenants(t, testStore, []*apiv2.TenantServiceCreateRequest{
		{Name: "john.doe@github"},
		{Name: "will.smith@github"},
		{Name: "tina.turner@github"},
	})
	test.CreateProjects(t, testStore, []*apiv2.ProjectServiceCreateRequest{
		{Name: p0, Login: "john.doe@github"},
	})
	test.CreateProjectMemberships(t, testStore, p0, []*apiv2.ProjectMemberServiceCreateRequest{
		{Project: p0, Member: "john.doe@github", Role: apiv2.ProjectRole_PROJECT_ROLE_OWNER},
		{Project: p0, Member: "will.smith@github", Role: apiv2.ProjectRole_PROJECT_ROLE_EDITOR},
	})

	tests := []struct {
		name    string
		rq      *adminv2.ProjectServiceRemoveMemberRequest
		want    *adminv2.ProjectServiceRemoveMemberResponse
		wantErr error
	}{
		{
			name: "remove member",
			rq: &adminv2.ProjectServiceRemoveMemberRequest{
				Project: p0,
				Member:  "will.smith@github",
			},
			want: &adminv2.ProjectServiceRemoveMemberResponse{
				Member: &apiv2.ProjectMember{
					Member:  "will.smith@github",
					Project: p0,
					Role:    apiv2.ProjectRole_PROJECT_ROLE_EDITOR,
					Meta:    &apiv2.Meta{},
				},
			},
			wantErr: nil,
		},
		{
			name: "remove member that is not in the project",
			rq: &adminv2.ProjectServiceRemoveMemberRequest{
				Project: p0,
				Member:  "tina.turner@github",
			},
			want:    nil,
			wantErr: errorutil.NotFound("member not found in project %q", p0),
		},
		{
			name: "remove last owner",
			rq: &adminv2.ProjectServiceRemoveMemberRequest{
				Project: p0,
				Member:  "john.doe@github",
			},
			want:    nil,
			wantErr: errorutil.FailedPrecondition("cannot remove last owner of a project"),
		},
		{
			name: "remove member from non-existent project",
			rq: &adminv2.ProjectServiceRemoveMemberRequest{
				Project: "non-existent",
				Member:  "tina.turner@github",
			},
			want:    nil,
			wantErr: errorutil.NotFound("project with id:non-existent not found sql: no rows in result set"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := &projectServiceServer{
				log:  log,
				repo: testStore.Store,
			}

			tok := testStore.GetToken("john.doe@github", &apiv2.TokenServiceCreateRequest{
				Expires:   durationpb.New(time.Hour),
				AdminRole: new(apiv2.AdminRole_ADMIN_ROLE_EDITOR),
			})

			reqCtx := token.ContextWithToken(t.Context(), tok)
			if tt.wantErr == nil {
				test.Validate(t, tt.rq)
			}
			got, err := u.RemoveMember(reqCtx, tt.rq)
			if diff := cmp.Diff(err, tt.wantErr, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("diff = %s", diff)
			}

			if tt.wantErr == nil {
				if diff := cmp.Diff(
					tt.want, got,
					protocmp.Transform(),
					protocmp.IgnoreFields(
						&apiv2.Meta{}, "created_at", "updated_at",
					),
				); diff != "" {
					t.Errorf("%v, want %v diff: %s", got, tt.want, diff)
				}
			}
		})
	}
}
