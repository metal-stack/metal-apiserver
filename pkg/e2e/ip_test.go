package e2e

import (
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/metal-stack/api/go/client"
	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/api/go/metalstack/api/v2/apiv2connect"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/durationpb"
)

func TestIPCreate(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	userName := t.Name()
	baseURL, adminToken, tenantTokenSecrets, closer := StartApiserver(t, log, userName)
	defer closer()
	require.NotNil(t, baseURL, adminToken)
	log.Info("token", "secret", tenantTokenSecrets[userName])

	adminClient, err := client.New(&client.DialConfig{
		BaseURL:   baseURL,
		Token:     adminToken,
		UserAgent: "integration test admin",
		Log:       log,
	})
	require.NoError(t, err)
	ctx := t.Context()

	internet, err := adminClient.Adminv2().Network().Create(ctx, &adminv2.NetworkServiceCreateRequest{
		Id:       new("internet"),
		Prefixes: []string{"1.2.3.0/24"},
		Type:     apiv2.NetworkType_NETWORK_TYPE_EXTERNAL,
		Vrf:      new(uint32(11)),
	})
	require.NoError(t, err)

	project1, err := adminClient.Apiv2().Project().Create(ctx, &apiv2.ProjectServiceCreateRequest{Name: "p1", Login: userName})
	require.NoError(t, err)
	require.NotEmpty(t, project1)

	tokenResp, err := adminClient.Adminv2().Token().Create(ctx, &adminv2.TokenServiceCreateRequest{
		User: &userName,
		TokenCreateRequest: &apiv2.TokenServiceCreateRequest{
			Description: userName,
			Permissions: []*apiv2.TypedMethodPermission{
				{
					Permissiontype: &apiv2.TypedMethodPermission_Project{
						Project: &apiv2.ProjectPermissions{
							Project: project1.Project.Uuid,
							Methods: []string{
								apiv2connect.IPServiceCreateProcedure,
								apiv2connect.IPServiceGetProcedure,
							},
						},
					},
				},
			},
			Expires: durationpb.New(10 * time.Minute),
		},
	})
	require.NoError(t, err)

	userClient, err := client.New(&client.DialConfig{
		BaseURL:   baseURL,
		Token:     tokenResp.Secret,
		UserAgent: "e2e ip-create user",
		Log:       log,
	})
	require.NoError(t, err)

	ipcr, err := userClient.Apiv2().IP().Create(ctx, &apiv2.IPServiceCreateRequest{
		Network: internet.Network.Id,
		Project: project1.Project.Uuid,
	})
	require.NoError(t, err)
	require.NotNil(t, ipcr)
}
