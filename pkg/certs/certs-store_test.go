package certs_test

import (
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/lestrrat-go/jwx/v3/jwk"
	"github.com/metal-stack/metal-apiserver/pkg/certs"
	"github.com/metal-stack/metal-apiserver/pkg/token"
	"github.com/stretchr/testify/require"
	"github.com/valkey-io/valkey-go"
)

func Test_certStore(t *testing.T) {
	t.Parallel()
	var (
		ctx = t.Context()
		s   = miniredis.RunT(t)
	)

	c, err := valkey.NewClient(valkey.ClientOption{
		InitAddress: []string{s.Addr()},
		// This is required because otherwise we get:
		// unknown subcommand 'TRACKING'. Try CLIENT HELP.: [CLIENT TRACKING ON OPTIN]
		// ClientOption.DisableCache must be true for valkey not supporting client-side caching or not supporting RESP3
		DisableCache: true,
	})
	require.NoError(t, err)

	store := certs.NewRedisStore(&certs.Config{
		RenewCertBeforeExpiration: new(4 * token.MaxExpiration),
		ValkeyClient:              c,
	})

	set, rawSet, err := store.PublicKeys(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, rawSet)
	require.Equal(t, 0, set.Len())

	privateKey, err := store.LatestPrivate(ctx)
	require.NoError(t, err)
	require.NotNil(t, privateKey)

	set, rawSet, err = store.PublicKeys(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, rawSet)
	require.Equal(t, 1, set.Len())

	firstKey, err := jwk.Import(privateKey)
	require.NoError(t, err)

	rotatedPrivateKey, err := store.LatestPrivate(ctx)
	require.NoError(t, err)
	require.NotNil(t, rotatedPrivateKey)
	require.False(t, privateKey.Equal(rotatedPrivateKey))

	set, rawSet, err = store.PublicKeys(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, rawSet)
	require.Equal(t, 2, set.Len())

	secondKey, err := jwk.Import(rotatedPrivateKey)
	require.NoError(t, err)
	require.False(t, jwk.Equal(firstKey, secondKey))
}
