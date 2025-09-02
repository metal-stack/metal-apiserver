package project

import (
	"log/slog"
	"os"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/go-cmp/cmp"
	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
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
		{TenantId: "john.doe@github", Role: apiv2.ProjectRole_PROJECT_ROLE_OWNER},
		{TenantId: "will.smith@github", Role: apiv2.ProjectRole_PROJECT_ROLE_EDITOR},
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

func Test_projectServiceServer_List(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	testStore, closer := test.StartRepositoryWithCleanup(t, log, test.WithCockroach(true))
	defer closer()
	repo := testStore.Store

	test.CreateTenants(t, testStore, []*apiv2.TenantServiceCreateRequest{
		{Name: "john.doe@github"},
		{Name: "will.smith@github"},
	})
	test.CreateProjects(t, repo, []*apiv2.ProjectServiceCreateRequest{
		{Name: "john.doe@github", Login: "john.doe@github"},
		{Name: "will.smith@github", Login: "will.smith@github"},
		{Name: "b950f4f5-d8b8-4252-aa02-ae08a1d2b044", Login: "john.doe@github"},
	})

	test.CreateProjectMemberships(t, testStore, "john.doe@github", []*repository.ProjectMemberCreateRequest{
		{TenantId: "john.doe@github", Role: apiv2.ProjectRole_PROJECT_ROLE_OWNER},
	})
	test.CreateProjectMemberships(t, testStore, "will.smith@github", []*repository.ProjectMemberCreateRequest{
		{TenantId: "will.smith@github", Role: apiv2.ProjectRole_PROJECT_ROLE_OWNER},
	})
	test.CreateProjectMemberships(t, testStore, "b950f4f5-d8b8-4252-aa02-ae08a1d2b044", []*repository.ProjectMemberCreateRequest{
		{TenantId: "john.doe@github", Role: apiv2.ProjectRole_PROJECT_ROLE_EDITOR},
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
						Name:   "b950f4f5-d8b8-4252-aa02-ae08a1d2b044",
						Uuid:   "b950f4f5-d8b8-4252-aa02-ae08a1d2b044",
						Tenant: "john.doe@github",
					},
					{
						Meta:   &apiv2.Meta{},
						Name:   "john.doe@github",
						Uuid:   "john.doe@github",
						Tenant: "john.doe@github",
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "list the projects filtered by id",
			rq: &apiv2.ProjectServiceListRequest{
				Id: pointer.Pointer("john.doe@github"),
			},
			want: &apiv2.ProjectServiceListResponse{
				Projects: []*apiv2.Project{
					{
						Meta:   &apiv2.Meta{},
						Name:   "john.doe@github",
						Uuid:   "john.doe@github",
						Tenant: "john.doe@github",
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "list the projects filtered by name",
			rq: &apiv2.ProjectServiceListRequest{
				Name: pointer.Pointer("b950f4f5-d8b8-4252-aa02-ae08a1d2b044"),
			},
			want: &apiv2.ProjectServiceListResponse{
				Projects: []*apiv2.Project{
					{
						Meta:   &apiv2.Meta{},
						Name:   "b950f4f5-d8b8-4252-aa02-ae08a1d2b044",
						Uuid:   "b950f4f5-d8b8-4252-aa02-ae08a1d2b044",
						Tenant: "john.doe@github",
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "list the projects filtered by tenant 1",
			rq: &apiv2.ProjectServiceListRequest{
				Tenant: pointer.Pointer("john.doe@github"),
			},
			want: &apiv2.ProjectServiceListResponse{
				Projects: []*apiv2.Project{
					{
						Meta:   &apiv2.Meta{},
						Name:   "b950f4f5-d8b8-4252-aa02-ae08a1d2b044",
						Uuid:   "b950f4f5-d8b8-4252-aa02-ae08a1d2b044",
						Tenant: "john.doe@github",
					},
					{
						Meta:   &apiv2.Meta{},
						Name:   "john.doe@github",
						Uuid:   "john.doe@github",
						Tenant: "john.doe@github",
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "list the projects filtered by tenant 2",
			rq: &apiv2.ProjectServiceListRequest{
				Tenant: pointer.Pointer("non-existing"),
			},
			want:    &apiv2.ProjectServiceListResponse{},
			wantErr: nil,
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

func Test_projectServiceServer_Create(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	testStore, closer := test.StartRepositoryWithCleanup(t, log, test.WithCockroach(true))
	defer closer()
	repo := testStore.Store

	test.CreateTenants(t, testStore, []*apiv2.TenantServiceCreateRequest{
		{Name: "john.doe@github"},
	})

	tests := []struct {
		name        string
		rq          *apiv2.ProjectServiceCreateRequest
		want        *apiv2.ProjectServiceCreateResponse
		wantMembers []*apiv2.ProjectMember
		wantErr     error
	}{
		{
			name: "create a project",
			rq: &apiv2.ProjectServiceCreateRequest{
				Name:        "My New Org Project",
				Description: "project desc",
				AvatarUrl:   pointer.Pointer("http://test"),
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
					AvatarUrl:   pointer.Pointer("http://test"),
				},
			},
			wantMembers: []*apiv2.ProjectMember{
				{
					Id:   "john.doe@github",
					Role: apiv2.ProjectRole_PROJECT_ROLE_OWNER,
				},
			},
			wantErr: nil,
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

			assert.NotEmpty(t, got.Msg.Project.Uuid)

			if diff := cmp.Diff(
				tt.want, pointer.SafeDeref(got).Msg,
				protocmp.Transform(),
				protocmp.IgnoreFields(
					&apiv2.Meta{}, "created_at", "updated_at",
				),
				protocmp.IgnoreFields(
					&apiv2.Project{}, "uuid",
				),
			); diff != "" {
				t.Errorf("%v, want %v diff: %s", got.Msg, tt.want, diff)
			}
		})
	}
}

func Test_projectServiceServer_Update(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	testStore, closer := test.StartRepositoryWithCleanup(t, log, test.WithCockroach(true))
	defer closer()
	repo := testStore.Store

	test.CreateTenants(t, testStore, []*apiv2.TenantServiceCreateRequest{
		{Name: "john.doe@github"},
	})

	test.CreateProjects(t, repo, []*apiv2.ProjectServiceCreateRequest{{
		Name:        "john.doe@github",
		Description: "old desc",
		AvatarUrl:   pointer.Pointer("http://old"),
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
			name: "create a project",
			rq: &apiv2.ProjectServiceUpdateRequest{
				Name:        pointer.Pointer("new name"),
				Description: pointer.Pointer("new desc"),
				AvatarUrl:   pointer.Pointer("http://new"),
				Labels: &apiv2.UpdateLabels{
					Update: &apiv2.Labels{
						Labels: map[string]string{
							"c": "d",
						},
					},
				},
				Project: "john.doe@github",
			},
			want: &apiv2.ProjectServiceUpdateResponse{
				Project: &apiv2.Project{
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
					AvatarUrl:   pointer.Pointer("http://new"),
					Uuid:        "john.doe@github",
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
				repo:        repo,
				inviteStore: testStore.GetProjectInviteStore(),
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

func Test_projectServiceServer_Delete(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	testStore, closer := test.StartRepositoryWithCleanup(t, log, test.WithCockroach(true))
	defer closer()
	repo := testStore.Store

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
				Project: "john.doe@github",
			},
			existingProjects: []*apiv2.ProjectServiceCreateRequest{
				{Name: "john.doe@github", Login: "john.doe@github"},
			},
			want: &apiv2.ProjectServiceDeleteResponse{
				Project: &apiv2.Project{
					Meta:   &apiv2.Meta{},
					Name:   "john.doe@github",
					Uuid:   "john.doe@github",
					Tenant: "john.doe@github",
				},
			},
			wantErr: nil,
		},
		{
			name: "delete non-existing project",
			rq: &apiv2.ProjectServiceDeleteRequest{
				Project: "project-a",
			},
			existingProjects: []*apiv2.ProjectServiceCreateRequest{
				{Name: "john.doe@github", Login: "john.doe@github"},
			},
			wantErr: errorutil.NotFound("project with id:project-a not found sql: no rows in result set"),
		},
		{
			name: "cannot delete project when machines are still present",
			rq: &apiv2.ProjectServiceDeleteRequest{
				Project: "john.doe@github",
			},
			existingProjects: []*apiv2.ProjectServiceCreateRequest{
				{Name: "john.doe@github", Login: "john.doe@github"},
			},
			existingMachines: []*metal.Machine{
				{
					Allocation: &metal.MachineAllocation{
						UUID:    "1",
						Project: "john.doe@github",
					},
				},
			},
			wantErr: errorutil.FailedPrecondition("there are still machines associated with this project, you need to delete them first"),
		},
		{
			name: "cannot delete project when ips are still present",
			rq: &apiv2.ProjectServiceDeleteRequest{
				Project: "john.doe@github",
			},
			existingProjects: []*apiv2.ProjectServiceCreateRequest{
				{Name: "john.doe@github", Login: "john.doe@github"},
			},
			existingNetworks: []*adminv2.NetworkServiceCreateRequest{
				{Id: pointer.Pointer("internet"), Prefixes: []string{"1.2.3.0/24"}, Type: apiv2.NetworkType_NETWORK_TYPE_EXTERNAL, Vrf: pointer.Pointer(uint32(11))},
			},
			existingIPs: []*apiv2.IPServiceCreateRequest{
				{Name: pointer.Pointer("ip1"), Ip: pointer.Pointer("1.2.3.4"), Project: "john.doe@github", Network: "internet"},
			},
			wantErr: errorutil.FailedPrecondition("there are still ips associated with this project, you need to delete them first"),
		},
		{
			name: "cannot delete project when networks are still present",
			rq: &apiv2.ProjectServiceDeleteRequest{
				Project: "john.doe@github",
			},
			existingProjects: []*apiv2.ProjectServiceCreateRequest{
				{Name: "john.doe@github", Login: "john.doe@github"},
			},
			existingNetworks: []*adminv2.NetworkServiceCreateRequest{
				{Id: pointer.Pointer("project-internet"), Project: pointer.Pointer("john.doe@github"), Prefixes: []string{"1.2.4.0/24"}, Type: apiv2.NetworkType_NETWORK_TYPE_EXTERNAL, Vrf: pointer.Pointer(uint32(12))},
			},
			wantErr: errorutil.FailedPrecondition("there are still networks associated with this project, you need to delete them first"),
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

			test.CreateProjects(t, repo, tt.existingProjects)
			test.CreateNetworks(t, repo, tt.existingNetworks)
			test.CreateIPs(t, repo, tt.existingIPs)
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

func Test_projectServiceServer_MemberUpdate(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	testStore, closer := test.StartRepositoryWithCleanup(t, log, test.WithCockroach(true))
	defer closer()
	repo := testStore.Store

	tests := []struct {
		name                   string
		rq                     *apiv2.ProjectServiceUpdateMemberRequest
		existingTenants        []*apiv2.TenantServiceCreateRequest
		existingTenantMembers  map[string][]*repository.TenantMemberCreateRequest
		existingProjects       []*apiv2.ProjectServiceCreateRequest
		existingProjectMembers map[string][]*repository.ProjectMemberCreateRequest
		want                   *apiv2.ProjectServiceUpdateMemberResponse
		wantErr                error
	}{
		{
			name: "update a member",
			rq: &apiv2.ProjectServiceUpdateMemberRequest{
				Project: "b950f4f5-d8b8-4252-aa02-ae08a1d2b044",
				Member:  "will.smith@github",
				Role:    apiv2.ProjectRole_PROJECT_ROLE_EDITOR,
			},
			existingTenants: []*apiv2.TenantServiceCreateRequest{
				{Name: "john.doe@github"},
				{Name: "will.smith@github"},
			},
			existingProjects: []*apiv2.ProjectServiceCreateRequest{
				{Name: "john.doe@github", Login: "john.doe@github"},
				{Name: "b950f4f5-d8b8-4252-aa02-ae08a1d2b044", Login: "john.doe@github"},
				{Name: "will.smith@github", Login: "will.smith@github"},
			},
			existingProjectMembers: map[string][]*repository.ProjectMemberCreateRequest{
				"b950f4f5-d8b8-4252-aa02-ae08a1d2b044": {
					{TenantId: "john.doe@github", Role: apiv2.ProjectRole_PROJECT_ROLE_OWNER},
					{TenantId: "will.smith@github", Role: apiv2.ProjectRole_PROJECT_ROLE_OWNER},
				},
			},
			want: &apiv2.ProjectServiceUpdateMemberResponse{
				ProjectMember: &apiv2.ProjectMember{
					Id:   "will.smith@github",
					Role: apiv2.ProjectRole_PROJECT_ROLE_EDITOR,
				},
			},
			wantErr: nil,
		},
		{
			name: "unable to demote last owner",
			rq: &apiv2.ProjectServiceUpdateMemberRequest{
				Project: "b950f4f5-d8b8-4252-aa02-ae08a1d2b044",
				Member:  "john.doe@github",
				Role:    apiv2.ProjectRole_PROJECT_ROLE_EDITOR,
			},
			existingTenants: []*apiv2.TenantServiceCreateRequest{
				{Name: "john.doe@github"},
			},
			existingProjects: []*apiv2.ProjectServiceCreateRequest{
				{Name: "john.doe@github", Login: "john.doe@github"},
				{Name: "b950f4f5-d8b8-4252-aa02-ae08a1d2b044", Login: "john.doe@github"},
			},
			existingProjectMembers: map[string][]*repository.ProjectMemberCreateRequest{
				"b950f4f5-d8b8-4252-aa02-ae08a1d2b044": {
					{TenantId: "john.doe@github", Role: apiv2.ProjectRole_PROJECT_ROLE_OWNER},
				},
			},
			wantErr: errorutil.FailedPrecondition("cannot demote last owner's permissions"),
		},
		{
			name: "unable to update a project member that is neither member of this project nor inherited member through tenant membership",
			rq: &apiv2.ProjectServiceUpdateMemberRequest{
				Project: "b950f4f5-d8b8-4252-aa02-ae08a1d2b044",
				Member:  "will.smith@github",
				Role:    apiv2.ProjectRole_PROJECT_ROLE_EDITOR,
			},
			existingTenants: []*apiv2.TenantServiceCreateRequest{
				{Name: "john.doe@github"},
				{Name: "will.smith@github"},
			},
			existingProjects: []*apiv2.ProjectServiceCreateRequest{
				{Name: "john.doe@github", Login: "john.doe@github"},
				{Name: "will.smith@github", Login: "will.smith@github"},
				{Name: "b950f4f5-d8b8-4252-aa02-ae08a1d2b044", Login: "john.doe@github"},
			},
			existingProjectMembers: map[string][]*repository.ProjectMemberCreateRequest{
				"b950f4f5-d8b8-4252-aa02-ae08a1d2b044": {
					{TenantId: "john.doe@github", Role: apiv2.ProjectRole_PROJECT_ROLE_OWNER},
				},
			},
			wantErr: errorutil.InvalidArgument("tenant is not part of the project's tenants"),
		},
		{
			name: "create direct membership if belongs to tenant",
			rq: &apiv2.ProjectServiceUpdateMemberRequest{
				Project: "b950f4f5-d8b8-4252-aa02-ae08a1d2b044",
				Member:  "will.smith@github",
				Role:    apiv2.ProjectRole_PROJECT_ROLE_EDITOR,
			},
			existingTenants: []*apiv2.TenantServiceCreateRequest{
				{Name: "john.doe@github"},
				{Name: "will.smith@github"},
			},
			existingTenantMembers: map[string][]*repository.TenantMemberCreateRequest{
				"john.doe@github": {
					{MemberID: "will.smith@github", Role: apiv2.TenantRole_TENANT_ROLE_EDITOR},
				},
			},
			existingProjects: []*apiv2.ProjectServiceCreateRequest{
				{Name: "john.doe@github", Login: "john.doe@github"},
				{Name: "will.smith@github", Login: "will.smith@github"},
				{Name: "b950f4f5-d8b8-4252-aa02-ae08a1d2b044", Login: "john.doe@github"},
			},
			existingProjectMembers: map[string][]*repository.ProjectMemberCreateRequest{
				"b950f4f5-d8b8-4252-aa02-ae08a1d2b044": {
					{TenantId: "john.doe@github", Role: apiv2.ProjectRole_PROJECT_ROLE_OWNER},
				},
			},
			want: &apiv2.ProjectServiceUpdateMemberResponse{
				ProjectMember: &apiv2.ProjectMember{
					Id:   "will.smith@github",
					Role: apiv2.ProjectRole_PROJECT_ROLE_EDITOR,
				},
			},
			wantErr: nil,
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

			test.CreateTenants(t, testStore, tt.existingTenants)
			if tt.existingTenantMembers != nil {
				for tenant, members := range tt.existingTenantMembers {
					test.CreateTenantMemberships(t, testStore, tenant, members)
				}
			}
			test.CreateProjects(t, repo, tt.existingProjects)
			if tt.existingProjectMembers != nil {
				for project, members := range tt.existingProjectMembers {
					test.CreateProjectMemberships(t, testStore, project, members)
				}
			}
			defer func() {
				testStore.DeleteProjects()
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
					&apiv2.ProjectMember{}, "created_at",
				),
			); diff != "" {
				t.Errorf("%v, want %v diff: %s", got.Msg, tt.want, diff)
			}
		})
	}
}

func Test_projectServiceServer_MemberRemove(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	testStore, closer := test.StartRepositoryWithCleanup(t, log, test.WithCockroach(true))
	defer closer()
	repo := testStore.Store

	tests := []struct {
		name                   string
		existingTenants        []*apiv2.TenantServiceCreateRequest
		existingTenantMembers  map[string][]*repository.TenantMemberCreateRequest
		existingProjects       []*apiv2.ProjectServiceCreateRequest
		existingProjectMembers map[string][]*repository.ProjectMemberCreateRequest
		rq                     *apiv2.ProjectServiceRemoveMemberRequest
		wantErr                error
	}{
		{
			name: "remove a member",
			rq: &apiv2.ProjectServiceRemoveMemberRequest{
				Project: "b950f4f5-d8b8-4252-aa02-ae08a1d2b044",
				Member:  "will.smith@github",
			},
			existingTenants: []*apiv2.TenantServiceCreateRequest{
				{Name: "john.doe@github"},
				{Name: "will.smith@github"},
			},
			existingProjects: []*apiv2.ProjectServiceCreateRequest{
				{Name: "john.doe@github", Login: "john.doe@github"},
				{Name: "b950f4f5-d8b8-4252-aa02-ae08a1d2b044", Login: "john.doe@github"},
				{Name: "will.smith@github", Login: "will.smith@github"},
			},
			existingProjectMembers: map[string][]*repository.ProjectMemberCreateRequest{
				"b950f4f5-d8b8-4252-aa02-ae08a1d2b044": {
					{TenantId: "john.doe@github", Role: apiv2.ProjectRole_PROJECT_ROLE_OWNER},
					{TenantId: "will.smith@github", Role: apiv2.ProjectRole_PROJECT_ROLE_OWNER},
				},
			},
			wantErr: nil,
		},
		{
			name: "unable to remove last owner",
			rq: &apiv2.ProjectServiceRemoveMemberRequest{
				Project: "b950f4f5-d8b8-4252-aa02-ae08a1d2b044",
				Member:  "john.doe@github",
			},
			existingTenants: []*apiv2.TenantServiceCreateRequest{
				{Name: "john.doe@github"},
			},
			existingProjects: []*apiv2.ProjectServiceCreateRequest{
				{Name: "john.doe@github", Login: "john.doe@github"},
				{Name: "b950f4f5-d8b8-4252-aa02-ae08a1d2b044", Login: "john.doe@github"},
			},
			existingProjectMembers: map[string][]*repository.ProjectMemberCreateRequest{
				"b950f4f5-d8b8-4252-aa02-ae08a1d2b044": {
					{TenantId: "john.doe@github", Role: apiv2.ProjectRole_PROJECT_ROLE_OWNER},
				},
			},
			wantErr: errorutil.FailedPrecondition("cannot remove last owner of a project"),
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

			test.CreateTenants(t, testStore, tt.existingTenants)
			if tt.existingTenantMembers != nil {
				for tenant, members := range tt.existingTenantMembers {
					test.CreateTenantMemberships(t, testStore, tenant, members)
				}
			}
			test.CreateProjects(t, repo, tt.existingProjects)
			if tt.existingProjectMembers != nil {
				for project, members := range tt.existingProjectMembers {
					test.CreateProjectMemberships(t, testStore, project, members)
				}
			}
			defer func() {
				testStore.DeleteProjects()
				testStore.DeleteTenants()
			}()

			tok := testStore.GetToken("john.doe@github", &apiv2.TokenServiceCreateRequest{
				Expires: durationpb.New(time.Hour),
				ProjectRoles: map[string]apiv2.ProjectRole{
					"john.doe@github": apiv2.ProjectRole_PROJECT_ROLE_OWNER,
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

func Test_projectServiceServer_Invite(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	testStore, closer := test.StartRepositoryWithCleanup(t, log, test.WithCockroach(true))
	defer closer()
	repo := testStore.Store

	test.CreateTenants(t, testStore, []*apiv2.TenantServiceCreateRequest{
		{Name: "john.doe@github"},
	})
	test.CreateProjects(t, repo, []*apiv2.ProjectServiceCreateRequest{
		{Name: "john.doe@github", Login: "john.doe@github"},
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
				Project: "john.doe@github",
				Role:    apiv2.ProjectRole_PROJECT_ROLE_EDITOR,
			},
			want: &apiv2.ProjectServiceInviteResponse{
				Invite: &apiv2.ProjectInvite{
					Project:     "john.doe@github",
					Role:        apiv2.ProjectRole_PROJECT_ROLE_EDITOR,
					Joined:      false,
					ProjectName: "john.doe@github",
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
				repo:        repo,
				inviteStore: testStore.GetProjectInviteStore(),
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

func Test_projectServiceServer_InviteGet(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	testStore, closer := test.StartRepositoryWithCleanup(t, log, test.WithCockroach(true))
	defer closer()
	repo := testStore.Store

	now := timestamppb.Now()

	test.CreateProjectInvites(t, testStore, []*apiv2.ProjectInvite{
		{
			Secret:      "abcdefghijklmnopqrstuvwxyz123456",
			Tenant:      "john.doe@github",
			Role:        apiv2.ProjectRole_PROJECT_ROLE_EDITOR,
			Joined:      false,
			TenantName:  "john.doe@github",
			Project:     "john.doe@github",
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
					Project:     "john.doe@github",
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
				repo:        repo,
				inviteStore: testStore.GetProjectInviteStore(),
				tokenStore:  testStore.GetTokenStore(),
			}

			tok := testStore.GetToken("john.doe@github", &apiv2.TokenServiceCreateRequest{
				Expires: durationpb.New(time.Hour),
				TenantRoles: map[string]apiv2.TenantRole{
					"john.doe@github": apiv2.TenantRole(apiv2.TenantRole_TENANT_ROLE_OWNER),
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

func Test_projectServiceServer_InvitesList(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	testStore, closer := test.StartRepositoryWithCleanup(t, log, test.WithCockroach(true))
	defer closer()
	repo := testStore.Store

	now := timestamppb.Now()

	test.CreateProjectInvites(t, testStore, []*apiv2.ProjectInvite{
		{
			Secret:      "abcdefghijklmnopqrstuvwxyz000000",
			Tenant:      "john.doe@github",
			Role:        apiv2.ProjectRole_PROJECT_ROLE_EDITOR,
			Joined:      false,
			TenantName:  "john.doe@github",
			Project:     "john.doe@github",
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
			Project:     "project-1",
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
				Project: "project-1",
			},
			want: &apiv2.ProjectServiceInvitesListResponse{
				Invites: []*apiv2.ProjectInvite{
					{
						Secret:      "abcdefghijklmnopqrstuvwxyz000001",
						Tenant:      "john.doe@github",
						Role:        apiv2.ProjectRole_PROJECT_ROLE_VIEWER,
						Joined:      false,
						TenantName:  "john.doe@github",
						Project:     "project-1",
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
				repo:        repo,
				inviteStore: testStore.GetProjectInviteStore(),
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

func Test_projectServiceServer_InviteDelete(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	testStore, closer := test.StartRepositoryWithCleanup(t, log, test.WithCockroach(true))
	defer closer()
	repo := testStore.Store

	now := timestamppb.Now()

	test.CreateProjectInvites(t, testStore, []*apiv2.ProjectInvite{
		{
			Secret:      "abcdefghijklmnopqrstuvwxyz123456",
			Tenant:      "john.doe@github",
			Role:        apiv2.ProjectRole_PROJECT_ROLE_EDITOR,
			Joined:      false,
			TenantName:  "john.doe@github",
			Project:     "john.doe@github",
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
				Project: "john.doe@github",
				Secret:  "abcdefghijklmnopqrstuvwxyz123456",
			},
			wantErr: nil,
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

			_, err = testStore.GetProjectInviteStore().GetInvite(t.Context(), tt.rq.Secret)
			assert.ErrorIs(t, err, invite.ErrInviteNotFound)
		})
	}
}

func Test_projectServiceServer_InviteAccept(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	testStore, closer := test.StartRepositoryWithCleanup(t, log, test.WithCockroach(true))
	defer closer()
	repo := testStore.Store

	anHour := timestamppb.New(time.Now().Add(time.Hour))

	tests := []struct {
		name                   string
		existingInvites        []*apiv2.ProjectInvite
		existingTenants        []*apiv2.TenantServiceCreateRequest
		existingProjects       []*apiv2.ProjectServiceCreateRequest
		existingProjectMembers map[string][]*repository.ProjectMemberCreateRequest
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
				{Name: "john.doe@github", Login: "john.doe@github"},
				{Name: "will.smith@github", Login: "will.smith@github"},
				{Name: "b950f4f5-d8b8-4252-aa02-ae08a1d2b044", Login: "john.doe@github"},
			},
			existingProjectMembers: map[string][]*repository.ProjectMemberCreateRequest{
				"john.doe@github":                      {{TenantId: "john.doe@github", Role: apiv2.ProjectRole_PROJECT_ROLE_OWNER}},
				"will.smith@github":                    {{TenantId: "will.smith@github", Role: apiv2.ProjectRole_PROJECT_ROLE_OWNER}},
				"b950f4f5-d8b8-4252-aa02-ae08a1d2b044": {{TenantId: "john.doe@github", Role: apiv2.ProjectRole_PROJECT_ROLE_OWNER}},
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
					Id:   "john.doe@github",
					Role: apiv2.ProjectRole_PROJECT_ROLE_OWNER,
				},
				{
					Id:   "will.smith@github",
					Role: apiv2.ProjectRole_PROJECT_ROLE_EDITOR,
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
				{Name: "john.doe@github", Login: "john.doe@github"},
				{Name: "will.smith@github", Login: "will.smith@github"},
				{Name: "b950f4f5-d8b8-4252-aa02-ae08a1d2b044", Login: "john.doe@github"},
			},
			existingProjectMembers: map[string][]*repository.ProjectMemberCreateRequest{
				"john.doe@github":   {{TenantId: "john.doe@github", Role: apiv2.ProjectRole_PROJECT_ROLE_OWNER}},
				"will.smith@github": {{TenantId: "will.smith@github", Role: apiv2.ProjectRole_PROJECT_ROLE_OWNER}},
				"b950f4f5-d8b8-4252-aa02-ae08a1d2b044": {
					{TenantId: "john.doe@github", Role: apiv2.ProjectRole_PROJECT_ROLE_OWNER},
					{TenantId: "will.smith@github", Role: apiv2.ProjectRole_PROJECT_ROLE_EDITOR},
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
				{Name: "will.smith@github", Login: "will.smith@github"},
			},
			existingProjectMembers: map[string][]*repository.ProjectMemberCreateRequest{
				"will.smith@github": {{TenantId: "will.smith@github", Role: apiv2.ProjectRole_PROJECT_ROLE_OWNER}},
			},
			existingInvites: []*apiv2.ProjectInvite{
				{
					Secret:      "abcdefghijklmnopqrstuvwxyz123456",
					Project:     "will.smith@github",
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
				repo:        repo,
				inviteStore: testStore.GetProjectInviteStore(),
				tokenStore:  testStore.GetTokenStore(),
			}

			test.CreateTenants(t, testStore, tt.existingTenants)
			test.CreateProjects(t, repo, tt.existingProjects)
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

			_, err = testStore.GetProjectInviteStore().GetInvite(t.Context(), tt.rq.Secret)
			require.ErrorIs(t, err, invite.ErrInviteNotFound)

			tok = testStore.GetToken("will.smith@github", &apiv2.TokenServiceCreateRequest{
				Expires: durationpb.New(time.Hour),
				ProjectRoles: map[string]apiv2.ProjectRole{
					"will.smith@github":    apiv2.ProjectRole_PROJECT_ROLE_OWNER,
					acceptResp.Msg.Project: apiv2.ProjectRole_PROJECT_ROLE_EDITOR,
				},
			})

			reqCtx = token.ContextWithToken(t.Context(), tok)

			getResp, err := u.Get(reqCtx, connect.NewRequest(&apiv2.ProjectServiceGetRequest{
				Project: acceptResp.Msg.Project,
			}))
			require.NoError(t, err)

			if diff := cmp.Diff(
				tt.wantMembers,
				pointer.SafeDeref(getResp).Msg.ProjectMembers,
				protocmp.Transform(),
				protocmp.IgnoreFields(
					&apiv2.ProjectMember{}, "created_at",
				),
			); diff != "" {
				t.Errorf("diff: %s", diff)
			}
		})
	}
}
