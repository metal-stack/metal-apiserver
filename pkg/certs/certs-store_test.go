package certs_test

import (
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/lestrrat-go/jwx/v2/jwk"
	"github.com/metal-stack/metal-apiserver/pkg/certs"
	"github.com/metal-stack/metal-apiserver/pkg/token"
	"github.com/metal-stack/metal-lib/pkg/pointer"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func Test_redisStore(t *testing.T) {
	var (
		ctx   = t.Context()
		s     = miniredis.RunT(t)
		c     = redis.NewClient(&redis.Options{Addr: s.Addr()})
		store = certs.NewRedisStore(&certs.Config{
			RenewCertBeforeExpiration: pointer.Pointer(4 * token.MaxExpiration),
			RedisClient:               c,
		})
	)

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

	firstKey, err := jwk.FromRaw(privateKey)
	require.NoError(t, err)

	rotatedPrivateKey, err := store.LatestPrivate(ctx)
	require.NoError(t, err)
	require.NotNil(t, rotatedPrivateKey)
	require.False(t, privateKey.Equal(rotatedPrivateKey))

	set, rawSet, err = store.PublicKeys(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, rawSet)
	require.Equal(t, 2, set.Len())

	secondKey, err := jwk.FromRaw(rotatedPrivateKey)
	require.NoError(t, err)
	require.False(t, jwk.Equal(firstKey, secondKey))
}
