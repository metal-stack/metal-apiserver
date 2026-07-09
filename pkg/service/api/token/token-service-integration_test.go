package token_test

import (
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"buf.build/go/protovalidate"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/metal-stack/api/go/errorutil"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	tokenservice "github.com/metal-stack/metal-apiserver/pkg/service/api/token"
	"github.com/metal-stack/metal-apiserver/pkg/test"
	"github.com/metal-stack/metal-apiserver/pkg/token"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/testing/protocmp"
)

func Test_tokenService_CreateConsoleTokenWithoutPermissionCheck(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	log := slog.Default()

	testStore, closer := test.StartRepositoryWithCleanup(t, log)
	defer closer()

	tokenStore := testStore.GetTokenStore()

	service := tokenservice.New(tokenservice.Config{
		Log:        log,
		TokenStore: tokenStore,
		CertStore:  testStore.GetCertStore(),
		Issuer:     "http://test",
		Repo:       testStore.Store,
	})

	got, err := service.CreateUserTokenWithoutPermissionCheck(ctx, "test", new(1*time.Minute))
	require.NoError(t, err)
	// verifying response

	require.NotNil(t, got)
	require.NotNil(t, got)
	require.NotNil(t, got.GetToken())

	assert.NotEmpty(t, got.GetSecret())
	assert.True(t, strings.HasPrefix(got.GetSecret(), "ey"), "not a valid jwt token") // jwt always starts with "ey" because it's b64 encoded JSON
	claims, err := parseJWTToken(got.GetSecret())
	require.NoError(t, err, "token claims not parsable")
	require.NotNil(t, claims)

	assert.NotEmpty(t, got.GetToken().GetUuid())
	assert.Equal(t, "test", got.GetToken().GetUser())

	// verifying keydb entry
	err = tokenStore.Set(ctx, got.GetToken())
	require.NoError(t, err)

	// listing tokens

	tokenList, err := service.List(token.ContextWithToken(ctx, got.Token), &apiv2.TokenServiceListRequest{})
	require.NoError(t, err)

	require.NotNil(t, tokenList)
	require.NotNil(t, tokenList)

	require.Len(t, tokenList.Tokens, 1)

	// Check still present
	_, err = tokenStore.Get(ctx, got.GetToken().GetUser(), got.GetToken().GetUuid())
	require.NoError(t, err)

	// Check unpresent after revocation
	err = tokenStore.Revoke(ctx, got.GetToken().GetUser(), got.GetToken().GetUuid())
	require.NoError(t, err)

	_, err = tokenStore.Get(ctx, got.GetToken().GetUser(), got.GetToken().GetUuid())
	require.Error(t, err)

	// List must now be empty
	tokenList, err = service.List(token.ContextWithToken(ctx, got.Token), &apiv2.TokenServiceListRequest{})
	require.NoError(t, err)

	require.NotNil(t, tokenList)
	require.NotNil(t, tokenList)
	require.Empty(t, tokenList.Tokens)
}

// parseJWTToken unverified to Claims to get Issuer,Subject, Roles and Permissions
func parseJWTToken(tokenString string) (*token.Claims, error) {
	if tokenString == "" {
		return nil, nil
	}

	claims := &token.Claims{}
	parser := jwt.NewParser()
	_, _, err := parser.ParseUnverified(string(tokenString), claims)

	if err != nil {
		return nil, err
	}

	return claims, nil
}

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
		req            *apiv2.TokenServiceListRequest
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
			req:   &apiv2.TokenServiceListRequest{},
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
			req: &apiv2.TokenServiceListRequest{},
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
				{
					Uuid: "8ff27ee2-209f-43e2-a15d-50143fb03229",
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
			req: &apiv2.TokenServiceListRequest{
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
			req: &apiv2.TokenServiceListRequest{
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
			name: "query user (does not see other users)",
			sessionToken: &apiv2.Token{
				User:         "phippy",
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
			},
			req: &apiv2.TokenServiceListRequest{
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
			want: nil,
		},
		{
			name: "query token type",
			sessionToken: &apiv2.Token{
				User:         "phippy",
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
			},
			req: &apiv2.TokenServiceListRequest{
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
					cmpopts.SortSlices(func(a, b *apiv2.Token) bool {
						return a.Uuid < b.Uuid
					}),
					protocmp.Transform(),
					protocmp.IgnoreFields(
						&apiv2.Token{}, "issued_at", "expires",
					),
					protocmp.IgnoreFields(
						&apiv2.Meta{}, "created_at", "updated_at",
					),
				); diff != "" {
					innerT.Errorf("diff: %s", diff)
				}
			}
		})
	}
}

func Test_Get(t *testing.T) {
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
		req               *apiv2.TokenServiceGetRequest
		state             state
		wantErr           error
		wantValidationErr string
		want              *apiv2.Token
	}{
		{
			name: "missing id in request",
			sessionToken: &apiv2.Token{
				User:         "phippy",
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
			},
			req: &apiv2.TokenServiceGetRequest{},
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
			want:              nil,
			wantValidationErr: "validation error: uuid: value is empty, which is not a valid UUID",
		},
		{
			name: "get non-existing",
			sessionToken: &apiv2.Token{
				User:         "phippy",
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
			},
			req: &apiv2.TokenServiceGetRequest{
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
			wantErr: errorutil.NotFound("token not found"),
		},
		{
			name: "cannot get another user's token",
			sessionToken: &apiv2.Token{
				User:         "phippy",
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
			},
			req: &apiv2.TokenServiceGetRequest{
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
			wantErr: errorutil.NotFound("token not found"),
		},
		{
			name: "get existing",
			sessionToken: &apiv2.Token{
				User:         "phippy",
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
			},
			req: &apiv2.TokenServiceGetRequest{
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
			want: &apiv2.Token{
				Uuid: "8ff27ee2-209f-43e2-a15d-50143fb03229",
				User: "phippy",
				Meta: &apiv2.Meta{},
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

			response, err := service.Get(ctx, tt.req)

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
				if diff := cmp.Diff(
					tt.want, response.Token,
					protocmp.Transform(),
					protocmp.IgnoreFields(
						&apiv2.Token{}, "issued_at", "expires",
					),
					protocmp.IgnoreFields(
						&apiv2.Meta{}, "created_at", "updated_at",
					),
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
		req               *apiv2.TokenServiceRevokeRequest
		state             state
		wantErr           error
		wantValidationErr string
		wantRemaining     []*apiv2.Token
	}{
		{
			name: "missing id in request",
			sessionToken: &apiv2.Token{
				User:         "phippy",
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
			},
			req: &apiv2.TokenServiceRevokeRequest{},
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
			wantValidationErr: "validation error: uuid: value is empty, which is not a valid UUID",
		},
		{
			name: "revoke non-existing",
			sessionToken: &apiv2.Token{
				User:         "phippy",
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
			},
			req: &apiv2.TokenServiceRevokeRequest{
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
			name: "cannot revoke another user's token",
			sessionToken: &apiv2.Token{
				User:         "phippy",
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
			},
			req: &apiv2.TokenServiceRevokeRequest{
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
				{
					Uuid: "9baa8668-2212-4fa5-a2e4-167084d0552d",
					User: "not phippy",
					Meta: &apiv2.Meta{},
				},
			},
			wantErr: errorutil.NotFound("token not found"),
		},
		{
			name: "revoke existing",
			sessionToken: &apiv2.Token{
				User:         "phippy",
				Permissions:  []*apiv2.MethodPermission{},
				ProjectRoles: map[string]apiv2.ProjectRole{},
				TenantRoles:  map[string]apiv2.TenantRole{},
			},
			req: &apiv2.TokenServiceRevokeRequest{
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
