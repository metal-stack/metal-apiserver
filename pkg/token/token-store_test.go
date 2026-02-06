package token

import (
	"log/slog"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-lib/pkg/pointer"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestRedisStore(t *testing.T) {
	ctx := t.Context()
	s := miniredis.RunT(t)
	c := redis.NewClient(&redis.Options{Addr: s.Addr()})

	store := NewRedisStore(c)

	johnDoeToken := &apiv2.Token{User: "john@doe.com", Uuid: "abc"}
	willSmithToken := &apiv2.Token{User: "will@smith.com", Uuid: "def"}
	frankZappaToken := &apiv2.Token{User: "frank@zappa.com", Uuid: "cde"}

	err := store.Set(ctx, johnDoeToken)
	require.NoError(t, err)

	err = store.Set(ctx, willSmithToken)
	require.NoError(t, err)

	tok, err := store.Get(ctx, johnDoeToken.User, johnDoeToken.Uuid)
	require.NoError(t, err)
	require.NotNil(t, tok)

	tok, err = store.Get(ctx, frankZappaToken.User, frankZappaToken.Uuid)
	require.Error(t, err)
	require.Nil(t, tok)

	tokens, err := store.List(ctx, "john@doe.com")
	require.NoError(t, err)
	assert.Len(t, tokens, 1)

	allTokens, err := store.AdminList(ctx)
	require.NoError(t, err)
	assert.Len(t, allTokens, 2)

	err = store.Revoke(ctx, johnDoeToken.User, johnDoeToken.Uuid)
	require.NoError(t, err)

	tok, err = store.Get(ctx, johnDoeToken.User, johnDoeToken.Uuid)
	require.Error(t, err)
	require.Nil(t, tok)
}

func TestRedisStoreSetAndGet(t *testing.T) {
	ctx := t.Context()
	s := miniredis.RunT(t)
	c := redis.NewClient(&redis.Options{Addr: s.Addr()})

	store := NewRedisStore(c)

	now := time.Now()

	inTok := &apiv2.Token{
		Uuid:        "bd21fe60-047c-45aa-812d-adc44e098a38",
		User:        "john@doe.com",
		Description: "abc",
		Permissions: []*apiv2.MethodPermission{
			{
				Subject: "a",
				Methods: []string{"b", "c"},
			},
		},
		Expires:   timestamppb.New(now),
		IssuedAt:  timestamppb.New(now),
		TokenType: apiv2.TokenType_TOKEN_TYPE_API,
		ProjectRoles: map[string]apiv2.ProjectRole{
			"8aa3f4c1-52a8-4656-86bc-4006ec016af6": apiv2.ProjectRole_PROJECT_ROLE_OWNER,
		},
		TenantRoles: map[string]apiv2.TenantRole{
			"foo@github": apiv2.TenantRole_TENANT_ROLE_OWNER,
			"bar@github": apiv2.TenantRole_TENANT_ROLE_EDITOR,
			"42@github":  apiv2.TenantRole_TENANT_ROLE_VIEWER,
		},
		AdminRole:    pointer.Pointer(apiv2.AdminRole_ADMIN_ROLE_VIEWER),
		InfraRole:    pointer.Pointer(apiv2.InfraRole_INFRA_ROLE_EDITOR),
		MachineRoles: map[string]apiv2.MachineRole{},
	}

	err := store.Set(ctx, inTok)
	require.NoError(t, err)

	require.NoError(t, store.Migrate(ctx, slog.Default()))

	outTok, err := store.Get(ctx, inTok.User, inTok.Uuid)
	require.NoError(t, err)
	require.NotNil(t, outTok)

	assert.Equal(t, inTok, outTok)
}
