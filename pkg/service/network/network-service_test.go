package network

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

func Test_networkServiceServer_Get(t *testing.T) {
	log := slog.Default()

	repo, closer := test.StartRepository(t, log)
	defer closer()

	ctx := t.Context()

	test.CreateTenants(t, repo, []*apiv2.TenantServiceCreateRequest{{Name: "t1"}})
	test.CreateProjects(t, repo, []*apiv2.ProjectServiceCreateRequest{{Name: "p1", Login: "t1"}, {Name: "p2", Login: "t1"}})
	test.CreateNetworks(t, repo, []*adminv2.NetworkServiceCreateRequest{{Id: pointer.Pointer("internet"), Prefixes: []string{"1.2.3.0/24"}}})

	tests := []struct {
		name    string
		rq      *apiv2.NetworkServiceGetRequest
		want    *apiv2.NetworkServiceGetResponse
		wantErr error
	}{
		{
			name: "get existing",
			rq:   &apiv2.NetworkServiceGetRequest{Id: "internet"},
			want: &apiv2.NetworkServiceGetResponse{
				Network: &apiv2.Network{
					Id:       "internet",
					Meta:     &apiv2.Meta{},
					Prefixes: []string{"1.2.3.0/24"},
				},
			},
			wantErr: nil,
		},
		{
			name:    "get non existing",
			rq:      &apiv2.NetworkServiceGetRequest{Id: "non-existing-network"},
			want:    nil,
			wantErr: errorutil.NotFound(`no network with id "non-existing-network" found`),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := &networkServiceServer{
				log:  log,
				repo: repo,
			}
			got, err := n.Get(ctx, connect.NewRequest(tt.rq))
			if diff := cmp.Diff(err, tt.wantErr, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("diff = %s", diff)
			}

			if diff := cmp.Diff(
				tt.want, pointer.SafeDeref(got).Msg,
				cmp.Options{
					protocmp.Transform(),
					protocmp.IgnoreFields(
						&apiv2.Network{}, "consumption",
					),
					protocmp.IgnoreFields(
						&apiv2.Meta{}, "created_at", "updated_at",
					),
				},
			); diff != "" {
				t.Errorf("networkServiceServer.Get() = %v, want %vņdiff: %s", got.Msg, tt.want, diff)
			}
		})
	}
}

func Test_networkServiceServer_List(t *testing.T) {
	log := slog.Default()

	repo, closer := test.StartRepository(t, log)
	defer closer()

	ctx := t.Context()

	test.CreateTenants(t, repo, []*apiv2.TenantServiceCreateRequest{{Name: "t1"}, {Name: "t0"}})
	test.CreateProjects(t, repo, []*apiv2.ProjectServiceCreateRequest{{Name: "p1", Login: "t1"}, {Name: "p2", Login: "t1"}, {Name: "p3", Login: "t1"}, {Name: "p0", Login: "t0"}})
	test.CreateNetworks(t, repo, []*adminv2.NetworkServiceCreateRequest{{Id: pointer.Pointer("internet"), Prefixes: []string{"1.2.3.0/24"}}})
	test.CreateNetworks(t, repo, []*adminv2.NetworkServiceCreateRequest{
		{Id: pointer.Pointer("p1-network-a"), Name: pointer.Pointer("P1 Network"), Project: pointer.Pointer("p1"), Prefixes: []string{"2.3.4.0/24"}},
		{Id: pointer.Pointer("p1-network-b"), Name: pointer.Pointer("P1 Network"), Project: pointer.Pointer("p1"), Prefixes: []string{"2.3.5.0/24"}},
		{Id: pointer.Pointer("p2-network-a"), Name: pointer.Pointer("P2 Network"), Project: pointer.Pointer("p2"), Prefixes: []string{"3.3.1.0/24"}},
		{Id: pointer.Pointer("p2-network-b"), Name: pointer.Pointer("P2 Network"), Project: pointer.Pointer("p2"), Prefixes: []string{"3.3.2.0/24"}},
		{Id: pointer.Pointer("p3-network-a"), Name: pointer.Pointer("P3 Network"), Project: pointer.Pointer("p3"), Prefixes: []string{"4.3.4.0/24"}},

		{Id: pointer.Pointer("underlay"), Name: pointer.Pointer("Underlay Network"), Project: pointer.Pointer("p0"), Prefixes: []string{"10.0.0.0/24"}},
	})

	tests := []struct {
		name    string
		rq      *apiv2.NetworkServiceListRequest
		want    *apiv2.NetworkServiceListResponse
		wantErr error
	}{
		{
			name: "list by id",
			rq:   &apiv2.NetworkServiceListRequest{Project: "", Query: &apiv2.NetworkQuery{Id: pointer.Pointer("internet")}},
			want: &apiv2.NetworkServiceListResponse{
				Networks: []*apiv2.Network{
					{Id: "internet", Meta: &apiv2.Meta{}, Prefixes: []string{"1.2.3.0/24"}},
				},
			},
			wantErr: nil,
		},
		{
			name: "list by project",
			rq:   &apiv2.NetworkServiceListRequest{Project: "p1", Query: &apiv2.NetworkQuery{Project: pointer.Pointer("p1")}},
			want: &apiv2.NetworkServiceListResponse{
				Networks: []*apiv2.Network{
					{Id: "p1-network-a", Meta: &apiv2.Meta{}, Name: pointer.Pointer("P1 Network"), Project: pointer.Pointer("p1"), Prefixes: []string{"2.3.4.0/24"}},
					{Id: "p1-network-b", Meta: &apiv2.Meta{}, Name: pointer.Pointer("P1 Network"), Project: pointer.Pointer("p1"), Prefixes: []string{"2.3.5.0/24"}},
				},
			},
			wantErr: nil,
		},
		{
			name:    "list by invalid (not owned) project",
			rq:      &apiv2.NetworkServiceListRequest{Project: "p1", Query: &apiv2.NetworkQuery{Project: pointer.Pointer("p0")}},
			want:    &apiv2.NetworkServiceListResponse{},
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := &networkServiceServer{
				log:  log,
				repo: repo,
			}
			got, err := n.List(ctx, connect.NewRequest(tt.rq))
			if diff := cmp.Diff(err, tt.wantErr, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("diff = %s", diff)
			}

			if diff := cmp.Diff(
				tt.want, pointer.SafeDeref(got).Msg,
				cmp.Options{
					protocmp.Transform(),
					protocmp.IgnoreFields(
						&apiv2.Network{}, "consumption",
					),
					protocmp.IgnoreFields(
						&apiv2.Meta{}, "created_at", "updated_at",
					),
				},
			); diff != "" {
				t.Errorf("networkServiceServer.List() = %v, want %vņdiff: %s", got.Msg, tt.want, diff)
			}
		})
	}
}

func Test_networkServiceServer_Update(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	repo, closer := test.StartRepository(t, log)
	defer closer()

	ctx := t.Context()

	test.CreateTenants(t, repo, []*apiv2.TenantServiceCreateRequest{{Name: "t1"}, {Name: "t0"}})
	test.CreateProjects(t, repo, []*apiv2.ProjectServiceCreateRequest{{Name: "p1", Login: "t1"}, {Name: "p2", Login: "t1"}, {Name: "p3", Login: "t1"}, {Name: "p0", Login: "t0"}})
	test.CreateNetworks(t, repo, []*adminv2.NetworkServiceCreateRequest{{Id: pointer.Pointer("internet"), Prefixes: []string{"1.2.3.0/24"}}})
	test.CreateNetworks(t, repo, []*adminv2.NetworkServiceCreateRequest{
		{Id: pointer.Pointer("p1-network-a"), Name: pointer.Pointer("P1 Network"), Project: pointer.Pointer("p1"), Prefixes: []string{"2.3.4.0/24"}},
		{Id: pointer.Pointer("p1-network-b"), Name: pointer.Pointer("P1 Network"), Project: pointer.Pointer("p1"), Prefixes: []string{"2.3.5.0/24"}},
		{Id: pointer.Pointer("p2-network-a"), Name: pointer.Pointer("P2 Network"), Project: pointer.Pointer("p2"), Prefixes: []string{"3.3.1.0/24"}},
		{Id: pointer.Pointer("p2-network-b"), Name: pointer.Pointer("P2 Network"), Project: pointer.Pointer("p2"), Prefixes: []string{"3.3.2.0/24"}},
		{Id: pointer.Pointer("p3-network-a"), Name: pointer.Pointer("P3 Network"), Project: pointer.Pointer("p3"), Prefixes: []string{"4.3.4.0/24"}, Labels: &apiv2.Labels{Labels: map[string]string{"color": "red"}}},

		{Id: pointer.Pointer("underlay"), Name: pointer.Pointer("Underlay Network"), Project: pointer.Pointer("p0"), Prefixes: []string{"10.0.0.0/24"}},
	})

	tests := []struct {
		name    string
		rq      *apiv2.NetworkServiceUpdateRequest
		want    *apiv2.NetworkServiceUpdateResponse
		wantErr error
	}{
		{
			name: "update name",
			rq:   &apiv2.NetworkServiceUpdateRequest{Id: "p1-network-a", Project: "p1", Name: pointer.Pointer("P1 Updated Network")},
			want: &apiv2.NetworkServiceUpdateResponse{
				Network: &apiv2.Network{Id: "p1-network-a", Meta: &apiv2.Meta{}, Name: pointer.Pointer("P1 Updated Network"), Project: pointer.Pointer("p1"), Prefixes: []string{"2.3.4.0/24"}},
			},
			wantErr: nil,
		},
		{
			name: "update description",
			rq:   &apiv2.NetworkServiceUpdateRequest{Id: "p1-network-b", Project: "p1", Description: pointer.Pointer("P1 Description")},
			want: &apiv2.NetworkServiceUpdateResponse{
				Network: &apiv2.Network{Id: "p1-network-b", Meta: &apiv2.Meta{}, Name: pointer.Pointer("P1 Network"), Description: pointer.Pointer("P1 Description"), Project: pointer.Pointer("p1"), Prefixes: []string{"2.3.5.0/24"}},
			},
			wantErr: nil,
		},
		{
			name: "update labels",
			rq:   &apiv2.NetworkServiceUpdateRequest{Id: "p3-network-a", Project: "p3", Labels: &apiv2.Labels{Labels: map[string]string{"size": "small"}}},
			want: &apiv2.NetworkServiceUpdateResponse{
				Network: &apiv2.Network{Id: "p3-network-a", Meta: &apiv2.Meta{Labels: &apiv2.Labels{Labels: map[string]string{"size": "small"}}}, Name: pointer.Pointer("P3 Network"), Project: pointer.Pointer("p3"), Prefixes: []string{"4.3.4.0/24"}},
			},
			wantErr: nil,
		},
		{
			name:    "update non-existing",
			rq:      &apiv2.NetworkServiceUpdateRequest{Id: "p4-network-a", Project: "p4"},
			want:    nil,
			wantErr: errorutil.NotFound(`no network with id "p4-network-a" found`),
		},
		{
			name:    "wrong project",
			rq:      &apiv2.NetworkServiceUpdateRequest{Id: "p3-network-a", Project: "p4"},
			want:    nil,
			wantErr: errorutil.NotFound(`network:p3-network-a project:p3 for scope:p4 not found`),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := &networkServiceServer{
				log:  log,
				repo: repo,
			}
			got, err := n.Update(ctx, connect.NewRequest(tt.rq))
			if diff := cmp.Diff(err, tt.wantErr, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("diff = %s", diff)
			}

			if diff := cmp.Diff(
				tt.want, pointer.SafeDeref(got).Msg,
				cmp.Options{
					protocmp.Transform(),
					protocmp.IgnoreFields(
						&apiv2.Network{}, "consumption",
					),
					protocmp.IgnoreFields(
						&apiv2.Meta{}, "created_at", "updated_at",
					),
				},
			); diff != "" {
				t.Errorf("networkServiceServer.Update() = %v, want %vņdiff: %s", got.Msg, tt.want, diff)
			}
		})
	}
}

func Test_networkServiceServer_Create(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	repo, closer := test.StartRepository(t, log)
	defer closer()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintln(w, "a image")
	}))
	defer ts.Close()

	validURL := ts.URL

	ctx := t.Context()

	test.CreateTenants(t, repo, []*apiv2.TenantServiceCreateRequest{{Name: "t1"}, {Name: "t0"}})
	test.CreateProjects(t, repo, []*apiv2.ProjectServiceCreateRequest{{Name: "p1", Login: "t1"}, {Name: "p2", Login: "t1"}, {Name: "p3", Login: "t1"}, {Name: "p0", Login: "t0"}})

	test.CreatePartitions(t, repo, []*adminv2.PartitionServiceCreateRequest{
		{Partition: &apiv2.Partition{Id: "partition-one", BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}}},
		{Partition: &apiv2.Partition{Id: "partition-two", BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}}},
		{Partition: &apiv2.Partition{Id: "partition-three", BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}}},
	})

	test.CreateNetworks(t, repo, []*adminv2.NetworkServiceCreateRequest{{Id: pointer.Pointer("internet"), Prefixes: []string{"1.2.3.0/24"}}})
	test.CreateNetworks(t, repo, []*adminv2.NetworkServiceCreateRequest{
		{
			Id:                       pointer.Pointer("tenant-super-network"),
			Prefixes:                 []string{"10.100.0.0/14"},
			DefaultChildPrefixLength: []*apiv2.ChildPrefixLength{{AddressFamily: apiv2.IPAddressFamily_IP_ADDRESS_FAMILY_V4, Length: 22}},
			Options:                  &apiv2.NetworkOptions{PrivateSuper: true},
			Partition:                pointer.Pointer("partition-one"),
		},
		{
			Id:                       pointer.Pointer("tenant-super-network-v6"),
			Prefixes:                 []string{"2001:db8::/96"},
			DefaultChildPrefixLength: []*apiv2.ChildPrefixLength{{AddressFamily: apiv2.IPAddressFamily_IP_ADDRESS_FAMILY_V6, Length: 112}},
			Options:                  &apiv2.NetworkOptions{PrivateSuper: true},
			Partition:                pointer.Pointer("partition-two"),
		},
		{
			Id:                       pointer.Pointer("tenant-super-network-dualstack"),
			Prefixes:                 []string{"2001:dc8::/96", "10.200.0.0/14"},
			DefaultChildPrefixLength: []*apiv2.ChildPrefixLength{{AddressFamily: apiv2.IPAddressFamily_IP_ADDRESS_FAMILY_V6, Length: 112}, {AddressFamily: apiv2.IPAddressFamily_IP_ADDRESS_FAMILY_V4, Length: 22}},
			Options:                  &apiv2.NetworkOptions{PrivateSuper: true},
			Partition:                pointer.Pointer("partition-three"),
		},
		{Id: pointer.Pointer("underlay"), Name: pointer.Pointer("Underlay Network"), Project: pointer.Pointer("p0"), Prefixes: []string{"10.0.0.0/24"}},
	})

	tests := []struct {
		name    string
		rq      *apiv2.NetworkServiceCreateRequest
		want    *apiv2.NetworkServiceCreateResponse
		wantErr error
	}{
		{
			name: "simple network defaults to v4",
			rq:   &apiv2.NetworkServiceCreateRequest{Project: "p1", Name: pointer.Pointer("My Machine Network"), Partition: pointer.Pointer("partition-one")},
			want: &apiv2.NetworkServiceCreateResponse{
				Network: &apiv2.Network{
					Name:            pointer.Pointer("My Machine Network"),
					Meta:            &apiv2.Meta{},
					ParentNetworkId: pointer.Pointer("tenant-super-network"),
					Partition:       pointer.Pointer("partition-one"),
					Project:         pointer.Pointer("p1"),
					Prefixes:        []string{"10.100.0.0/22"},
				}},
			wantErr: nil,
		},
		{
			name: "simple network defaults to v6",
			rq:   &apiv2.NetworkServiceCreateRequest{Project: "p1", Name: pointer.Pointer("My Machine Network"), Partition: pointer.Pointer("partition-two")},
			want: &apiv2.NetworkServiceCreateResponse{
				Network: &apiv2.Network{
					Name:            pointer.Pointer("My Machine Network"),
					Meta:            &apiv2.Meta{},
					ParentNetworkId: pointer.Pointer("tenant-super-network-v6"),
					Partition:       pointer.Pointer("partition-two"),
					Project:         pointer.Pointer("p1"),
					Prefixes:        []string{"2001:db8::/112"},
				}},
			wantErr: nil,
		},
		{
			name: "simple network defaults to dualstack",
			rq:   &apiv2.NetworkServiceCreateRequest{Project: "p1", Name: pointer.Pointer("My Machine Network"), Partition: pointer.Pointer("partition-three")},
			want: &apiv2.NetworkServiceCreateResponse{
				Network: &apiv2.Network{
					Name:            pointer.Pointer("My Machine Network"),
					Meta:            &apiv2.Meta{},
					ParentNetworkId: pointer.Pointer("tenant-super-network-dualstack"),
					Partition:       pointer.Pointer("partition-three"),
					Project:         pointer.Pointer("p1"),
					Prefixes:        []string{"10.200.0.0/22", "2001:dc8::/112"},
				}},
			wantErr: nil,
		},
		{
			name: "simple network force v6 by af",
			rq: &apiv2.NetworkServiceCreateRequest{
				Project:       "p2",
				Name:          pointer.Pointer("My Machine Network"),
				Partition:     pointer.Pointer("partition-three"),
				AddressFamily: apiv2.IPAddressFamily_IP_ADDRESS_FAMILY_V6.Enum(),
			},
			want: &apiv2.NetworkServiceCreateResponse{
				Network: &apiv2.Network{
					Name:            pointer.Pointer("My Machine Network"),
					Meta:            &apiv2.Meta{},
					ParentNetworkId: pointer.Pointer("tenant-super-network-dualstack"),
					Partition:       pointer.Pointer("partition-three"),
					Project:         pointer.Pointer("p2"),
					Prefixes:        []string{"2001:dc8::1:0/112"},
				}},
			wantErr: nil,
		},
		{
			name: "simple network force v4 by af",
			rq: &apiv2.NetworkServiceCreateRequest{
				Project:       "p2",
				Name:          pointer.Pointer("My Machine Network"),
				Partition:     pointer.Pointer("partition-three"),
				AddressFamily: apiv2.IPAddressFamily_IP_ADDRESS_FAMILY_V4.Enum(),
			},
			want: &apiv2.NetworkServiceCreateResponse{
				Network: &apiv2.Network{
					Name:            pointer.Pointer("My Machine Network"),
					Meta:            &apiv2.Meta{},
					ParentNetworkId: pointer.Pointer("tenant-super-network-dualstack"),
					Partition:       pointer.Pointer("partition-three"),
					Project:         pointer.Pointer("p2"),
					Prefixes:        []string{"10.200.4.0/22"},
				}},
			wantErr: nil,
		},
		{
			name: "simple network dualstack but v6 with different length",
			rq: &apiv2.NetworkServiceCreateRequest{
				Project:   "p2",
				Name:      pointer.Pointer("My Machine Network"),
				Partition: pointer.Pointer("partition-three"),
				Length:    []*apiv2.ChildPrefixLength{{AddressFamily: apiv2.IPAddressFamily_IP_ADDRESS_FAMILY_V6, Length: 116}}},
			want: &apiv2.NetworkServiceCreateResponse{
				Network: &apiv2.Network{
					Name:            pointer.Pointer("My Machine Network"),
					Meta:            &apiv2.Meta{},
					ParentNetworkId: pointer.Pointer("tenant-super-network-dualstack"),
					Partition:       pointer.Pointer("partition-three"),
					Project:         pointer.Pointer("p2"),
					Prefixes:        []string{"10.200.8.0/22", "2001:dc8::2:0/116"},
				}},
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := &networkServiceServer{
				log:  log,
				repo: repo,
			}
			got, err := n.Create(ctx, connect.NewRequest(tt.rq))
			if diff := cmp.Diff(err, tt.wantErr, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("diff = %s", diff)
			}

			if diff := cmp.Diff(
				tt.want, pointer.SafeDeref(got).Msg,
				cmp.Options{
					protocmp.Transform(),
					protocmp.IgnoreFields(
						&apiv2.Network{}, "consumption", "id", "vrf",
					),
					protocmp.IgnoreFields(
						&apiv2.Meta{}, "created_at", "updated_at",
					),
				},
			); diff != "" {
				t.Errorf("networkServiceServer.Create() = %v, want %vņdiff: %s", got.Msg, tt.want, diff)
			}
		})
	}
}

func Test_networkServiceServer_Delete(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	repo, closer := test.StartRepository(t, log)
	defer closer()

	ctx := t.Context()

	test.CreateTenants(t, repo, []*apiv2.TenantServiceCreateRequest{{Name: "t1"}, {Name: "t0"}})
	test.CreateProjects(t, repo, []*apiv2.ProjectServiceCreateRequest{{Name: "p1", Login: "t1"}, {Name: "p2", Login: "t1"}, {Name: "p3", Login: "t1"}, {Name: "p0", Login: "t0"}})
	test.CreateNetworks(t, repo, []*adminv2.NetworkServiceCreateRequest{{Id: pointer.Pointer("internet"), Prefixes: []string{"1.2.3.0/24"}}})
	test.CreateNetworks(t, repo, []*adminv2.NetworkServiceCreateRequest{
		{Id: pointer.Pointer("p1-network-a"), Name: pointer.Pointer("P1 Network"), Project: pointer.Pointer("p1"), Prefixes: []string{"2.3.4.0/24"}},
		{Id: pointer.Pointer("p1-network-b"), Name: pointer.Pointer("P1 Network"), Project: pointer.Pointer("p1"), Prefixes: []string{"2.3.5.0/24"}},
		{Id: pointer.Pointer("p2-network-a"), Name: pointer.Pointer("P2 Network"), Project: pointer.Pointer("p2"), Prefixes: []string{"3.3.1.0/24"}},
		{Id: pointer.Pointer("p2-network-b"), Name: pointer.Pointer("P2 Network"), Project: pointer.Pointer("p2"), Prefixes: []string{"3.3.2.0/24"}},
		{Id: pointer.Pointer("p3-network-a"), Name: pointer.Pointer("P3 Network"), Project: pointer.Pointer("p3"), Prefixes: []string{"4.3.4.0/24"}, Labels: &apiv2.Labels{Labels: map[string]string{"color": "red"}}},

		{Id: pointer.Pointer("underlay"), Name: pointer.Pointer("Underlay Network"), Project: pointer.Pointer("p0"), Prefixes: []string{"10.0.0.0/24"}},
	})

	tests := []struct {
		name    string
		rq      *apiv2.NetworkServiceDeleteRequest
		want    *apiv2.NetworkServiceDeleteResponse
		wantErr error
	}{
		{
			name: "delete existing",
			rq:   &apiv2.NetworkServiceDeleteRequest{Id: "p1-network-a", Project: "p1"},
			want: &apiv2.NetworkServiceDeleteResponse{
				Network: &apiv2.Network{Id: "p1-network-a", Meta: &apiv2.Meta{}, Name: pointer.Pointer("P1 Network"), Project: pointer.Pointer("p1"), Prefixes: []string{"2.3.4.0/24"}},
			},
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := &networkServiceServer{
				log:  log,
				repo: repo,
			}
			got, err := n.Delete(ctx, connect.NewRequest(tt.rq))
			if diff := cmp.Diff(err, tt.wantErr, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("diff = %s", diff)
			}
			// TODO check if network is actually deleted

			if diff := cmp.Diff(
				tt.want, pointer.SafeDeref(got).Msg,
				cmp.Options{
					protocmp.Transform(),
					protocmp.IgnoreFields(
						&apiv2.Network{}, "consumption", "id", "vrf",
					),
					protocmp.IgnoreFields(
						&apiv2.Meta{}, "created_at", "updated_at",
					),
				},
			); diff != "" {
				t.Errorf("networkServiceServer.Create() = %v, want %vņdiff: %s", got.Msg, tt.want, diff)
			}
		})
	}
}
