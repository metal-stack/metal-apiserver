package invite

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/go-cmp/cmp"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	"github.com/stretchr/testify/require"
	"github.com/valkey-io/valkey-go"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func Test_validateInviteSecret(t *testing.T) {
	secret, err := GenerateInviteSecret()
	require.NoError(t, err)

	tests := []struct {
		name    string
		s       string
		wantErr error
	}{
		{
			name:    "valid secret key returned from generate func",
			s:       secret,
			wantErr: nil,
		},
		{
			name:    "unexpected length",
			s:       "foo",
			wantErr: fmt.Errorf("unexpected invite secret length"),
		},
		{
			name:    "unexpected chars",
			s:       strings.Repeat("*", inviteSecretLength),
			wantErr: fmt.Errorf("invite secret contains unexpected characters: '*'"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateInviteSecret(tt.s)
			if diff := cmp.Diff(tt.wantErr, err, errorutil.ErrorStringComparer()); diff != "" {
				t.Errorf("error diff (+got -want):\n %s", diff)
			}
		})
	}
}

func Test_ProjectInvite(t *testing.T) {
	secret, err := GenerateInviteSecret()
	require.NoError(t, err)

	inOneHour := timestamppb.New(time.Now().Add(time.Hour))
	mr := miniredis.RunT(t)
	c, err := valkey.NewClient(valkey.ClientOption{
		InitAddress: []string{mr.Addr()},
		// This is required because otherwise we get:
		// unknown subcommand 'TRACKING'. Try CLIENT HELP.: [CLIENT TRACKING ON OPTIN]
		// ClientOption.DisableCache must be true for valkey not supporting client-side caching or not supporting RESP3
		DisableCache: true,
	})
	require.NoError(t, err)

	var (
		store = NewProjectRedisStore(c)
		ctx   = t.Context()

		i = &apiv2.ProjectInvite{
			Secret:      secret,
			Project:     "foo",
			Role:        apiv2.ProjectRole_PROJECT_ROLE_EDITOR,
			Joined:      false,
			ProjectName: "bar",
			Tenant:      "tenant",
			TenantName:  "tenant with name",
			ExpiresAt:   inOneHour,
			JoinedAt:    nil,
		}
	)
	defer mr.Close()

	err = store.SetInvite(ctx, i)
	require.NoError(t, err)

	got, err := store.GetInvite(ctx, i.Secret)
	require.NoError(t, err)
	require.Equal(t, i, got)

	gotList, err := store.ListInvites(ctx, i.Project)
	require.NoError(t, err)
	require.Equal(t, []*apiv2.ProjectInvite{i}, gotList)

	err = store.DeleteInvite(ctx, i)
	require.NoError(t, err)

	gotList, err = store.ListInvites(ctx, i.Project)
	require.NoError(t, err)
	require.Empty(t, gotList)
}

func Test_TenantInvite(t *testing.T) {
	secret, err := GenerateInviteSecret()
	require.NoError(t, err)

	inOneHour := timestamppb.New(time.Now().Add(time.Hour))
	mr := miniredis.RunT(t)
	c, err := valkey.NewClient(valkey.ClientOption{
		InitAddress: []string{mr.Addr()},
		// This is required because otherwise we get:
		// unknown subcommand 'TRACKING'. Try CLIENT HELP.: [CLIENT TRACKING ON OPTIN]
		// ClientOption.DisableCache must be true for valkey not supporting client-side caching or not supporting RESP3
		DisableCache: true,
	})
	require.NoError(t, err)

	var (
		ctx   = t.Context()
		store = NewTenantRedisStore(c)
		i     = &apiv2.TenantInvite{
			Secret:           secret,
			TargetTenant:     "target",
			Role:             apiv2.TenantRole_TENANT_ROLE_EDITOR,
			Joined:           false,
			TargetTenantName: "target with name",
			Tenant:           "tenant",
			TenantName:       "tenant with name",
			ExpiresAt:        inOneHour,
			JoinedAt:         nil,
		}
	)
	defer mr.Close()

	err = store.SetInvite(ctx, i)
	require.NoError(t, err)

	got, err := store.GetInvite(ctx, i.Secret)
	require.NoError(t, err)
	require.Equal(t, i, got)

	gotList, err := store.ListInvites(ctx, i.TargetTenant)
	require.NoError(t, err)
	require.Equal(t, []*apiv2.TenantInvite{i}, gotList)

	err = store.DeleteInvite(ctx, i)
	require.NoError(t, err)

	gotList, err = store.ListInvites(ctx, i.Tenant)
	require.NoError(t, err)
	require.Empty(t, gotList)
}
