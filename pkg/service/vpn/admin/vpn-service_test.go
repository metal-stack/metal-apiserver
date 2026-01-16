package admin

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	v1 "github.com/juanfont/headscale/gen/go/headscale/v1"
	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"tailscale.com/tsnet"

	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	"github.com/metal-stack/metal-apiserver/pkg/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var (
	m1 = "00000000-0000-0000-0000-000000000001"
	m2 = "00000000-0000-0000-0000-000000000002"

	p0 = "00000000-0000-0000-0000-000000000000"
	p1 = "00000000-0000-0000-0000-000000000001"
	p2 = "00000000-0000-0000-0000-000000000002"
)

func Test_vpnService_AuthKey(t *testing.T) {
	t.Parallel()

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	testStore, repoCloser := test.StartRepositoryWithCleanup(t, log, test.WithPostgres(false))
	repo := testStore.Store

	headscaleClient, endpoint, _, headscaleCloser := test.StartHeadscale(t)

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

func Test_vpnService_DeleteNode(t *testing.T) {
	t.Parallel()

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	ctx := t.Context()
	headscaleClient, endpoint, controllerURL, headscaleCloser := test.StartHeadscale(t)

	user, err := headscaleClient.CreateUser(ctx, &v1.CreateUserRequest{
		Name: p1,
	})
	require.NoError(t, err)

	key, err := headscaleClient.CreatePreAuthKey(ctx, &v1.CreatePreAuthKeyRequest{
		User:       user.User.Id,
		Ephemeral:  true,
		Expiration: timestamppb.New(time.Now().Add(time.Minute)),
	})
	require.NoError(t, err)

	connectVPNClient(t, m1, controllerURL, key.PreAuthKey.Key)

	defer func() {
		headscaleCloser()
	}()

	tests := []struct {
		name      string
		machineID string
		projectID string
		want      *v1.Node
		wantErr   error
	}{
		{
			name:      "delete existing node",
			machineID: m1,
			projectID: p1,
			want: &v1.Node{
				Name:           m1,
				GivenName:      m1,
				RegisterMethod: v1.RegisterMethod_REGISTER_METHOD_AUTH_KEY,
				User:           user.User,
				Online:         true,
			},
			wantErr: nil,
		},
		{
			name:      "delete non existing node",
			machineID: "m-nonexisting",
			projectID: p1,
			want:      nil,
			wantErr:   errorutil.NotFound("node with id m-nonexisting and project %s not found", p1),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := &vpnService{
				log:                          log,
				headscaleClient:              headscaleClient,
				headscaleControlplaneAddress: endpoint,
			}

			got, err := v.DeleteNode(ctx, tt.machineID, tt.projectID)
			if diff := cmp.Diff(err, tt.wantErr, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("diff = %s", diff)
				return
			}
			if diff := cmp.Diff(
				tt.want, got,
				protocmp.Transform(),
				protocmp.IgnoreFields(
					&v1.Node{}, "id", "created_at", "disco_key", "expiry", "ip_addresses", "last_seen", "machine_key", "node_key", "pre_auth_key",
				),
				cmpopts.IgnoreUnexported(),
			); diff != "" {
				t.Errorf("%v, want %v diff: %s", got, tt.want, diff)
			}
		})
	}
}

func Test_vpnService_CreateUser(t *testing.T) {
	t.Parallel()

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	ctx := t.Context()
	headscaleClient, _, _, headscaleCloser := test.StartHeadscale(t)
	defer headscaleCloser()

	_, err := headscaleClient.CreateUser(ctx, &v1.CreateUserRequest{
		Name: p1,
	})
	require.NoError(t, err)

	tests := []struct {
		name     string
		username string
		want     *v1.User
		wantErr  error
	}{
		{
			name:     "create new user",
			username: p2,
			want:     &v1.User{Name: p2},
		},
		{
			name:     "create existing user",
			username: p1,
			want:     nil,
			wantErr:  errorutil.Conflict("rpc error: code = Internal desc = failed to create user: creating user: constraint failed: UNIQUE constraint failed: users.name (2067)"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := &vpnService{
				log:             log,
				headscaleClient: headscaleClient,
			}
			got, err := v.CreateUser(t.Context(), tt.username)
			if diff := cmp.Diff(err, tt.wantErr, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("diff = %s", diff)
				return
			}
			if diff := cmp.Diff(
				tt.want, got,
				protocmp.Transform(),
				protocmp.IgnoreFields(
					&v1.User{}, "id", "created_at",
				),
				cmpopts.IgnoreUnexported(),
			); diff != "" {
				t.Errorf("%v, want %v diff: %s", got, tt.want, diff)
			}
		})
	}
}

func Test_vpnService_userExists(t *testing.T) {
	t.Parallel()

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	ctx := t.Context()
	headscaleClient, _, _, headscaleCloser := test.StartHeadscale(t)
	defer headscaleCloser()

	_, err := headscaleClient.CreateUser(ctx, &v1.CreateUserRequest{
		Name: p1,
	})
	require.NoError(t, err)

	tests := []struct {
		name     string
		username string
		want     *v1.User
		exists   bool
	}{
		{
			name:     "get existing user",
			username: p1,
			want:     &v1.User{Name: p1},
			exists:   true,
		},
		{
			name:     "get non existing user",
			username: p2,
			want:     nil,
			exists:   false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := &vpnService{
				log:             log,
				headscaleClient: headscaleClient,
			}
			got, got2 := v.userExists(context.Background(), tt.username)
			if diff := cmp.Diff(got2, tt.exists); diff != "" {
				t.Errorf("diff = %s", diff)
				return
			}
			if diff := cmp.Diff(
				tt.want, got,
				protocmp.Transform(),
				protocmp.IgnoreFields(
					&v1.User{}, "id", "created_at",
				),
				cmpopts.IgnoreUnexported(),
			); diff != "" {
				t.Errorf("%v, want %v diff: %s", got, tt.want, diff)
			}
		})
	}
}

func Test_vpnService_EvaluateVPNConnected(t *testing.T) {
	t.Parallel()

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	ctx := t.Context()

	testStore, repocloser := test.StartRepositoryWithCleanup(t, log)
	repo := testStore.Store

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintln(w, "a image")
	}))

	validURL := ts.URL
	defer ts.Close()

	headscaleClient, endpoint, controllerURL, headscaleCloser := test.StartHeadscale(t)

	user, err := headscaleClient.CreateUser(ctx, &v1.CreateUserRequest{
		Name: p1,
	})
	require.NoError(t, err)

	key, err := headscaleClient.CreatePreAuthKey(ctx, &v1.CreatePreAuthKeyRequest{
		User:       user.User.Id,
		Ephemeral:  true,
		Expiration: timestamppb.New(time.Now().Add(time.Minute)),
	})
	require.NoError(t, err)

	test.CreateTenants(t, testStore, []*apiv2.TenantServiceCreateRequest{{Name: "t1"}})
	test.CreateProjects(t, repo, []*apiv2.ProjectServiceCreateRequest{{Name: p1, Login: "t1"}, {Name: p2, Login: "t1"}})
	test.CreatePartitions(t, repo, []*adminv2.PartitionServiceCreateRequest{
		{
			Partition: &apiv2.Partition{Id: "partition-1", BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}},
		},
	})
	test.CreateSizes(t, repo, []*adminv2.SizeServiceCreateRequest{
		{
			Size: &apiv2.Size{Id: "c1-large-x86"},
		},
	})
	test.CreateImages(t, repo, []*adminv2.ImageServiceCreateRequest{
		{Image: &apiv2.Image{Id: "debian-12", Url: validURL, Features: []apiv2.ImageFeature{apiv2.ImageFeature_IMAGE_FEATURE_MACHINE}}},
	})

	defer func() {
		repocloser()
		headscaleCloser()
	}()

	tests := []struct {
		name             string
		nodesToCreate    []string
		machinesToCreate []*metal.Machine
		want             []*apiv2.Machine
		wantErr          error
	}{
		{
			name:          "one node, no machines",
			nodesToCreate: []string{m1},
			want:          nil,
			wantErr:       nil,
		},
		{
			name:          "one node, one machine",
			nodesToCreate: []string{m1},
			machinesToCreate: []*metal.Machine{
				{
					Base:        metal.Base{ID: m1},
					PartitionID: "partition-1", SizeID: "c1-large-x86",
					Allocation: &metal.MachineAllocation{Project: p1, ImageID: "debian-12", VPN: &metal.MachineVPN{ControlPlaneAddress: endpoint}},
				},
			},
			want: []*apiv2.Machine{{
				Meta:      &apiv2.Meta{},
				Uuid:      m1,
				Partition: &apiv2.Partition{Meta: &apiv2.Meta{}, Id: "partition-1", BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}},
				Size:      &apiv2.Size{Meta: &apiv2.Meta{}, Id: "c1-large-x86"},
				Allocation: &apiv2.MachineAllocation{
					Meta:    &apiv2.Meta{},
					Project: p1,
					Image:   &apiv2.Image{},
					Vpn: &apiv2.MachineVPN{
						ControlPlaneAddress: endpoint,
						Connected:           true,
						Ips:                 []string{"100.64.0.1", "fd7a:115c:a1e0::1"},
					},
				},
				Bios:     &apiv2.MachineBios{},
				Hardware: &apiv2.MachineHardware{},
			}},
			wantErr: nil,
		},

		{
			name:          "one more node, one more machine",
			nodesToCreate: []string{m2},
			machinesToCreate: []*metal.Machine{
				{
					Base:        metal.Base{ID: m2},
					PartitionID: "partition-1", SizeID: "c1-large-x86",
					Allocation: &metal.MachineAllocation{Project: p1, ImageID: "debian-12", VPN: &metal.MachineVPN{ControlPlaneAddress: endpoint}},
				},
			},
			want: []*apiv2.Machine{{
				Meta:      &apiv2.Meta{},
				Uuid:      m2,
				Partition: &apiv2.Partition{Meta: &apiv2.Meta{}, Id: "partition-1", BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}},
				Size:      &apiv2.Size{Meta: &apiv2.Meta{}, Id: "c1-large-x86"},
				Allocation: &apiv2.MachineAllocation{
					Meta:    &apiv2.Meta{},
					Project: p1,
					Image:   &apiv2.Image{},
					Vpn: &apiv2.MachineVPN{
						ControlPlaneAddress: endpoint,
						Connected:           true,
						Ips:                 []string{"100.64.0.1", "fd7a:115c:a1e0::1"}, // TODO not sure why the same ip ?
					},
				},
				Bios:     &apiv2.MachineBios{},
				Hardware: &apiv2.MachineHardware{},
			}},
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, n := range tt.nodesToCreate {
				connectVPNClient(t, n, controllerURL, key.PreAuthKey.Key)
			}
			test.CreateMachines(t, testStore, tt.machinesToCreate)

			v := &vpnService{
				log:             log,
				repo:            testStore.Store,
				headscaleClient: headscaleClient,
			}
			got, err := v.EvaluateVPNConnected(ctx)
			if diff := cmp.Diff(err, tt.wantErr, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("diff = %s", diff)
				return
			}
			if diff := cmp.Diff(
				tt.want, got,
				protocmp.Transform(),
				protocmp.IgnoreFields(
					&apiv2.Machine{}, "meta", "status", "recent_provisioning_events",
				),
				protocmp.IgnoreFields(
					&apiv2.Meta{}, "created_at", "updated_at",
				),
				protocmp.IgnoreFields(
					&apiv2.Image{}, "classification", "description", "expires_at", "features", "id", "meta", "name", "url",
				),
				protocmp.IgnoreFields(
					&apiv2.MachineBios{},
				),
				protocmp.IgnoreFields(
					&apiv2.MachineHardware{},
				),
				cmpopts.IgnoreUnexported(),
			); diff != "" {
				t.Errorf("%v, want %v diff: %s", got, tt.want, diff)
			}
		})
	}
}

func Test_vpnService_SetDefaultPolicy(t *testing.T) {
	t.Parallel()

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	ctx := t.Context()
	headscaleClient, _, _, headscaleCloser := test.StartHeadscale(t)
	defer headscaleCloser()

	_, err := headscaleClient.CreateUser(ctx, &v1.CreateUserRequest{
		Name: p1,
	})
	require.NoError(t, err)

	tests := []struct {
		name    string
		wantErr error
	}{
		{
			name:    "set policy",
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := &vpnService{
				log:             log,
				headscaleClient: headscaleClient,
			}
			err := v.SetDefaultPolicy(ctx)
			if diff := cmp.Diff(err, tt.wantErr, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("diff = %s", diff)
				return
			}
			resp, err := v.headscaleClient.GetPolicy(t.Context(), &v1.GetPolicyRequest{})
			require.NoError(t, err)
			require.JSONEq(t, defaultPolicy, resp.Policy)
		})
	}
}

func connectVPNClient(t testing.TB, hostname, controllerURL, authkey string) {
	s := &tsnet.Server{
		Hostname:   hostname,
		ControlURL: controllerURL,
		AuthKey:    authkey,
	}
	lc, err := s.LocalClient()
	require.NoError(t, err)
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		status, err := lc.Status(t.Context())
		require.NoError(c, err)
		require.True(c, status.Self.Online)
	}, 10*time.Second, 50*time.Millisecond)
}
