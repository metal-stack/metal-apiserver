package auth

import (
	"context"
	"fmt"
	"log/slog"
	"testing"
	"testing/synctest"
	"time"

	"github.com/lestrrat-go/jwx/v3/jwk"
	"github.com/metal-stack/metal-apiserver/pkg/certs"
	"github.com/metal-stack/metal-apiserver/pkg/repository"
	"github.com/metal-stack/metal-apiserver/pkg/test"
	"github.com/metal-stack/metal-apiserver/pkg/token"
	"github.com/stretchr/testify/require"
)

func Test_jwt_cert_rotation(t *testing.T) {
	t.Parallel()
	oldMaxExpiration := certs.MaxTokenExpiration
	oldDefaultExpiration := token.DefaultExpiration

	certs.MaxTokenExpiration = 5 * time.Second
	token.DefaultExpiration = 5 * time.Second
	defer func() {
		certs.MaxTokenExpiration = oldMaxExpiration
		token.DefaultExpiration = oldDefaultExpiration
	}()
	renewCertBeforeExpiration := 7 * time.Second

	t.Logf("token lifetime: %s, certificate lifetime: %s, issue new signing certificate after: %s", token.DefaultExpiration, 2*certs.MaxTokenExpiration, 2*certs.MaxTokenExpiration-renewCertBeforeExpiration)

	log := slog.Default()

	testStore, closer := test.StartRepositoryWithCleanup(t, log, test.WithRenewCertBeforeExpiration(&renewCertBeforeExpiration))
	defer closer()

	var (
		certStore  = testStore.GetCertStore()
		tokenStore = testStore.GetTokenStore()
	)

	auth := func() *auth {
		o, err := NewAuthenticatorInterceptor(Config{
			Log:            log,
			CertStore:      certStore,
			CertCacheTime:  new(0 * time.Second),
			TokenStore:     tokenStore,
			AllowedIssuers: []string{test.TokenIssuer},
		})
		require.NoError(t, err)

		return o
	}()

	synctest.Test(t, func(t *testing.T) {

		ctx := t.Context()

		var (
			token1     = ""
			token2     = ""
			token3     = ""
			previousAt *time.Duration
		)
		steps := []struct {
			name string
			at   time.Duration
			task func(t *testing.T)
		}{
			{
				name: "token 1",
				at:   0 * time.Second,
				task: func(t *testing.T) {
					token1 = createNewConsoleToken(t, ctx, testStore.Store)
					expectCertStore(t, ctx, certStore, 1)
					expectTokenWorks(t, ctx, auth, token1)
				},
			},
			{
				name: "token2",
				at:   2 * time.Second,
				task: func(t *testing.T) {
					token2 = createNewConsoleToken(t, ctx, testStore.Store)
					expectCertStore(t, ctx, certStore, 1)
					expectTokenWorks(t, ctx, auth, token1)
					expectTokenWorks(t, ctx, auth, token2)
				},
			},
			{
				name: "token3, next signing cert gets created",
				at:   4 * time.Second,
				task: func(t *testing.T) {
					token3 = createNewConsoleToken(t, ctx, testStore.Store)
					expectCertStore(t, ctx, certStore, 2)
					expectTokenWorks(t, ctx, auth, token1)
					expectTokenWorks(t, ctx, auth, token2)
					expectTokenWorks(t, ctx, auth, token3)
				},
			},
			{
				name: "token1 expired, token 2 and 3 still work",
				at:   6 * time.Second,
				task: func(t *testing.T) {
					token3 = createNewConsoleToken(t, ctx, testStore.Store)
					expectCertStore(t, ctx, certStore, 2)
					expectTokenExpired(t, ctx, auth, token1)
					expectTokenWorks(t, ctx, auth, token2)
					expectTokenWorks(t, ctx, auth, token3)
				},
			},
			{
				name: "token1 and token2 expired, token 3 still works",
				at:   8 * time.Second,
				task: func(t *testing.T) {
					expectCertStore(t, ctx, certStore, 2)
					expectTokenExpired(t, ctx, auth, token1)
					expectTokenExpired(t, ctx, auth, token2)
					expectTokenWorks(t, ctx, auth, token3)
				},
			},
			{
				name: "all tokens expired, first signing cert is gone",
				at:   11 * time.Second,
				task: func(t *testing.T) {
					expectCertStore(t, ctx, certStore, 1)
					expectTokenNoPublicKeyForSignatureFound(t, ctx, auth, token1)
					expectTokenNoPublicKeyForSignatureFound(t, ctx, auth, token2)
					expectTokenExpired(t, ctx, auth, token3)
				},
			},
			{
				name: "all tokens expired, all signing certs gone",
				at:   15 * time.Second,
				task: func(t *testing.T) {
					expectCertStore(t, ctx, certStore, 0)
					expectTokenNoPublicKeyForSignatureFound(t, ctx, auth, token1)
					expectTokenNoPublicKeyForSignatureFound(t, ctx, auth, token2)
					expectTokenNoPublicKeyForSignatureFound(t, ctx, auth, token3)
				},
			},
		}

		time.Sleep(1 * time.Second)

		for _, step := range steps {
			forwardText := ""
			if previousAt != nil {
				forward := step.at - *previousAt
				forwardText = fmt.Sprintf(" (forwarding by %s)", forward)
				time.Sleep(forward)
				testStore.GetMiniRedis().FastForward(forward)
			}
			previousAt = &step.at

			t.Logf("%s: running step at %q%s: %q", time.Now(), step.at, forwardText, step.name)

			step.task(t)
		}
	})
}

func createNewConsoleToken(t *testing.T, ctx context.Context, repo *repository.Store) string {
	resp, err := repo.UnscopedToken().AdditionalMethods().CreateUserTokenWithoutPermissionCheck(ctx, "test-user", nil)
	require.NoError(t, err)

	return resp.Secret
}

func expectTokenWorks(t *testing.T, ctx context.Context, auth *auth, bearer string) {
	err := checkToken(ctx, auth, bearer)
	require.NoError(t, err)
}

func expectTokenExpired(t *testing.T, ctx context.Context, auth *auth, bearer string) {
	err := checkToken(ctx, auth, bearer)
	require.Error(t, err)
	require.ErrorContains(t, err, "token has invalid claims: token is expired")
}

func expectTokenNoPublicKeyForSignatureFound(t *testing.T, ctx context.Context, auth *auth, bearer string) {
	err := checkToken(ctx, auth, bearer)
	require.Error(t, err)
	require.ErrorContains(t, err, "no suitable publickey to validate signature found")
}

func checkToken(ctx context.Context, auth *auth, bearer string) error {
	jwtTokenFunc := func(_ string) string {
		return "Bearer " + bearer
	}

	_, err := auth.extractAndValidateJWTToken(ctx, jwtTokenFunc)

	return err
}

func expectCertStore(t *testing.T, ctx context.Context, c certs.CertStore, publicKeyAmount int) {
	_, rawSet, err := c.PublicKeys(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, rawSet)

	set, err := jwk.ParseString(rawSet)
	require.NoError(t, err)
	require.Equal(t, publicKeyAmount, set.Len())
}
