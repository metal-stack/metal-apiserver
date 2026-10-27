package tenantmember

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

const (
	p0 = "00000000-0000-0000-0000-000000000000"
)

func Test_tenantMemberServiceServer_Get(t *testing.T) {
	t.Parallel()

	testStore, closer := test.StartRepositoryWithCleanup(t, test.WithPostgres(true))
	log := testStore.GetLogger()
	defer closer()

	test.CreateTenants(t, testStore, []*apiv2.TenantServiceCreateRequest{
		{Name: "john.doe@github"},
		{Name: "will.smith@github"},
		{Name: "tina.turner@github"}, // tenant guest member
	})
	test.CreateTenantMemberships(t, testStore, "john.doe@github", []*apiv2.TenantMemberServiceCreateRequest{
		{Member: "john.doe@github", Role: apiv2.TenantRole_TENANT_ROLE_OWNER},
		{Member: "will.smith@github", Role: apiv2.TenantRole_TENANT_ROLE_EDITOR},
	})
	test.CreateProjects(t, testStore, []*apiv2.ProjectServiceCreateRequest{
		{Login: "john.doe@github", Name: "a"},
	})
	test.CreateProjectMemberships(t, testStore, "a", []*apiv2.ProjectMemberServiceCreateRequest{
		{Member: "john.doe@github", Role: apiv2.ProjectRole_PROJECT_ROLE_OWNER},
		{Member: "will.smith@github", Role: apiv2.ProjectRole_PROJECT_ROLE_VIEWER},
		{Member: "tina.turner@github", Role: apiv2.ProjectRole_PROJECT_ROLE_EDITOR},
	})

	tests := []struct {
		name         string
		sessionToken *apiv2.Token
		req          *apiv2.TenantMemberServiceGetRequest
		want         *apiv2.TenantMemberServiceGetResponse
		wantErr      error
	}{
		{
			name: "get own member",
			sessionToken: &apiv2.Token{
				User:      "john.doe@github",
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
				TenantRoles: map[string]apiv2.TenantRole{
					"john.doe@github": apiv2.TenantRole_TENANT_ROLE_OWNER,
				},
			},
			req: &apiv2.TenantMemberServiceGetRequest{
				Login:  "john.doe@github",
				Member: "john.doe@github",
			},
			want: &apiv2.TenantMemberServiceGetResponse{
				Member: &apiv2.TenantMember{
					Member:   "john.doe@github",
					Role:     apiv2.TenantRole_TENANT_ROLE_OWNER,
					Tenant:   "john.doe@github",
					Meta:     &apiv2.Meta{},
					Projects: []string{"a"},
				},
			},
			wantErr: nil,
		},
		{
			name: "get other member",
			sessionToken: &apiv2.Token{
				User:      "john.doe@github",
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
				TenantRoles: map[string]apiv2.TenantRole{
					"john.doe@github": apiv2.TenantRole_TENANT_ROLE_OWNER,
				},
			},
			req: &apiv2.TenantMemberServiceGetRequest{
				Login:  "john.doe@github",
				Member: "will.smith@github",
			},
			want: &apiv2.TenantMemberServiceGetResponse{
				Member: &apiv2.TenantMember{
					Member:   "will.smith@github",
					Role:     apiv2.TenantRole_TENANT_ROLE_EDITOR,
					Tenant:   "john.doe@github",
					Meta:     &apiv2.Meta{},
					Projects: []string{"a"},
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
			},
			req: &apiv2.TenantMemberServiceGetRequest{
				Login:  "john.doe@github",
				Member: "foo",
			},
			want:    nil,
			wantErr: errorutil.NotFound(`member not found in tenant "john.doe@github"`),
		},
		{
			name: "guest member does not see participating projects",
			sessionToken: &apiv2.Token{
				User:      "tina.turner@github",
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
				TenantRoles: map[string]apiv2.TenantRole{
					"john.doe@github":    apiv2.TenantRole_TENANT_ROLE_GUEST,
					"tina.turner@github": apiv2.TenantRole_TENANT_ROLE_OWNER,
				},
			},
			req: &apiv2.TenantMemberServiceGetRequest{
				Login:  "john.doe@github",
				Member: "will.smith@github",
			},
			want: &apiv2.TenantMemberServiceGetResponse{
				Member: &apiv2.TenantMember{
					Member:   "will.smith@github",
					Role:     apiv2.TenantRole_TENANT_ROLE_EDITOR,
					Tenant:   "john.doe@github",
					Meta:     &apiv2.Meta{},
					Projects: nil,
				},
			},
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &tenantMemberServiceServer{
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

func Test_tenantMemberServiceServer_Create(t *testing.T) {
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
		{Member: "will.smith@github", Role: apiv2.TenantRole_TENANT_ROLE_OWNER},
	})

	tests := []struct {
		name    string
		req     *apiv2.TenantMemberServiceCreateRequest
		want    *apiv2.TenantMemberServiceCreateResponse
		wantErr error
	}{
		{
			name: "create a member",
			req: &apiv2.TenantMemberServiceCreateRequest{
				Login:  "will.smith@github",
				Member: "tina.turner@github",
				Role:   apiv2.TenantRole_TENANT_ROLE_EDITOR,
				Labels: &apiv2.Labels{
					Labels: map[string]string{
						"a": "b",
					},
				},
			},
			want: &apiv2.TenantMemberServiceCreateResponse{
				Member: &apiv2.TenantMember{
					Member: "tina.turner@github",
					Role:   apiv2.TenantRole_TENANT_ROLE_EDITOR,
					Tenant: "will.smith@github",
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
			name: "create a member in a non-existing tenant",
			req: &apiv2.TenantMemberServiceCreateRequest{
				Login:  "unknown.tenant@github",
				Member: "tina.turner@github",
				Role:   apiv2.TenantRole_TENANT_ROLE_EDITOR,
			},
			want:    nil,
			wantErr: errorutil.NotFound("unable to find tenant:unknown.tenant@github for tenantMember"),
		},
		{
			name: "create a non-existing member",
			req: &apiv2.TenantMemberServiceCreateRequest{
				Login:  "john.doe@github",
				Member: "foo",
				Role:   apiv2.TenantRole_TENANT_ROLE_VIEWER,
			},
			want:    nil,
			wantErr: errorutil.NotFound("unable to find member:foo for tenantMember"),
		},
		{
			name: "create a member with unspecified role",
			req: &apiv2.TenantMemberServiceCreateRequest{
				Login:  "john.doe@github",
				Member: "tina.turner@github",
				Role:   apiv2.TenantRole_TENANT_ROLE_UNSPECIFIED,
			},
			want:    nil,
			wantErr: errorutil.InvalidArgument("role must not be TENANT_ROLE_UNSPECIFIED"),
		},
		{
			name: "create a membership twice",
			req: &apiv2.TenantMemberServiceCreateRequest{
				Login:  "john.doe@github",
				Member: "john.doe@github",
				Role:   apiv2.TenantRole_TENANT_ROLE_OWNER,
			},
			want:    nil,
			wantErr: errorutil.Conflict(`"john.doe@github" is already member of tenant "john.doe@github"`),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &tenantMemberServiceServer{
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

func Test_tenantMemberServiceServer_Update(t *testing.T) {
	t.Parallel()

	testStore, closer := test.StartRepositoryWithCleanup(t, test.WithPostgres(true))
	log := testStore.GetLogger()
	defer closer()

	test.CreateTenants(t, testStore, []*apiv2.TenantServiceCreateRequest{
		{Name: "john.doe@github"},
		{Name: "tina.turner@github"},
		{Name: "a"},
	})
	test.CreateTenantMemberships(t, testStore, "john.doe@github", []*apiv2.TenantMemberServiceCreateRequest{
		{Member: "john.doe@github", Role: apiv2.TenantRole_TENANT_ROLE_OWNER},
		{Member: "tina.turner@github", Role: apiv2.TenantRole_TENANT_ROLE_VIEWER},
	})
	test.CreateTenantMemberships(t, testStore, "a", []*apiv2.TenantMemberServiceCreateRequest{
		{Member: "tina.turner@github", Role: apiv2.TenantRole_TENANT_ROLE_OWNER},
	})

	tests := []struct {
		name    string
		req     *apiv2.TenantMemberServiceUpdateRequest
		want    *apiv2.TenantMemberServiceUpdateResponse
		wantErr error
	}{
		{
			name: "update a member",
			req: &apiv2.TenantMemberServiceUpdateRequest{
				Login:      "john.doe@github",
				Member:     "tina.turner@github",
				Role:       new(apiv2.TenantRole_TENANT_ROLE_EDITOR),
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
			want: &apiv2.TenantMemberServiceUpdateResponse{
				Member: &apiv2.TenantMember{
					Member: "tina.turner@github",
					Role:   apiv2.TenantRole_TENANT_ROLE_EDITOR,
					Tenant: "john.doe@github",
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
		},
		{
			name: "cannot demote a user within their own default tenant",
			req: &apiv2.TenantMemberServiceUpdateRequest{
				Login:      "john.doe@github",
				Member:     "john.doe@github",
				Role:       new(apiv2.TenantRole_TENANT_ROLE_EDITOR),
				UpdateMeta: &apiv2.UpdateMeta{},
			},
			want:    nil,
			wantErr: errorutil.FailedPrecondition("cannot demote a user's role within their own default tenant"),
		},
		{
			name: "demote last owner",
			req: &apiv2.TenantMemberServiceUpdateRequest{
				Login:      "a",
				Member:     "tina.turner@github",
				Role:       new(apiv2.TenantRole_TENANT_ROLE_EDITOR),
				UpdateMeta: &apiv2.UpdateMeta{},
			},
			want:    nil,
			wantErr: errorutil.FailedPrecondition("cannot demote last owner's permissions"),
		},
		{
			name: "update non-existing member",
			req: &apiv2.TenantMemberServiceUpdateRequest{
				Login:      "john.doe@github",
				Member:     "foo",
				Role:       new(apiv2.TenantRole_TENANT_ROLE_EDITOR),
				UpdateMeta: &apiv2.UpdateMeta{},
			},
			want:    nil,
			wantErr: errorutil.NotFound(`member not found in tenant "john.doe@github"`),
		},
		{
			name: "update member from another tenant",
			req: &apiv2.TenantMemberServiceUpdateRequest{
				Login:      "a",
				Member:     "john.doe@github",
				Role:       new(apiv2.TenantRole_TENANT_ROLE_EDITOR),
				UpdateMeta: &apiv2.UpdateMeta{},
			},
			want:    nil,
			wantErr: errorutil.NotFound(`member not found in tenant "a"`),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &tenantMemberServiceServer{
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

func Test_tenantMemberServiceServer_Delete(t *testing.T) {
	t.Parallel()

	testStore, closer := test.StartRepositoryWithCleanup(t, test.WithPostgres(true))
	log := testStore.GetLogger()
	defer closer()

	test.CreateTenants(t, testStore, []*apiv2.TenantServiceCreateRequest{
		{Name: "john.doe@github"},
		{Name: "tina.turner@github"},
		{Name: "a"},
	})
	test.CreateTenantMemberships(t, testStore, "john.doe@github", []*apiv2.TenantMemberServiceCreateRequest{
		{Member: "john.doe@github", Role: apiv2.TenantRole_TENANT_ROLE_OWNER},
		{Member: "tina.turner@github", Role: apiv2.TenantRole_TENANT_ROLE_VIEWER},
	})
	test.CreateTenantMemberships(t, testStore, "a", []*apiv2.TenantMemberServiceCreateRequest{
		{Member: "tina.turner@github", Role: apiv2.TenantRole_TENANT_ROLE_OWNER},
	})

	tests := []struct {
		name    string
		req     *apiv2.TenantMemberServiceDeleteRequest
		want    *apiv2.TenantMemberServiceDeleteResponse
		wantErr error
	}{
		{
			name: "delete a member",
			req: &apiv2.TenantMemberServiceDeleteRequest{
				Login:  "john.doe@github",
				Member: "tina.turner@github",
			},
			want: &apiv2.TenantMemberServiceDeleteResponse{
				Member: &apiv2.TenantMember{
					Member: "tina.turner@github",
					Role:   apiv2.TenantRole_TENANT_ROLE_VIEWER,
					Tenant: "john.doe@github",
					Meta:   &apiv2.Meta{},
				},
			},
		},
		{
			name: "cannot delete a user from their own default tenant",
			req: &apiv2.TenantMemberServiceDeleteRequest{
				Login:  "john.doe@github",
				Member: "john.doe@github",
			},
			want:    nil,
			wantErr: errorutil.FailedPrecondition("cannot remove a member from their own default tenant"),
		},
		{
			name: "delete last owner",
			req: &apiv2.TenantMemberServiceDeleteRequest{
				Login:  "a",
				Member: "tina.turner@github",
			},
			want:    nil,
			wantErr: errorutil.FailedPrecondition("cannot remove last owner of a tenant"),
		},
		{
			name: "delete non-existing member",
			req: &apiv2.TenantMemberServiceDeleteRequest{
				Login:  "john.doe@github",
				Member: "foo",
			},
			want:    nil,
			wantErr: errorutil.NotFound(`member not found in tenant "john.doe@github"`),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &tenantMemberServiceServer{
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

func Test_tenantMemberServiceServer_List(t *testing.T) {
	t.Parallel()

	testStore, closer := test.StartRepositoryWithCleanup(t, test.WithPostgres(true))
	log := testStore.GetLogger()
	defer closer()

	test.CreateTenants(t, testStore, []*apiv2.TenantServiceCreateRequest{
		{Name: "john.doe@github"},
		{Name: "will.smith@github"},
		{Name: "tina.turner@github"},
		{Name: "ed.wood@github"},
	})
	test.CreateTenantMemberships(t, testStore, "john.doe@github", []*apiv2.TenantMemberServiceCreateRequest{
		{Member: "john.doe@github", Role: apiv2.TenantRole_TENANT_ROLE_OWNER},
		{Member: "will.smith@github", Role: apiv2.TenantRole_TENANT_ROLE_OWNER},
		{Member: "tina.turner@github", Role: apiv2.TenantRole_TENANT_ROLE_VIEWER, Labels: &apiv2.Labels{Labels: map[string]string{"a": "b"}}},
	})

	test.CreateProjects(t, testStore, []*apiv2.ProjectServiceCreateRequest{
		{Login: "john.doe@github", Name: p0},
	})
	test.CreateProjectMemberships(t, testStore, p0, []*apiv2.ProjectMemberServiceCreateRequest{
		{Project: p0, Member: "ed.wood@github", Role: apiv2.ProjectRole_PROJECT_ROLE_EDITOR, Labels: &apiv2.Labels{Labels: map[string]string{"c": "d"}}},
	})

	tests := []struct {
		name    string
		req     *apiv2.TenantMemberServiceListRequest
		want    *apiv2.TenantMemberServiceListResponse
		wantErr error
	}{
		{
			name: "list members",
			req: &apiv2.TenantMemberServiceListRequest{
				Login: "john.doe@github",
			},
			want: &apiv2.TenantMemberServiceListResponse{
				Members: []*apiv2.TenantMember{
					{
						Member: "ed.wood@github",
						Role:   apiv2.TenantRole_TENANT_ROLE_GUEST,
						Tenant: "john.doe@github",
						Meta:   &apiv2.Meta{},
					},
					{
						Member: "john.doe@github",
						Role:   apiv2.TenantRole_TENANT_ROLE_OWNER,
						Tenant: "john.doe@github",
						Meta:   &apiv2.Meta{},
					},
					{
						Member: "tina.turner@github",
						Role:   apiv2.TenantRole_TENANT_ROLE_VIEWER,
						Tenant: "john.doe@github",
						Meta: &apiv2.Meta{
							Labels: &apiv2.Labels{
								Labels: map[string]string{
									"a": "b",
								},
							},
						},
					},
					{
						Member: "will.smith@github",
						Role:   apiv2.TenantRole_TENANT_ROLE_OWNER,
						Tenant: "john.doe@github",
						Meta:   &apiv2.Meta{},
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "list members filtered by owner role",
			req: &apiv2.TenantMemberServiceListRequest{
				Login: "john.doe@github",
				Query: &apiv2.TenantMemberQuery{
					Role: new(apiv2.TenantRole_TENANT_ROLE_OWNER),
				},
			},
			want: &apiv2.TenantMemberServiceListResponse{
				Members: []*apiv2.TenantMember{
					{
						Member: "john.doe@github",
						Role:   apiv2.TenantRole_TENANT_ROLE_OWNER,
						Tenant: "john.doe@github",
						Meta:   &apiv2.Meta{},
					},
					{
						Member: "will.smith@github",
						Role:   apiv2.TenantRole_TENANT_ROLE_OWNER,
						Tenant: "john.doe@github",
						Meta:   &apiv2.Meta{},
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "list members filtered by guest role",
			req: &apiv2.TenantMemberServiceListRequest{
				Login: "john.doe@github",
				Query: &apiv2.TenantMemberQuery{
					Role: new(apiv2.TenantRole_TENANT_ROLE_GUEST),
				},
			},
			want: &apiv2.TenantMemberServiceListResponse{
				Members: []*apiv2.TenantMember{
					{
						Member: "ed.wood@github",
						Role:   apiv2.TenantRole_TENANT_ROLE_GUEST,
						Tenant: "john.doe@github",
						Meta:   &apiv2.Meta{},
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "list members filtered by member",
			req: &apiv2.TenantMemberServiceListRequest{
				Login: "john.doe@github",
				Query: &apiv2.TenantMemberQuery{
					Member: new("tina.turner@github"),
				},
			},
			want: &apiv2.TenantMemberServiceListResponse{
				Members: []*apiv2.TenantMember{
					{
						Member: "tina.turner@github",
						Role:   apiv2.TenantRole_TENANT_ROLE_VIEWER,
						Tenant: "john.doe@github",
						Meta: &apiv2.Meta{
							Labels: &apiv2.Labels{
								Labels: map[string]string{
									"a": "b",
								},
							},
						},
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "list members filtered by label",
			req: &apiv2.TenantMemberServiceListRequest{
				Login: "john.doe@github",
				Query: &apiv2.TenantMemberQuery{
					Labels: &apiv2.Labels{
						Labels: map[string]string{
							"a": "b",
						},
					},
				},
			},
			want: &apiv2.TenantMemberServiceListResponse{
				Members: []*apiv2.TenantMember{
					{
						Member: "tina.turner@github",
						Role:   apiv2.TenantRole_TENANT_ROLE_VIEWER,
						Tenant: "john.doe@github",
						Meta: &apiv2.Meta{
							Labels: &apiv2.Labels{
								Labels: map[string]string{
									"a": "b",
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
			p := &tenantMemberServiceServer{
				log:  log,
				repo: testStore.Store,
			}

			if tt.wantErr == nil {
				test.Validate(t, tt.req)
			}

			got, err := p.List(t.Context(), tt.req)

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

func Test_tenantMemberServiceServer_Leave(t *testing.T) {
	t.Parallel()

	testStore, closer := test.StartRepositoryWithCleanup(t, test.WithPostgres(true))
	log := testStore.GetLogger()
	defer closer()

	test.CreateTenants(t, testStore, []*apiv2.TenantServiceCreateRequest{
		{Name: "john.doe@github"},
		{Name: "tina.turner@github"},
		{Name: "a"},
	})
	test.CreateTenantMemberships(t, testStore, "john.doe@github", []*apiv2.TenantMemberServiceCreateRequest{
		{Member: "john.doe@github", Role: apiv2.TenantRole_TENANT_ROLE_OWNER},
		{Member: "tina.turner@github", Role: apiv2.TenantRole_TENANT_ROLE_VIEWER},
	})
	test.CreateTenantMemberships(t, testStore, "a", []*apiv2.TenantMemberServiceCreateRequest{
		{Member: "tina.turner@github", Role: apiv2.TenantRole_TENANT_ROLE_OWNER},
	})

	tests := []struct {
		name         string
		sessionToken *apiv2.Token
		req          *apiv2.TenantMemberServiceLeaveRequest
		want         *apiv2.TenantMemberServiceLeaveResponse
		wantErr      error
	}{
		{
			name: "leave a tenant",
			sessionToken: &apiv2.Token{
				User:      "tina.turner@github",
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
				TenantRoles: map[string]apiv2.TenantRole{
					"tina.turner@github": apiv2.TenantRole_TENANT_ROLE_OWNER,
					"john.doe@github":    apiv2.TenantRole_TENANT_ROLE_VIEWER,
					"a":                  apiv2.TenantRole_TENANT_ROLE_OWNER,
				},
			},
			req: &apiv2.TenantMemberServiceLeaveRequest{
				Login: "john.doe@github",
			},
			want: &apiv2.TenantMemberServiceLeaveResponse{
				Member: &apiv2.TenantMember{
					Member: "tina.turner@github",
					Role:   apiv2.TenantRole_TENANT_ROLE_VIEWER,
					Tenant: "john.doe@github",
					Meta:   &apiv2.Meta{},
				},
			},
			wantErr: nil,
		},
		{
			name: "cannot leave own default tenant",
			sessionToken: &apiv2.Token{
				User:      "john.doe@github",
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
				TenantRoles: map[string]apiv2.TenantRole{
					"john.doe@github": apiv2.TenantRole_TENANT_ROLE_OWNER,
				},
			},
			req: &apiv2.TenantMemberServiceLeaveRequest{
				Login: "john.doe@github",
			},
			want:    nil,
			wantErr: errorutil.FailedPrecondition("cannot remove a member from their own default tenant"),
		},
		{
			// BUG: the last owner protection never triggers (role read from the wrong meta),
			// the request fails later in the backend instead.
			name: "last owner cannot leave",
			sessionToken: &apiv2.Token{
				User:      "tina.turner@github",
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
				TenantRoles: map[string]apiv2.TenantRole{
					"tina.turner@github": apiv2.TenantRole_TENANT_ROLE_OWNER,
					"john.doe@github":    apiv2.TenantRole_TENANT_ROLE_VIEWER,
					"a":                  apiv2.TenantRole_TENANT_ROLE_OWNER,
				},
			},
			req: &apiv2.TenantMemberServiceLeaveRequest{
				Login: "a",
			},
			want:    nil,
			wantErr: errorutil.FailedPrecondition("cannot remove last owner of a tenant"),
		},
		{
			name: "non-member leaves",
			sessionToken: &apiv2.Token{
				User:      "john.doe@github",
				TokenType: apiv2.TokenType_TOKEN_TYPE_API,
				TenantRoles: map[string]apiv2.TenantRole{
					"john.doe@github": apiv2.TenantRole_TENANT_ROLE_OWNER,
				},
			},
			req: &apiv2.TenantMemberServiceLeaveRequest{
				Login: "a",
			},
			want:    nil,
			wantErr: errorutil.NotFound(`member not found in tenant "a"`),
		},
		{
			name:         "no token",
			sessionToken: nil,
			req: &apiv2.TenantMemberServiceLeaveRequest{
				Login: "john.doe@github",
			},
			want:    nil,
			wantErr: errorutil.Unauthenticated("no token found in request"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &tenantMemberServiceServer{
				log:  log,
				repo: testStore.Store,
			}

			if tt.wantErr == nil {
				test.Validate(t, tt.req)
			}

			ctx := token.ContextWithToken(t.Context(), tt.sessionToken)

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
