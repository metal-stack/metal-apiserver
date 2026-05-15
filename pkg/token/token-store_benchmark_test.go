package token_test

import (
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/token"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/valkey-io/valkey-go"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func BenchmarkTokenSetAndGet(b *testing.B) {
	ctx := b.Context()
	s := miniredis.RunT(b)
	c, err := valkey.NewClient(valkey.ClientOption{
		InitAddress:  []string{s.Addr()},
		DisableCache: true,
	})
	require.NoError(b, err)
	store := token.NewRedisStore(c)

	inOneHour := time.Now().Add(time.Hour)

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
		Expires:   timestamppb.New(inOneHour),
		IssuedAt:  timestamppb.New(inOneHour),
		TokenType: apiv2.TokenType_TOKEN_TYPE_API,
		ProjectRoles: map[string]apiv2.ProjectRole{
			"8aa3f4c1-52a8-4656-86bc-4006ec016af6": apiv2.ProjectRole_PROJECT_ROLE_OWNER,
		},
		TenantRoles: map[string]apiv2.TenantRole{
			"foo@github": apiv2.TenantRole_TENANT_ROLE_OWNER,
			"bar@github": apiv2.TenantRole_TENANT_ROLE_EDITOR,
			"42@github":  apiv2.TenantRole_TENANT_ROLE_VIEWER,
		},
		AdminRole:    new(apiv2.AdminRole_ADMIN_ROLE_VIEWER),
		InfraRole:    new(apiv2.InfraRole_INFRA_ROLE_EDITOR),
		MachineRoles: map[string]apiv2.MachineRole{},
	}

	for b.Loop() {
		err := store.Set(ctx, inTok)
		require.NoError(b, err)

		outTok, err := store.Get(ctx, inTok.User, inTok.Uuid)
		require.NoError(b, err)
		require.NotNil(b, outTok)

		assert.Equal(b, inTok, outTok)
	}
}
