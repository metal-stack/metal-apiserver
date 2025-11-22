package auth

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"
	v2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/certs"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	"github.com/metal-stack/metal-apiserver/pkg/repository"
	"github.com/metal-stack/metal-apiserver/pkg/token"
	"github.com/metal-stack/metal-lib/pkg/pointer"
	"github.com/metal-stack/metal-lib/pkg/testcommon"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func prepare(t *testing.T) (certs.CertStore, *ecdsa.PrivateKey) {
	s := miniredis.RunT(t)
	c := redis.NewClient(&redis.Options{Addr: s.Addr()})

	// creating an initial signing certificate
	store := certs.NewRedisStore(&certs.Config{
		RedisClient: c,
	})
	_, err := store.LatestPrivate(t.Context())
	require.NoError(t, err)

	key, err := store.LatestPrivate(t.Context())
	require.NoError(t, err)

	return store, key
}

func Test_authorize_with_permissions(t *testing.T) {
	pk, err := ecdsa.GenerateKey(elliptic.P521(), rand.Reader)
	require.NoError(t, err)
	var (
		expired             = -time.Hour
		certStore, key      = prepare(t)
		defaultIssuer       = "https://api-server"
		maliciousSigningKey = pk
	)

	tests := []struct {
		name               string
		subject            string
		permissions        []*v2.MethodPermission
		projectRoles       map[string]v2.ProjectRole
		tenantRoles        map[string]v2.TenantRole
		adminRole          *v2.AdminRole
		userJwtMutateFn    func(t *testing.T, jwt string) string
		expiration         *time.Duration
		req                any
		projectsAndTenants *repository.ProjectsAndTenants
		tokenType          v2.TokenType
		wantErr            error
	}{
		{
			name: "machine get not allowed, token signed with invalid private key",
			req:  v2.MachineServiceGetRequest{},
			userJwtMutateFn: func(t *testing.T, _ string) string {
				jwt := generateJWT(t, "", defaultIssuer, maliciousSigningKey, time.Now().Add(time.Hour), time.Now(), time.Now())

				require.NoError(t, err)
				return jwt
			},
			wantErr: errorutil.Unauthenticated("token signature is invalid: crypto/ecdsa: verification error"),
		},
		{
			name: "machine get not allowed, token used before not before date",
			req:  v2.MachineServiceGetRequest{},
			userJwtMutateFn: func(t *testing.T, _ string) string {
				jwt := generateJWT(t, "", defaultIssuer, key, time.Now().Add(time.Hour), time.Now(), time.Now().Add(time.Hour))

				require.NoError(t, err)
				return jwt
			},
			wantErr: errorutil.Unauthenticated("token has invalid claims: token is not valid yet"),
		},
		{
			name:       "machine get not allowed, token already expired",
			subject:    "john.doe@github",
			req:        v2.MachineServiceGetRequest{},
			expiration: &expired,
			permissions: []*v2.MethodPermission{
				{
					Subject: "john.doe@github",
					Methods: []string{"/metalstack.api.v2.IPService/Get"},
				},
			},
			wantErr: errorutil.Unauthenticated("token has invalid claims: token is expired"),
		},
		{
			name:    "token service malformed token",
			subject: "john.doe@github",
			req:     v2.TokenServiceCreateRequest{},
			userJwtMutateFn: func(_ *testing.T, jwt string) string {
				return jwt + "foo"
			},
			tenantRoles: map[string]v2.TenantRole{
				"john.doe@github": v2.TenantRole_TENANT_ROLE_OWNER,
			},
			wantErr: errorutil.Unauthenticated("token signature is invalid: crypto/ecdsa: verification error"),
		},
		{
			name:    "token service untrusted issuer",
			subject: "john.doe@github",
			req:     v2.TokenServiceCreateRequest{},
			userJwtMutateFn: func(t *testing.T, _ string) string {
				jwt := generateJWT(t, "", "unknown-issuer", key, time.Now().Add(time.Hour), time.Now(), time.Now())
				require.NoError(t, err)
				return jwt
			},
			tenantRoles: map[string]v2.TenantRole{
				"john.doe@github": v2.TenantRole_TENANT_ROLE_OWNER,
			},
			wantErr: errorutil.Unauthenticated("invalid token issuer: unknown-issuer"),
		},
		{
			name:    "token service allowed",
			subject: "john.doe@github",
			req:     v2.TokenServiceCreateRequest{},
			tenantRoles: map[string]v2.TenantRole{
				"john.doe@github": v2.TenantRole_TENANT_ROLE_OWNER,
			},
			wantErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := miniredis.RunT(t)
			defer s.Close()

			ctx := t.Context()
			tokenStore := token.NewRedisStore(redis.NewClient(&redis.Options{Addr: s.Addr()}))

			exp := time.Hour
			if tt.expiration != nil {
				exp = *tt.expiration
			}

			tokenType := v2.TokenType_TOKEN_TYPE_API
			if tt.tokenType != v2.TokenType_TOKEN_TYPE_UNSPECIFIED {
				tokenType = tt.tokenType
			}

			jwt, tok, err := token.NewJWT(tokenType, tt.subject, defaultIssuer, exp, key)
			require.NoError(t, err)

			err = tokenStore.Set(ctx, tok)
			require.NoError(t, err)

			o, err := NewAuthenticatorInterceptor(Config{
				Log:            slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug})),
				CertStore:      certStore,
				CertCacheTime:  pointer.Pointer(0 * time.Second),
				TokenStore:     tokenStore,
				AllowedIssuers: []string{defaultIssuer},
			})
			require.NoError(t, err)

			if tt.userJwtMutateFn != nil {
				jwt = tt.userJwtMutateFn(t, jwt)
			}

			jwtTokenFunc := func(_ string) string {
				return "Bearer " + jwt
			}

			_, err = o.extractAndValidateJWTToken(ctx, jwtTokenFunc)
			if diff := cmp.Diff(tt.wantErr, err, testcommon.ErrorStringComparer()); diff != "" {
				t.Errorf("error diff (+got -want):\n %s", diff)
			}
		})
	}
}

func generateJWT(t *testing.T, subject, issuer string, secret crypto.PrivateKey, expiresAt, issuedAt, notBefore time.Time) string {
	claims := &jwt.RegisteredClaims{
		ExpiresAt: jwt.NewNumericDate(expiresAt),
		IssuedAt:  jwt.NewNumericDate(issuedAt),
		NotBefore: jwt.NewNumericDate(notBefore),

		// ID is for your traceability, doesn't have to be UUID:
		ID: uuid.New().String(),

		// put name/title/ID of whoever will be using this JWT here:
		Subject: subject,
		Issuer:  issuer,
	}

	jwtWithClaims := jwt.NewWithClaims(jwt.SigningMethodES512, claims)
	var (
		jwt string
		err error
	)
	if secret != nil {
		jwt, err = jwtWithClaims.SignedString(secret)
		require.NoError(t, err)
	} else {
		jwt = jwtWithClaims.Raw
	}
	return jwt
}
