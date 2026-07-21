package main

import (
	"testing"
	"time"

	"buf.build/go/protoyaml"
	"github.com/google/go-cmp/cmp"
	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/api/go/metalstack/api/v2/apiv2connect"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/durationpb"
)

func TestTokenCreateConfigTest(t *testing.T) {
	tokens := map[string]*adminv2.TokenServiceCreateRequest{
		"admin_editor_token": {
			User: new("metal-stack"),
			TokenCreateRequest: &apiv2.TokenServiceCreateRequest{
				AdminRole: apiv2.AdminRole_ADMIN_ROLE_EDITOR.Enum(),
				Expires:   durationpb.New(time.Hour),
			},
		},
		"metal_console": {
			User: new("metal-console"),
			TokenCreateRequest: &apiv2.TokenServiceCreateRequest{
				Permissions: []*apiv2.PermissionsByVisibility{
					{
						Visibility: &apiv2.PermissionsByVisibility_Self{
							Self: &apiv2.SelfPermissions{
								Methods: []string{
									apiv2connect.MethodServiceTokenScopedListProcedure,
									apiv2connect.TokenServiceRefreshProcedure,
								},
							},
						},
					},
				},
				Expires: durationpb.New(time.Hour * 48),
			},
		},
	}

	userInput := `
admin_editor_token:
    user: metal-stack
    tokenCreateRequest:
        expires: 3600s
        adminRole: ADMIN_ROLE_EDITOR
metal_console:
    user: metal-console
    tokenCreateRequest:
        permissions:
            - self:
                methods:
                  - /metalstack.api.v2.MethodService/TokenScopedList
                  - /metalstack.api.v2.TokenService/Refresh
        expires: 172800s
    `

	var config map[string]any
	err := yaml.Unmarshal([]byte(userInput), &config)
	require.NoError(t, err)

	for k, v := range config {
		unmodifiedProtoBytes, err := yaml.Marshal(v)
		require.NoError(t, err)

		var tcr adminv2.TokenServiceCreateRequest
		err = protoyaml.Unmarshal(unmodifiedProtoBytes, &tcr)
		require.NoError(t, err)

		if diff := cmp.Diff(tokens[k], &tcr, protocmp.Transform()); diff != "" {
			t.Errorf("failed: %s", diff)
		}
	}
}
