package main

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	"github.com/metal-stack/metal-apiserver/pkg/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/durationpb"
	"tailscale.com/tsnet"
)

var (
	m1 = "00000000-0000-0000-0000-000000000001"
	m2 = "00000000-0000-0000-0000-000000000002"

	p1 = "00000000-0000-0000-0000-000000000001"
	p2 = "00000000-0000-0000-0000-000000000002"
)

func Test_evaluateVPNConnected(t *testing.T) {
	t.Parallel()

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	ctx := t.Context()

	testStore, repocloser := test.StartRepositoryWithCleanup(t, log, test.WithHeadscale(true))
	defer repocloser()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintln(w, "a image")
	}))

	validURL := ts.URL
	defer ts.Close()

	test.CreateTenants(t, testStore, []*apiv2.TenantServiceCreateRequest{{Name: "t1"}})
	test.CreateProjects(t, testStore, []*apiv2.ProjectServiceCreateRequest{{Name: p1, Login: "t1"}, {Name: p2, Login: "t1"}})
	test.CreatePartitions(t, testStore, []*adminv2.PartitionServiceCreateRequest{
		{
			Partition: &apiv2.Partition{Id: "partition-1", BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}},
		},
	})
	test.CreateSizes(t, testStore, []*adminv2.SizeServiceCreateRequest{
		{
			Size: &apiv2.Size{Id: "c1-large-x86"},
		},
	})
	test.CreateImages(t, testStore, []*adminv2.ImageServiceCreateRequest{
		{Image: &apiv2.Image{Id: "debian-12", Url: validURL, Features: []apiv2.ImageFeature{apiv2.ImageFeature_IMAGE_FEATURE_MACHINE}}},
	})

	key, err := testStore.UnscopedVPN().CreateAuthKey(ctx, &adminv2.VPNServiceAuthKeyRequest{
		Project:   p1,
		Ephemeral: true,
		Expires:   durationpb.New(time.Minute),
	})
	require.NoError(t, err)

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
					Allocation: &metal.MachineAllocation{Project: p1, ImageID: "debian-12", VPN: &metal.MachineVPN{ControlPlaneAddress: testStore.UnscopedVPN().ControlPlaneAddress()}},
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
						ControlPlaneAddress: testStore.UnscopedVPN().ControlPlaneAddress(),
						Connected:           true,
						Ips:                 []string{"100.64.0.1", "fd7a:115c:a1e0::1"},
					},
				},
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
					Allocation: &metal.MachineAllocation{Project: p1, ImageID: "debian-12", VPN: &metal.MachineVPN{ControlPlaneAddress: testStore.UnscopedVPN().ControlPlaneAddress()}},
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
						ControlPlaneAddress: testStore.UnscopedVPN().ControlPlaneAddress(),
						Connected:           true,
						Ips:                 []string{"100.64.0.1", "fd7a:115c:a1e0::1"}, // TODO not sure why the same ip ?
					},
				},
				Hardware: &apiv2.MachineHardware{},
			}},
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, n := range tt.nodesToCreate {
				connectVPNClient(t, n, testStore.GetHeadscaleControllerURL(), key.AuthKey)
			}

			test.CreateMachines(t, testStore, tt.machinesToCreate)

			require.EventuallyWithT(t, func(c *assert.CollectT) {
				got, err := evaluateVPNConnected(ctx, log, testStore.Store)
				if diff := cmp.Diff(err, tt.wantErr, errorutil.ConnectErrorComparer()); diff != "" {
					c.Errorf("diff = %s", diff)
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
					cmpopts.IgnoreUnexported(),
				); diff != "" {
					c.Errorf("%v, want %v diff: %s", got, tt.want, diff)
				}
			}, 30*time.Second, 1*time.Second)
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
