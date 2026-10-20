package project

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/metal-stack/api/go/errorutil"
	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-apiserver/pkg/invite"
	"github.com/metal-stack/metal-apiserver/pkg/test"
	"github.com/metal-stack/metal-apiserver/pkg/token"
	"github.com/metal-stack/metal-lib/pkg/pointer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// FIXME add proto validation before each service call
var (
	p0  = "00000000-0000-0000-0000-000000000000"
	p1  = "00000000-0000-0000-0000-000000000001"
	p99 = "00000000-0000-0000-0000-000000000099"
)

func Test_projectServiceServer_Get(t *testing.T) {
	t.Parallel()

	testStore, closer := test.StartRepositoryWithCleanup(t, test.WithPostgres(true))
	log := testStore.GetLogger()
	defer closer()

	test.CreateTenants(t, testStore, []*apiv2.TenantServiceCreateRequest{
		{Name: "john.doe@github"},
		{Name: "will.smith@github"},  // direct project membership
		{Name: "tina.turner@github"}, // inherited project membership
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
	test.CreateTenantMemberships(t, testStore, "john.doe@github", []*apiv2.TenantMemberServiceCreateRequest{
		{Member: "john.doe@github", Role: apiv2.TenantRole_TENANT_ROLE_OWNER},
		{Member: "tina.turner@github", Role: apiv2.TenantRole_TENANT_ROLE_OWNER},
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
				Project: p0,
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
					Uuid:        p0,
					Tenant:      "john.doe@github",
					Name:        p0,
					Description: "a description",
					AvatarUrl:   new("http://test"),
				},
			},
			wantErr: nil,
		},
		{
			name: "get a project as guest member",
			rq: &apiv2.ProjectServiceGetRequest{
				Project: p0,
			},
			as: new(apiv2.TenantRole_TENANT_ROLE_GUEST),
			want: &apiv2.ProjectServiceGetResponse{
				Project: &apiv2.Project{
					Meta: &apiv2.Meta{
						Labels: &apiv2.Labels{
							Labels: map[string]string{
								"a": "b",
							},
						},
					},
					Name:        p0,
					Description: "a description",
					AvatarUrl:   new("http://test"),
					Uuid:        p0,
					Tenant:      "john.doe@github",
				},
			},
			wantErr: nil,
		},
		{
			name: "get a project that does not exist",
			rq: &apiv2.ProjectServiceGetRequest{
				Project: p99,
			},
			want:    nil,
			wantErr: errorutil.NotFound("project with id:%s not found sql: no rows in result set", p99),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := &projectServiceServer{
				log:         log,
				repo:        testStore.Store,
				inviteStore: testStore.GetProjectInviteStore(),
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
					p0: apiv2.ProjectRole_PROJECT_ROLE_OWNER,
				},
			})

			reqCtx := token.ContextWithToken(t.Context(), tok)
			if tt.wantErr == nil {
				// Execute proto based validation
				test.Validate(t, tt.rq)
			}
			got, err := u.Get(reqCtx, tt.rq)
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
		{Name: "b950f4f5-d8b8-4252-aa02-ae08a1d2b044", Login: "john.doe@github", Labels: &apiv2.Labels{Labels: map[string]string{"a": "b", "c": "d"}}},
	})

	test.CreateProjectMemberships(t, testStore, p0, []*apiv2.ProjectMemberServiceCreateRequest{
		{Member: "john.doe@github", Role: apiv2.ProjectRole_PROJECT_ROLE_OWNER},
	})
	test.CreateProjectMemberships(t, testStore, "will.smith@github", []*apiv2.ProjectMemberServiceCreateRequest{
		{Member: "will.smith@github", Role: apiv2.ProjectRole_PROJECT_ROLE_OWNER},
	})
	test.CreateProjectMemberships(t, testStore, "b950f4f5-d8b8-4252-aa02-ae08a1d2b044", []*apiv2.ProjectMemberServiceCreateRequest{
		{Member: "john.doe@github", Role: apiv2.ProjectRole_PROJECT_ROLE_EDITOR},
	})

	tests := []struct {
		name    string
		rq      *apiv2.ProjectServiceListRequest
		want    *apiv2.ProjectServiceListResponse
		wantErr error
	}{
		{
			name: "list the projects",
			rq:   &apiv2.ProjectServiceListRequest{},
			want: &apiv2.ProjectServiceListResponse{
				Projects: []*apiv2.Project{
					{
						Meta:   &apiv2.Meta{},
						Name:   p0,
						Uuid:   p0,
						Tenant: "john.doe@github",
					},
					{
						Meta: &apiv2.Meta{Labels: &apiv2.Labels{
							Labels: map[string]string{"a": "b", "c": "d"},
						}},
						Name:   "b950f4f5-d8b8-4252-aa02-ae08a1d2b044",
						Uuid:   "b950f4f5-d8b8-4252-aa02-ae08a1d2b044",
						Tenant: "john.doe@github",
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "list the projects filtered by id",
			rq: &apiv2.ProjectServiceListRequest{
				Query: &apiv2.ProjectQuery{
					Uuid: new(p0),
				},
			},
			want: &apiv2.ProjectServiceListResponse{
				Projects: []*apiv2.Project{
					{
						Meta:   &apiv2.Meta{},
						Name:   p0,
						Uuid:   p0,
						Tenant: "john.doe@github",
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "list the projects filtered by name",
			rq: &apiv2.ProjectServiceListRequest{
				Query: &apiv2.ProjectQuery{
					Name: new("b950f4f5-d8b8-4252-aa02-ae08a1d2b044"),
				},
			},
			want: &apiv2.ProjectServiceListResponse{
				Projects: []*apiv2.Project{
					{
						Meta: &apiv2.Meta{Labels: &apiv2.Labels{
							Labels: map[string]string{"a": "b", "c": "d"},
						}},
						Name:   "b950f4f5-d8b8-4252-aa02-ae08a1d2b044",
						Uuid:   "b950f4f5-d8b8-4252-aa02-ae08a1d2b044",
						Tenant: "john.doe@github",
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "list the projects filtered by one label",
			rq: &apiv2.ProjectServiceListRequest{
				Query: &apiv2.ProjectQuery{
					Labels: &apiv2.Labels{
						Labels: map[string]string{"a": "b"},
					},
				},
			},
			want: &apiv2.ProjectServiceListResponse{
				Projects: []*apiv2.Project{
					{
						Meta: &apiv2.Meta{
							Labels: &apiv2.Labels{
								Labels: map[string]string{"a": "b", "c": "d"},
							},
						},
						Name:   "b950f4f5-d8b8-4252-aa02-ae08a1d2b044",
						Tenant: "john.doe@github",
						Uuid:   "b950f4f5-d8b8-4252-aa02-ae08a1d2b044",
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "list the projects filtered by one label with wrong value",
			rq: &apiv2.ProjectServiceListRequest{
				Query: &apiv2.ProjectQuery{
					Labels: &apiv2.Labels{
						Labels: map[string]string{"a": "c"},
					},
				},
			},
			want:    &apiv2.ProjectServiceListResponse{},
			wantErr: nil,
		},
		{
			name: "list the projects filtered by partially matching label",
			rq: &apiv2.ProjectServiceListRequest{
				Query: &apiv2.ProjectQuery{
					Labels: &apiv2.Labels{
						Labels: map[string]string{"a": "b", "e": "f"},
					},
				},
			},
			want:    &apiv2.ProjectServiceListResponse{},
			wantErr: nil,
		},
		{
			name: "list the projects filtered by tenant 1",
			rq: &apiv2.ProjectServiceListRequest{
				Query: &apiv2.ProjectQuery{
					Tenant: new("john.doe@github"),
				},
			},
			want: &apiv2.ProjectServiceListResponse{
				Projects: []*apiv2.Project{
					{
						Meta:   &apiv2.Meta{},
						Name:   p0,
						Uuid:   p0,
						Tenant: "john.doe@github",
					},
					{
						Meta: &apiv2.Meta{Labels: &apiv2.Labels{
							Labels: map[string]string{"a": "b", "c": "d"},
						}},
						Name:   "b950f4f5-d8b8-4252-aa02-ae08a1d2b044",
						Uuid:   "b950f4f5-d8b8-4252-aa02-ae08a1d2b044",
						Tenant: "john.doe@github",
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "list the projects filtered by tenant 2",
			rq: &apiv2.ProjectServiceListRequest{
				Query: &apiv2.ProjectQuery{
					Tenant: new(p99),
				},
			},
			want:    &apiv2.ProjectServiceListResponse{},
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := &projectServiceServer{
				log:         log,
				repo:        testStore.Store,
				inviteStore: testStore.GetProjectInviteStore(),
			}

			tok := testStore.GetToken("john.doe@github", &apiv2.TokenServiceCreateRequest{
				Expires: durationpb.New(time.Hour),
				TenantRoles: map[string]apiv2.TenantRole{
					"john.doe@github": apiv2.TenantRole_TENANT_ROLE_OWNER,
				},
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
		rq      *apiv2.ProjectServiceCreateRequest
		want    *apiv2.ProjectServiceCreateResponse
		wantErr error
	}{
		{
			name: "create a project",
			rq: &apiv2.ProjectServiceCreateRequest{
				Name:        "My New Org Project",
				Description: "project desc",
				AvatarUrl:   new("http://test"),
				Labels: &apiv2.Labels{
					Labels: map[string]string{
						"a": "b",
					},
				},
				Login: "john.doe@github",
			},
			want: &apiv2.ProjectServiceCreateResponse{
				Project: &apiv2.Project{
					Meta: &apiv2.Meta{
						Labels: &apiv2.Labels{
							Labels: map[string]string{
								"a": "b",
							},
						},
					},
					Name:        "My New Org Project",
					Description: "project desc",
					Tenant:      "john.doe@github",
					AvatarUrl:   new("http://test"),
				},
			},
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := &projectServiceServer{
				log:         log,
				repo:        testStore.Store,
				inviteStore: testStore.GetProjectInviteStore(),
			}

			tok := testStore.GetToken("john.doe@github", &apiv2.TokenServiceCreateRequest{
				Expires: durationpb.New(time.Hour),
				TenantRoles: map[string]apiv2.TenantRole{
					"john.doe@github": apiv2.TenantRole_TENANT_ROLE_OWNER,
				},
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

			assert.NotEmpty(t, got.Project.Uuid)

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

		})
	}
}

func Test_projectServiceServer_Update(t *testing.T) {
	t.Parallel()

	testStore, closer := test.StartRepositoryWithCleanup(t, test.WithPostgres(true))
	log := testStore.GetLogger()
	defer closer()

	test.CreateTenants(t, testStore, []*apiv2.TenantServiceCreateRequest{
		{Name: "john.doe@github"},
	})

	test.CreateProjects(t, testStore, []*apiv2.ProjectServiceCreateRequest{{
		Name:        p0,
		Description: "old desc",
		AvatarUrl:   new("http://old"),
		Labels: &apiv2.Labels{
			Labels: map[string]string{
				"a": "b",
			},
		},
		Login: "john.doe@github",
	}})

	tests := []struct {
		name    string
		rq      *apiv2.ProjectServiceUpdateRequest
		want    *apiv2.ProjectServiceUpdateResponse
		wantErr error
	}{
		{
			name: "update a project",
			rq: &apiv2.ProjectServiceUpdateRequest{
				// equals the ID, UUID
				Project:     p0,
				Name:        new("new name"),
				UpdateMeta:  &apiv2.UpdateMeta{},
				Description: new("new desc"),
				AvatarUrl:   new("http://new"),
				Labels: &apiv2.UpdateLabels{
					Strategy: &apiv2.UpdateLabels_Patch{
						Patch: &apiv2.LabelsPatch{
							Update: &apiv2.Labels{
								Labels: map[string]string{
									"c": "d",
								},
							},
						},
					},
				},
			},
			want: &apiv2.ProjectServiceUpdateResponse{
				Project: &apiv2.Project{
					Uuid: p0,
					Meta: &apiv2.Meta{
						Labels: &apiv2.Labels{
							Labels: map[string]string{
								"a": "b",
								"c": "d",
							},
						},
					},
					Name:        "new name",
					Description: "new desc",
					AvatarUrl:   new("http://new"),
					Tenant:      "john.doe@github",
				},
			},
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := &projectServiceServer{
				log:         log,
				repo:        testStore.Store,
				inviteStore: testStore.GetProjectInviteStore(),
			}

			tok := testStore.GetToken("john.doe@github", &apiv2.TokenServiceCreateRequest{
				Expires: durationpb.New(time.Hour),
				TenantRoles: map[string]apiv2.TenantRole{
					"john.doe@github": apiv2.TenantRole_TENANT_ROLE_OWNER,
				},
			})

			reqCtx := token.ContextWithToken(t.Context(), tok)
			if tt.wantErr == nil {
				// Execute proto based validation
				test.Validate(t, tt.rq)
			}
			got, err := u.Update(reqCtx, tt.rq)
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

func Test_projectServiceServer_Delete(t *testing.T) {
	t.Parallel()

	testStore, closer := test.StartRepositoryWithCleanup(t, test.WithPostgres(true))
	log := testStore.GetLogger()
	defer closer()

	test.CreateTenants(t, testStore, []*apiv2.TenantServiceCreateRequest{
		{Name: "john.doe@github"},
	})

	tests := []struct {
		name             string
		rq               *apiv2.ProjectServiceDeleteRequest
		want             *apiv2.ProjectServiceDeleteResponse
		existingProjects []*apiv2.ProjectServiceCreateRequest
		existingMachines []*metal.Machine
		existingNetworks []*adminv2.NetworkServiceCreateRequest
		existingIPs      []*apiv2.IPServiceCreateRequest
		wantErr          error
	}{
		{
			name: "delete a project",
			rq: &apiv2.ProjectServiceDeleteRequest{
				Project: p0,
			},
			existingProjects: []*apiv2.ProjectServiceCreateRequest{
				{Name: p0, Login: "john.doe@github"},
			},
			want: &apiv2.ProjectServiceDeleteResponse{
				Project: &apiv2.Project{
					Meta:   &apiv2.Meta{},
					Name:   p0,
					Uuid:   p0,
					Tenant: "john.doe@github",
				},
			},
			wantErr: nil,
		},
		{
			name: "delete non-existing project",
			rq: &apiv2.ProjectServiceDeleteRequest{
				Project: p99,
			},
			existingProjects: []*apiv2.ProjectServiceCreateRequest{
				{Name: "john.doe@github", Login: "john.doe@github"},
			},
			wantErr: errorutil.NotFound("project with id:%s not found sql: no rows in result set", p99),
		},
		{
			name: "cannot delete project when machines are still present",
			rq: &apiv2.ProjectServiceDeleteRequest{
				Project: p0,
			},
			existingProjects: []*apiv2.ProjectServiceCreateRequest{
				{Name: p0, Login: "john.doe@github"},
			},
			existingMachines: []*metal.Machine{
				{
					Allocation: &metal.MachineAllocation{
						UUID:    "1",
						Project: p0,
					},
				},
			},
			wantErr: errorutil.FailedPrecondition("there are still machines associated with this project, you need to delete them first"),
		},
		{
			name: "cannot delete project when ips are still present",
			rq: &apiv2.ProjectServiceDeleteRequest{
				Project: p0,
			},
			existingProjects: []*apiv2.ProjectServiceCreateRequest{
				{Name: p0, Login: "john.doe@github"},
			},
			existingNetworks: []*adminv2.NetworkServiceCreateRequest{
				{Id: new("internet"), Prefixes: []string{"1.2.3.0/24"}, Type: apiv2.NetworkType_NETWORK_TYPE_EXTERNAL, Vrf: new(uint32(11))},
			},
			existingIPs: []*apiv2.IPServiceCreateRequest{
				{Name: new("ip1"), Ip: new("1.2.3.4"), Project: p0, Network: "internet"},
			},
			wantErr: errorutil.FailedPrecondition("there are still ips associated with this project, you need to delete them first"),
		},
		{
			name: "cannot delete project when networks are still present",
			rq: &apiv2.ProjectServiceDeleteRequest{
				Project: p0,
			},
			existingProjects: []*apiv2.ProjectServiceCreateRequest{
				{Name: p0, Login: "john.doe@github"},
			},
			existingNetworks: []*adminv2.NetworkServiceCreateRequest{
				{Id: new("project-internet"), Project: new(p0), Prefixes: []string{"1.2.4.0/24"}, Type: apiv2.NetworkType_NETWORK_TYPE_EXTERNAL, Vrf: new(uint32(12))},
			},
			wantErr: errorutil.FailedPrecondition("there are still networks associated with this project, you need to delete them first"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := &projectServiceServer{
				log:         log,
				repo:        testStore.Store,
				inviteStore: testStore.GetProjectInviteStore(),
			}

			test.CreateProjects(t, testStore, tt.existingProjects)
			test.CreateNetworks(t, testStore, tt.existingNetworks)
			test.CreateIPs(t, testStore, tt.existingIPs)
			test.CreateMachines(t, testStore, tt.existingMachines)
			defer func() {
				test.DeleteMachines(t, testStore)
				test.DeleteIPs(t, testStore)
				test.DeleteNetworks(t, testStore)
				testStore.DeleteProjects()
			}()

			tok := testStore.GetToken("john.doe@github", &apiv2.TokenServiceCreateRequest{
				Expires: durationpb.New(time.Hour),
				TenantRoles: map[string]apiv2.TenantRole{
					"john.doe@github": apiv2.TenantRole_TENANT_ROLE_OWNER,
				},
			})

			reqCtx := token.ContextWithToken(t.Context(), tok)
			if tt.wantErr == nil {
				// Execute proto based validation
				test.Validate(t, tt.rq)
			}
			got, err := u.Delete(reqCtx, tt.rq)
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

func Test_projectServiceServer_Invite(t *testing.T) {
	t.Parallel()

	testStore, closer := test.StartRepositoryWithCleanup(t, test.WithPostgres(true))
	log := testStore.GetLogger()
	defer closer()

	test.CreateTenants(t, testStore, []*apiv2.TenantServiceCreateRequest{
		{Name: "john.doe@github"},
	})
	test.CreateProjects(t, testStore, []*apiv2.ProjectServiceCreateRequest{
		{Name: p0, Login: "john.doe@github"},
		{Name: "b950f4f5-d8b8-4252-aa02-ae08a1d2b044", Login: "john.doe@github"},
	})

	tests := []struct {
		name    string
		rq      *apiv2.ProjectServiceInviteRequest
		want    *apiv2.ProjectServiceInviteResponse
		wantErr error
	}{
		{
			name: "create a project invite",
			rq: &apiv2.ProjectServiceInviteRequest{
				Project: p0,
				Role:    apiv2.ProjectRole_PROJECT_ROLE_EDITOR,
			},
			want: &apiv2.ProjectServiceInviteResponse{
				Invite: &apiv2.ProjectInvite{
					Project:     p0,
					Role:        apiv2.ProjectRole_PROJECT_ROLE_EDITOR,
					Joined:      false,
					ProjectName: p0,
					Tenant:      "john.doe@github",
					TenantName:  "john.doe@github",
				},
			},
			wantErr: nil,
		},
		{
			name: "create an invite for another project",
			rq: &apiv2.ProjectServiceInviteRequest{
				Project: "b950f4f5-d8b8-4252-aa02-ae08a1d2b044",
				Role:    apiv2.ProjectRole_PROJECT_ROLE_VIEWER,
			},
			want: &apiv2.ProjectServiceInviteResponse{
				Invite: &apiv2.ProjectInvite{
					Project:     "b950f4f5-d8b8-4252-aa02-ae08a1d2b044",
					Role:        apiv2.ProjectRole_PROJECT_ROLE_VIEWER,
					Joined:      false,
					ProjectName: "b950f4f5-d8b8-4252-aa02-ae08a1d2b044",
					Tenant:      "john.doe@github",
					TenantName:  "john.doe@github",
				},
			},
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := &projectServiceServer{
				log:         log,
				repo:        testStore.Store,
				inviteStore: testStore.GetProjectInviteStore(),
			}

			tok := testStore.GetToken("john.doe@github", &apiv2.TokenServiceCreateRequest{
				Expires: durationpb.New(time.Hour),
				TenantRoles: map[string]apiv2.TenantRole{
					"john.doe@github": apiv2.TenantRole_TENANT_ROLE_OWNER,
				},
			})

			reqCtx := token.ContextWithToken(t.Context(), tok)
			if tt.wantErr == nil {
				// Execute proto based validation
				test.Validate(t, tt.rq)
			}
			got, err := u.Invite(reqCtx, tt.rq)
			require.NoError(t, err)

			assert.Len(t, got.Invite.Secret, 32)
			assert.WithinDuration(t, time.Now().Add(7*24*time.Hour), got.Invite.ExpiresAt.AsTime(), 1*time.Minute)
			tt.want.Invite.Secret = got.Invite.GetSecret()
			tt.want.Invite.ExpiresAt = got.Invite.GetExpiresAt()

			if diff := cmp.Diff(
				tt.want,
				got,
				protocmp.Transform(),
			); diff != "" {
				t.Errorf("diff: %s", diff)
			}
		})
	}
}

func Test_projectServiceServer_InviteGet(t *testing.T) {
	t.Parallel()

	testStore, closer := test.StartRepositoryWithCleanup(t, test.WithPostgres(true))
	log := testStore.GetLogger()
	defer closer()

	now := timestamppb.Now()

	test.CreateProjectInvites(t, testStore, []*apiv2.ProjectInvite{
		{
			Secret:      "abcdefghijklmnopqrstuvwxyz123456",
			Tenant:      "john.doe@github",
			Role:        apiv2.ProjectRole_PROJECT_ROLE_EDITOR,
			Joined:      false,
			TenantName:  "john.doe@github",
			Project:     p0,
			ProjectName: "john.doe@github",
			ExpiresAt:   now,
			JoinedAt:    nil,
		},
	})

	tests := []struct {
		name    string
		rq      *apiv2.ProjectServiceInviteGetRequest
		want    *apiv2.ProjectServiceInviteGetResponse
		wantErr error
	}{
		{
			name: "get an invite",
			rq: &apiv2.ProjectServiceInviteGetRequest{
				Secret: "abcdefghijklmnopqrstuvwxyz123456",
			},
			want: &apiv2.ProjectServiceInviteGetResponse{
				Invite: &apiv2.ProjectInvite{
					Tenant:      "john.doe@github",
					Role:        apiv2.ProjectRole_PROJECT_ROLE_EDITOR,
					Joined:      false,
					TenantName:  "john.doe@github",
					Project:     p0,
					ProjectName: "john.doe@github",
					Secret:      "abcdefghijklmnopqrstuvwxyz123456",
					ExpiresAt:   now,
					JoinedAt:    nil,
				},
			},
			wantErr: nil,
		},
		{
			name: "get an invite that does not exist",
			rq: &apiv2.ProjectServiceInviteGetRequest{
				Secret: "abcdefghijklmnopqrstuvwxyz987654",
			},
			wantErr: errorutil.NotFound("the given invitation does not exist anymore"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := &projectServiceServer{
				log:         log,
				repo:        testStore.Store,
				inviteStore: testStore.GetProjectInviteStore(),
			}

			tok := testStore.GetToken("john.doe@github", &apiv2.TokenServiceCreateRequest{
				Expires: durationpb.New(time.Hour),
				TenantRoles: map[string]apiv2.TenantRole{
					"john.doe@github": apiv2.TenantRole(apiv2.TenantRole_TENANT_ROLE_OWNER),
				},
			})

			reqCtx := token.ContextWithToken(t.Context(), tok)

			if tt.wantErr == nil {
				// Execute proto based validation
				test.Validate(t, tt.rq)
			}
			got, err := u.InviteGet(reqCtx, tt.rq)
			if diff := cmp.Diff(err, tt.wantErr, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("diff = %s", diff)
			}

			if diff := cmp.Diff(
				tt.want,
				got,
				protocmp.Transform(),
			); diff != "" {
				t.Errorf("diff: %s", diff)
			}
		})
	}
}

func Test_projectServiceServer_InvitesList(t *testing.T) {
	t.Parallel()

	testStore, closer := test.StartRepositoryWithCleanup(t, test.WithPostgres(true))
	log := testStore.GetLogger()
	defer closer()

	now := timestamppb.Now()

	test.CreateProjectInvites(t, testStore, []*apiv2.ProjectInvite{
		{
			Secret:      "abcdefghijklmnopqrstuvwxyz000000",
			Tenant:      "john.doe@github",
			Role:        apiv2.ProjectRole_PROJECT_ROLE_EDITOR,
			Joined:      false,
			TenantName:  "john.doe@github",
			Project:     p0,
			ProjectName: "john.doe@github",
			ExpiresAt:   now,
			JoinedAt:    nil,
		},
		{
			Secret:      "abcdefghijklmnopqrstuvwxyz000001",
			Tenant:      "john.doe@github",
			Role:        apiv2.ProjectRole_PROJECT_ROLE_VIEWER,
			Joined:      false,
			TenantName:  "john.doe@github",
			Project:     p1,
			ProjectName: "project-1",
			ExpiresAt:   now,
			JoinedAt:    nil,
		},
	})

	tests := []struct {
		name    string
		rq      *apiv2.ProjectServiceInvitesListRequest
		want    *apiv2.ProjectServiceInvitesListResponse
		wantErr error
	}{
		{
			name: "list invites",
			rq: &apiv2.ProjectServiceInvitesListRequest{
				Project: p1,
			},
			want: &apiv2.ProjectServiceInvitesListResponse{
				Invites: []*apiv2.ProjectInvite{
					{
						Secret:      "abcdefghijklmnopqrstuvwxyz000001",
						Tenant:      "john.doe@github",
						Role:        apiv2.ProjectRole_PROJECT_ROLE_VIEWER,
						Joined:      false,
						TenantName:  "john.doe@github",
						Project:     p1,
						ProjectName: "project-1",
						ExpiresAt:   now,
						JoinedAt:    nil,
					},
				},
			},
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := &projectServiceServer{
				log:         log,
				repo:        testStore.Store,
				inviteStore: testStore.GetProjectInviteStore(),
			}

			tok := testStore.GetToken("john.doe@github", &apiv2.TokenServiceCreateRequest{
				Expires: durationpb.New(time.Hour),
				TenantRoles: map[string]apiv2.TenantRole{
					"john.doe@github": apiv2.TenantRole_TENANT_ROLE_OWNER,
				},
			})

			reqCtx := token.ContextWithToken(t.Context(), tok)
			if tt.wantErr == nil {
				// Execute proto based validation
				test.Validate(t, tt.rq)
			}

			got, err := u.InvitesList(reqCtx, tt.rq)
			if diff := cmp.Diff(err, tt.wantErr, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("diff = %s", diff)
			}

			if diff := cmp.Diff(
				tt.want,
				got,
				protocmp.Transform(),
			); diff != "" {
				t.Errorf("diff: %s", diff)
			}
		})
	}
}

func Test_projectServiceServer_InviteDelete(t *testing.T) {
	t.Parallel()

	testStore, closer := test.StartRepositoryWithCleanup(t, test.WithPostgres(true))
	log := testStore.GetLogger()
	defer closer()

	now := timestamppb.Now()

	test.CreateProjectInvites(t, testStore, []*apiv2.ProjectInvite{
		{
			Secret:      "abcdefghijklmnopqrstuvwxyz123456",
			Tenant:      "john.doe@github",
			Role:        apiv2.ProjectRole_PROJECT_ROLE_EDITOR,
			Joined:      false,
			TenantName:  "john.doe@github",
			Project:     p1,
			ProjectName: "john.doe@github",
			ExpiresAt:   now,
			JoinedAt:    nil,
		},
	})

	tests := []struct {
		name    string
		rq      *apiv2.ProjectServiceInviteDeleteRequest
		wantErr error
	}{
		{
			name: "delete invite",
			rq: &apiv2.ProjectServiceInviteDeleteRequest{
				Project: p1,
				Secret:  "abcdefghijklmnopqrstuvwxyz123456",
			},
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := &projectServiceServer{
				log:         log,
				repo:        testStore.Store,
				inviteStore: testStore.GetProjectInviteStore(),
			}

			tok := testStore.GetToken("john.doe@github", &apiv2.TokenServiceCreateRequest{
				Expires: durationpb.New(time.Hour),
				TenantRoles: map[string]apiv2.TenantRole{
					"john.doe@github": apiv2.TenantRole_TENANT_ROLE_OWNER,
				},
			})

			reqCtx := token.ContextWithToken(t.Context(), tok)
			if tt.wantErr == nil {
				// Execute proto based validation
				test.Validate(t, tt.rq)
			}

			_, err := u.InviteDelete(reqCtx, tt.rq)
			if diff := cmp.Diff(err, tt.wantErr, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("diff = %s", diff)
			}

			_, err = testStore.GetProjectInviteStore().GetInvite(t.Context(), tt.rq.Secret)
			assert.ErrorIs(t, err, invite.ErrInviteNotFound)
		})
	}
}

func Test_projectServiceServer_InviteAccept(t *testing.T) {
	t.Parallel()

	testStore, closer := test.StartRepositoryWithCleanup(t, test.WithPostgres(true))
	log := testStore.GetLogger()
	defer closer()

	anHour := timestamppb.New(time.Now().Add(time.Hour))

	tests := []struct {
		name                   string
		existingInvites        []*apiv2.ProjectInvite
		existingTenants        []*apiv2.TenantServiceCreateRequest
		existingProjects       []*apiv2.ProjectServiceCreateRequest
		existingProjectMembers map[string][]*apiv2.ProjectMemberServiceCreateRequest
		rq                     *apiv2.ProjectServiceInviteAcceptRequest
		want                   *apiv2.ProjectServiceInviteAcceptResponse
		wantMembers            []*apiv2.ProjectMember
		wantErr                error
	}{
		{
			name: "accept an invite",
			rq: &apiv2.ProjectServiceInviteAcceptRequest{
				Secret: "abcdefghijklmnopqrstuvwxyz123456",
			},
			existingTenants: []*apiv2.TenantServiceCreateRequest{
				{Name: "john.doe@github"},
				{Name: "will.smith@github"},
			},
			existingProjects: []*apiv2.ProjectServiceCreateRequest{
				{Name: p0, Login: "john.doe@github"},
				{Name: p1, Login: "will.smith@github"},
				{Name: "b950f4f5-d8b8-4252-aa02-ae08a1d2b044", Login: "john.doe@github"},
			},
			existingProjectMembers: map[string][]*apiv2.ProjectMemberServiceCreateRequest{
				p0:                                     {{Member: "john.doe@github", Role: apiv2.ProjectRole_PROJECT_ROLE_OWNER}},
				p1:                                     {{Member: "will.smith@github", Role: apiv2.ProjectRole_PROJECT_ROLE_OWNER}},
				"b950f4f5-d8b8-4252-aa02-ae08a1d2b044": {{Member: "john.doe@github", Role: apiv2.ProjectRole_PROJECT_ROLE_OWNER}},
			},
			existingInvites: []*apiv2.ProjectInvite{
				{
					Secret:      "abcdefghijklmnopqrstuvwxyz123456",
					Project:     "b950f4f5-d8b8-4252-aa02-ae08a1d2b044",
					Role:        apiv2.ProjectRole_PROJECT_ROLE_EDITOR,
					Joined:      false,
					ProjectName: "b950f4f5-d8b8-4252-aa02-ae08a1d2b044",
					Tenant:      "john.doe@github",
					TenantName:  "john.doe@github",
					ExpiresAt:   anHour,
					JoinedAt:    nil,
				},
			},
			want: &apiv2.ProjectServiceInviteAcceptResponse{
				Project:     "b950f4f5-d8b8-4252-aa02-ae08a1d2b044",
				ProjectName: "b950f4f5-d8b8-4252-aa02-ae08a1d2b044",
			},
			wantMembers: []*apiv2.ProjectMember{
				{
					Member: "john.doe@github",
					Role:   apiv2.ProjectRole_PROJECT_ROLE_OWNER,
					Meta:   &apiv2.Meta{},
				},
				{
					Member: "will.smith@github",
					Role:   apiv2.ProjectRole_PROJECT_ROLE_EDITOR,
					Meta:   &apiv2.Meta{},
				},
			},
			wantErr: nil,
		},
		{
			name: "cannot join twice",
			rq: &apiv2.ProjectServiceInviteAcceptRequest{
				Secret: "abcdefghijklmnopqrstuvwxyz123456",
			},
			existingTenants: []*apiv2.TenantServiceCreateRequest{
				{Name: "john.doe@github"},
				{Name: "will.smith@github"},
			},
			existingProjects: []*apiv2.ProjectServiceCreateRequest{
				{Name: p0, Login: "john.doe@github"},
				{Name: p1, Login: "will.smith@github"},
				{Name: "b950f4f5-d8b8-4252-aa02-ae08a1d2b044", Login: "john.doe@github"},
			},
			existingProjectMembers: map[string][]*apiv2.ProjectMemberServiceCreateRequest{
				p0: {{Member: "john.doe@github", Role: apiv2.ProjectRole_PROJECT_ROLE_OWNER}},
				p1: {{Member: "will.smith@github", Role: apiv2.ProjectRole_PROJECT_ROLE_OWNER}},
				"b950f4f5-d8b8-4252-aa02-ae08a1d2b044": {
					{Member: "john.doe@github", Role: apiv2.ProjectRole_PROJECT_ROLE_OWNER},
					{Member: "will.smith@github", Role: apiv2.ProjectRole_PROJECT_ROLE_EDITOR},
				},
			},
			existingInvites: []*apiv2.ProjectInvite{
				{
					Secret:      "abcdefghijklmnopqrstuvwxyz123456",
					Project:     "b950f4f5-d8b8-4252-aa02-ae08a1d2b044",
					Role:        apiv2.ProjectRole_PROJECT_ROLE_EDITOR,
					Joined:      false,
					ProjectName: "b950f4f5-d8b8-4252-aa02-ae08a1d2b044",
					Tenant:      "john.doe@github",
					TenantName:  "john.doe@github",
					ExpiresAt:   anHour,
					JoinedAt:    nil,
				},
			},
			wantErr: errorutil.Conflict("will.smith@github is already member of project b950f4f5-d8b8-4252-aa02-ae08a1d2b044"),
		},
		{
			name: "no self-joining",
			rq: &apiv2.ProjectServiceInviteAcceptRequest{
				Secret: "abcdefghijklmnopqrstuvwxyz123456",
			},
			existingTenants: []*apiv2.TenantServiceCreateRequest{
				{Name: "will.smith@github"},
			},
			existingProjects: []*apiv2.ProjectServiceCreateRequest{
				{Name: p1, Login: "will.smith@github"},
			},
			existingProjectMembers: map[string][]*apiv2.ProjectMemberServiceCreateRequest{
				p1: {{Member: "will.smith@github", Role: apiv2.ProjectRole_PROJECT_ROLE_OWNER}},
			},
			existingInvites: []*apiv2.ProjectInvite{
				{
					Secret:      "abcdefghijklmnopqrstuvwxyz123456",
					Project:     p1,
					Role:        apiv2.ProjectRole_PROJECT_ROLE_EDITOR,
					Joined:      false,
					ProjectName: "will.smith@github",
					Tenant:      "will.smith@github",
					TenantName:  "will.smith@github",
					ExpiresAt:   anHour,
					JoinedAt:    nil,
				},
			},
			wantErr: errorutil.InvalidArgument("an owner cannot accept invitations to own projects"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := &projectServiceServer{
				log:         log,
				repo:        testStore.Store,
				inviteStore: testStore.GetProjectInviteStore(),
			}

			test.CreateTenants(t, testStore, tt.existingTenants)
			test.CreateProjects(t, testStore, tt.existingProjects)
			if tt.existingProjectMembers != nil {
				for project, members := range tt.existingProjectMembers {
					test.CreateProjectMemberships(t, testStore, project, members)
				}
			}
			test.CreateProjectInvites(t, testStore, tt.existingInvites)
			defer func() {
				testStore.DeleteProjectInvites()
				testStore.DeleteProjects()
				testStore.DeleteTenants()
			}()

			tok := testStore.GetToken("will.smith@github", &apiv2.TokenServiceCreateRequest{
				Expires: durationpb.New(time.Hour),
				ProjectRoles: map[string]apiv2.ProjectRole{
					"will.smith@github": apiv2.ProjectRole_PROJECT_ROLE_OWNER,
				},
			})

			reqCtx := token.ContextWithToken(t.Context(), tok)
			if tt.wantErr == nil {
				// Execute proto based validation
				test.Validate(t, tt.rq)
			}
			acceptResp, err := u.InviteAccept(reqCtx, tt.rq)
			if diff := cmp.Diff(err, tt.wantErr, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("diff = %s", diff)
			}
			if err != nil {
				return
			}
			if diff := cmp.Diff(
				tt.want,
				pointer.SafeDeref(acceptResp),
				protocmp.Transform(),
			); diff != "" {
				t.Errorf("diff: %s", diff)
			}

			_, err = testStore.GetProjectInviteStore().GetInvite(t.Context(), tt.rq.Secret)
			require.ErrorIs(t, err, invite.ErrInviteNotFound)

			tok = testStore.GetToken("will.smith@github", &apiv2.TokenServiceCreateRequest{
				Expires: durationpb.New(time.Hour),
				ProjectRoles: map[string]apiv2.ProjectRole{
					"will.smith@github": apiv2.ProjectRole_PROJECT_ROLE_OWNER,
					acceptResp.Project:  apiv2.ProjectRole_PROJECT_ROLE_EDITOR,
				},
			})

			reqCtx = token.ContextWithToken(t.Context(), tok)

			getResp, err := testStore.ProjectMember(acceptResp.Project).List(reqCtx, nil)
			require.NoError(t, err)

			if diff := cmp.Diff(
				tt.wantMembers,
				getResp,
				protocmp.Transform(),
				protocmp.IgnoreFields(
					&apiv2.Meta{}, "created_at",
				),
			); diff != "" {
				t.Errorf("diff: %s", diff)
			}
		})
	}
}
