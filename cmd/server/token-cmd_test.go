package main

import (
	"testing"
	"time"

	"buf.build/go/protoyaml"
	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	"github.com/metal-stack/api/go/metalstack/admin/v2/adminv2connect"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/api/go/metalstack/api/v2/apiv2connect"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/durationpb"
)

func TestTokenCreateConfigTest(t *testing.T) {
	tokenCreateRequests := &adminv2.TokenServiceCreateMultiRequest{
		TokenCreateRequests: map[string]*adminv2.TokenServiceCreateRequest{
			"admin-editor-token": {
				User: new("metal-stack"),
				TokenCreateRequest: &apiv2.TokenServiceCreateRequest{
					AdminRole: apiv2.AdminRole_ADMIN_ROLE_EDITOR.Enum(),
					Expires:   durationpb.New(time.Hour),
				},
			},
			"metal-console": {
				User: new("metal-console"),
				TokenCreateRequest: &apiv2.TokenServiceCreateRequest{
					Permissions: []*apiv2.MethodPermission{
						{
							Subject: "*",
							Methods: []string{
								adminv2connect.MachineServiceGetProcedure,
								apiv2connect.MethodServiceTokenScopedListProcedure,
								apiv2connect.TokenServiceRefreshProcedure,
							},
						},
					},
					Expires: durationpb.New(time.Hour * 48),
				},
			},
		},
	}

	yamlBytes, err := protoyaml.Marshal(tokenCreateRequests)
	require.NoError(t, err)

	require.YAMLEq(t, `tokenCreateRequests:
    admin-editor-token:
        user: metal-stack
        tokenCreateRequest:
            expires: 3600s
            adminRole: ADMIN_ROLE_EDITOR
    metal-console:
        user: metal-console
        tokenCreateRequest:
            permissions:
                - subject: '*'
                  methods:
                    - /metalstack.admin.v2.MachineService/Get
                    - /metalstack.api.v2.MethodService/TokenScopedList
                    - /metalstack.api.v2.TokenService/Refresh
            expires: 172800s
`, string(yamlBytes))
}
