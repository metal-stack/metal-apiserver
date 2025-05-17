package ip

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"connectrpc.com/connect"
	"github.com/google/go-cmp/cmp"
	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	"github.com/metal-stack/metal-apiserver/pkg/test"
	"github.com/metal-stack/metal-lib/pkg/pointer"
	"google.golang.org/protobuf/testing/protocmp"
)

func Test_ipServiceServer_Get(t *testing.T) {
	log := slog.Default()

	repo, closer := test.StartRepository(t, log)
	defer closer()

	ctx := t.Context()

	test.CreateTenants(t, repo, []*apiv2.TenantServiceCreateRequest{{Name: "t1"}})
	test.CreateProjects(t, repo, []*apiv2.ProjectServiceCreateRequest{{Name: "p1", Login: "t1"}, {Name: "p2", Login: "t1"}})
	test.CreateNetworks(t, repo, []*adminv2.NetworkServiceCreateRequest{{Id: pointer.Pointer("internet"), Prefixes: []string{"1.2.3.0/24"}, Type: apiv2.NetworkType_NETWORK_TYPE_EXTERNAL, Vrf: pointer.Pointer(uint32(11))}})
	test.CreateIPs(t, repo, []*apiv2.IPServiceCreateRequest{{Ip: pointer.Pointer("1.2.3.4"), Project: "p1", Network: "internet"}})

	tests := []struct {
		name    string
		rq      *apiv2.IPServiceGetRequest
		want    *apiv2.IPServiceGetResponse
		wantErr error
	}{
		{
			name:    "get existing",
			rq:      &apiv2.IPServiceGetRequest{Ip: "1.2.3.4", Project: "p1"},
			want:    &apiv2.IPServiceGetResponse{Ip: &apiv2.IP{Ip: "1.2.3.4", Project: "p1", Network: "internet", Type: apiv2.IPType_IP_TYPE_EPHEMERAL, Meta: &apiv2.Meta{}}},
			wantErr: nil,
		},
		{
			name:    "get non existing",
			rq:      &apiv2.IPServiceGetRequest{Ip: "1.2.3.5"},
			want:    nil,
			wantErr: errorutil.NotFound(`no ip with id "1.2.3.5" found`),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			i := &ipServiceServer{
				log:  log,
				repo: repo,
			}
			got, err := i.Get(ctx, connect.NewRequest(tt.rq))
			if diff := cmp.Diff(err, tt.wantErr, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("diff = %s", diff)
			}

			if diff := cmp.Diff(
				tt.want, pointer.SafeDeref(got).Msg,
				protocmp.Transform(),
				protocmp.IgnoreFields(
					&apiv2.IP{}, "uuid",
				),
				protocmp.IgnoreFields(
					&apiv2.Meta{}, "created_at", "updated_at",
				),
			); diff != "" {
				t.Errorf("ipServiceServer.Get() = %v, want %vņdiff: %s", got.Msg, tt.want, diff)
			}
		})
	}
}

func Test_ipServiceServer_List(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	repo, closer := test.StartRepository(t, log)
	defer closer()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintln(w, "a image")
	}))
	defer ts.Close()

	validURL := ts.URL

	ctx := t.Context()

	test.CreateTenants(t, repo, []*apiv2.TenantServiceCreateRequest{{Name: "t1"}})
	test.CreateProjects(t, repo, []*apiv2.ProjectServiceCreateRequest{{Name: "p0", Login: "t1"}, {Name: "p1", Login: "t1"}, {Name: "p2", Login: "t1"}})
	test.CreatePartitions(t, repo, []*adminv2.PartitionServiceCreateRequest{
		{Partition: &apiv2.Partition{Id: "partition-one", BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}}},
		{Partition: &apiv2.Partition{Id: "partition-two", BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}}},
	})

	nws := []*adminv2.NetworkServiceCreateRequest{
		{Id: pointer.Pointer("internet"), Prefixes: []string{"1.2.3.0/24"}, Type: apiv2.NetworkType_NETWORK_TYPE_EXTERNAL, Vrf: pointer.Pointer(uint32(11))},
		{Id: pointer.Pointer("internetv6"), Prefixes: []string{"2001:db8::/96"}, Type: apiv2.NetworkType_NETWORK_TYPE_EXTERNAL, Vrf: pointer.Pointer(uint32(12))},
		{Id: pointer.Pointer("n3"), Prefixes: []string{"2.3.4.0/24"}, Type: apiv2.NetworkType_NETWORK_TYPE_EXTERNAL, Vrf: pointer.Pointer(uint32(13))},
		{
			Id:                       pointer.Pointer("tenant-super-network"),
			Prefixes:                 []string{"10.100.0.0/14"},
			DefaultChildPrefixLength: &apiv2.ChildPrefixLength{Ipv4: pointer.Pointer(uint32(22))},
			Type:                     apiv2.NetworkType_NETWORK_TYPE_SUPER,
			Partition:                pointer.Pointer("partition-one"),
		},
	}

	ips := []*apiv2.IPServiceCreateRequest{
		{Name: pointer.Pointer("ip1"), Ip: pointer.Pointer("1.2.3.4"), Project: "p1", Network: "internet"},
		{Name: pointer.Pointer("ip2"), Ip: pointer.Pointer("1.2.3.5"), Project: "p1", Network: "internet"},
		{Name: pointer.Pointer("ip3"), Ip: pointer.Pointer("1.2.3.6"), Project: "p1", Network: "internet"},
		{Name: pointer.Pointer("ip4"), Ip: pointer.Pointer("2001:db8::1"), Project: "p2", Network: "internetv6"},
		{Name: pointer.Pointer("ip5"), Ip: pointer.Pointer("2.3.4.5"), Project: "p2", Network: "n3"},
	}

	test.CreateNetworks(t, repo, nws)
	test.CreateIPs(t, repo, ips)

	tests := []struct {
		name    string
		rq      *apiv2.IPServiceListRequest
		want    *apiv2.IPServiceListResponse
		wantErr error
	}{
		{
			name: "get by ip",
			rq:   &apiv2.IPServiceListRequest{Project: "p1", Query: &apiv2.IPQuery{Ip: pointer.Pointer("1.2.3.4"), Project: pointer.Pointer("p1")}},
			want: &apiv2.IPServiceListResponse{Ips: []*apiv2.IP{{Name: "ip1", Ip: "1.2.3.4", Project: "p1", Network: "internet", Type: apiv2.IPType_IP_TYPE_EPHEMERAL, Meta: &apiv2.Meta{}}}},
		},
		{
			name: "get all of p1",
			rq:   &apiv2.IPServiceListRequest{Project: "p1", Query: &apiv2.IPQuery{}},
			want: &apiv2.IPServiceListResponse{Ips: []*apiv2.IP{
				{Name: "ip1", Ip: "1.2.3.4", Project: "p1", Network: "internet", Type: apiv2.IPType_IP_TYPE_EPHEMERAL, Meta: &apiv2.Meta{}},
				{Name: "ip2", Ip: "1.2.3.5", Project: "p1", Network: "internet", Type: apiv2.IPType_IP_TYPE_EPHEMERAL, Meta: &apiv2.Meta{}},
				{Name: "ip3", Ip: "1.2.3.6", Project: "p1", Network: "internet", Type: apiv2.IPType_IP_TYPE_EPHEMERAL, Meta: &apiv2.Meta{}},
			}},
		},
		{
			name: "get by project",
			rq:   &apiv2.IPServiceListRequest{Project: "p1", Query: &apiv2.IPQuery{Project: pointer.Pointer("p1")}},
			want: &apiv2.IPServiceListResponse{Ips: []*apiv2.IP{
				{Name: "ip1", Ip: "1.2.3.4", Project: "p1", Network: "internet", Type: apiv2.IPType_IP_TYPE_EPHEMERAL, Meta: &apiv2.Meta{}},
				{Name: "ip2", Ip: "1.2.3.5", Project: "p1", Network: "internet", Type: apiv2.IPType_IP_TYPE_EPHEMERAL, Meta: &apiv2.Meta{}},
				{Name: "ip3", Ip: "1.2.3.6", Project: "p1", Network: "internet", Type: apiv2.IPType_IP_TYPE_EPHEMERAL, Meta: &apiv2.Meta{}}}},
		},
		{
			name: "get by addressfamily",
			rq:   &apiv2.IPServiceListRequest{Project: "p2", Query: &apiv2.IPQuery{AddressFamily: apiv2.IPAddressFamily_IP_ADDRESS_FAMILY_V6.Enum(), Project: pointer.Pointer("p2")}},
			want: &apiv2.IPServiceListResponse{Ips: []*apiv2.IP{{Name: "ip4", Ip: "2001:db8::1", Project: "p2", Network: "internetv6", Type: apiv2.IPType_IP_TYPE_EPHEMERAL, Meta: &apiv2.Meta{}}}},
		},
		{
			name: "get by parent prefix cidr",
			rq:   &apiv2.IPServiceListRequest{Project: "p2", Query: &apiv2.IPQuery{ParentPrefixCidr: pointer.Pointer("2.3.4.0/24"), Project: pointer.Pointer("p2")}},
			want: &apiv2.IPServiceListResponse{Ips: []*apiv2.IP{{Name: "ip5", Ip: "2.3.4.5", Project: "p2", Network: "n3", Type: apiv2.IPType_IP_TYPE_EPHEMERAL, Meta: &apiv2.Meta{}}}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			i := &ipServiceServer{
				log:  log,
				repo: repo,
			}
			got, err := i.List(ctx, connect.NewRequest(tt.rq))
			if diff := cmp.Diff(err, tt.wantErr, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("diff = %s", diff)
			}

			if diff := cmp.Diff(
				tt.want, pointer.SafeDeref(got).Msg,
				protocmp.Transform(),
				protocmp.IgnoreFields(
					&apiv2.IP{}, "uuid",
				),
				protocmp.IgnoreFields(
					&apiv2.Meta{}, "created_at", "updated_at",
				),
			); diff != "" {
				t.Errorf("ipServiceServer.List() = %v, want %vņdiff: %s", got.Msg, tt.want, diff)
			}
		})
	}
}

func Test_ipServiceServer_Update(t *testing.T) {
	log := slog.Default()

	repo, closer := test.StartRepository(t, log)
	defer closer()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintln(w, "a image")
	}))
	defer ts.Close()

	validURL := ts.URL

	ctx := t.Context()

	test.CreateTenants(t, repo, []*apiv2.TenantServiceCreateRequest{{Name: "t1"}})
	test.CreateProjects(t, repo, []*apiv2.ProjectServiceCreateRequest{{Name: "p0", Login: "t1"}, {Name: "p1", Login: "t1"}, {Name: "p2", Login: "t1"}})
	test.CreatePartitions(t, repo, []*adminv2.PartitionServiceCreateRequest{
		{Partition: &apiv2.Partition{Id: "partition-one", BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}}},
		{Partition: &apiv2.Partition{Id: "partition-two", BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}}},
	})

	nws := []*adminv2.NetworkServiceCreateRequest{
		{Id: pointer.Pointer("internet"), Prefixes: []string{"1.2.3.0/24"}, Type: apiv2.NetworkType_NETWORK_TYPE_EXTERNAL, Vrf: pointer.Pointer(uint32(11))},
		{Id: pointer.Pointer("internetv6"), Prefixes: []string{"2001:db8::/96"}, Type: apiv2.NetworkType_NETWORK_TYPE_EXTERNAL, Vrf: pointer.Pointer(uint32(12))},
		{Id: pointer.Pointer("n3"), Prefixes: []string{"2.3.4.0/24"}, Type: apiv2.NetworkType_NETWORK_TYPE_EXTERNAL, Vrf: pointer.Pointer(uint32(13))},
		{
			Id:                       pointer.Pointer("tenant-super-network"),
			Prefixes:                 []string{"10.100.0.0/14"},
			DefaultChildPrefixLength: &apiv2.ChildPrefixLength{Ipv4: pointer.Pointer(uint32(22))},
			Type:                     apiv2.NetworkType_NETWORK_TYPE_SUPER,
			Partition:                pointer.Pointer("partition-one"),
		},
	}

	ips := []*apiv2.IPServiceCreateRequest{
		{Name: pointer.Pointer("ip1"), Ip: pointer.Pointer("1.2.3.4"), Project: "p1", Network: "internet"},
		{Name: pointer.Pointer("ip2"), Ip: pointer.Pointer("1.2.3.5"), Project: "p1", Network: "internet"},
		{Name: pointer.Pointer("ip3"), Ip: pointer.Pointer("1.2.3.6"), Project: "p1", Network: "internet"},
		{Name: pointer.Pointer("ip4"), Ip: pointer.Pointer("2001:db8::1"), Project: "p2", Network: "internetv6", Labels: &apiv2.Labels{Labels: map[string]string{"color": "red"}}},
		{Name: pointer.Pointer("ip5"), Ip: pointer.Pointer("2.3.4.5"), Project: "p2", Network: "n3"},
		{Name: pointer.Pointer("ip6"), Ip: pointer.Pointer("2.3.4.6"), Project: "p2", Network: "n3", Type: apiv2.IPType_IP_TYPE_STATIC.Enum()},
	}

	test.CreateNetworks(t, repo, nws)
	test.CreateIPs(t, repo, ips)

	tests := []struct {
		name    string
		rq      *apiv2.IPServiceUpdateRequest
		want    *apiv2.IPServiceUpdateResponse
		wantErr error
	}{
		{
			name: "update name",
			rq:   &apiv2.IPServiceUpdateRequest{Ip: "1.2.3.4", Project: "p1", Name: pointer.Pointer("ip1-changed")},
			want: &apiv2.IPServiceUpdateResponse{Ip: &apiv2.IP{Name: "ip1-changed", Ip: "1.2.3.4", Project: "p1", Network: "internet", Type: apiv2.IPType_IP_TYPE_EPHEMERAL, Meta: &apiv2.Meta{}}},
		},
		{
			name: "update description",
			rq:   &apiv2.IPServiceUpdateRequest{Ip: "1.2.3.5", Project: "p1", Description: pointer.Pointer("test was here")},
			want: &apiv2.IPServiceUpdateResponse{Ip: &apiv2.IP{Name: "ip2", Ip: "1.2.3.5", Project: "p1", Description: "test was here", Network: "internet", Type: apiv2.IPType_IP_TYPE_EPHEMERAL, Meta: &apiv2.Meta{}}},
		},
		{
			name: "update type",
			rq:   &apiv2.IPServiceUpdateRequest{Ip: "1.2.3.6", Project: "p1", Type: apiv2.IPType_IP_TYPE_STATIC.Enum()},
			want: &apiv2.IPServiceUpdateResponse{Ip: &apiv2.IP{Name: "ip3", Ip: "1.2.3.6", Project: "p1", Network: "internet", Type: apiv2.IPType_IP_TYPE_STATIC, Meta: &apiv2.Meta{}}},
		},
		{
			name: "update tags",
			rq:   &apiv2.IPServiceUpdateRequest{Ip: "2001:db8::1", Project: "p2", Labels: &apiv2.UpdateLabels{Update: &apiv2.Labels{Labels: map[string]string{"color": "red", "purpose": "lb"}}}},
			want: &apiv2.IPServiceUpdateResponse{Ip: &apiv2.IP{Name: "ip4", Ip: "2001:db8::1", Project: "p2", Network: "internetv6", Type: apiv2.IPType_IP_TYPE_EPHEMERAL, Meta: &apiv2.Meta{Labels: &apiv2.Labels{Labels: map[string]string{"color": "red", "purpose": "lb"}}}}},
		},
		{
			name: "delete tags",
			rq: &apiv2.IPServiceUpdateRequest{
				Ip: "2001:db8::1", Project: "p2",
				Labels: &apiv2.UpdateLabels{Remove: []string{"color", "purpose"}}},
			want: &apiv2.IPServiceUpdateResponse{
				Ip: &apiv2.IP{
					Name:    "ip4",
					Ip:      "2001:db8::1",
					Project: "p2",
					Network: "internetv6",
					Type:    apiv2.IPType_IP_TYPE_EPHEMERAL,
					Meta:    &apiv2.Meta{},
				},
			},
		},
		{
			name:    "update error",
			rq:      &apiv2.IPServiceUpdateRequest{Ip: "2.3.4.6", Project: "p2", Type: apiv2.IPType_IP_TYPE_EPHEMERAL.Enum()},
			want:    nil,
			wantErr: errorutil.InvalidArgument("cannot change type of ip address from static to ephemeral"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			i := &ipServiceServer{
				log:  log,
				repo: repo,
			}

			got, err := i.Update(ctx, connect.NewRequest(tt.rq))
			if diff := cmp.Diff(err, tt.wantErr, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("diff = %s", diff)
			}

			if diff := cmp.Diff(
				tt.want, pointer.SafeDeref(got).Msg,
				protocmp.Transform(),
				protocmp.IgnoreFields(
					&apiv2.IP{}, "uuid",
				),
				protocmp.IgnoreFields(
					&apiv2.Meta{}, "created_at", "updated_at",
				),
			); diff != "" {
				t.Errorf("ipServiceServer.Update() = %v, want %vņdiff: %s", got.Msg, tt.want, diff)
			}
		})
	}
}

func Test_ipServiceServer_Delete(t *testing.T) {
	log := slog.Default()

	repo, closer := test.StartRepository(t, log)
	defer closer()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintln(w, "a image")
	}))
	defer ts.Close()

	validURL := ts.URL

	ctx := t.Context()

	test.CreateTenants(t, repo, []*apiv2.TenantServiceCreateRequest{{Name: "t1"}})
	test.CreateProjects(t, repo, []*apiv2.ProjectServiceCreateRequest{{Name: "p0", Login: "t1"}, {Name: "p1", Login: "t1"}, {Name: "p2", Login: "t1"}})
	test.CreatePartitions(t, repo, []*adminv2.PartitionServiceCreateRequest{
		{Partition: &apiv2.Partition{Id: "partition-one", BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}}},
		{Partition: &apiv2.Partition{Id: "partition-two", BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}}},
	})

	nws := []*adminv2.NetworkServiceCreateRequest{
		{Id: pointer.Pointer("internet"), Prefixes: []string{"1.2.3.0/24"}, Type: apiv2.NetworkType_NETWORK_TYPE_EXTERNAL, Vrf: pointer.Pointer(uint32(11))},
		{Id: pointer.Pointer("internetv6"), Prefixes: []string{"2001:db8::/96"}, Type: apiv2.NetworkType_NETWORK_TYPE_EXTERNAL, Vrf: pointer.Pointer(uint32(12))},
		{Id: pointer.Pointer("n3"), Prefixes: []string{"2.3.4.0/24"}, Type: apiv2.NetworkType_NETWORK_TYPE_EXTERNAL, Vrf: pointer.Pointer(uint32(13))},
		{
			Id:                       pointer.Pointer("tenant-super-network"),
			Prefixes:                 []string{"10.100.0.0/14"},
			DefaultChildPrefixLength: &apiv2.ChildPrefixLength{Ipv4: pointer.Pointer(uint32(22))},
			Type:                     apiv2.NetworkType_NETWORK_TYPE_SUPER,
			Partition:                pointer.Pointer("partition-one"),
		},
	}

	ips := []*apiv2.IPServiceCreateRequest{
		{Name: pointer.Pointer("ip1"), Ip: pointer.Pointer("1.2.3.4"), Project: "p1", Network: "internet"},
		{Name: pointer.Pointer("ip2"), Ip: pointer.Pointer("1.2.3.5"), Project: "p1", Network: "internet"},
		{Name: pointer.Pointer("ip3"), Ip: pointer.Pointer("1.2.3.6"), Project: "p1", Network: "internet", MachineId: pointer.Pointer("abc")},
		{Name: pointer.Pointer("ip4"), Ip: pointer.Pointer("2001:db8::1"), Project: "p2", Network: "internetv6", Labels: &apiv2.Labels{Labels: map[string]string{"color": "red"}}},
		{Name: pointer.Pointer("ip5"), Ip: pointer.Pointer("2.3.4.5"), Project: "p2", Network: "n3"},
	}

	test.CreateNetworks(t, repo, nws)
	test.CreateIPs(t, repo, ips)

	tests := []struct {
		name    string
		rq      *apiv2.IPServiceDeleteRequest
		want    *apiv2.IPServiceDeleteResponse
		wantErr error
	}{
		{
			name: "delete known ip",
			rq:   &apiv2.IPServiceDeleteRequest{Ip: "1.2.3.4", Project: "p1"},
			want: &apiv2.IPServiceDeleteResponse{Ip: &apiv2.IP{Name: "ip1", Ip: "1.2.3.4", Project: "p1", Network: "internet", Type: apiv2.IPType_IP_TYPE_EPHEMERAL, Meta: &apiv2.Meta{}}},
		},
		{
			name:    "delete unknown ip",
			rq:      &apiv2.IPServiceDeleteRequest{Ip: "1.2.3.7", Project: "p1"},
			want:    nil,
			wantErr: errorutil.NotFound(`no ip with id "1.2.3.7" found`),
		},
		{
			name:    "delete machine ip",
			rq:      &apiv2.IPServiceDeleteRequest{Ip: "1.2.3.6", Project: "p1"},
			want:    nil,
			wantErr: errorutil.InvalidArgument("ip with machine scope cannot be deleted"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			i := &ipServiceServer{
				log:  log,
				repo: repo,
			}
			got, err := i.Delete(ctx, connect.NewRequest(tt.rq))
			if diff := cmp.Diff(err, tt.wantErr, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("diff = %s", diff)
			}

			if diff := cmp.Diff(
				tt.want, pointer.SafeDeref(got).Msg,
				protocmp.Transform(),
				protocmp.IgnoreFields(
					&apiv2.IP{}, "uuid",
				),
				protocmp.IgnoreFields(
					&apiv2.Meta{}, "created_at", "updated_at",
				),
			); diff != "" {
				t.Errorf("ipServiceServer.Delete() = %v, want %vņdiff: %s", got.Msg, tt.want, diff)
			}
		})
	}
}

func Test_ipServiceServer_Create(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	repo, closer := test.StartRepository(t, log)
	defer closer()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintln(w, "a image")
	}))
	defer ts.Close()

	validURL := ts.URL

	ctx := t.Context()

	test.CreateTenants(t, repo, []*apiv2.TenantServiceCreateRequest{{Name: "t1"}})
	test.CreateProjects(t, repo, []*apiv2.ProjectServiceCreateRequest{{Name: "p0", Login: "t1"}, {Name: "p1", Login: "t1"}, {Name: "p2", Login: "t1"}})
	test.CreatePartitions(t, repo, []*adminv2.PartitionServiceCreateRequest{
		{Partition: &apiv2.Partition{Id: "partition-one", BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}}},
		{Partition: &apiv2.Partition{Id: "partition-two", BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}}},
		{Partition: &apiv2.Partition{Id: "partition-three", BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}}},
	})

	nws := []*adminv2.NetworkServiceCreateRequest{
		{Id: pointer.Pointer("internet"), Prefixes: []string{"1.2.3.0/24"}, Type: apiv2.NetworkType_NETWORK_TYPE_EXTERNAL, Vrf: pointer.Pointer(uint32(11))},
		{Id: pointer.Pointer("internetv6"), Prefixes: []string{"2001:db8::/96"}, Type: apiv2.NetworkType_NETWORK_TYPE_EXTERNAL, Vrf: pointer.Pointer(uint32(12))},
		{Id: pointer.Pointer("tenant-network"), Partition: pointer.Pointer("partition-one"), Prefixes: []string{"10.2.0.0/24"}, Type: apiv2.NetworkType_NETWORK_TYPE_SUPER, DefaultChildPrefixLength: &apiv2.ChildPrefixLength{Ipv4: pointer.Pointer(uint32(28))}},
		{Id: pointer.Pointer("tenant-network-v6"), Partition: pointer.Pointer("partition-two"), Prefixes: []string{"2001:db8:1::/64"}, Type: apiv2.NetworkType_NETWORK_TYPE_SUPER, DefaultChildPrefixLength: &apiv2.ChildPrefixLength{Ipv6: pointer.Pointer(uint32(80))}},
		{Id: pointer.Pointer("tenant-network-dualstack"), Partition: pointer.Pointer("partition-three"), Prefixes: []string{"10.3.0.0/24", "2001:db8:2::/64"}, Type: apiv2.NetworkType_NETWORK_TYPE_SUPER, DefaultChildPrefixLength: &apiv2.ChildPrefixLength{Ipv4: pointer.Pointer(uint32(28)), Ipv6: pointer.Pointer(uint32(80))}},
		{Id: pointer.Pointer("tenant-super-network-namespaced"), Prefixes: []string{"10.100.0.0/14"}, DefaultChildPrefixLength: &apiv2.ChildPrefixLength{Ipv4: pointer.Pointer(uint32(22))}, Type: apiv2.NetworkType_NETWORK_TYPE_SUPER_NAMESPACED},
	}
	ips := []*apiv2.IPServiceCreateRequest{
		{Name: pointer.Pointer("ip1"), Ip: pointer.Pointer("1.2.3.4"), Project: "p1", Network: "internet"},
		{Name: pointer.Pointer("ip2"), Ip: pointer.Pointer("1.2.3.5"), Project: "p1", Network: "internet"},
		{Name: pointer.Pointer("ip3"), Ip: pointer.Pointer("1.2.3.6"), Project: "p1", Network: "internet"},
		{Name: pointer.Pointer("ip4"), Ip: pointer.Pointer("2001:db8:1::1"), Project: "p2", Network: "tenant-network-v6", Labels: &apiv2.Labels{Labels: map[string]string{"color": "red"}}},
		{Name: pointer.Pointer("ip5"), Ip: pointer.Pointer("10.2.0.5"), Project: "p2", Network: "tenant-network"},
	}
	childNetworks := []*apiv2.NetworkServiceCreateRequest{
		{
			Name:            pointer.Pointer("private-namespaced-1"),
			Project:         "p1",
			ParentNetworkId: pointer.Pointer("tenant-super-network-namespaced"),
		},
		{
			Name:            pointer.Pointer("private-namespaced-2"),
			Project:         "p2",
			ParentNetworkId: pointer.Pointer("tenant-super-network-namespaced"),
		},
	}

	test.CreateNetworks(t, repo, nws)
	childNetworksMap := test.AllocateNetworks(t, repo, childNetworks)
	test.CreateIPs(t, repo, ips)

	tests := []struct {
		name    string
		rq      *apiv2.IPServiceCreateRequest
		want    *apiv2.IPServiceCreateResponse
		wantErr error
	}{
		{
			name: "create random ephemeral ipv4",
			rq: &apiv2.IPServiceCreateRequest{
				Network: "internet",
				Project: "p1",
			},
			want: &apiv2.IPServiceCreateResponse{
				Ip: &apiv2.IP{Ip: "1.2.3.1", Network: "internet", Project: "p1", Type: apiv2.IPType_IP_TYPE_EPHEMERAL, Meta: &apiv2.Meta{}},
			},
		},
		{
			name: "create random ephemeral ipv6",
			rq: &apiv2.IPServiceCreateRequest{
				Network: "tenant-network-v6",
				Project: "p1",
			},
			want: &apiv2.IPServiceCreateResponse{
				Ip: &apiv2.IP{Ip: "2001:db8:1::2", Network: "tenant-network-v6", Project: "p1", Type: apiv2.IPType_IP_TYPE_EPHEMERAL, Meta: &apiv2.Meta{}},
			},
		},
		{
			name: "create specific ephemeral ipv6",
			rq: &apiv2.IPServiceCreateRequest{
				Network: "tenant-network-v6",
				Project: "p1",
				Ip:      pointer.Pointer("2001:db8:1::99"),
			},
			want: &apiv2.IPServiceCreateResponse{
				Ip: &apiv2.IP{Ip: "2001:db8:1::99", Network: "tenant-network-v6", Project: "p1", Type: apiv2.IPType_IP_TYPE_EPHEMERAL, Meta: &apiv2.Meta{}},
			},
		},
		{
			name: "create random ephemeral ipv4 from a dualstack network",
			rq: &apiv2.IPServiceCreateRequest{
				Network: "tenant-network-dualstack",
				Project: "p1",
			},
			want: &apiv2.IPServiceCreateResponse{
				Ip: &apiv2.IP{Ip: "10.3.0.1", Network: "tenant-network-dualstack", Project: "p1", Type: apiv2.IPType_IP_TYPE_EPHEMERAL, Meta: &apiv2.Meta{}},
			},
		},
		{
			name: "create random ephemeral ipv6 from a dualstack network",
			rq: &apiv2.IPServiceCreateRequest{
				Network:       "tenant-network-dualstack",
				Project:       "p1",
				AddressFamily: apiv2.IPAddressFamily_IP_ADDRESS_FAMILY_V6.Enum(),
			},
			want: &apiv2.IPServiceCreateResponse{
				Ip: &apiv2.IP{Ip: "2001:db8:2::1", Network: "tenant-network-dualstack", Project: "p1", Type: apiv2.IPType_IP_TYPE_EPHEMERAL, Meta: &apiv2.Meta{}},
			},
		},
		{
			name: "create specific ephemeral ipv4",
			rq: &apiv2.IPServiceCreateRequest{
				Network: "internet",
				Project: "p1",
				Ip:      pointer.Pointer("1.2.3.99"),
			},
			want: &apiv2.IPServiceCreateResponse{
				Ip: &apiv2.IP{Ip: "1.2.3.99", Network: "internet", Project: "p1", Type: apiv2.IPType_IP_TYPE_EPHEMERAL, Meta: &apiv2.Meta{}},
			},
		},
		{
			name: "create specific static ipv4",
			rq: &apiv2.IPServiceCreateRequest{
				Network: "internet",
				Project: "p1",
				Ip:      pointer.Pointer("1.2.3.100"),
				Type:    apiv2.IPType_IP_TYPE_STATIC.Enum(),
			},
			want: &apiv2.IPServiceCreateResponse{
				Ip: &apiv2.IP{Ip: "1.2.3.100", Network: "internet", Project: "p1", Type: apiv2.IPType_IP_TYPE_STATIC, Meta: &apiv2.Meta{}},
			},
		},
		{
			name: "create ip in a namespaced private",
			rq: &apiv2.IPServiceCreateRequest{
				Network: childNetworksMap["private-namespaced-1"],
				Project: "p1",
				Type:    apiv2.IPType_IP_TYPE_STATIC.Enum(),
			},
			want: &apiv2.IPServiceCreateResponse{
				Ip: &apiv2.IP{Ip: "10.100.0.1", Network: childNetworksMap["private-namespaced-1"], Project: "p1", Type: apiv2.IPType_IP_TYPE_STATIC, Meta: &apiv2.Meta{}},
			},
		},
		{
			name: "create specific ipv4 which is already allocated",
			rq: &apiv2.IPServiceCreateRequest{
				Network: "internet",
				Project: "p1",
				Ip:      pointer.Pointer("1.2.3.1"),
			},
			want:    nil,
			wantErr: errorutil.Conflict("AlreadyAllocatedError: given ip:1.2.3.1 is already allocated"),
		},
		{
			name: "allocate a static specific ip outside prefix",
			rq: &apiv2.IPServiceCreateRequest{
				Network: "internet",
				Project: "p1",
				Ip:      pointer.Pointer("1.3.0.1"),
			},
			want:    nil,
			wantErr: errorutil.InvalidArgument("specific ip 1.3.0.1 not contained in any of the defined prefixes"),
		},
		{
			name: "allocate a random ip with unavailable addressfamily",
			rq: &apiv2.IPServiceCreateRequest{
				Network:       "tenant-network-v6",
				Project:       "p1",
				AddressFamily: apiv2.IPAddressFamily_IP_ADDRESS_FAMILY_V4.Enum(),
			},
			want:    nil,
			wantErr: errorutil.InvalidArgument("there is no prefix for the given addressfamily:IPv4 present in network:tenant-network-v6 [IPv6]"),
		},
		{
			name: "allocate a random ip with unavailable addressfamily",
			rq: &apiv2.IPServiceCreateRequest{
				Network:       "tenant-network",
				Project:       "p1",
				AddressFamily: apiv2.IPAddressFamily_IP_ADDRESS_FAMILY_V6.Enum(),
			},
			want:    nil,
			wantErr: errorutil.InvalidArgument("there is no prefix for the given addressfamily:IPv6 present in network:tenant-network [IPv4]"),
		},
		{
			name: "disallow creating an ip address in a project-scoped network that does not belong to the request project",
			rq: &apiv2.IPServiceCreateRequest{
				Network:       childNetworksMap["private-namespaced-2"],
				Project:       "p1",
				AddressFamily: apiv2.IPAddressFamily_IP_ADDRESS_FAMILY_V6.Enum(),
			},
			want:    nil,
			wantErr: errorutil.InvalidArgument("ip creation not allowed in network %s", childNetworksMap["private-namespaced-2"]),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			i := &ipServiceServer{
				log:  log,
				repo: repo,
			}
			got, err := i.Create(
				ctx, connect.NewRequest(tt.rq),
			)
			if diff := cmp.Diff(err, tt.wantErr, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("diff = %s", diff)
			}

			if diff := cmp.Diff(
				tt.want, pointer.SafeDeref(got).Msg,
				protocmp.Transform(),
				protocmp.IgnoreFields(
					&apiv2.IP{}, "uuid",
				),
				protocmp.IgnoreFields(
					&apiv2.Meta{}, "created_at", "updated_at",
				),
			); diff != "" {
				t.Errorf("ipServiceServer.Create() = %v, want %vņdiff: %s", got.Msg, tt.want, diff)
			}
		})
	}
}
