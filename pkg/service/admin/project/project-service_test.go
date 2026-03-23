package admin

import (
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	"github.com/metal-stack/metal-apiserver/pkg/repository"
	"github.com/metal-stack/metal-apiserver/pkg/test"
	"github.com/metal-stack/metal-apiserver/pkg/token"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/durationpb"
)

var (
	p0 = "00000000-0000-0000-0000-000000000000"
)

func Test_projectServiceServer_List(t *testing.T) {
	t.Parallel()

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	testStore, closer := test.StartRepositoryWithCleanup(t, log, test.WithPostgres(true))
	defer closer()

	test.CreateTenants(t, testStore, []*apiv2.TenantServiceCreateRequest{
		{Name: "john.doe@github"},
		{Name: "jane.roe@github"},
	})
	test.CreateProjects(t, testStore, []*apiv2.ProjectServiceCreateRequest{
		{Name: p0, Login: "john.doe@github"},
		{Name: "jane.roe@github", Login: "jane.roe@github"},
		{Name: "b950f4f5-d8b8-4252-aa02-ae08a1d2b044", Login: "john.doe@github"},
	})

	test.CreateProjectMemberships(t, testStore, p0, []*repository.ProjectMemberCreateRequest{
		{TenantId: "john.doe@github", Role: apiv2.ProjectRole_PROJECT_ROLE_OWNER},
	})
	test.CreateProjectMemberships(t, testStore, "jane.roe@github", []*repository.ProjectMemberCreateRequest{
		{TenantId: "jane.roe@github", Role: apiv2.ProjectRole_PROJECT_ROLE_OWNER},
	})
	test.CreateProjectMemberships(t, testStore, "b950f4f5-d8b8-4252-aa02-ae08a1d2b044", []*repository.ProjectMemberCreateRequest{
		{TenantId: "john.doe@github", Role: apiv2.ProjectRole_PROJECT_ROLE_EDITOR},
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
						Name:   "b950f4f5-d8b8-4252-aa02-ae08a1d2b044",
						Uuid:   "b950f4f5-d8b8-4252-aa02-ae08a1d2b044",
						Tenant: "john.doe@github",
					},
					{
						Meta:   &apiv2.Meta{},
						Name:   "jane.roe@github",
						Uuid:   "jane.roe@github",
						Tenant: "jane.roe@github",
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "list the projects filtered by tenant 1",
			rq: &adminv2.ProjectServiceListRequest{
				Tenant: new("john.doe@github"),
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
			rq: &adminv2.ProjectServiceListRequest{
				Tenant: new("jane.roe@github"),
			},
			want: &adminv2.ProjectServiceListResponse{
				Projects: []*apiv2.Project{
					{
						Meta:   &apiv2.Meta{},
						Name:   "jane.roe@github",
						Uuid:   "jane.roe@github",
						Tenant: "jane.roe@github",
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
			role := apiv2.AdminRole_ADMIN_ROLE_EDITOR
			tok := testStore.GetToken("john.doe@github", &apiv2.TokenServiceCreateRequest{
				Expires:   durationpb.New(time.Hour),
				AdminRole: &role,
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
