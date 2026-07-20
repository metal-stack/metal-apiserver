package admin_test

import (
	"context"
	"log/slog"
	"testing"

	"buf.build/go/protovalidate"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/metal-stack/api/go/errorutil"
	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/repository/api"
	tokenservice "github.com/metal-stack/metal-apiserver/pkg/service/admin/token"
	"github.com/metal-stack/metal-apiserver/pkg/test"
	"github.com/metal-stack/metal-apiserver/pkg/token"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/testing/protocmp"
)

func Test_List(t *testing.T) {
	t.Parallel()

	log := slog.Default()

	testStore, closer := test.StartRepositoryWithCleanup(t, log)
	defer closer()

	type state struct {
		existingTokens []*apiv2.Token
	}
	tests := []struct {
		name           string
		sessionToken   *apiv2.Token
		req            *adminv2.TokenServiceListRequest
		state          state
		wantErr        bool
		wantErrMessage string
		want           []*apiv2.Token
	}{
		{
			name: "no tokens",
			sessionToken: &apiv2.Token{
				User:         "phippy",
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
			},
			req:   &adminv2.TokenServiceListRequest{},
			state: state{},
			want:  nil,
		},
		{
			name: "list tokens",
			sessionToken: &apiv2.Token{
				User:         "phippy",
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
			},
			req: &adminv2.TokenServiceListRequest{},
			state: state{
				existingTokens: []*apiv2.Token{
					{
						Uuid: "c223af4d-b3f5-4df6-8815-52b80323930d",
						User: "phippy",
					},
					{
						Uuid: "8ff27ee2-209f-43e2-a15d-50143fb03229",
						User: "phippy",
					},
					{
						Uuid: "9baa8668-2212-4fa5-a2e4-167084d0552d",
						User: "not phippy",
					},
				},
			},
			want: []*apiv2.Token{
				{
					Uuid: "9baa8668-2212-4fa5-a2e4-167084d0552d",
					User: "not phippy",
					Meta: &apiv2.Meta{},
				},
				{
					Uuid: "8ff27ee2-209f-43e2-a15d-50143fb03229",
					User: "phippy",
					Meta: &apiv2.Meta{},
				},
				{
					Uuid: "c223af4d-b3f5-4df6-8815-52b80323930d",
					User: "phippy",
					Meta: &apiv2.Meta{},
				},
			},
		},
		{
			name: "query uuid",
			sessionToken: &apiv2.Token{
				User:         "phippy",
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
			},
			req: &adminv2.TokenServiceListRequest{
				Query: &apiv2.TokenQuery{
					Uuid: new("c223af4d-b3f5-4df6-8815-52b80323930d"),
				},
			},
			state: state{
				existingTokens: []*apiv2.Token{
					{
						Uuid: "c223af4d-b3f5-4df6-8815-52b80323930d",
						User: "phippy",
					},
					{
						Uuid: "8ff27ee2-209f-43e2-a15d-50143fb03229",
						User: "phippy",
					},
					{
						Uuid: "9baa8668-2212-4fa5-a2e4-167084d0552d",
						User: "not phippy",
					},
				},
			},
			want: []*apiv2.Token{
				{
					Uuid: "c223af4d-b3f5-4df6-8815-52b80323930d",
					User: "phippy",
					Meta: &apiv2.Meta{},
				},
			},
		},
		{
			name: "query description and labels",
			sessionToken: &apiv2.Token{
				User:         "phippy",
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
			},
			req: &adminv2.TokenServiceListRequest{
				Query: &apiv2.TokenQuery{
					Labels: &apiv2.Labels{
						Labels: map[string]string{
							"a": "b",
							"c": "d",
						},
					},
					Description: new("test"),
				},
			},
			state: state{
				existingTokens: []*apiv2.Token{
					{
						Uuid:        "c223af4d-b3f5-4df6-8815-52b80323930d",
						User:        "phippy",
						Description: "test",
						Meta: &apiv2.Meta{
							Labels: &apiv2.Labels{
								Labels: map[string]string{
									"c": "d",
									"a": "b",
								},
							},
						},
					},
					{
						Uuid:        "8ff27ee2-209f-43e2-a15d-50143fb03229",
						User:        "phippy",
						Description: "nope",
						Meta: &apiv2.Meta{
							Labels: &apiv2.Labels{
								Labels: map[string]string{
									"a": "b",
									"c": "d",
								},
							},
						},
					},
					{
						Uuid:        "9baa8668-2212-4fa5-a2e4-167084d0552d",
						User:        "phippy",
						Description: "test",
						Meta: &apiv2.Meta{
							Labels: &apiv2.Labels{
								Labels: map[string]string{
									"a": "b",
									"c": "nope",
								},
							},
						},
					},
				},
			},
			want: []*apiv2.Token{
				{
					Uuid:        "c223af4d-b3f5-4df6-8815-52b80323930d",
					User:        "phippy",
					Description: "test",
					Meta: &apiv2.Meta{
						Labels: &apiv2.Labels{
							Labels: map[string]string{
								"a": "b",
								"c": "d",
							},
						},
					},
				},
			},
		},
		{
			name: "query user",
			sessionToken: &apiv2.Token{
				User:         "phippy",
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
			},
			req: &adminv2.TokenServiceListRequest{
				Query: &apiv2.TokenQuery{
					User: new("not phippy"),
				},
			},
			state: state{
				existingTokens: []*apiv2.Token{
					{
						Uuid: "c223af4d-b3f5-4df6-8815-52b80323930d",
						User: "phippy",
					},
					{
						Uuid: "8ff27ee2-209f-43e2-a15d-50143fb03229",
						User: "phippy",
					},
					{
						Uuid: "9baa8668-2212-4fa5-a2e4-167084d0552d",
						User: "not phippy",
					},
				},
			},
			want: []*apiv2.Token{
				{
					Uuid: "9baa8668-2212-4fa5-a2e4-167084d0552d",
					User: "not phippy",
					Meta: &apiv2.Meta{},
				},
			},
		},
		{
			name: "query token type",
			sessionToken: &apiv2.Token{
				User:         "phippy",
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
			},
			req: &adminv2.TokenServiceListRequest{
				Query: &apiv2.TokenQuery{
					TokenType: new(apiv2.TokenType_TOKEN_TYPE_API),
				},
			},
			state: state{
				existingTokens: []*apiv2.Token{
					{
						Uuid:      "c223af4d-b3f5-4df6-8815-52b80323930d",
						User:      "phippy",
						TokenType: apiv2.TokenType_TOKEN_TYPE_API,
					},
					{
						Uuid:      "8ff27ee2-209f-43e2-a15d-50143fb03229",
						User:      "phippy",
						TokenType: apiv2.TokenType_TOKEN_TYPE_USER,
					},
				},
			},
			want: []*apiv2.Token{
				{
					Uuid:      "c223af4d-b3f5-4df6-8815-52b80323930d",
					User:      "phippy",
					TokenType: apiv2.TokenType_TOKEN_TYPE_API,
					Meta:      &apiv2.Meta{},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(innerT *testing.T) {
			defer testStore.Cleanup(t)

			ctx, cancel := context.WithCancel(token.ContextWithToken(innerT.Context(), tt.sessionToken))
			defer cancel()

			for _, tok := range tt.state.existingTokens {
				err := testStore.GetTokenStore().Set(ctx, tok)
				require.NoError(innerT, err)
			}

			service := tokenservice.New(tokenservice.Config{
				Log:  log,
				Repo: testStore.Store,
			})

			if tt.wantErr == false {
				// Execute proto based validation
				err := protovalidate.Validate(tt.req)
				require.NoError(innerT, err)
			}

			response, err := service.List(ctx, tt.req)

			switch {
			case tt.wantErr && err != nil:
				if diff := cmp.Diff(tt.wantErrMessage, err.Error()); diff != "" {
					innerT.Errorf("diff = %s", diff)
				}

			case tt.wantErr && err == nil:
				innerT.Fatalf("want error %q, got response %q", tt.wantErrMessage, response)
			case err != nil:
				innerT.Fatalf("want response, got error %q", err)

			default:
				if diff := cmp.Diff(
					tt.want, response.Tokens,
					protocmp.Transform(),
					protocmp.IgnoreFields(
						&apiv2.Token{}, "issued_at", "expires",
					),
					protocmp.IgnoreFields(
						&apiv2.Meta{}, "created_at", "updated_at", "generation",
					),
					cmpopts.SortSlices(func(a, b *apiv2.Token) bool {
						return a.Uuid < b.Uuid
					}),
				); diff != "" {
					innerT.Errorf("diff: %s", diff)
				}
			}
		})
	}
}

func Test_Revoke(t *testing.T) {
	t.Parallel()

	log := slog.Default()

	testStore, closer := test.StartRepositoryWithCleanup(t, log)
	defer closer()

	type state struct {
		existingTokens []*apiv2.Token
	}
	tests := []struct {
		name              string
		sessionToken      *apiv2.Token
		req               *adminv2.TokenServiceRevokeRequest
		state             state
		wantErr           error
		wantValidationErr string
		wantRemaining     []*apiv2.Token
	}{
		{
			name: "missing user and id in request",
			sessionToken: &apiv2.Token{
				User:         "phippy",
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
			},
			req: &adminv2.TokenServiceRevokeRequest{},
			state: state{
				existingTokens: []*apiv2.Token{
					{
						Uuid: "c223af4d-b3f5-4df6-8815-52b80323930d",
						User: "phippy",
					},
					{
						Uuid: "8ff27ee2-209f-43e2-a15d-50143fb03229",
						User: "phippy",
					},
					{
						Uuid: "9baa8668-2212-4fa5-a2e4-167084d0552d",
						User: "not phippy",
					},
				},
			},
			wantRemaining: []*apiv2.Token{
				{
					Uuid: "c223af4d-b3f5-4df6-8815-52b80323930d",
					User: "phippy",
					Meta: &apiv2.Meta{},
				},
				{
					Uuid: "8ff27ee2-209f-43e2-a15d-50143fb03229",
					User: "phippy",
					Meta: &apiv2.Meta{},
				},
				{
					Uuid: "9baa8668-2212-4fa5-a2e4-167084d0552d",
					User: "not phippy",
					Meta: &apiv2.Meta{},
				},
			},
			wantValidationErr: `validation errors:
 - uuid: value is empty, which is not a valid UUID
 - user: must be within 2 and 512 characters`,
		},
		{
			name: "revoke non-existing",
			sessionToken: &apiv2.Token{
				User:         "phippy",
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
			},
			req: &adminv2.TokenServiceRevokeRequest{
				User: "phippy",
				Uuid: "57460ff2-30e9-45e5-93c8-7f9ca85a92af",
			},
			state: state{
				existingTokens: []*apiv2.Token{
					{
						Uuid: "c223af4d-b3f5-4df6-8815-52b80323930d",
						User: "phippy",
					},
					{
						Uuid: "8ff27ee2-209f-43e2-a15d-50143fb03229",
						User: "phippy",
					},
					{
						Uuid: "9baa8668-2212-4fa5-a2e4-167084d0552d",
						User: "not phippy",
					},
				},
			},
			wantRemaining: []*apiv2.Token{
				{
					Uuid: "c223af4d-b3f5-4df6-8815-52b80323930d",
					User: "phippy",
					Meta: &apiv2.Meta{},
				},
				{
					Uuid: "8ff27ee2-209f-43e2-a15d-50143fb03229",
					User: "phippy",
					Meta: &apiv2.Meta{},
				},
				{
					Uuid: "9baa8668-2212-4fa5-a2e4-167084d0552d",
					User: "not phippy",
					Meta: &apiv2.Meta{},
				},
			},
			wantErr: errorutil.NotFound("token not found"),
		},
		{
			name: "admin can revoke another user's token",
			sessionToken: &apiv2.Token{
				User:         "phippy",
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
			},
			req: &adminv2.TokenServiceRevokeRequest{
				User: "not phippy",
				Uuid: "9baa8668-2212-4fa5-a2e4-167084d0552d",
			},
			state: state{
				existingTokens: []*apiv2.Token{
					{
						Uuid: "c223af4d-b3f5-4df6-8815-52b80323930d",
						User: "phippy",
					},
					{
						Uuid: "8ff27ee2-209f-43e2-a15d-50143fb03229",
						User: "phippy",
					},
					{
						Uuid: "9baa8668-2212-4fa5-a2e4-167084d0552d",
						User: "not phippy",
					},
				},
			},
			wantRemaining: []*apiv2.Token{
				{
					Uuid: "c223af4d-b3f5-4df6-8815-52b80323930d",
					User: "phippy",
					Meta: &apiv2.Meta{},
				},
				{
					Uuid: "8ff27ee2-209f-43e2-a15d-50143fb03229",
					User: "phippy",
					Meta: &apiv2.Meta{},
				},
			},
		},
		{
			name: "revoke",
			sessionToken: &apiv2.Token{
				User:         "phippy",
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
			},
			req: &adminv2.TokenServiceRevokeRequest{
				User: "phippy",
				Uuid: "8ff27ee2-209f-43e2-a15d-50143fb03229",
			},
			state: state{
				existingTokens: []*apiv2.Token{
					{
						Uuid: "c223af4d-b3f5-4df6-8815-52b80323930d",
						User: "phippy",
					},
					{
						Uuid: "8ff27ee2-209f-43e2-a15d-50143fb03229",
						User: "phippy",
					},
					{
						Uuid: "9baa8668-2212-4fa5-a2e4-167084d0552d",
						User: "not phippy",
					},
				},
			},
			wantRemaining: []*apiv2.Token{
				{
					Uuid: "c223af4d-b3f5-4df6-8815-52b80323930d",
					User: "phippy",
					Meta: &apiv2.Meta{},
				},
				{
					Uuid: "9baa8668-2212-4fa5-a2e4-167084d0552d",
					User: "not phippy",
					Meta: &apiv2.Meta{},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(innerT *testing.T) {
			defer testStore.Cleanup(t)

			ctx, cancel := context.WithCancel(token.ContextWithToken(innerT.Context(), tt.sessionToken))
			defer cancel()

			for _, tok := range tt.state.existingTokens {
				err := testStore.GetTokenStore().Set(ctx, tok)
				require.NoError(innerT, err)
			}

			service := tokenservice.New(tokenservice.Config{
				Log:  log,
				Repo: testStore.Store,
			})

			// Execute proto based validation
			err := protovalidate.Validate(tt.req)
			if tt.wantValidationErr != "" {
				if diff := cmp.Diff(tt.wantValidationErr, err.Error(), errorutil.ErrorStringComparer()); diff != "" {
					innerT.Errorf("diff = %s", diff)
				}

				return
			}

			require.NoError(innerT, err)

			response, err := service.Revoke(ctx, tt.req)

			switch {
			case tt.wantErr != nil && err != nil:
				if diff := cmp.Diff(tt.wantErr, err, errorutil.ErrorStringComparer()); diff != "" {
					innerT.Errorf("diff = %s", diff)
				}

			case tt.wantErr != nil && err == nil:
				innerT.Fatalf("want error %q, got response %q", tt.wantErr, response)
			case err != nil:
				innerT.Fatalf("want response, got error %q", err)

			default:
				remaining, err := testStore.GetTokenStore().AdminList(ctx)
				require.NoError(innerT, err)

				if diff := cmp.Diff(
					tt.wantRemaining, remaining,
					protocmp.Transform(),
					protocmp.IgnoreFields(
						&apiv2.Token{}, "issued_at", "expires",
					),
					protocmp.IgnoreFields(
						&apiv2.Meta{}, "created_at", "updated_at",
					),
					cmpopts.SortSlices(func(a, b *apiv2.Token) bool {
						return a.Uuid < b.Uuid
					}),
				); diff != "" {
					innerT.Errorf("diff: %s", diff)
				}
			}
		})
	}
}

func Test_Create(t *testing.T) {
	t.Parallel()

	log := slog.Default()

	testStore, closer := test.StartRepositoryWithCleanup(t, log, test.WithValkey(true), test.WithPostgres(true))
	defer closer()

	type state struct {
		providerTenant string
		projectRoles   map[string]apiv2.ProjectRole
		tenantRoles    map[string]apiv2.TenantRole
	}
	tests := []struct {
		name           string
		sessionToken   *apiv2.Token
		req            *adminv2.TokenServiceCreateRequest
		state          state
		wantErr        bool
		wantErrMessage string
		wantToken      *apiv2.Token
	}{
		{
			name: "phippy can create token for user foo",
			sessionToken: &apiv2.Token{
				User:         "phippy",
				TokenType:    apiv2.TokenType_TOKEN_TYPE_API,
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
			},
			req: &adminv2.TokenServiceCreateRequest{
				TokenCreateRequest: &apiv2.TokenServiceCreateRequest{
					Description: "empty token",
				},
				User: new("foo"),
			},
			state: state{
				providerTenant: test.DefaultProviderTenant,
				tenantRoles: map[string]apiv2.TenantRole{
					test.DefaultProviderTenant: apiv2.TenantRole_TENANT_ROLE_OWNER,
				},
			},
			wantToken: &apiv2.Token{
				User:        "foo",
				Description: "empty token",
				TokenType:   apiv2.TokenType_TOKEN_TYPE_API,
				Meta:        &apiv2.Meta{},
			},
		},
		{
			name: "pixie-core can create token for metal-hammer with machine roles",
			sessionToken: &apiv2.Token{
				User:         "pixie-core",
				TokenType:    apiv2.TokenType_TOKEN_TYPE_API,
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
				MachineRoles: map[string]apiv2.MachineRole{
					"de240964-ff9f-4e3d-95b2-8a96e43788f1": apiv2.MachineRole_MACHINE_ROLE_EDITOR,
				},
			},
			req: &adminv2.TokenServiceCreateRequest{
				TokenCreateRequest: &apiv2.TokenServiceCreateRequest{
					Description: "machine token",
					MachineRoles: map[string]apiv2.MachineRole{
						"de240964-ff9f-4e3d-95b2-8a96e43788f1": apiv2.MachineRole_MACHINE_ROLE_EDITOR,
					},
				},
				User: new("metal-hammer"),
			},
			state: state{
				providerTenant: test.DefaultProviderTenant,
				tenantRoles: map[string]apiv2.TenantRole{
					test.DefaultProviderTenant: apiv2.TenantRole_TENANT_ROLE_OWNER,
				},
			},
			wantToken: &apiv2.Token{
				User:        "metal-hammer",
				Description: "machine token",
				TokenType:   apiv2.TokenType_TOKEN_TYPE_API,
				MachineRoles: map[string]apiv2.MachineRole{
					"de240964-ff9f-4e3d-95b2-8a96e43788f1": apiv2.MachineRole_MACHINE_ROLE_EDITOR,
				},
				Meta: &apiv2.Meta{},
			},
		},
		{
			name: "bar can not create token for user foo",
			sessionToken: &apiv2.Token{
				User:         "bar",
				TokenType:    apiv2.TokenType_TOKEN_TYPE_API,
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
			},
			req: &adminv2.TokenServiceCreateRequest{
				TokenCreateRequest: &apiv2.TokenServiceCreateRequest{
					Description: "empty token",
				},
				User: new("foo"),
			},
			state: state{
				providerTenant: "phippy",
			},
			wantToken:      nil,
			wantErr:        true,
			wantErrMessage: "permission_denied: only admins can specify token user",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(innerT *testing.T) {
			defer testStore.Cleanup(t)

			ctx, cancel := context.WithCancel(token.ContextWithToken(innerT.Context(), tt.sessionToken))
			defer cancel()

			test.CreateTenants(innerT, testStore, []*apiv2.TenantServiceCreateRequest{
				{
					Name: tt.sessionToken.User,
				},
			})
			test.CreateTenantMemberships(innerT, testStore, tt.sessionToken.User, []*api.TenantMemberCreateRequest{
				{
					MemberID: tt.sessionToken.User,
					Role:     apiv2.TenantRole_TENANT_ROLE_OWNER,
				},
			})

			for id, perm := range tt.state.tenantRoles {
				if id != tt.sessionToken.User {
					test.CreateTenants(innerT, testStore, []*apiv2.TenantServiceCreateRequest{
						{
							Name: id,
						},
					})
				}
				test.CreateTenantMemberships(innerT, testStore, id, []*api.TenantMemberCreateRequest{
					{
						MemberID: tt.sessionToken.User,
						Role:     perm,
					},
				})
			}

			for id, perm := range tt.state.projectRoles {
				test.CreateProjects(innerT, testStore, []*apiv2.ProjectServiceCreateRequest{
					{
						Login: tt.sessionToken.User,
						Name:  id,
					},
				})
				test.CreateProjectMemberships(innerT, testStore, id, []*api.ProjectMemberCreateRequest{
					{
						TenantId: tt.sessionToken.User,
						Role:     perm,
					},
				})
			}
			service := tokenservice.New(tokenservice.Config{
				Log:  log,
				Repo: testStore.Store,
			})

			if tt.wantErr == false {
				// Execute proto based validation
				err := protovalidate.Validate(tt.req)
				require.NoError(t, err)
			}

			response, err := service.Create(ctx, tt.req)
			switch {
			case tt.wantErr && err != nil:
				if dff := cmp.Diff(tt.wantErrMessage, err.Error()); dff != "" {
					t.Fatal(dff)
				}
			case tt.wantErr && err == nil:
				t.Fatalf("want error %q, got response %q", tt.wantErrMessage, response)
			case err != nil:
				t.Fatalf("want response, got error %q", err)

			default:
				if response.Secret == "" {
					t.Error("response secret for token may not be empty")
				}
				require.NotNil(t, tt.wantToken, "token returned, nil expected")

				if diff := cmp.Diff(
					tt.wantToken, response.Token,
					protocmp.Transform(),
					protocmp.IgnoreFields(
						&apiv2.Token{}, "issued_at", "expires", "uuid",
					),
					protocmp.IgnoreFields(
						&apiv2.Meta{}, "created_at", "updated_at", "generation",
					),
				); diff != "" {
					innerT.Errorf("diff: %s", diff)
				}
			}
		})
	}
}
