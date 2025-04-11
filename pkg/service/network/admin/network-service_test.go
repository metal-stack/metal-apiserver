package admin

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
		{Partition: &apiv2.Partition{Id: "partition-four", BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}}},
	})

	test.CreateNetworks(t, repo, []*adminv2.NetworkServiceCreateRequest{
		{
			Id:                       pointer.Pointer("tenant-super-network"),
			Prefixes:                 []string{"10.100.0.0/14"},
			DefaultChildPrefixLength: &apiv2.ChildPrefixLength{Ipv4: pointer.Pointer(uint32(22))},
			Type:                     apiv2.NetworkType_NETWORK_TYPE_PRIVATE_SUPER,
			Partition:                pointer.Pointer("partition-one"),
		},
		{
			Id:                       pointer.Pointer("tenant-super-network-v6"),
			Prefixes:                 []string{"2001:db8::/96"},
			DefaultChildPrefixLength: &apiv2.ChildPrefixLength{Ipv6: pointer.Pointer(uint32(112))},
			Type:                     apiv2.NetworkType_NETWORK_TYPE_PRIVATE_SUPER,
			Partition:                pointer.Pointer("partition-two"),
		},
		{
			Id:                       pointer.Pointer("tenant-super-network-dualstack"),
			Prefixes:                 []string{"2001:dc8::/96", "10.200.0.0/14"},
			DefaultChildPrefixLength: &apiv2.ChildPrefixLength{Ipv4: pointer.Pointer(uint32(22)), Ipv6: pointer.Pointer(uint32(112))},
			Type:                     apiv2.NetworkType_NETWORK_TYPE_PRIVATE_SUPER,
			Partition:                pointer.Pointer("partition-three"),
		},
		{
			Id:        pointer.Pointer("underlay"),
			Name:      pointer.Pointer("Underlay Network"),
			Project:   pointer.Pointer("p0"),
			Partition: pointer.Pointer("partition-one"),
			Prefixes:  []string{"10.0.0.0/24"},
			Type:      apiv2.NetworkType_NETWORK_TYPE_UNDERLAY,
		},
	})

	tests := []struct {
		name    string
		rq      *adminv2.NetworkServiceCreateRequest
		want    *adminv2.NetworkServiceCreateResponse
		wantErr error
	}{
		{
			name: "internet without project",
			rq:   &adminv2.NetworkServiceCreateRequest{Id: pointer.Pointer("internet"), Prefixes: []string{"1.2.3.0/24"}},
			want: &adminv2.NetworkServiceCreateResponse{
				Network: &apiv2.Network{
					Id:       "internet",
					Meta:     &apiv2.Meta{},
					Prefixes: []string{"1.2.3.0/24"},
					Type:     apiv2.NetworkType_NETWORK_TYPE_SHARED.Enum(),
				}},
			wantErr: nil,
		},
		{
			name:    "internet already exists",
			rq:      &adminv2.NetworkServiceCreateRequest{Id: pointer.Pointer("internet"), Prefixes: []string{"2.3.4.0/24"}},
			want:    nil,
			wantErr: errorutil.Conflict("cannot create network in database, entity already exists: internet"),
		},
		{
			name:    "internet-2 without project overlaps internet",
			rq:      &adminv2.NetworkServiceCreateRequest{Id: pointer.Pointer("internet-2"), Prefixes: []string{"1.2.3.0/24"}},
			want:    nil,
			wantErr: errorutil.Conflict("1.2.3.0/24 overlaps 1.2.3.0/24"),
		},
		{
			name:    "internet-3 with malformed prefixes",
			rq:      &adminv2.NetworkServiceCreateRequest{Id: pointer.Pointer("internet-3"), Prefixes: []string{"1.2.3.4.0/24"}},
			want:    nil,
			wantErr: errorutil.InvalidArgument(`given cidr "1.2.3.4.0/24" is not a valid ip with mask: netip.ParsePrefix("1.2.3.4.0/24"): ParseAddr("1.2.3.4.0"): IPv4 address too long`),
		},
		{
			name:    "internet-3 with malformed destinationprefixes",
			rq:      &adminv2.NetworkServiceCreateRequest{Id: pointer.Pointer("internet-3"), Prefixes: []string{"2.3.4.0/24"}, DestinationPrefixes: []string{"1.2.3.4.0/24"}},
			want:    nil,
			wantErr: errorutil.InvalidArgument(`given cidr "1.2.3.4.0/24" is not a valid ip with mask: netip.ParsePrefix("1.2.3.4.0/24"): ParseAddr("1.2.3.4.0"): IPv4 address too long`),
		},
		{
			name:    "internet-3 with mixed af for prefixes and destinationprefixes",
			rq:      &adminv2.NetworkServiceCreateRequest{Id: pointer.Pointer("internet-3"), Prefixes: []string{"2.3.4.0/24"}, DestinationPrefixes: []string{"2002:db8::/96"}},
			want:    nil,
			wantErr: errorutil.InvalidArgument(`addressfamily:IPv6 of destination prefixes is not present in existing prefixes`),
		},
		{
			name:    "super network without defaultchildprefixes",
			rq:      &adminv2.NetworkServiceCreateRequest{Id: pointer.Pointer("super-1"), Prefixes: []string{"2.3.4.0/24"}, Type: apiv2.NetworkType_NETWORK_TYPE_PRIVATE_SUPER},
			want:    nil,
			wantErr: errorutil.InvalidArgument(`private super network must always contain a defaultchildprefixlength`),
		},
		{
			name: "super network without defaultchildprefixes",
			rq: &adminv2.NetworkServiceCreateRequest{
				Id:                       pointer.Pointer("super-1"),
				Prefixes:                 []string{"2.3.4.0/24"},
				DefaultChildPrefixLength: &apiv2.ChildPrefixLength{Ipv4: pointer.Pointer(uint32(26))},
			},
			want:    nil,
			wantErr: errorutil.InvalidArgument(`defaultchildprefixlength can only be set for privatesuper networks`),
		},
		{
			name: "super network with defaultchildprefixes, but wrong af",
			rq: &adminv2.NetworkServiceCreateRequest{
				Id:                       pointer.Pointer("super-1"),
				Prefixes:                 []string{"2.3.4.0/24"},
				Type:                     apiv2.NetworkType_NETWORK_TYPE_PRIVATE_SUPER,
				DefaultChildPrefixLength: &apiv2.ChildPrefixLength{Ipv6: pointer.Pointer(uint32(112))},
			},
			want:    nil,
			wantErr: errorutil.InvalidArgument(`childprefixlength for addressfamily: "IPv6" specified, but no "IPv6" addressfamily found in prefixes`),
		},
		{
			name: "super network with defaultchildprefixes, but wrong length",
			rq: &adminv2.NetworkServiceCreateRequest{
				Id:                       pointer.Pointer("super-1"),
				Prefixes:                 []string{"2.3.4.0/24"},
				Type:                     apiv2.NetworkType_NETWORK_TYPE_PRIVATE_SUPER,
				DefaultChildPrefixLength: &apiv2.ChildPrefixLength{Ipv4: pointer.Pointer(uint32(22))}},
			want:    nil,
			wantErr: errorutil.InvalidArgument(`given defaultchildprefixlength 22 is not greater than prefix length of:2.3.4.0/24`),
		},
		{
			name: "network with additionalannounceblecidrs but not privatesuper",
			rq: &adminv2.NetworkServiceCreateRequest{
				Id:                         pointer.Pointer("super-1"),
				Prefixes:                   []string{"2.3.4.0/24"},
				AdditionalAnnouncableCidrs: []string{"10.100.0.0/24"},
			},

			want:    nil,
			wantErr: errorutil.InvalidArgument(`additionalannouncablecidrs can only be set in a private super network`),
		},
		{
			name: "network with unknown partition",
			rq: &adminv2.NetworkServiceCreateRequest{
				Id:        pointer.Pointer("super-1"),
				Prefixes:  []string{"2.3.4.0/24"},
				Partition: pointer.Pointer("partition-42"),
			},

			want:    nil,
			wantErr: errorutil.NotFound(`no partition with id "partition-42" found`),
		},
		{
			name: "super network already exist in this partition",
			rq: &adminv2.NetworkServiceCreateRequest{
				Id:                       pointer.Pointer("super-1"),
				Prefixes:                 []string{"2.3.4.0/24"},
				Partition:                pointer.Pointer("partition-one"),
				DefaultChildPrefixLength: &apiv2.ChildPrefixLength{Ipv4: pointer.Pointer(uint32(26))},
				Type:                     apiv2.NetworkType_NETWORK_TYPE_PRIVATE_SUPER,
			},

			want:    nil,
			wantErr: errorutil.InvalidArgument(`partition with id "partition-one" already has a network of type NETWORK_TYPE_PRIVATE_SUPER`),
		},
		{
			name: "underlay network already exist in this partition",
			rq: &adminv2.NetworkServiceCreateRequest{
				Id:        pointer.Pointer("underlay-1"),
				Prefixes:  []string{"2.3.4.0/24"},
				Partition: pointer.Pointer("partition-one"),
				Type:      apiv2.NetworkType_NETWORK_TYPE_UNDERLAY,
			},

			want:    nil,
			wantErr: errorutil.InvalidArgument(`partition with id "partition-one" already has a network of type NETWORK_TYPE_UNDERLAY`),
		},
		{
			name: "super network can not have nat",
			rq: &adminv2.NetworkServiceCreateRequest{
				Id:                       pointer.Pointer("super-1"),
				Prefixes:                 []string{"2.3.4.0/24"},
				DefaultChildPrefixLength: &apiv2.ChildPrefixLength{Ipv4: pointer.Pointer(uint32(26))},
				NatType:                  apiv2.NATType_NAT_TYPE_IPV4_MASQUERADE.Enum(),
				Type:                     apiv2.NetworkType_NETWORK_TYPE_PRIVATE_SUPER,
			},
			want:    nil,
			wantErr: errorutil.InvalidArgument(`network with type:NETWORK_TYPE_PRIVATE_SUPER does not support nat`),
		},
		{
			name: "underlay network can not have nat",
			rq: &adminv2.NetworkServiceCreateRequest{
				Id:       pointer.Pointer("underlay-1"),
				Prefixes: []string{"2.3.4.0/24"},
				NatType:  apiv2.NATType_NAT_TYPE_IPV4_MASQUERADE.Enum(),
				Type:     apiv2.NetworkType_NETWORK_TYPE_UNDERLAY,
			},
			want:    nil,
			wantErr: errorutil.InvalidArgument(`network with type:NETWORK_TYPE_UNDERLAY does not support nat`),
		},
		{
			name: "overlapping prefixes",
			rq: &adminv2.NetworkServiceCreateRequest{
				Id:       pointer.Pointer("overlapping prefixes"),
				Prefixes: []string{"5.0.0.0/16", "5.0.1.0/24"},
			},
			want:    nil,
			wantErr: errorutil.InvalidArgument(`5.0.1.0/24 overlaps 5.0.0.0/16`),
		},
		{
			name: "overlapping destinationprefixes",
			rq: &adminv2.NetworkServiceCreateRequest{
				Id:       pointer.Pointer("overlapping destinationprefixes"),
				Prefixes: []string{"6.0.0.0/16", "6.0.1.0/24"},
			},
			want:    nil,
			wantErr: errorutil.InvalidArgument(`6.0.1.0/24 overlaps 6.0.0.0/16`),
		},
		{
			name: "dualstack internet without project",
			rq:   &adminv2.NetworkServiceCreateRequest{Id: pointer.Pointer("internet-dualstack"), Prefixes: []string{"2.3.4.0/24", "2002:db8::/96"}},
			want: &adminv2.NetworkServiceCreateResponse{
				Network: &apiv2.Network{
					Id:       "internet-dualstack",
					Meta:     &apiv2.Meta{},
					Prefixes: []string{"2.3.4.0/24", "2002:db8::/96"},
					Type:     apiv2.NetworkType_NETWORK_TYPE_SHARED.Enum(),
				}},
			wantErr: nil,
		},
		{
			name: "dualstack super",
			rq: &adminv2.NetworkServiceCreateRequest{
				Id:                         pointer.Pointer("dualstack-super"),
				Name:                       pointer.Pointer("Super Network"),
				Description:                pointer.Pointer("Super Network"),
				Prefixes:                   []string{"2002:dc8::/96", "11.200.0.0/14"},
				DefaultChildPrefixLength:   &apiv2.ChildPrefixLength{Ipv4: pointer.Pointer(uint32(22)), Ipv6: pointer.Pointer(uint32(112))},
				Type:                       apiv2.NetworkType_NETWORK_TYPE_PRIVATE_SUPER,
				Partition:                  pointer.Pointer("partition-four"),
				Vrf:                        pointer.Pointer(uint32(9)),
				AdditionalAnnouncableCidrs: []string{"10.100.0.0/16"},
				Labels:                     &apiv2.Labels{Labels: map[string]string{"super": "true"}},
			},
			want: &adminv2.NetworkServiceCreateResponse{
				Network: &apiv2.Network{
					Id:                         "dualstack-super",
					Meta:                       &apiv2.Meta{Labels: &apiv2.Labels{Labels: map[string]string{"super": "true"}}},
					Name:                       pointer.Pointer("Super Network"),
					Description:                pointer.Pointer("Super Network"),
					Prefixes:                   []string{"11.200.0.0/14", "2002:dc8::/96"},
					DefaultChildPrefixLength:   &apiv2.ChildPrefixLength{Ipv4: pointer.Pointer(uint32(22)), Ipv6: pointer.Pointer(uint32(112))},
					Type:                       apiv2.NetworkType_NETWORK_TYPE_PRIVATE_SUPER.Enum(),
					Partition:                  pointer.Pointer("partition-four"),
					Vrf:                        pointer.Pointer(uint32(9)),
					AdditionalAnnouncableCidrs: []string{"10.100.0.0/16"},
				}},
			wantErr: nil,
		},
		{
			name: "underlay",
			rq: &adminv2.NetworkServiceCreateRequest{
				Id: pointer.Pointer("underlay-2"), Name: pointer.Pointer("Underlay Network"), Project: pointer.Pointer("p0"), Prefixes: []string{"16.0.0.0/24"}, Type: apiv2.NetworkType_NETWORK_TYPE_UNDERLAY,
			},
			want: &adminv2.NetworkServiceCreateResponse{
				Network: &apiv2.Network{
					Id:       "underlay-2",
					Meta:     &apiv2.Meta{},
					Name:     pointer.Pointer("Underlay Network"),
					Project:  pointer.Pointer("p0"),
					Prefixes: []string{"16.0.0.0/24"},
					Type:     apiv2.NetworkType_NETWORK_TYPE_UNDERLAY.Enum(),
				},
			},
			wantErr: nil,
		},
		// TODO: parentNetworkID, Addressfamily
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
				t.Errorf("networkServiceServer.Create() = %v, want %vņdiff: %s", pointer.SafeDeref(got).Msg, tt.want, diff)
			}
		})
	}
}

func Test_networkServiceServer_Delete(t *testing.T) {
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
		{Partition: &apiv2.Partition{Id: "partition-four", BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}}},
	})

	test.CreateNetworks(t, repo, []*adminv2.NetworkServiceCreateRequest{
		{
			Id:                       pointer.Pointer("tenant-super-network"),
			Prefixes:                 []string{"10.100.0.0/14"},
			DefaultChildPrefixLength: &apiv2.ChildPrefixLength{Ipv4: pointer.Pointer(uint32(22))},
			Type:                     apiv2.NetworkType_NETWORK_TYPE_PRIVATE_SUPER,
			Partition:                pointer.Pointer("partition-one"),
		},
		{
			Id:                       pointer.Pointer("tenant-super-network-v6"),
			Prefixes:                 []string{"2001:db8::/96"},
			DefaultChildPrefixLength: &apiv2.ChildPrefixLength{Ipv6: pointer.Pointer(uint32(112))},
			Type:                     apiv2.NetworkType_NETWORK_TYPE_PRIVATE_SUPER,
			Partition:                pointer.Pointer("partition-two"),
		},
		{
			Id:                       pointer.Pointer("tenant-super-network-dualstack"),
			Prefixes:                 []string{"2001:dc8::/96", "10.200.0.0/14"},
			DefaultChildPrefixLength: &apiv2.ChildPrefixLength{Ipv4: pointer.Pointer(uint32(22)), Ipv6: pointer.Pointer(uint32(112))},
			Type:                     apiv2.NetworkType_NETWORK_TYPE_PRIVATE_SUPER,
			Partition:                pointer.Pointer("partition-three"),
		},
		{
			Id:       pointer.Pointer("underlay"),
			Name:     pointer.Pointer("Underlay Network"),
			Project:  pointer.Pointer("p0"),
			Prefixes: []string{"10.0.0.0/24"},
			Type:     apiv2.NetworkType_NETWORK_TYPE_UNDERLAY,
		},
	})
	networkMap := test.AllocateNetworks(t, repo, []*apiv2.NetworkServiceCreateRequest{
		{Name: pointer.Pointer("tenant-1"), Project: "p1", Partition: pointer.Pointer("partition-one")},
		{Name: pointer.Pointer("tenant-2"), Project: "p1", Partition: pointer.Pointer("partition-one")},
	})

	test.CreateIPs(t, repo, []*apiv2.IPServiceCreateRequest{
		{Network: networkMap["tenant-1"], Project: "p1", Name: pointer.Pointer("ip-1")},
	})

	tests := []struct {
		name    string
		rq      *adminv2.NetworkServiceDeleteRequest
		want    *adminv2.NetworkServiceDeleteResponse
		wantErr error
	}{
		{
			name:    "network has ips",
			rq:      &adminv2.NetworkServiceDeleteRequest{Id: networkMap["tenant-1"]},
			want:    nil,
			wantErr: errorutil.InvalidArgument(`there are still 1 ips present in one of the prefixes:{10.100.0.0 22}`),
		},
		{
			name:    "super network has child",
			rq:      &adminv2.NetworkServiceDeleteRequest{Id: "tenant-super-network"},
			want:    nil,
			wantErr: errorutil.InvalidArgument(`cannot remove network with existing child networks`),
		},
		{
			name:    "not existing",
			rq:      &adminv2.NetworkServiceDeleteRequest{Id: "not-existing"},
			want:    nil,
			wantErr: errorutil.NotFound(`no network with id "not-existing" found`),
		},
		{
			name: "existing",
			rq:   &adminv2.NetworkServiceDeleteRequest{Id: networkMap["tenant-2"]},
			want: &adminv2.NetworkServiceDeleteResponse{
				Network: &apiv2.Network{
					Id:              networkMap["tenant-2"],
					Meta:            &apiv2.Meta{},
					Name:            pointer.Pointer("tenant-2"),
					Partition:       pointer.Pointer("partition-one"),
					Project:         pointer.Pointer("p1"),
					Prefixes:        []string{"10.100.4.0/22"},
					Vrf:             pointer.Pointer(uint32(5)),
					ParentNetworkId: pointer.Pointer("tenant-super-network"),
					Type:            apiv2.NetworkType_NETWORK_TYPE_PRIVATE.Enum(),
				},
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
				t.Errorf("networkServiceServer.Delete() = %v, want %vņdiff: %s", pointer.SafeDeref(got).Msg, tt.want, diff)
			}
		})
	}
}

func Test_networkServiceServer_List(t *testing.T) {
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
		{Partition: &apiv2.Partition{Id: "partition-four", BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}}},
	})

	test.CreateNetworks(t, repo, []*adminv2.NetworkServiceCreateRequest{
		{
			Id:                       pointer.Pointer("tenant-super-network"),
			Prefixes:                 []string{"10.100.0.0/14"},
			DefaultChildPrefixLength: &apiv2.ChildPrefixLength{Ipv4: pointer.Pointer(uint32(22))},
			Type:                     apiv2.NetworkType_NETWORK_TYPE_PRIVATE_SUPER,
			Partition:                pointer.Pointer("partition-one"),
		},
		{
			Id:                       pointer.Pointer("tenant-super-network-v6"),
			Prefixes:                 []string{"2001:db8::/96"},
			DefaultChildPrefixLength: &apiv2.ChildPrefixLength{Ipv6: pointer.Pointer(uint32(112))},
			Type:                     apiv2.NetworkType_NETWORK_TYPE_PRIVATE_SUPER,
			Partition:                pointer.Pointer("partition-two"),
		},
		{
			Id:                       pointer.Pointer("tenant-super-network-dualstack"),
			Prefixes:                 []string{"2001:dc8::/96", "10.200.0.0/14"},
			DefaultChildPrefixLength: &apiv2.ChildPrefixLength{Ipv4: pointer.Pointer(uint32(22)), Ipv6: pointer.Pointer(uint32(112))},
			Type:                     apiv2.NetworkType_NETWORK_TYPE_PRIVATE_SUPER,
			Partition:                pointer.Pointer("partition-three"),
		},
		{
			Id:       pointer.Pointer("underlay"),
			Name:     pointer.Pointer("Underlay Network"),
			Project:  pointer.Pointer("p0"),
			Prefixes: []string{"10.0.0.0/24"},
			Type:     apiv2.NetworkType_NETWORK_TYPE_UNDERLAY,
		},
		{
			Id:                  pointer.Pointer("internet"),
			Prefixes:            []string{"20.0.0.0/24"},
			DestinationPrefixes: []string{"0.0.0.0/0"},
			Type:                apiv2.NetworkType_NETWORK_TYPE_SHARED,
		},
	})
	networkMap := test.AllocateNetworks(t, repo, []*apiv2.NetworkServiceCreateRequest{
		{Name: pointer.Pointer("tenant-1"), Project: "p1", Partition: pointer.Pointer("partition-one")},
		{Name: pointer.Pointer("tenant-2"), Project: "p1", Partition: pointer.Pointer("partition-one"), Labels: &apiv2.Labels{Labels: map[string]string{"size": "small", "color": "blue"}}},
	})

	test.CreateIPs(t, repo, []*apiv2.IPServiceCreateRequest{
		{Network: networkMap["tenant-1"], Project: "p1", Name: pointer.Pointer("ip-1")},
	})

	tests := []struct {
		name    string
		rq      *adminv2.NetworkServiceListRequest
		want    *adminv2.NetworkServiceListResponse
		wantErr error
	}{
		// {
		// 	name: "list all",
		// 	rq:   &adminv2.NetworkServiceListRequest{},
		// 	want: &adminv2.NetworkServiceListResponse{
		// 		Networks: []*apiv2.Network{
		// 			{
		// 				Id:              networkMap["tenant-1"],
		// 				Meta:            &apiv2.Meta{},
		// 				Name:            pointer.Pointer("tenant-1"),
		// 				Partition:       pointer.Pointer("partition-one"),
		// 				Project:         pointer.Pointer("p1"),
		// 				Prefixes:        []string{"10.100.0.0/22"},
		// 				Vrf:             pointer.Pointer(uint32(4)),
		// 				ParentNetworkId: pointer.Pointer("tenant-super-network"),
		// 			},
		// 			{
		// 				Id:              networkMap["tenant-2"],
		// 				Meta:            &apiv2.Meta{},
		// 				Name:            pointer.Pointer("tenant-2"),
		// 				Partition:       pointer.Pointer("partition-one"),
		// 				Project:         pointer.Pointer("p1"),
		// 				Prefixes:        []string{"10.100.4.0/22"},
		// 				Vrf:             pointer.Pointer(uint32(5)),
		// 				ParentNetworkId: pointer.Pointer("tenant-super-network"),
		// 			},
		// 			{
		// 				Id:                       "tenant-super-network-v6",
		// 				Meta:                     &apiv2.Meta{},
		// 				Partition:                pointer.Pointer("partition-two"),
		// 				Prefixes:                 []string{"2001:db8::/96"},
		// 				Options:                  &apiv2.NetworkOptions{PrivateSuper: true},
		// 				DefaultChildPrefixLength: []*apiv2.ChildPrefixLength{{AddressFamily: apiv2.IPAddressFamily_IP_ADDRESS_FAMILY_V6, Length: 112}},
		// 			},
		// 			{
		// 				Id:                       "tenant-super-network",
		// 				Meta:                     &apiv2.Meta{},
		// 				Partition:                pointer.Pointer("partition-one"),
		// 				Prefixes:                 []string{"10.100.0.0/14"},
		// 				Options:                  &apiv2.NetworkOptions{PrivateSuper: true},
		// 				DefaultChildPrefixLength: []*apiv2.ChildPrefixLength{{AddressFamily: apiv2.IPAddressFamily_IP_ADDRESS_FAMILY_V4, Length: 22}},
		// 			},
		// 			{
		// 				Id:                       "tenant-super-network-dualstack",
		// 				Meta:                     &apiv2.Meta{},
		// 				Partition:                pointer.Pointer("partition-three"),
		// 				Prefixes:                 []string{"10.200.0.0/14", "2001:dc8::/96"},
		// 				Options:                  &apiv2.NetworkOptions{PrivateSuper: true},
		// 				DefaultChildPrefixLength: []*apiv2.ChildPrefixLength{{AddressFamily: apiv2.IPAddressFamily_IP_ADDRESS_FAMILY_V4, Length: 22}, {AddressFamily: apiv2.IPAddressFamily_IP_ADDRESS_FAMILY_V6, Length: 112}},
		// 			},
		// 			{
		// 				Id:       "underlay",
		// 				Meta:     &apiv2.Meta{},
		// 				Name:     pointer.Pointer("Underlay Network"),
		// 				Project:  pointer.Pointer("p0"),
		// 				Prefixes: []string{"10.0.0.0/24"},
		// 				Options:  &apiv2.NetworkOptions{Underlay: true},
		// 			},
		// 		},
		// 	},
		// 	wantErr: nil,
		// },
		{
			name: "specific id",
			rq: &adminv2.NetworkServiceListRequest{
				Query: &apiv2.NetworkQuery{Id: pointer.Pointer(networkMap["tenant-1"])},
			},
			want: &adminv2.NetworkServiceListResponse{
				Networks: []*apiv2.Network{
					{
						Id:              networkMap["tenant-1"],
						Meta:            &apiv2.Meta{},
						Name:            pointer.Pointer("tenant-1"),
						Partition:       pointer.Pointer("partition-one"),
						Project:         pointer.Pointer("p1"),
						Prefixes:        []string{"10.100.0.0/22"},
						Vrf:             pointer.Pointer(uint32(4)),
						ParentNetworkId: pointer.Pointer("tenant-super-network"),
						Type:            apiv2.NetworkType_NETWORK_TYPE_PRIVATE.Enum(),
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "underlay",
			rq: &adminv2.NetworkServiceListRequest{
				Query: &apiv2.NetworkQuery{Type: apiv2.NetworkType_NETWORK_TYPE_UNDERLAY.Enum()},
			},
			want: &adminv2.NetworkServiceListResponse{
				Networks: []*apiv2.Network{
					{
						Id:       "underlay",
						Meta:     &apiv2.Meta{},
						Name:     pointer.Pointer("Underlay Network"),
						Project:  pointer.Pointer("p0"),
						Prefixes: []string{"10.0.0.0/24"},
						Type:     apiv2.NetworkType_NETWORK_TYPE_UNDERLAY.Enum(),
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "super tenant in partition-one",
			rq: &adminv2.NetworkServiceListRequest{
				Query: &apiv2.NetworkQuery{Partition: pointer.Pointer("partition-one"), Type: apiv2.NetworkType_NETWORK_TYPE_PRIVATE_SUPER.Enum()},
			},
			want: &adminv2.NetworkServiceListResponse{
				Networks: []*apiv2.Network{
					{
						Id:                       "tenant-super-network",
						Meta:                     &apiv2.Meta{},
						Partition:                pointer.Pointer("partition-one"),
						Prefixes:                 []string{"10.100.0.0/14"},
						Type:                     apiv2.NetworkType_NETWORK_TYPE_PRIVATE_SUPER.Enum(),
						DefaultChildPrefixLength: &apiv2.ChildPrefixLength{Ipv4: pointer.Pointer(uint32(22))},
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "with v6 prefixes",
			rq: &adminv2.NetworkServiceListRequest{
				Query: &apiv2.NetworkQuery{AddressFamily: apiv2.IPAddressFamily_IP_ADDRESS_FAMILY_V6.Enum()},
			},
			want: &adminv2.NetworkServiceListResponse{
				Networks: []*apiv2.Network{
					{
						Id:                       "tenant-super-network-dualstack",
						Meta:                     &apiv2.Meta{},
						Partition:                pointer.Pointer("partition-three"),
						Prefixes:                 []string{"10.200.0.0/14", "2001:dc8::/96"},
						Type:                     apiv2.NetworkType_NETWORK_TYPE_PRIVATE_SUPER.Enum(),
						DefaultChildPrefixLength: &apiv2.ChildPrefixLength{Ipv4: pointer.Pointer(uint32(22)), Ipv6: pointer.Pointer(uint32(112))},
					},
					{
						Id:                       "tenant-super-network-v6",
						Meta:                     &apiv2.Meta{},
						Partition:                pointer.Pointer("partition-two"),
						Prefixes:                 []string{"2001:db8::/96"},
						Type:                     apiv2.NetworkType_NETWORK_TYPE_PRIVATE_SUPER.Enum(),
						DefaultChildPrefixLength: &apiv2.ChildPrefixLength{Ipv6: pointer.Pointer(uint32(112))},
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "specific prefixes",
			rq: &adminv2.NetworkServiceListRequest{
				Query: &apiv2.NetworkQuery{Prefixes: []string{"2001:dc8::/96"}},
			},
			want: &adminv2.NetworkServiceListResponse{
				Networks: []*apiv2.Network{
					{
						Id:                       "tenant-super-network-dualstack",
						Meta:                     &apiv2.Meta{},
						Partition:                pointer.Pointer("partition-three"),
						Prefixes:                 []string{"10.200.0.0/14", "2001:dc8::/96"},
						Type:                     apiv2.NetworkType_NETWORK_TYPE_PRIVATE_SUPER.Enum(),
						DefaultChildPrefixLength: &apiv2.ChildPrefixLength{Ipv4: pointer.Pointer(uint32(22)), Ipv6: pointer.Pointer(uint32(112))},
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "specific destinationprefixes",
			rq: &adminv2.NetworkServiceListRequest{
				Query: &apiv2.NetworkQuery{DestinationPrefixes: []string{"0.0.0.0/0"}},
			},
			want: &adminv2.NetworkServiceListResponse{
				Networks: []*apiv2.Network{
					{
						Id:                  "internet",
						Meta:                &apiv2.Meta{},
						Prefixes:            []string{"20.0.0.0/24"},
						DestinationPrefixes: []string{"0.0.0.0/0"},
						Type:                apiv2.NetworkType_NETWORK_TYPE_SHARED.Enum(),
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "by labels",
			rq: &adminv2.NetworkServiceListRequest{
				Query: &apiv2.NetworkQuery{Labels: &apiv2.Labels{Labels: map[string]string{"size": "small"}}},
			},
			want: &adminv2.NetworkServiceListResponse{
				Networks: []*apiv2.Network{
					{
						Id:              networkMap["tenant-2"],
						Meta:            &apiv2.Meta{Labels: &apiv2.Labels{Labels: map[string]string{"size": "small", "color": "blue"}}},
						Name:            pointer.Pointer("tenant-2"),
						Partition:       pointer.Pointer("partition-one"),
						Project:         pointer.Pointer("p1"),
						Prefixes:        []string{"10.100.4.0/22"},
						Vrf:             pointer.Pointer(uint32(5)),
						ParentNetworkId: pointer.Pointer("tenant-super-network"),
						Type:            apiv2.NetworkType_NETWORK_TYPE_PRIVATE.Enum(),
					},
				},
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
				t.Errorf("networkServiceServer.List() = %v, want %vņdiff: %s", pointer.SafeDeref(got).Msg, tt.want, diff)
			}
		})
	}
}

func Test_networkServiceServer_Update(t *testing.T) {
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
		{Partition: &apiv2.Partition{Id: "partition-four", BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}}},
	})

	test.CreateNetworks(t, repo, []*adminv2.NetworkServiceCreateRequest{
		{
			Id:                       pointer.Pointer("tenant-super-network"),
			Prefixes:                 []string{"10.100.0.0/14"},
			DefaultChildPrefixLength: &apiv2.ChildPrefixLength{Ipv4: pointer.Pointer(uint32(22))},
			Type:                     apiv2.NetworkType_NETWORK_TYPE_PRIVATE_SUPER,
			Partition:                pointer.Pointer("partition-one"),
		},
		{
			Id:                       pointer.Pointer("tenant-super-network-v6"),
			Prefixes:                 []string{"2001:db8::/96"},
			DefaultChildPrefixLength: &apiv2.ChildPrefixLength{Ipv6: pointer.Pointer(uint32(112))},
			Type:                     apiv2.NetworkType_NETWORK_TYPE_PRIVATE_SUPER,
			Partition:                pointer.Pointer("partition-two"),
		},
		{
			Id:                       pointer.Pointer("tenant-super-network-dualstack"),
			Prefixes:                 []string{"2001:dc8::/96", "10.200.0.0/14"},
			DefaultChildPrefixLength: &apiv2.ChildPrefixLength{Ipv4: pointer.Pointer(uint32(22)), Ipv6: pointer.Pointer(uint32(112))},
			Type:                     apiv2.NetworkType_NETWORK_TYPE_PRIVATE_SUPER,
			Partition:                pointer.Pointer("partition-three"),
		},
		{
			Id:       pointer.Pointer("underlay"),
			Name:     pointer.Pointer("Underlay Network"),
			Project:  pointer.Pointer("p0"),
			Prefixes: []string{"10.0.0.0/24"},
			Type:     apiv2.NetworkType_NETWORK_TYPE_UNDERLAY,
		},
		{
			Id:                  pointer.Pointer("internet"),
			Prefixes:            []string{"20.0.0.0/24"},
			DestinationPrefixes: []string{"0.0.0.0/0"},
			Type:                apiv2.NetworkType_NETWORK_TYPE_SHARED,
		},
	})

	networkMap := test.AllocateNetworks(t, repo, []*apiv2.NetworkServiceCreateRequest{
		{Name: pointer.Pointer("tenant-1"), Project: "p1", Partition: pointer.Pointer("partition-one")},
		{Name: pointer.Pointer("tenant-2"), Project: "p1", Partition: pointer.Pointer("partition-one"), Labels: &apiv2.Labels{Labels: map[string]string{"size": "small", "color": "blue"}}},
	})

	tests := []struct {
		name    string
		rq      *adminv2.NetworkServiceUpdateRequest
		want    *adminv2.NetworkServiceUpdateResponse
		wantErr error
	}{
		{
			name: "add label to tenant network",
			rq: &adminv2.NetworkServiceUpdateRequest{
				Id:     networkMap["tenant-1"],
				Labels: &apiv2.Labels{Labels: map[string]string{"color": "red", "size": "large"}},
			},
			want: &adminv2.NetworkServiceUpdateResponse{
				Network: &apiv2.Network{
					Id: networkMap["tenant-1"],
					Meta: &apiv2.Meta{
						Labels: &apiv2.Labels{Labels: map[string]string{"color": "red", "size": "large"}},
					},
					Name:            pointer.Pointer("tenant-1"),
					Partition:       pointer.Pointer("partition-one"),
					Project:         pointer.Pointer("p1"),
					Prefixes:        []string{"10.100.0.0/22"},
					Vrf:             pointer.Pointer(uint32(10)),
					ParentNetworkId: pointer.Pointer("tenant-super-network"),
					Type:            apiv2.NetworkType_NETWORK_TYPE_PRIVATE.Enum(),
				},
			},
			wantErr: nil,
		},
		{
			name: "add prefixes to tenant network",
			rq: &adminv2.NetworkServiceUpdateRequest{
				Id:       networkMap["tenant-1"],
				Prefixes: []string{"10.100.0.0/22", "10.101.0.0/22", "10.102.0.0/22"},
			},
			want:    nil,
			wantErr: errorutil.InvalidArgument("cannot change prefixes in child networks"),
		},
		{
			name: "add prefixes to tenant super network",
			rq: &adminv2.NetworkServiceUpdateRequest{
				Id:       "tenant-super-network",
				Prefixes: []string{"10.100.0.0/14", "10.101.0.0/14"},
			},
			want: &adminv2.NetworkServiceUpdateResponse{
				Network: &apiv2.Network{
					Id:                       "tenant-super-network",
					Meta:                     &apiv2.Meta{},
					Partition:                pointer.Pointer("partition-one"),
					Prefixes:                 []string{"10.100.0.0/14", "10.101.0.0/14"},
					Type:                     apiv2.NetworkType_NETWORK_TYPE_PRIVATE_SUPER.Enum(),
					DefaultChildPrefixLength: &apiv2.ChildPrefixLength{Ipv4: pointer.Pointer(uint32(22))},
				},
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
				t.Errorf("networkServiceServer.Update() = %v, want %vņdiff: %s", pointer.SafeDeref(got).Msg, tt.want, diff)
			}
		})
	}
}
