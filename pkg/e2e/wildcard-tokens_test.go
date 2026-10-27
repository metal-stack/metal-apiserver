package e2e

import (
	"testing"
	"time"

	"github.com/metal-stack/api/go/client"
	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/api/go/metalstack/api/v2/apiv2connect"
	"github.com/metal-stack/metal-apiserver/pkg/request"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/durationpb"
)

func TestWildcardTokens(t *testing.T) {
	t.Parallel()
	log, baseURL, adminToken, tenantTokenSecrets, closer := StartApiserver(t, "normal-tenant-a", "normal-tenant-b")
	defer closer()
	require.NotNil(t, baseURL, adminToken)

	var tenantProjectID *string

	t.Run("admin provides internet network", func(t *testing.T) {
		adminClient, err := client.New(&client.DialConfig{
			BaseURL:   baseURL,
			Token:     adminToken,
			UserAgent: "integration test admin",
			Log:       log,
		})
		require.NoError(t, err)

		_, err = adminClient.Adminv2().Network().Create(t.Context(), &adminv2.NetworkServiceCreateRequest{
			Id:       new("internet"),
			Prefixes: []string{"1.2.3.0/24"},
			Type:     apiv2.NetworkType_NETWORK_TYPE_EXTERNAL,
			Vrf:      new(uint32(11)),
		})
		require.NoError(t, err)
	})

	t.Run("tenant-a sets up an ip address", func(t *testing.T) {
		userClient, err := client.New(&client.DialConfig{
			BaseURL:   baseURL,
			Token:     tenantTokenSecrets["normal-tenant-a"],
			UserAgent: "just a normal tenant",
			Log:       log,
		})
		require.NoError(t, err)

		resp, err := userClient.Apiv2().Project().Create(t.Context(), &apiv2.ProjectServiceCreateRequest{
			Name:        "my first project",
			Description: "my first project",
			Login:       "normal-tenant-a",
		})
		require.NoError(t, err)

		tenantProjectID = &resp.Project.Uuid

		_, err = userClient.Apiv2().IP().Create(t.Context(), &apiv2.IPServiceCreateRequest{
			Network: "internet",
			Project: *tenantProjectID,
		})
		require.NoError(t, err)
	})

	require.NotNil(t, tenantProjectID)

	t.Run("tenant-b sets up wildcard token and requests tenant-a resources", func(t *testing.T) {
		userClient, err := client.New(&client.DialConfig{
			BaseURL:   baseURL,
			Token:     tenantTokenSecrets["normal-tenant-b"],
			UserAgent: "just a normal tenant",
			Log:       log,
		})
		require.NoError(t, err)

		ownProject, err := userClient.Apiv2().Project().Create(t.Context(), &apiv2.ProjectServiceCreateRequest{
			Name:        "my first project",
			Description: "my first project",
			Login:       "normal-tenant-b",
		})
		require.NoError(t, err)

		wildCardToken, err := userClient.Apiv2().Token().Create(t.Context(), &apiv2.TokenServiceCreateRequest{
			Description: "wildcard token",
			Permissions: []*apiv2.PermissionsByVisibility{
				{
					Visibility: &apiv2.PermissionsByVisibility_Tenant{
						Tenant: &apiv2.TenantPermissions{
							Login: request.AnySubject,
							Methods: []string{
								apiv2connect.AuditServiceListProcedure,
							},
						},
					},
				},
				{
					Visibility: &apiv2.PermissionsByVisibility_Project{
						Project: &apiv2.ProjectPermissions{
							Project: request.AnySubject,
							Methods: []string{
								apiv2connect.IPServiceListProcedure,
							},
						},
					},
				},
			},
			Expires: durationpb.New(10 * time.Minute),
		})
		require.NoError(t, err)

		userClient, err = client.New(&client.DialConfig{
			BaseURL:   baseURL,
			Token:     wildCardToken.Secret,
			UserAgent: "a wildcard secret",
			Log:       log,
		})
		require.NoError(t, err)

		_, err = userClient.Apiv2().Audit().List(t.Context(), &apiv2.AuditServiceListRequest{
			Login: "normal-tenant-b",
		})
		require.NoError(t, err)

		_, err = userClient.Apiv2().Audit().List(t.Context(), &apiv2.AuditServiceListRequest{
			Login: "normal-tenant-a",
		})
		require.ErrorContains(t, err, `is not allowed because it is not part of the token permissions, allowed subjects are:["normal-tenant-b"]`)

		_, err = userClient.Apiv2().IP().List(t.Context(), &apiv2.IPServiceListRequest{
			Project: ownProject.Project.Uuid,
		})
		require.NoError(t, err)

		_, err = userClient.Apiv2().IP().List(t.Context(), &apiv2.IPServiceListRequest{
			Project: *tenantProjectID,
		})
		require.ErrorContains(t, err, "is not allowed")

	})
}
