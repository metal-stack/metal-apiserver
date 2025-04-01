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
			rq:      &adminv2.NetworkServiceCreateRequest{Id: pointer.Pointer("super-1"), Prefixes: []string{"2.3.4.0/24"}, Options: &apiv2.NetworkOptions{PrivateSuper: true}},
			want:    nil,
			wantErr: errorutil.InvalidArgument(`private super network must always contain a defaultchildprefixlength`),
		},
		{
			name: "super network without defaultchildprefixes",
			rq: &adminv2.NetworkServiceCreateRequest{
				Id:       pointer.Pointer("super-1"),
				Prefixes: []string{"2.3.4.0/24"},
				DefaultChildPrefixLength: []*apiv2.ChildPrefixLength{
					{AddressFamily: apiv2.IPAddressFamily_IP_ADDRESS_FAMILY_V4, Length: 26},
				},
			},
			want:    nil,
			wantErr: errorutil.InvalidArgument(`defaultchildprefixlength can only be set for privatesuper networks`),
		},
		{
			name: "super network with defaultchildprefixes, but wrong af",
			rq: &adminv2.NetworkServiceCreateRequest{
				Id:       pointer.Pointer("super-1"),
				Prefixes: []string{"2.3.4.0/24"},
				Options:  &apiv2.NetworkOptions{PrivateSuper: true},
				DefaultChildPrefixLength: []*apiv2.ChildPrefixLength{
					{AddressFamily: apiv2.IPAddressFamily_IP_ADDRESS_FAMILY_V6, Length: 112},
				}},
			want:    nil,
			wantErr: errorutil.InvalidArgument(`private super network must always contain a defaultchildprefixlength per addressfamily:IPv4`),
		},
		{
			name: "super network with defaultchildprefixes, but wrong length",
			rq: &adminv2.NetworkServiceCreateRequest{
				Id:       pointer.Pointer("super-1"),
				Prefixes: []string{"2.3.4.0/24"},
				Options:  &apiv2.NetworkOptions{PrivateSuper: true},
				DefaultChildPrefixLength: []*apiv2.ChildPrefixLength{
					{AddressFamily: apiv2.IPAddressFamily_IP_ADDRESS_FAMILY_V4, Length: 22},
				}},
			want:    nil,
			wantErr: errorutil.InvalidArgument(`given defaultchildprefixlength 22 is not greater than prefix length of:2.3.4.0/24`),
		},
		{
			name: "network with additionalannounceblecidrs but not privatesuper",
			rq: &adminv2.NetworkServiceCreateRequest{
				Id:                          pointer.Pointer("super-1"),
				Prefixes:                    []string{"2.3.4.0/24"},
				Options:                     &apiv2.NetworkOptions{PrivateSuper: false},
				AdditionalAnnounceableCidrs: []string{"10.100.0.0/24"},
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
				Id:        pointer.Pointer("super-1"),
				Prefixes:  []string{"2.3.4.0/24"},
				Partition: pointer.Pointer("partition-one"),
				DefaultChildPrefixLength: []*apiv2.ChildPrefixLength{
					{AddressFamily: apiv2.IPAddressFamily_IP_ADDRESS_FAMILY_V4, Length: 26},
				},
				Options: &apiv2.NetworkOptions{PrivateSuper: true},
			},

			want:    nil,
			wantErr: errorutil.InvalidArgument(`partition with id "partition-one" already has a private super network`),
		},
		{
			name: "underlay network already exist in this partition",
			rq: &adminv2.NetworkServiceCreateRequest{
				Id:        pointer.Pointer("underlay-1"),
				Prefixes:  []string{"2.3.4.0/24"},
				Partition: pointer.Pointer("partition-one"),
				Options:   &apiv2.NetworkOptions{Underlay: true},
			},

			want:    nil,
			wantErr: errorutil.InvalidArgument(`partition with id "partition-one" already has an underlay network`),
		},
		{
			name: "underlay network already exist in this partition",
			rq: &adminv2.NetworkServiceCreateRequest{
				Id:        pointer.Pointer("underlay-1"),
				Prefixes:  []string{"2.3.4.0/24"},
				Partition: pointer.Pointer("partition-one"),
				Options:   &apiv2.NetworkOptions{Underlay: true},
			},
			want: nil,
			// FIXME: this is not true, see above
			wantErr: errorutil.InvalidArgument(`partition with id "partition-one" already has an underlay network`),
		},
		{
			name: "super network can not have nat",
			rq: &adminv2.NetworkServiceCreateRequest{
				Id:       pointer.Pointer("super-1"),
				Prefixes: []string{"2.3.4.0/24"},
				DefaultChildPrefixLength: []*apiv2.ChildPrefixLength{
					{AddressFamily: apiv2.IPAddressFamily_IP_ADDRESS_FAMILY_V4, Length: 26},
				},
				Options: &apiv2.NetworkOptions{Nat: true, PrivateSuper: true},
			},
			want:    nil,
			wantErr: errorutil.InvalidArgument(`private super or underlay network is not supposed to NAT`),
		},
		{
			name: "underlay network can not have nat",
			rq: &adminv2.NetworkServiceCreateRequest{
				Id:       pointer.Pointer("underlay-1"),
				Prefixes: []string{"2.3.4.0/24"},
				Options:  &apiv2.NetworkOptions{Underlay: true, Nat: true},
			},
			want:    nil,
			wantErr: errorutil.InvalidArgument(`private super or underlay network is not supposed to NAT`),
		},
		{
			name: "dualstack internet without project",
			rq:   &adminv2.NetworkServiceCreateRequest{Id: pointer.Pointer("internet-dualstack"), Prefixes: []string{"2.3.4.0/24", "2002:db8::/96"}},
			want: &adminv2.NetworkServiceCreateResponse{
				Network: &apiv2.Network{
					Id:       "internet-dualstack",
					Meta:     &apiv2.Meta{},
					Prefixes: []string{"2.3.4.0/24", "2002:db8::/96"},
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
