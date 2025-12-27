package boot

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/google/go-cmp/cmp"
	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	infrav2 "github.com/metal-stack/api/go/metalstack/infra/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	"github.com/metal-stack/metal-apiserver/pkg/test"
	"github.com/metal-stack/metal-lib/pkg/pointer"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/testing/protocmp"
)

var (
	m0 = "00000000-0000-0000-0000-000000000000"
	m1 = "00000000-0000-0000-0000-000000000001"

	p1 = "00000000-0000-0000-0000-000000000001"
	p2 = "00000000-0000-0000-0000-000000000002"
)

func Test_bootServiceServer_Boot(t *testing.T) {
	t.Parallel()

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	testStore, closer := test.StartRepositoryWithCleanup(t, log)
	defer closer()
	repo := testStore.Store

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintln(w, "a image")
	}))

	validURL := ts.URL
	defer ts.Close()

	ctx := t.Context()

	test.CreatePartitions(t, repo, []*adminv2.PartitionServiceCreateRequest{
		{
			Partition: &apiv2.Partition{Id: "partition-1", BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL, Commandline: "console=ttyS1"}},
		},
	})

	tests := []struct {
		name    string
		req     *infrav2.BootServiceBootRequest
		want    *infrav2.BootServiceBootResponse
		wantErr error
	}{
		{
			name: "partition is present",
			req:  &infrav2.BootServiceBootRequest{Mac: "00:00:00:00:00:01", Partition: "partition-1"},
			want: &infrav2.BootServiceBootResponse{
				Kernel:       validURL,
				InitRamDisks: []string{validURL},
				Cmdline:      pointer.Pointer("console=ttyS1"),
			},
			wantErr: nil,
		},
		{
			name:    "partition is not present",
			req:     &infrav2.BootServiceBootRequest{Mac: "00:00:00:00:00:01", Partition: "partition-2"},
			want:    nil,
			wantErr: errorutil.NotFound(`no partition with id "partition-2" found`),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &bootServiceServer{
				log:                  log,
				repo:                 repo,
				bmcSuperuserPassword: "",
			}
			got, err := b.Boot(ctx, tt.req)
			if diff := cmp.Diff(tt.wantErr, err, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("bootServiceServer.Boot() error diff = %s", diff)
			}
			if diff := cmp.Diff(
				tt.want, got,
				protocmp.Transform(),
				protocmp.IgnoreFields(
					&apiv2.Meta{}, "created_at", "updated_at",
				),
			); diff != "" {
				t.Errorf("bootServiceServer.Boot()  = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_bootServiceServer_Dhcp(t *testing.T) {
	t.Parallel()

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	testStore, closer := test.StartRepositoryWithCleanup(t, log)
	defer closer()
	repo := testStore.Store

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintln(w, "a image")
	}))

	validURL := ts.URL
	defer ts.Close()

	ctx := t.Context()

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

	// We need to create machines directly on the database because there is no MachineCreateRequest available and never will.
	test.CreateMachines(t, testStore, []*metal.Machine{
		{Base: metal.Base{ID: m1}, PartitionID: "partition-1", SizeID: "c1-large-x86"},
	})

	tests := []struct {
		name    string
		req     *infrav2.BootServiceDhcpRequest
		want    *infrav2.BootServiceDhcpResponse
		wantErr error
	}{
		{
			name: "unknown machine pxe boots",
			req: &infrav2.BootServiceDhcpRequest{
				Uuid:      m0,
				Partition: "partition-1",
			},
			want:    &infrav2.BootServiceDhcpResponse{},
			wantErr: nil,
		},
		// FIXME machine is not present after this run
		// {
		// 	name: "existing machine pxe boots",
		// 	req: &infrav2.BootServiceDhcpRequest{
		// 		Uuid:      m1,
		// 		Partition: "partition-1",
		// 	},
		// 	want:    &infrav2.BootServiceDhcpResponse{},
		// 	wantErr: nil,
		// },
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &bootServiceServer{
				log:                  log,
				repo:                 repo,
				bmcSuperuserPassword: "",
			}
			got, err := b.Dhcp(ctx, tt.req)
			if diff := cmp.Diff(tt.wantErr, err, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("bootServiceServer.Dhcp() error diff = %s", diff)
			}
			if diff := cmp.Diff(
				tt.want, got,
				protocmp.Transform(),
				protocmp.IgnoreFields(
					&apiv2.Meta{}, "created_at", "updated_at",
				),
			); diff != "" {
				t.Errorf("bootServiceServer.Dhcp()  = %v, want %v", got, tt.want)
			}
			m, err := repo.UnscopedMachine().Get(ctx, tt.req.Uuid)
			require.NoError(t, err)
			require.NotNil(t, m)
		})
	}
}

func Test_bootServiceServer_SuperUserPassword(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		password string
		req      *infrav2.BootServiceSuperUserPasswordRequest
		want     *infrav2.BootServiceSuperUserPasswordResponse
		wantErr  error
	}{
		{
			name:     "password is set",
			password: "geheim",
			req:      &infrav2.BootServiceSuperUserPasswordRequest{Uuid: m0},
			want:     &infrav2.BootServiceSuperUserPasswordResponse{FeatureDisabled: false, SuperUserPassword: "geheim"},
		},
		{
			name: "password is not set",
			req:  &infrav2.BootServiceSuperUserPasswordRequest{Uuid: m0},
			want: &infrav2.BootServiceSuperUserPasswordResponse{FeatureDisabled: true, SuperUserPassword: ""},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &bootServiceServer{
				log:                  slog.Default(),
				bmcSuperuserPassword: tt.password,
			}
			got, err := b.SuperUserPassword(context.Background(), tt.req)
			if diff := cmp.Diff(tt.wantErr, err, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("bootServiceServer.SuperUserPassword() error diff = %s", diff)
			}
			if diff := cmp.Diff(
				tt.want, got,
				protocmp.Transform(),
				protocmp.IgnoreFields(
					&apiv2.Meta{}, "created_at", "updated_at",
				),
			); diff != "" {
				t.Errorf("bootServiceServer.SuperUserPassword()  = %v, want %v", got, tt.want)
			}
		})
	}
}
