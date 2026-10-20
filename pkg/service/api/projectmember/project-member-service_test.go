package projectmember

import (
	"sort"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/metal-stack/api/go/errorutil"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/test"
	"github.com/metal-stack/metal-apiserver/pkg/token"
	"google.golang.org/protobuf/testing/protocmp"
)

var (
	p0  = "00000000-0000-0000-0000-000000000000"
	p99 = "00000000-0000-0000-0000-000000000099"
)

func Test_projectMemberServiceServer_Get(t *testing.T) {
	t.Parallel()

	testStore, closer := test.StartRepositoryWithCleanup(t, test.WithPostgres(true))
	log := testStore.GetLogger()
	defer closer()

	test.CreateTenants(t, testStore, []*apiv2.TenantServiceCreateRequest{
		{Name: "john.doe@github"},
		{Name: "will.smith@github"},
		{Name: "tina.turner@github"},
	})
	test.CreateTenantMemberships(t, testStore, "john.doe@github", []*apiv2.TenantMemberServiceCreateRequest{
		{Member: "john.doe@github", Role: apiv2.TenantRole_TENANT_ROLE_OWNER},
		{Member: "tina.turner@github", Role: apiv2.TenantRole_TENANT_ROLE_OWNER},
		{Member: "will.smith@github", Role: apiv2.TenantRole_TENANT_ROLE_OWNER},
	})

	test.CreateProjects(t, testStore, []*apiv2.ProjectServiceCreateRequest{
		{
			Name:        p0,
			Description: "a description",
			AvatarUrl:   new("http://test"),
			Labels: &apiv2.Labels{
				Labels: map[string]string{
					"a": "b",
				},
			},
			Login: "john.doe@github",
		},
	})
	test.CreateProjectMemberships(t, testStore, p0, []*apiv2.ProjectMemberServiceCreateRequest{
		{Member: "john.doe@github", Role: apiv2.ProjectRole_PROJECT_ROLE_OWNER},
		{Member: "will.smith@github", Role: apiv2.ProjectRole_PROJECT_ROLE_EDITOR},
	})

	tests := []struct {
		name         string
		sessionToken *apiv2.Token
		req          *apiv2.ProjectMemberServiceGetRequest
		want         *apiv2.ProjectMemberServiceGetResponse
		wantErr      error
	}{
		{
			name: "get a direct member",
			sessionToken: &apiv2.Token{
				User:      "john.doe@github",
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
				TenantRoles: map[string]apiv2.TenantRole{
					"john.doe@github": apiv2.TenantRole_TENANT_ROLE_OWNER,
				},
				ProjectRoles: map[string]apiv2.ProjectRole{
					p0: apiv2.ProjectRole_PROJECT_ROLE_OWNER,
				},
			},
			req: &apiv2.ProjectMemberServiceGetRequest{
				Project: p0,
				Member:  "will.smith@github",
			},
			want: &apiv2.ProjectMemberServiceGetResponse{
				Member: &apiv2.ProjectMember{
					Member:              "will.smith@github",
					Role:                apiv2.ProjectRole_PROJECT_ROLE_EDITOR,
					Project:             p0,
					Meta:                &apiv2.Meta{},
					InheritedMembership: false,
				},
			},
			wantErr: nil,
		},
		{
			name: "get an inherited member",
			sessionToken: &apiv2.Token{
				User:      "john.doe@github",
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
				TenantRoles: map[string]apiv2.TenantRole{
					"john.doe@github": apiv2.TenantRole_TENANT_ROLE_OWNER,
				},
				ProjectRoles: map[string]apiv2.ProjectRole{
					p0: apiv2.ProjectRole_PROJECT_ROLE_OWNER,
				},
			},
			req: &apiv2.ProjectMemberServiceGetRequest{
				Project: p0,
				Member:  "tina.turner@github",
			},
			want: &apiv2.ProjectMemberServiceGetResponse{
				Member: &apiv2.ProjectMember{
					Member:              "tina.turner@github",
					Role:                apiv2.ProjectRole_PROJECT_ROLE_OWNER,
					Project:             p0,
					Meta:                &apiv2.Meta{},
					InheritedMembership: true,
				},
			},
			wantErr: nil,
		},
		{
			name: "get non-existing member",
			sessionToken: &apiv2.Token{
				User:      "john.doe@github",
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
				TenantRoles: map[string]apiv2.TenantRole{
					"john.doe@github": apiv2.TenantRole_TENANT_ROLE_OWNER,
				},
				ProjectRoles: map[string]apiv2.ProjectRole{
					p0: apiv2.ProjectRole_PROJECT_ROLE_OWNER,
				},
			},
			req: &apiv2.ProjectMemberServiceGetRequest{
				Project: p0,
				Member:  "foo",
			},
			want:    nil,
			wantErr: errorutil.NotFound(`member not found in project "00000000-0000-0000-0000-000000000000"`),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &projectMemberServiceServer{
				log:  log,
				repo: testStore.Store,
			}

			if tt.wantErr == nil {
				test.Validate(t, tt.req)
			}

			ctx := token.ContextWithToken(t.Context(), tt.sessionToken)

			got, err := p.Get(ctx, tt.req)

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

func Test_projectMemberServiceServer_Create(t *testing.T) {
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
		{
			Name:        p0,
			Description: "a description",
			AvatarUrl:   new("http://test"),
			Labels: &apiv2.Labels{
				Labels: map[string]string{
					"a": "b",
				},
			},
			Login: "john.doe@github",
		},
	})
	test.CreateProjectMemberships(t, testStore, p0, []*apiv2.ProjectMemberServiceCreateRequest{
		{Member: "john.doe@github", Role: apiv2.ProjectRole_PROJECT_ROLE_OWNER},
		{Member: "will.smith@github", Role: apiv2.ProjectRole_PROJECT_ROLE_EDITOR},
	})

	tests := []struct {
		name    string
		req     *apiv2.ProjectMemberServiceCreateRequest
		want    *apiv2.ProjectMemberServiceCreateResponse
		wantErr error
	}{
		{
			name: "create a member",
			req: &apiv2.ProjectMemberServiceCreateRequest{
				Project: p0,
				Member:  "tina.turner@github",
				Role:    apiv2.ProjectRole_PROJECT_ROLE_VIEWER,
				Labels: &apiv2.Labels{
					Labels: map[string]string{
						"a": "b",
					},
				},
			},
			want: &apiv2.ProjectMemberServiceCreateResponse{
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
			name: "create a member of a non-existing tenant",
			req: &apiv2.ProjectMemberServiceCreateRequest{
				Project: p0,
				Member:  "foo",
				Role:    apiv2.ProjectRole_PROJECT_ROLE_VIEWER,
			},
			want:    nil,
			wantErr: errorutil.NotFound("unable to find tenant:foo for projectMember"),
		},
		{
			name: "create a member in a non-existing project",
			req: &apiv2.ProjectMemberServiceCreateRequest{
				Project: p99,
				Member:  "will.smith@github",
				Role:    apiv2.ProjectRole_PROJECT_ROLE_VIEWER,
			},
			want:    nil,
			wantErr: errorutil.NotFound("unable to find project:%s for projectMember", p99),
		},
		{
			name: "create a member with unspecified role",
			req: &apiv2.ProjectMemberServiceCreateRequest{
				Project: p0,
				Member:  "tina.turner@github",
				Role:    apiv2.ProjectRole_PROJECT_ROLE_UNSPECIFIED,
			},
			want:    nil,
			wantErr: errorutil.InvalidArgument("role must not be PROJECT_ROLE_UNSPECIFIED"),
		},
		{
			name: "create a membership twice",
			req: &apiv2.ProjectMemberServiceCreateRequest{
				Project: p0,
				Member:  "will.smith@github",
				Role:    apiv2.ProjectRole_PROJECT_ROLE_VIEWER,
			},
			want:    nil,
			wantErr: errorutil.Conflict(`"will.smith@github" is already member of project %q`, p0),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &projectMemberServiceServer{
				log:  log,
				repo: testStore.Store,
			}

			if tt.wantErr == nil {
				test.Validate(t, tt.req)
			}

			got, err := p.Create(t.Context(), tt.req)

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

func Test_projectMemberServiceServer_Update(t *testing.T) {
	t.Parallel()

	testStore, closer := test.StartRepositoryWithCleanup(t, test.WithPostgres(true))
	log := testStore.GetLogger()
	defer closer()

	test.CreateTenants(t, testStore, []*apiv2.TenantServiceCreateRequest{
		{Name: "john.doe@github"},
		{Name: "will.smith@github"},
	})
	test.CreateProjects(t, testStore, []*apiv2.ProjectServiceCreateRequest{
		{
			Name:        p0,
			Description: "a description",
			AvatarUrl:   new("http://test"),
			Labels: &apiv2.Labels{
				Labels: map[string]string{
					"a": "b",
				},
			},
			Login: "john.doe@github",
		},
	})
	test.CreateProjectMemberships(t, testStore, p0, []*apiv2.ProjectMemberServiceCreateRequest{
		{Member: "john.doe@github", Role: apiv2.ProjectRole_PROJECT_ROLE_OWNER},
		{Member: "will.smith@github", Role: apiv2.ProjectRole_PROJECT_ROLE_EDITOR},
	})

	tests := []struct {
		name    string
		req     *apiv2.ProjectMemberServiceUpdateRequest
		want    *apiv2.ProjectMemberServiceUpdateResponse
		wantErr error
	}{
		{
			name: "update a member",
			req: &apiv2.ProjectMemberServiceUpdateRequest{
				Project:    p0,
				Member:     "will.smith@github",
				Role:       new(apiv2.ProjectRole_PROJECT_ROLE_VIEWER),
				UpdateMeta: &apiv2.UpdateMeta{},
				Labels: &apiv2.UpdateLabels{
					Strategy: &apiv2.UpdateLabels_Replace{
						Replace: &apiv2.Labels{
							Labels: map[string]string{
								"c": "d",
							},
						},
					},
				},
			},
			want: &apiv2.ProjectMemberServiceUpdateResponse{
				Member: &apiv2.ProjectMember{
					Member:  "will.smith@github",
					Role:    apiv2.ProjectRole_PROJECT_ROLE_VIEWER,
					Project: p0,
					Meta: &apiv2.Meta{
						Generation: 1,
						Labels: &apiv2.Labels{
							Labels: map[string]string{
								"c": "d",
							},
						},
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "demote last owner",
			req: &apiv2.ProjectMemberServiceUpdateRequest{
				Project:    p0,
				Member:     "john.doe@github",
				Role:       new(apiv2.ProjectRole_PROJECT_ROLE_EDITOR),
				UpdateMeta: &apiv2.UpdateMeta{},
			},
			want:    nil,
			wantErr: errorutil.FailedPrecondition("cannot demote last owner's permissions"),
		},
		{
			name: "update non-existing member",
			req: &apiv2.ProjectMemberServiceUpdateRequest{
				Project:    p0,
				Member:     "foo",
				Role:       new(apiv2.ProjectRole_PROJECT_ROLE_VIEWER),
				UpdateMeta: &apiv2.UpdateMeta{},
			},
			want:    nil,
			wantErr: errorutil.NotFound("member not found in project %q", p0),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &projectMemberServiceServer{
				log:  log,
				repo: testStore.Store,
			}

			if tt.wantErr == nil {
				test.Validate(t, tt.req)
			}

			got, err := p.Update(t.Context(), tt.req)

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

func Test_projectMemberServiceServer_Delete(t *testing.T) {
	t.Parallel()

	testStore, closer := test.StartRepositoryWithCleanup(t, test.WithPostgres(true))
	log := testStore.GetLogger()
	defer closer()

	test.CreateTenants(t, testStore, []*apiv2.TenantServiceCreateRequest{
		{Name: "john.doe@github"},
		{Name: "will.smith@github"},
	})
	test.CreateProjects(t, testStore, []*apiv2.ProjectServiceCreateRequest{
		{
			Name:        p0,
			Description: "a description",
			AvatarUrl:   new("http://test"),
			Labels: &apiv2.Labels{
				Labels: map[string]string{
					"a": "b",
				},
			},
			Login: "john.doe@github",
		},
	})
	test.CreateProjectMemberships(t, testStore, p0, []*apiv2.ProjectMemberServiceCreateRequest{
		{Member: "john.doe@github", Role: apiv2.ProjectRole_PROJECT_ROLE_OWNER},
		{Member: "will.smith@github", Role: apiv2.ProjectRole_PROJECT_ROLE_EDITOR},
	})

	tests := []struct {
		name    string
		req     *apiv2.ProjectMemberServiceDeleteRequest
		want    *apiv2.ProjectMemberServiceDeleteResponse
		wantErr error
	}{
		{
			name: "delete a member",
			req: &apiv2.ProjectMemberServiceDeleteRequest{
				Project: p0,
				Member:  "will.smith@github",
			},
			want: &apiv2.ProjectMemberServiceDeleteResponse{
				Member: &apiv2.ProjectMember{
					Member:  "will.smith@github",
					Role:    apiv2.ProjectRole_PROJECT_ROLE_EDITOR,
					Project: p0,
					Meta:    &apiv2.Meta{},
				},
			},
			wantErr: nil,
		},
		{
			name: "delete last owner",
			req: &apiv2.ProjectMemberServiceDeleteRequest{
				Project: p0,
				Member:  "john.doe@github",
			},
			want:    nil,
			wantErr: errorutil.FailedPrecondition("cannot remove last owner of a project"),
		},
		{
			name: "delete non-existing member",
			req: &apiv2.ProjectMemberServiceDeleteRequest{
				Project: p0,
				Member:  "foo",
			},
			want:    nil,
			wantErr: errorutil.NotFound("member not found in project %q", p0),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &projectMemberServiceServer{
				log:  log,
				repo: testStore.Store,
			}

			if tt.wantErr == nil {
				test.Validate(t, tt.req)
			}

			got, err := p.Delete(t.Context(), tt.req)

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

func Test_projectMemberServiceServer_List(t *testing.T) {
	t.Parallel()

	testStore, closer := test.StartRepositoryWithCleanup(t, test.WithPostgres(true))
	log := testStore.GetLogger()
	defer closer()

	test.CreateTenants(t, testStore, []*apiv2.TenantServiceCreateRequest{
		{Name: "john.doe@github"},
		{Name: "will.smith@github"},
		{Name: "tina.turner@github"},
	})
	test.CreateTenantMemberships(t, testStore, "john.doe@github", []*apiv2.TenantMemberServiceCreateRequest{
		{Member: "john.doe@github", Role: apiv2.TenantRole_TENANT_ROLE_OWNER},
		{Member: "tina.turner@github", Role: apiv2.TenantRole_TENANT_ROLE_VIEWER, Labels: &apiv2.Labels{Labels: map[string]string{"a": "b"}}},
	})

	test.CreateProjects(t, testStore, []*apiv2.ProjectServiceCreateRequest{
		{
			Name:        p0,
			Description: "a description",
			AvatarUrl:   new("http://test"),
			Labels: &apiv2.Labels{
				Labels: map[string]string{
					"a": "b",
				},
			},
			Login: "john.doe@github",
		},
	})
	test.CreateProjectMemberships(t, testStore, p0, []*apiv2.ProjectMemberServiceCreateRequest{
		{Member: "john.doe@github", Role: apiv2.ProjectRole_PROJECT_ROLE_OWNER, Labels: &apiv2.Labels{Labels: map[string]string{"c": "d"}}},
		{Member: "will.smith@github", Role: apiv2.ProjectRole_PROJECT_ROLE_EDITOR},
	})

	tests := []struct {
		name         string
		sessionToken *apiv2.Token
		req          *apiv2.ProjectMemberServiceListRequest
		want         *apiv2.ProjectMemberServiceListResponse
		wantErr      error
	}{
		{
			name: "list members as tenant owner",
			sessionToken: &apiv2.Token{
				User:      "john.doe@github",
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
				TenantRoles: map[string]apiv2.TenantRole{
					"john.doe@github": apiv2.TenantRole_TENANT_ROLE_OWNER,
				},
				ProjectRoles: map[string]apiv2.ProjectRole{
					p0: apiv2.ProjectRole_PROJECT_ROLE_OWNER,
				},
			},
			req: &apiv2.ProjectMemberServiceListRequest{
				Project: p0,
			},
			want: &apiv2.ProjectMemberServiceListResponse{
				Members: []*apiv2.ProjectMember{
					{
						Member:  "john.doe@github",
						Role:    apiv2.ProjectRole_PROJECT_ROLE_OWNER,
						Project: p0,
						Meta: &apiv2.Meta{
							Labels: &apiv2.Labels{
								Labels: map[string]string{
									"c": "d",
								},
							},
						},
					},
					{
						Member:              "tina.turner@github",
						Role:                apiv2.ProjectRole_PROJECT_ROLE_VIEWER,
						Project:             p0,
						Meta:                &apiv2.Meta{},
						InheritedMembership: true,
					},
					{
						Member:  "will.smith@github",
						Role:    apiv2.ProjectRole_PROJECT_ROLE_EDITOR,
						Project: p0,
						Meta:    &apiv2.Meta{},
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "list members as non-tenant member",
			sessionToken: &apiv2.Token{
				User:      "will.smith@github",
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
				TenantRoles: map[string]apiv2.TenantRole{
					"will.smith@github": apiv2.TenantRole_TENANT_ROLE_OWNER,
				},
				ProjectRoles: map[string]apiv2.ProjectRole{
					p0: apiv2.ProjectRole_PROJECT_ROLE_EDITOR,
				},
			},
			req: &apiv2.ProjectMemberServiceListRequest{
				Project: p0,
			},
			want: &apiv2.ProjectMemberServiceListResponse{
				Members: []*apiv2.ProjectMember{
					{
						Member:  "john.doe@github",
						Role:    apiv2.ProjectRole_PROJECT_ROLE_OWNER,
						Project: p0,
						Meta: &apiv2.Meta{
							Labels: &apiv2.Labels{
								Labels: map[string]string{
									"c": "d",
								},
							},
						},
					},
					{
						Member:  "will.smith@github",
						Role:    apiv2.ProjectRole_PROJECT_ROLE_EDITOR,
						Project: p0,
						Meta:    &apiv2.Meta{},
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "list members filtered by role",
			sessionToken: &apiv2.Token{
				User:      "john.doe@github",
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
				TenantRoles: map[string]apiv2.TenantRole{
					"john.doe@github": apiv2.TenantRole_TENANT_ROLE_OWNER,
				},
				ProjectRoles: map[string]apiv2.ProjectRole{
					p0: apiv2.ProjectRole_PROJECT_ROLE_OWNER,
				},
			},
			req: &apiv2.ProjectMemberServiceListRequest{
				Project: p0,
				Query: &apiv2.ProjectMemberQuery{
					Role: new(apiv2.ProjectRole_PROJECT_ROLE_OWNER),
				},
			},
			want: &apiv2.ProjectMemberServiceListResponse{
				Members: []*apiv2.ProjectMember{
					{
						Member:  "john.doe@github",
						Role:    apiv2.ProjectRole_PROJECT_ROLE_OWNER,
						Project: p0,
						Meta: &apiv2.Meta{
							Labels: &apiv2.Labels{
								Labels: map[string]string{
									"c": "d",
								},
							},
						},
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "list members filtered by member",
			sessionToken: &apiv2.Token{
				User:      "john.doe@github",
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
				TenantRoles: map[string]apiv2.TenantRole{
					"john.doe@github": apiv2.TenantRole_TENANT_ROLE_OWNER,
				},
				ProjectRoles: map[string]apiv2.ProjectRole{
					p0: apiv2.ProjectRole_PROJECT_ROLE_OWNER,
				},
			},
			req: &apiv2.ProjectMemberServiceListRequest{
				Project: p0,
				Query: &apiv2.ProjectMemberQuery{
					Member: new("will.smith@github"),
				},
			},
			want: &apiv2.ProjectMemberServiceListResponse{
				Members: []*apiv2.ProjectMember{
					{
						Member:  "will.smith@github",
						Role:    apiv2.ProjectRole_PROJECT_ROLE_EDITOR,
						Project: p0,
						Meta:    &apiv2.Meta{},
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "list members filtered by label",
			sessionToken: &apiv2.Token{
				User:      "john.doe@github",
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
				TenantRoles: map[string]apiv2.TenantRole{
					"john.doe@github": apiv2.TenantRole_TENANT_ROLE_OWNER,
				},
				ProjectRoles: map[string]apiv2.ProjectRole{
					p0: apiv2.ProjectRole_PROJECT_ROLE_OWNER,
				},
			},
			req: &apiv2.ProjectMemberServiceListRequest{
				Project: p0,
				Query: &apiv2.ProjectMemberQuery{
					Labels: &apiv2.Labels{
						Labels: map[string]string{
							"c": "d",
						},
					},
				},
			},
			want: &apiv2.ProjectMemberServiceListResponse{
				Members: []*apiv2.ProjectMember{
					{
						Member:  "john.doe@github",
						Role:    apiv2.ProjectRole_PROJECT_ROLE_OWNER,
						Project: p0,
						Meta: &apiv2.Meta{
							Labels: &apiv2.Labels{
								Labels: map[string]string{
									"c": "d",
								},
							},
						},
					},
				},
			},
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &projectMemberServiceServer{
				log:  log,
				repo: testStore.Store,
			}

			if tt.wantErr == nil {
				test.Validate(t, tt.req)
			}

			ctx := token.ContextWithToken(t.Context(), tt.sessionToken)

			got, err := p.List(ctx, tt.req)

			if diff := cmp.Diff(err, tt.wantErr, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("diff = %s", diff)
			}

			sort.Slice(got.GetMembers(), func(i, j int) bool {
				return got.GetMembers()[i].Member < got.GetMembers()[j].Member
			})

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

func Test_projectMemberServiceServer_Leave(t *testing.T) {
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
		{
			Name:        p0,
			Description: "a description",
			AvatarUrl:   new("http://test"),
			Labels: &apiv2.Labels{
				Labels: map[string]string{
					"a": "b",
				},
			},
			Login: "john.doe@github",
		},
	})
	test.CreateProjectMemberships(t, testStore, p0, []*apiv2.ProjectMemberServiceCreateRequest{
		{Member: "john.doe@github", Role: apiv2.ProjectRole_PROJECT_ROLE_OWNER},
		{Member: "will.smith@github", Role: apiv2.ProjectRole_PROJECT_ROLE_EDITOR},
	})

	tests := []struct {
		name         string
		sessionToken *apiv2.Token
		req          *apiv2.ProjectMemberServiceLeaveRequest
		want         *apiv2.ProjectMemberServiceLeaveResponse
		wantErr      error
	}{
		{
			name: "leave a project",
			sessionToken: &apiv2.Token{
				User:      "will.smith@github",
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
			},
			req: &apiv2.ProjectMemberServiceLeaveRequest{
				Project: p0,
			},
			want: &apiv2.ProjectMemberServiceLeaveResponse{
				Member: &apiv2.ProjectMember{
					Member:  "will.smith@github",
					Role:    apiv2.ProjectRole_PROJECT_ROLE_EDITOR,
					Project: p0,
					Meta:    &apiv2.Meta{},
				},
			},
			wantErr: nil,
		},
		{
			name: "last owner cannot leave",
			sessionToken: &apiv2.Token{
				User:      "john.doe@github",
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
			},
			req: &apiv2.ProjectMemberServiceLeaveRequest{
				Project: p0,
			},
			want:    nil,
			wantErr: errorutil.FailedPrecondition("cannot remove last owner of a project"),
		},
		{
			name: "non-member leaves",
			sessionToken: &apiv2.Token{
				User:      "tina.turner@github",
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
			},
			req: &apiv2.ProjectMemberServiceLeaveRequest{
				Project: p0,
			},
			want:    nil,
			wantErr: errorutil.NotFound("member not found in project %q", p0),
		},
		{
			name:         "no token",
			sessionToken: nil,
			req: &apiv2.ProjectMemberServiceLeaveRequest{
				Project: p0,
			},
			want:    nil,
			wantErr: errorutil.Unauthenticated("no token found in request"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &projectMemberServiceServer{
				log:  log,
				repo: testStore.Store,
			}

			ctx := token.ContextWithToken(t.Context(), tt.sessionToken)

			if tt.wantErr == nil {
				test.Validate(t, tt.req)
			}

			got, err := p.Leave(ctx, tt.req)

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
