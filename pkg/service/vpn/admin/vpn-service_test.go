package admin

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/davecgh/go-spew/spew"
	v1 "github.com/juanfont/headscale/gen/go/headscale/v1"
	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-lib/pkg/pointer"

	"github.com/metal-stack/metal-apiserver/pkg/test"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/durationpb"
)

var (
	p0 = "00000000-0000-0000-0000-000000000000"
)

func Test_vpnService_Authkey(t *testing.T) {
	t.Parallel()

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	testStore, repoCloser := test.StartRepositoryWithCleanup(t, log, test.WithPostgres(false))
	repo := testStore.Store

	headscaleClient, endpoint, headscaleCloser := test.StartHeadscale(t)

	defer func() {
		repoCloser()
		headscaleCloser()
	}()

	test.CreateTenants(t, testStore, []*apiv2.TenantServiceCreateRequest{
		{Name: "john.doe@github"},
	})
	test.CreateProjects(t, repo, []*apiv2.ProjectServiceCreateRequest{
		{
			Name:        p0,
			Description: "a description",
			Login:       "john.doe@github",
		},
	})
	tests := []struct {
		name    string
		req     *adminv2.VPNServiceAuthkeyRequest
		want    *adminv2.VPNServiceAuthkeyResponse
		wantErr error
	}{

		{
			name: "create a new authkey",
			req: &adminv2.VPNServiceAuthkeyRequest{
				Project:   p0,
				Ephemeral: false,
				Expires:   durationpb.New(time.Hour),
			},
			want: &adminv2.VPNServiceAuthkeyResponse{
				Address: endpoint,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := &vpnService{
				log:                          log,
				repo:                         testStore.Store,
				headscaleClient:              headscaleClient,
				headscaleControlplaneAddress: endpoint,
			}

			got, gotErr := v.Authkey(t.Context(), tt.req)
			if gotErr != nil {
				if tt.wantErr == nil {
					t.Errorf("Authkey() failed: %v", gotErr)
				}
				return
			}
			if tt.wantErr != nil {
				t.Fatal("Authkey() succeeded unexpectedly")
			}
			require.Equal(t, tt.want.Address, got.Address)
			require.Greater(t, len(got.Authkey), 10)
		})
	}
}

func Test_vpnService_DeleteNode(t *testing.T) {
	t.Skip()
	t.Parallel()

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	ctx := t.Context()
	headscaleClient, endpoint, headscaleCloser := test.StartHeadscale(t)

	user, err := headscaleClient.CreateUser(ctx, &v1.CreateUserRequest{
		Name: "p1",
	})
	require.NoError(t, err)

	key, err := headscaleClient.CreatePreAuthKey(ctx, &v1.CreatePreAuthKeyRequest{
		User: user.User.Id,
	})
	require.NoError(t, err)
	spew.Dump(key)

	node, err := headscaleClient.RegisterNode(ctx, &v1.RegisterNodeRequest{
		User: "p1",
		Key:  key.String(),
	})
	require.NoError(t, err)

	defer func() {
		headscaleCloser()
	}()

	tests := []struct {
		name      string
		machineID string
		projectID string
		wantErr   bool
	}{
		{
			name:      "delete existing node",
			machineID: node.Node.Name,
			projectID: "p1",
			wantErr:   false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := &vpnService{
				log:                          log,
				headscaleClient:              headscaleClient,
				headscaleControlplaneAddress: endpoint,
			}

			gotErr := v.DeleteNode(ctx, tt.machineID, tt.projectID)
			if gotErr != nil {
				if !tt.wantErr {
					t.Errorf("DeleteNode() failed: %v", gotErr)
				}
				return
			}
			if tt.wantErr {
				t.Fatal("DeleteNode() succeeded unexpectedly")
			}
		})
	}
}

func Test_vpnService_CreateUser(t *testing.T) {
	t.Parallel()

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	ctx := t.Context()
	headscaleClient, _, headscaleCloser := test.StartHeadscale(t)
	defer headscaleCloser()

	_, err := headscaleClient.CreateUser(ctx, &v1.CreateUserRequest{
		Name: "p1",
	})
	require.NoError(t, err)

	tests := []struct {
		name     string
		username string
		want     *string
		wantErr  bool
	}{
		{
			name:     "create new user",
			username: "p2",
			want:     pointer.Pointer("p2"),
		},
		{
			name:     "create existing user",
			username: "p1",
			want:     nil,
			wantErr:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := &vpnService{
				log:             log,
				headscaleClient: headscaleClient,
			}
			got, gotErr := v.CreateUser(t.Context(), tt.username)
			if gotErr != nil {
				if !tt.wantErr {
					t.Errorf("CreateUser() failed: %v", gotErr)
				}
				return
			}
			if tt.wantErr {
				t.Fatal("CreateUser() succeeded unexpectedly")
			}
			if got.Name != *tt.want {
				t.Errorf("CreateUser() got:%s want:%s", got.Name, *tt.want)
			}
		})
	}
}

func Test_vpnService_UserExists(t *testing.T) {
	t.Parallel()

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	ctx := t.Context()
	headscaleClient, _, headscaleCloser := test.StartHeadscale(t)
	defer headscaleCloser()

	_, err := headscaleClient.CreateUser(ctx, &v1.CreateUserRequest{
		Name: "p1",
	})
	require.NoError(t, err)

	tests := []struct {
		name     string
		username string
		want     *string
		want2    bool
	}{
		{
			name:     "get existing user",
			username: "p1",
			want:     pointer.Pointer("p1"),
			want2:    true,
		},
		{
			name:     "get non existing user",
			username: "p2",
			want:     nil,
			want2:    false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := &vpnService{
				log:             log,
				headscaleClient: headscaleClient,
			}
			got, got2 := v.UserExists(context.Background(), tt.username)
			if got2 != tt.want2 {
				t.Errorf("UserExists() = %v, want %v", got2, tt.want2)
			}
			if !got2 {
				return
			}
			if got.Name != *tt.want {
				t.Errorf("UserExists() got:%s want:%s", got.Name, *tt.want)
			}
		})
	}
}
