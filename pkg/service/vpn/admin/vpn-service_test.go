package admin

import (
	"log/slog"
	"os"
	"testing"
	"time"

	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"

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

	testStore, repoCloser := test.StartRepositoryWithCleanup(t, log, test.WithCockroach(false))
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
				log:              log,
				repo:             testStore.Store,
				headscaleClient:  headscaleClient,
				headscaleAddress: endpoint,
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
