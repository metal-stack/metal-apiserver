package admin

import (
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"

	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	"github.com/metal-stack/metal-apiserver/pkg/test"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/durationpb"
)

var (
	p0 = "00000000-0000-0000-0000-000000000000"
)

func Test_vpnService_AuthKey(t *testing.T) {
	t.Parallel()

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	testStore, repoCloser := test.StartRepositoryWithCleanup(t, log, test.WithPostgres(false), test.WithHeadscale(true))

	defer func() {
		repoCloser()
	}()

	test.CreateTenants(t, testStore, []*apiv2.TenantServiceCreateRequest{
		{Name: "john.doe@github"},
	})
	test.CreateProjects(t, testStore, []*apiv2.ProjectServiceCreateRequest{
		{
			Name:        p0,
			Description: "a description",
			Login:       "john.doe@github",
		},
	})
	tests := []struct {
		name    string
		req     *adminv2.VPNServiceAuthKeyRequest
		want    *adminv2.VPNServiceAuthKeyResponse
		wantErr error
	}{

		{
			name: "create a new authkey",
			req: &adminv2.VPNServiceAuthKeyRequest{
				Project:   p0,
				Ephemeral: false,
				Expires:   durationpb.New(time.Hour),
			},
			want: &adminv2.VPNServiceAuthKeyResponse{
				Address: testStore.UnscopedVPN().ControlPlaneAddress(),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := &vpnService{
				log:  log,
				repo: testStore.Store,
			}

			got, err := v.AuthKey(t.Context(), tt.req)
			if diff := cmp.Diff(err, tt.wantErr, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("diff = %s", diff)
				return
			}
			require.Equal(t, tt.want.Address, got.Address)
			require.NotEmpty(t, got.AuthKey)
		})
	}
}
