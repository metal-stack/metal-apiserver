package e2e

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/metal-stack/api/go/client"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/testing/protocmp"
)

// TestMemberCreateForAdmins checks that admins can create membership associations for arbitrary tenants and projects
func TestMemberCreateForAdmins(t *testing.T) {
	log, baseURL, adminToken, tenantTokens, closer := StartApiserver(t, "tenant-a", "tenant-b")
	defer closer()
	require.NotNil(t, baseURL, adminToken)

	adminClient, err := client.New(&client.DialConfig{
		BaseURL:   baseURL,
		Token:     adminToken,
		UserAgent: "integration test admin",
		Log:       log,
	})
	require.NoError(t, err)
	ctx := t.Context()

	_, err = adminClient.Apiv2().TenantMember().Create(ctx, &apiv2.TenantMemberServiceCreateRequest{
		Login:  "tenant-a",
		Role:   apiv2.TenantRole_TENANT_ROLE_EDITOR,
		Member: "tenant-b",
	},
	)
	require.NoError(t, err)

	project1, err := adminClient.Apiv2().Project().Create(ctx, &apiv2.ProjectServiceCreateRequest{Name: "p1", Login: "tenant-a"})
	require.NoError(t, err)
	require.NotEmpty(t, project1)

	_, err = adminClient.Apiv2().ProjectMember().Create(ctx, &apiv2.ProjectMemberServiceCreateRequest{
		Project: project1.Project.Uuid,
		Role:    apiv2.ProjectRole_PROJECT_ROLE_OWNER,
		Member:  "tenant-b",
	},
	)
	require.NoError(t, err)

	userClient, err := client.New(&client.DialConfig{
		BaseURL:   baseURL,
		Token:     tenantTokens["tenant-a"],
		UserAgent: "e2e user",
		Log:       log,
	})
	require.NoError(t, err)

	projectMemberResp, err := userClient.Apiv2().ProjectMember().List(ctx, &apiv2.ProjectMemberServiceListRequest{
		Project: project1.Project.Uuid,
	})
	require.NoError(t, err)
	require.NotNil(t, projectMemberResp)

	tenantMemberResp, err := userClient.Apiv2().TenantMember().List(ctx, &apiv2.TenantMemberServiceListRequest{
		Login: "tenant-a",
	})
	require.NoError(t, err)
	require.NotNil(t, tenantMemberResp)

	if diff := cmp.Diff(
		[]*apiv2.ProjectMember{
			{
				Member:  "tenant-a",
				Role:    apiv2.ProjectRole_PROJECT_ROLE_OWNER,
				Meta:    &apiv2.Meta{},
				Project: project1.Project.Uuid,
			},
			{
				Member:  "tenant-b",
				Role:    apiv2.ProjectRole_PROJECT_ROLE_OWNER,
				Meta:    &apiv2.Meta{},
				Project: project1.Project.Uuid,
			},
		},
		projectMemberResp.Members,
		protocmp.Transform(),
		protocmp.IgnoreFields(
			&apiv2.Meta{}, "created_at",
		),
		cmpopts.SortSlices(func(a, b *apiv2.ProjectMember) bool {
			return a.Member < b.Member
		}),
	); diff != "" {
		t.Errorf("diff: %s", diff)
	}

	if diff := cmp.Diff(
		[]*apiv2.TenantMember{
			{
				Member:   "tenant-a",
				Role:     apiv2.TenantRole_TENANT_ROLE_OWNER,
				Meta:     &apiv2.Meta{},
				Tenant:   "tenant-a",
				Projects: []string{project1.Project.Uuid},
			},
			{
				Member:   "tenant-b",
				Role:     apiv2.TenantRole_TENANT_ROLE_EDITOR,
				Meta:     &apiv2.Meta{},
				Tenant:   "tenant-a",
				Projects: []string{project1.Project.Uuid},
			},
		},
		tenantMemberResp.Members,
		protocmp.Transform(),
		protocmp.IgnoreFields(
			&apiv2.Meta{}, "created_at",
		),
		cmpopts.SortSlices(func(a, b *apiv2.TenantMember) bool {
			return a.Member < b.Member
		}),
	); diff != "" {
		t.Errorf("diff: %s", diff)
	}
}
