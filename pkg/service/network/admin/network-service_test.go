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

var (
	tenants  = []*apiv2.TenantServiceCreateRequest{{Name: "t1"}, {Name: "t0"}}
	projects = []*apiv2.ProjectServiceCreateRequest{{Name: "p1", Login: "t1"}, {Name: "p2", Login: "t1"}, {Name: "p3", Login: "t1"}, {Name: "p0", Login: "t0"}}
)

func Test_networkServiceServer_CreateChildNetwork(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	testStore, closer := test.StartRepositoryWithCleanup(t, log)
	defer closer()
	repo := testStore.Store

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintln(w, "a image")
	}))
	defer ts.Close()

	validURL := ts.URL

	ctx := t.Context()

	test.CreateTenants(t, repo, tenants)
	test.CreateProjects(t, repo, projects)

	test.CreatePartitions(t, repo, []*adminv2.PartitionServiceCreateRequest{
		{Partition: &apiv2.Partition{Id: "partition-one", BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}}},
	})

	tests := []struct {
		name      string
		preparefn func(t *testing.T)
		rq        *adminv2.NetworkServiceCreateRequest
		want      *adminv2.NetworkServiceCreateResponse
		wantErr   error
	}{
		{
			name:      "create a private network, no super network found",
			preparefn: nil,
			rq: &adminv2.NetworkServiceCreateRequest{
				Type:    apiv2.NetworkType_NETWORK_TYPE_CHILD,
				Name:    pointer.Pointer("private-1"),
				Project: pointer.Pointer("p1"),
			},
			want:    nil,
			wantErr: errorutil.InvalidArgument("no parent network found"),
		},
		{
			name: "create a private network, super network created before",
			preparefn: func(t *testing.T) {
				test.CreateNetworks(t, repo, []*adminv2.NetworkServiceCreateRequest{
					{
						Id:                       pointer.Pointer("tenant-super-network"),
						Prefixes:                 []string{"10.100.0.0/14"},
						DefaultChildPrefixLength: &apiv2.ChildPrefixLength{Ipv4: pointer.Pointer(uint32(22))},
						Type:                     apiv2.NetworkType_NETWORK_TYPE_SUPER,
						Partition:                pointer.Pointer("partition-one"),
					},
				})
			},
			rq: &adminv2.NetworkServiceCreateRequest{
				Type:            apiv2.NetworkType_NETWORK_TYPE_CHILD,
				Name:            pointer.Pointer("private-1"),
				Project:         pointer.Pointer("p1"),
				ParentNetworkId: pointer.Pointer("tenant-super-network"),
			},
			want: &adminv2.NetworkServiceCreateResponse{
				Network: &apiv2.Network{
					Meta:            &apiv2.Meta{},
					Type:            apiv2.NetworkType_NETWORK_TYPE_CHILD.Enum(),
					Name:            pointer.Pointer("private-1"),
					Project:         pointer.Pointer("p1"),
					ParentNetworkId: pointer.Pointer("tenant-super-network"),
					Prefixes:        []string{"10.100.0.0/22"},
				},
			},
			wantErr: nil,
		},
		{
			name: "create a private network, super network created before, parent network id not given",
			preparefn: func(t *testing.T) {
				test.CreateNetworks(t, repo, []*adminv2.NetworkServiceCreateRequest{
					{
						Id:                       pointer.Pointer("tenant-super-network"),
						Prefixes:                 []string{"10.100.0.0/14"},
						DefaultChildPrefixLength: &apiv2.ChildPrefixLength{Ipv4: pointer.Pointer(uint32(22))},
						Type:                     apiv2.NetworkType_NETWORK_TYPE_SUPER,
						Partition:                pointer.Pointer("partition-one"),
					},
				})
			},
			rq: &adminv2.NetworkServiceCreateRequest{
				Type:      apiv2.NetworkType_NETWORK_TYPE_CHILD,
				Name:      pointer.Pointer("private-1"),
				Project:   pointer.Pointer("p1"),
				Partition: pointer.Pointer("partition-one"),
			},
			want: &adminv2.NetworkServiceCreateResponse{
				Network: &apiv2.Network{
					Meta:            &apiv2.Meta{},
					Type:            apiv2.NetworkType_NETWORK_TYPE_CHILD.Enum(),
					Name:            pointer.Pointer("private-1"),
					Project:         pointer.Pointer("p1"),
					Partition:       pointer.Pointer("partition-one"),
					ParentNetworkId: pointer.Pointer("tenant-super-network"),
					Prefixes:        []string{"10.100.0.0/22"},
				},
			},
			wantErr: nil,
		},
		{
			name: "create a private network, super namespaced network created before parent network id given",
			preparefn: func(t *testing.T) {
				test.CreateNetworks(t, repo, []*adminv2.NetworkServiceCreateRequest{
					{
						Id:                       pointer.Pointer("tenant-super-network-namespaced"),
						Prefixes:                 []string{"10.100.0.0/14"},
						DefaultChildPrefixLength: &apiv2.ChildPrefixLength{Ipv4: pointer.Pointer(uint32(22))},
						Type:                     apiv2.NetworkType_NETWORK_TYPE_SUPER_NAMESPACED,
					},
				})
			},
			rq: &adminv2.NetworkServiceCreateRequest{
				Type:            apiv2.NetworkType_NETWORK_TYPE_CHILD,
				Name:            pointer.Pointer("private-1"),
				Project:         pointer.Pointer("p1"),
				ParentNetworkId: pointer.Pointer("tenant-super-network-namespaced"),
			},
			want: &adminv2.NetworkServiceCreateResponse{
				Network: &apiv2.Network{
					Meta:            &apiv2.Meta{},
					Type:            apiv2.NetworkType_NETWORK_TYPE_CHILD.Enum(),
					Name:            pointer.Pointer("private-1"),
					Project:         pointer.Pointer("p1"),
					Namespace:       pointer.Pointer("p1"),
					ParentNetworkId: pointer.Pointer("tenant-super-network-namespaced"),
					Prefixes:        []string{"10.100.0.0/22"},
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

			defer func() {
				test.DeleteNetworks(t, testStore)
			}()

			if tt.preparefn != nil {
				tt.preparefn(t)
			}

			got, err := n.Create(ctx, connect.NewRequest(tt.rq))
			if diff := cmp.Diff(err, tt.wantErr, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("diff = %s", diff)
			}

			if diff := cmp.Diff(
				tt.want, pointer.SafeDeref(got).Msg,
				protocmp.Transform(),
				protocmp.IgnoreFields(
					&apiv2.Network{}, "consumption", "id", "vrf",
				),
				protocmp.IgnoreFields(
					&apiv2.Meta{}, "created_at", "updated_at",
				),
			); diff != "" {
				t.Errorf("networkServiceServer.Create() = %v, want %vņdiff: %s", pointer.SafeDeref(got).Msg, tt.want, diff)
			}
		})
	}
}

func Test_networkServiceServer_CreateSuper(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	testStore, closer := test.StartRepositoryWithCleanup(t, log)
	defer closer()
	repo := testStore.Store

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintln(w, "a image")
	}))
	defer ts.Close()

	validURL := ts.URL

	ctx := t.Context()

	test.CreateTenants(t, repo, tenants)
	test.CreateProjects(t, repo, projects)

	test.CreatePartitions(t, repo, []*adminv2.PartitionServiceCreateRequest{
		{Partition: &apiv2.Partition{Id: "partition-one", BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}}},
		{Partition: &apiv2.Partition{Id: "partition-two", BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}}},
	})

	tests := []struct {
		name      string
		preparefn func(t *testing.T)
		rq        *adminv2.NetworkServiceCreateRequest
		want      *adminv2.NetworkServiceCreateResponse
		wantErr   error
	}{
		{
			name:      "create a super network without defaultchildprefixlength",
			preparefn: nil,
			rq: &adminv2.NetworkServiceCreateRequest{
				Id:        pointer.Pointer("tenant-super-network"),
				Prefixes:  []string{"10.100.0.0/14"},
				Type:      apiv2.NetworkType_NETWORK_TYPE_SUPER,
				Partition: pointer.Pointer("partition-one"),
			},
			want:    nil,
			wantErr: errorutil.InvalidArgument("defaultchildprefixlength must not be nil"),
		},
		{
			name: "create a super network in partition where already a super exists",
			preparefn: func(t *testing.T) {
				test.CreateNetworks(t, repo, []*adminv2.NetworkServiceCreateRequest{
					{
						Id:                       pointer.Pointer("tenant-super-network"),
						Prefixes:                 []string{"10.100.0.0/14"},
						DefaultChildPrefixLength: &apiv2.ChildPrefixLength{Ipv4: pointer.Pointer(uint32(22))},
						Type:                     apiv2.NetworkType_NETWORK_TYPE_SUPER,
						Partition:                pointer.Pointer("partition-one"),
					},
				})
			},
			rq: &adminv2.NetworkServiceCreateRequest{
				Id:                       pointer.Pointer("tenant-super-network-2"),
				Prefixes:                 []string{"11.100.0.0/14"},
				DefaultChildPrefixLength: &apiv2.ChildPrefixLength{Ipv4: pointer.Pointer(uint32(22))},
				Type:                     apiv2.NetworkType_NETWORK_TYPE_SUPER,
				Partition:                pointer.Pointer("partition-one"),
			},
			want:    nil,
			wantErr: errorutil.InvalidArgument(`partition with id "partition-one" already has a network of type NETWORK_TYPE_SUPER`),
		},
		{
			name: "create a super network in second partition where already a super exists in first partition",
			preparefn: func(t *testing.T) {
				test.CreateNetworks(t, repo, []*adminv2.NetworkServiceCreateRequest{
					{
						Id:                       pointer.Pointer("tenant-super-network"),
						Prefixes:                 []string{"10.100.0.0/14"},
						DefaultChildPrefixLength: &apiv2.ChildPrefixLength{Ipv4: pointer.Pointer(uint32(22))},
						Type:                     apiv2.NetworkType_NETWORK_TYPE_SUPER,
						Partition:                pointer.Pointer("partition-one"),
					},
				})
			},
			rq: &adminv2.NetworkServiceCreateRequest{
				Id:                       pointer.Pointer("tenant-super-network-2"),
				Prefixes:                 []string{"11.100.0.0/14"},
				DefaultChildPrefixLength: &apiv2.ChildPrefixLength{Ipv4: pointer.Pointer(uint32(22))},
				Type:                     apiv2.NetworkType_NETWORK_TYPE_SUPER,
				Partition:                pointer.Pointer("partition-two"),
			},
			want: &adminv2.NetworkServiceCreateResponse{
				Network: &apiv2.Network{
					Id:                       "tenant-super-network-2",
					Meta:                     &apiv2.Meta{},
					Prefixes:                 []string{"11.100.0.0/14"},
					DefaultChildPrefixLength: &apiv2.ChildPrefixLength{Ipv4: pointer.Pointer(uint32(22))},
					Type:                     apiv2.NetworkType_NETWORK_TYPE_SUPER.Enum(),
					Partition:                pointer.Pointer("partition-two"),
				},
			},
			wantErr: nil,
		},
		{
			name:      "create a super network without defaultchildprefixlength",
			preparefn: nil,
			rq: &adminv2.NetworkServiceCreateRequest{
				Id:                       pointer.Pointer("tenant-super-network"),
				Prefixes:                 []string{"10.100.0.0/14"},
				DefaultChildPrefixLength: &apiv2.ChildPrefixLength{Ipv4: pointer.Pointer(uint32(22))},
				Type:                     apiv2.NetworkType_NETWORK_TYPE_SUPER_NAMESPACED,
				Partition:                pointer.Pointer("partition-one"),
			},
			want:    nil,
			wantErr: errorutil.InvalidArgument("partition must not be specified for namespaced private super"),
		},
		{
			name:      "create a super network with defaultchildprefixlength but wrong af",
			preparefn: nil,
			rq: &adminv2.NetworkServiceCreateRequest{
				Id:                       pointer.Pointer("tenant-super-network"),
				Prefixes:                 []string{"10.100.0.0/14"},
				DefaultChildPrefixLength: &apiv2.ChildPrefixLength{Ipv6: pointer.Pointer(uint32(112))},
				Type:                     apiv2.NetworkType_NETWORK_TYPE_SUPER,
				Partition:                pointer.Pointer("partition-one"),
			},
			want:    nil,
			wantErr: errorutil.InvalidArgument(`childprefixlength for addressfamily: "IPv6" specified, but no "IPv6" addressfamily found in prefixes`),
		},
		{
			name:      "create a super network with childprefixlength but wrong length",
			preparefn: nil,
			rq: &adminv2.NetworkServiceCreateRequest{
				Id:                       pointer.Pointer("tenant-super-network"),
				Prefixes:                 []string{"10.100.0.0/24"},
				DefaultChildPrefixLength: &apiv2.ChildPrefixLength{Ipv4: pointer.Pointer(uint32(22))},
				Type:                     apiv2.NetworkType_NETWORK_TYPE_SUPER,
				Partition:                pointer.Pointer("partition-one"),
			},
			want:    nil,
			wantErr: errorutil.InvalidArgument(`given childprefixlength 22 is not greater than prefix length of:10.100.0.0/24`),
		},
		{
			name:      "create a super network with minchildprefixlength but wrong length",
			preparefn: nil,
			rq: &adminv2.NetworkServiceCreateRequest{
				Id:                       pointer.Pointer("tenant-super-network"),
				Prefixes:                 []string{"10.100.0.0/24"},
				DefaultChildPrefixLength: &apiv2.ChildPrefixLength{Ipv4: pointer.Pointer(uint32(26))},
				MinChildPrefixLength:     &apiv2.ChildPrefixLength{Ipv4: pointer.Pointer(uint32(20))},
				Type:                     apiv2.NetworkType_NETWORK_TYPE_SUPER,
				Partition:                pointer.Pointer("partition-one"),
			},
			want:    nil,
			wantErr: errorutil.InvalidArgument(`given childprefixlength 20 is not greater than prefix length of:10.100.0.0/24`),
		},
		{
			name:      "create a super network with minchildprefixlength but wrong length",
			preparefn: nil,
			rq: &adminv2.NetworkServiceCreateRequest{
				Id:                         pointer.Pointer("tenant-super-network"),
				Prefixes:                   []string{"10.100.0.0/24"},
				DefaultChildPrefixLength:   &apiv2.ChildPrefixLength{Ipv4: pointer.Pointer(uint32(26))},
				MinChildPrefixLength:       &apiv2.ChildPrefixLength{Ipv4: pointer.Pointer(uint32(25))},
				AdditionalAnnouncableCidrs: []string{"3.4.5.6.0/23"},
				Type:                       apiv2.NetworkType_NETWORK_TYPE_SUPER,
				Partition:                  pointer.Pointer("partition-one"),
			},
			want:    nil,
			wantErr: errorutil.InvalidArgument(`given cidr:"3.4.5.6.0/23" in additionalannouncablecidrs is malformed:netip.ParsePrefix("3.4.5.6.0/23"): ParseAddr("3.4.5.6.0"): IPv4 address too long`),
		},
		{
			name:      "create a super network",
			preparefn: nil,
			rq: &adminv2.NetworkServiceCreateRequest{
				Id:                         pointer.Pointer("tenant-super-network"),
				Prefixes:                   []string{"10.100.0.0/14", "2001:db8::/96"},
				DefaultChildPrefixLength:   &apiv2.ChildPrefixLength{Ipv4: pointer.Pointer(uint32(22)), Ipv6: pointer.Pointer(uint32(112))},
				MinChildPrefixLength:       &apiv2.ChildPrefixLength{Ipv4: pointer.Pointer(uint32(20)), Ipv6: pointer.Pointer(uint32(104))},
				AdditionalAnnouncableCidrs: []string{"10.240.0.0/12"},
				Type:                       apiv2.NetworkType_NETWORK_TYPE_SUPER,
				Partition:                  pointer.Pointer("partition-one"),
			},
			want: &adminv2.NetworkServiceCreateResponse{
				Network: &apiv2.Network{
					Id:                         "tenant-super-network",
					Meta:                       &apiv2.Meta{},
					Prefixes:                   []string{"10.100.0.0/14", "2001:db8::/96"},
					DefaultChildPrefixLength:   &apiv2.ChildPrefixLength{Ipv4: pointer.Pointer(uint32(22)), Ipv6: pointer.Pointer(uint32(112))},
					MinChildPrefixLength:       &apiv2.ChildPrefixLength{Ipv4: pointer.Pointer(uint32(20)), Ipv6: pointer.Pointer(uint32(104))},
					AdditionalAnnouncableCidrs: []string{"10.240.0.0/12"},
					Type:                       apiv2.NetworkType_NETWORK_TYPE_SUPER.Enum(),
					Partition:                  pointer.Pointer("partition-one"),
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

			defer func() {
				test.DeleteNetworks(t, testStore)
			}()

			if tt.preparefn != nil {
				tt.preparefn(t)
			}

			got, err := n.Create(ctx, connect.NewRequest(tt.rq))
			if diff := cmp.Diff(err, tt.wantErr, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("diff = %s", diff)
			}

			if diff := cmp.Diff(
				tt.want, pointer.SafeDeref(got).Msg,
				protocmp.Transform(),
				protocmp.IgnoreFields(
					&apiv2.Network{}, "consumption", "id", "vrf",
				),
				protocmp.IgnoreFields(
					&apiv2.Meta{}, "created_at", "updated_at",
				),
			); diff != "" {
				t.Errorf("networkServiceServer.Create() = %v, want %vņdiff: %s", pointer.SafeDeref(got).Msg, tt.want, diff)
			}
		})
	}
}

func Test_networkServiceServer_CreateExternal(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	testStore, closer := test.StartRepositoryWithCleanup(t, log)
	defer closer()
	repo := testStore.Store

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintln(w, "a image")
	}))
	defer ts.Close()

	validURL := ts.URL

	ctx := t.Context()

	test.CreateTenants(t, repo, tenants)
	test.CreateProjects(t, repo, projects)

	test.CreatePartitions(t, repo, []*adminv2.PartitionServiceCreateRequest{
		{Partition: &apiv2.Partition{Id: "partition-one", BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}}},
		{Partition: &apiv2.Partition{Id: "partition-two", BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}}},
	})

	tests := []struct {
		name      string
		preparefn func(t *testing.T)
		rq        *adminv2.NetworkServiceCreateRequest
		want      *adminv2.NetworkServiceCreateResponse
		wantErr   error
	}{
		{
			name: "internet already exists",
			preparefn: func(t *testing.T) {
				test.CreateNetworks(t, repo, []*adminv2.NetworkServiceCreateRequest{
					{
						Id:       pointer.Pointer("internet"),
						Prefixes: []string{"10.0.0.0/24"},
						Type:     apiv2.NetworkType_NETWORK_TYPE_EXTERNAL,
						Vrf:      pointer.Pointer(uint32(10)),
					},
				})
			},
			rq: &adminv2.NetworkServiceCreateRequest{
				Id:       pointer.Pointer("internet"),
				Prefixes: []string{"2.3.4.0/24"},
				Type:     apiv2.NetworkType_NETWORK_TYPE_EXTERNAL,
				Vrf:      pointer.Pointer(uint32(11)),
			},
			want:    nil,
			wantErr: errorutil.Conflict("network with id:internet already exists"),
		},
		{
			name: "internet-2 overlaps internet",
			preparefn: func(t *testing.T) {
				test.CreateNetworks(t, repo, []*adminv2.NetworkServiceCreateRequest{
					{
						Id:       pointer.Pointer("some-network"),
						Prefixes: []string{"1.2.3.0/24"},
						Type:     apiv2.NetworkType_NETWORK_TYPE_EXTERNAL,
						Vrf:      pointer.Pointer(uint32(20)),
					},
				})
			},
			rq: &adminv2.NetworkServiceCreateRequest{
				Id:       pointer.Pointer("internet-2"),
				Prefixes: []string{"1.2.3.0/24"},
				Type:     apiv2.NetworkType_NETWORK_TYPE_EXTERNAL,
				Vrf:      pointer.Pointer(uint32(21)),
			}, want: nil,
			wantErr: errorutil.Conflict("1.2.3.0/24 overlaps 1.2.3.0/24"),
		},
		{
			name: "internet-3 with malformed prefixes",
			rq: &adminv2.NetworkServiceCreateRequest{
				Id:       pointer.Pointer("internet-3"),
				Prefixes: []string{"1.2.3.4.0/24"},
				Type:     apiv2.NetworkType_NETWORK_TYPE_EXTERNAL,
				Vrf:      pointer.Pointer(uint32(94)),
			},
			want:    nil,
			wantErr: errorutil.InvalidArgument(`given cidr "1.2.3.4.0/24" is not a valid ip with mask: netip.ParsePrefix("1.2.3.4.0/24"): ParseAddr("1.2.3.4.0"): IPv4 address too long`),
		},
		{
			name: "internet-3 project given",
			rq: &adminv2.NetworkServiceCreateRequest{
				Id:       pointer.Pointer("internet-3"),
				Prefixes: []string{"1.2.3.0/24"},
				Type:     apiv2.NetworkType_NETWORK_TYPE_EXTERNAL,
				Vrf:      pointer.Pointer(uint32(94)),
				Project:  pointer.Pointer("p1"),
			},
			want:    nil,
			wantErr: errorutil.InvalidArgument(`project must be nil`),
		},
		{
			name: "internet-3 with malformed destinationprefixes",
			rq: &adminv2.NetworkServiceCreateRequest{
				Id:                  pointer.Pointer("internet-3"),
				Prefixes:            []string{"1.2.3.4.0/24"},
				DestinationPrefixes: []string{"1.2.3.4.0/24"},
				Type:                apiv2.NetworkType_NETWORK_TYPE_EXTERNAL,
				Vrf:                 pointer.Pointer(uint32(94)),
			},
			want:    nil,
			wantErr: errorutil.InvalidArgument(`given cidr "1.2.3.4.0/24" is not a valid ip with mask: netip.ParsePrefix("1.2.3.4.0/24"): ParseAddr("1.2.3.4.0"): IPv4 address too long`),
		},
		{
			name: "internet-3 with mixed af for prefixes and destinationprefixes",
			rq: &adminv2.NetworkServiceCreateRequest{
				Id:                  pointer.Pointer("internet-3"),
				Prefixes:            []string{"1.2.3.0/24"},
				DestinationPrefixes: []string{"2002:db8::/96"},
				Type:                apiv2.NetworkType_NETWORK_TYPE_EXTERNAL,
				Vrf:                 pointer.Pointer(uint32(94)),
			},
			want:    nil,
			wantErr: errorutil.InvalidArgument(`addressfamily:IPv6 of destination prefixes is not present in existing prefixes`),
		},
		{
			name: "internet",
			rq: &adminv2.NetworkServiceCreateRequest{
				Id:       pointer.Pointer("internet"),
				Prefixes: []string{"1.2.3.0/24"},
				Type:     apiv2.NetworkType_NETWORK_TYPE_EXTERNAL,
				Vrf:      pointer.Pointer(uint32(91)),
			},
			want: &adminv2.NetworkServiceCreateResponse{
				Network: &apiv2.Network{
					Id:       "internet",
					Meta:     &apiv2.Meta{},
					Prefixes: []string{"1.2.3.0/24"},
					Vrf:      pointer.Pointer(uint32(91)),
					Type:     apiv2.NetworkType_NETWORK_TYPE_EXTERNAL.Enum(),
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

			defer func() {
				test.DeleteNetworks(t, testStore)
			}()

			if tt.preparefn != nil {
				tt.preparefn(t)
			}

			got, err := n.Create(ctx, connect.NewRequest(tt.rq))
			if diff := cmp.Diff(err, tt.wantErr, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("diff = %s", diff)
			}

			if diff := cmp.Diff(
				tt.want, pointer.SafeDeref(got).Msg,
				protocmp.Transform(),
				protocmp.IgnoreFields(
					&apiv2.Network{}, "consumption", "id", "vrf",
				),
				protocmp.IgnoreFields(
					&apiv2.Meta{}, "created_at", "updated_at",
				),
			); diff != "" {
				t.Errorf("networkServiceServer.Create() = %v, want %vņdiff: %s", pointer.SafeDeref(got).Msg, tt.want, diff)
			}
		})
	}
}

// FIXME adopt to new usecases, rename test func properly
func _Test_networkServiceServer_CreateSuperVrfShared(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	testStore, closer := test.StartRepositoryWithCleanup(t, log)
	defer closer()
	repo := testStore.Store

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintln(w, "a image")
	}))
	defer ts.Close()

	validURL := ts.URL

	ctx := t.Context()

	test.CreateTenants(t, repo, tenants)
	test.CreateProjects(t, repo, projects)

	test.CreatePartitions(t, repo, []*adminv2.PartitionServiceCreateRequest{
		{Partition: &apiv2.Partition{Id: "partition-one", BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}}},
		{Partition: &apiv2.Partition{Id: "partition-two", BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}}},
	})

	tests := []struct {
		name      string
		preparefn func(t *testing.T)
		rq        *adminv2.NetworkServiceCreateRequest
		want      *adminv2.NetworkServiceCreateResponse
		wantErr   error
	}{
		{
			name: "dc-interconnect already exists",
			preparefn: func(t *testing.T) {
				test.CreateNetworks(t, repo, []*adminv2.NetworkServiceCreateRequest{
					{
						Id:                       pointer.Pointer("dc-interconnect"),
						Prefixes:                 []string{"100.64.0.0/16"},
						Type:                     apiv2.NetworkType_NETWORK_TYPE_SUPER,
						DefaultChildPrefixLength: &apiv2.ChildPrefixLength{Ipv4: pointer.Pointer(uint32(28))},
						Vrf:                      pointer.Pointer(uint32(10)),
					},
				})
			},
			rq: &adminv2.NetworkServiceCreateRequest{
				Id:       pointer.Pointer("dc-interconnect"),
				Prefixes: []string{"2.3.4.0/24"},
				Type:     apiv2.NetworkType_NETWORK_TYPE_SUPER,
				Vrf:      pointer.Pointer(uint32(11)),
			},
			want:    nil,
			wantErr: errorutil.Conflict("network with id:dc-interconnect already exists"),
		},
		{
			name:      "dc-interconnect has no childprefixlength specified",
			preparefn: nil,
			rq: &adminv2.NetworkServiceCreateRequest{
				Id:       pointer.Pointer("dc-interconnect"),
				Prefixes: []string{"1.2.3.0/24"},
				Type:     apiv2.NetworkType_NETWORK_TYPE_SUPER,
				Vrf:      pointer.Pointer(uint32(21)),
			}, want: nil,
			wantErr: errorutil.InvalidArgument("defaultchildprefixlength must not be nil"),
		},
		{
			name: "dc-interconnect-2 with already existig dc-interconnect",
			preparefn: func(t *testing.T) {
				test.CreateNetworks(t, repo, []*adminv2.NetworkServiceCreateRequest{
					{
						Id:                       pointer.Pointer("dc-interconnect"),
						Prefixes:                 []string{"100.64.0.0/16"},
						Type:                     apiv2.NetworkType_NETWORK_TYPE_SUPER,
						DefaultChildPrefixLength: &apiv2.ChildPrefixLength{Ipv4: pointer.Pointer(uint32(28))},
						Vrf:                      pointer.Pointer(uint32(10)),
					},
				})
			},
			rq: &adminv2.NetworkServiceCreateRequest{
				Id:                       pointer.Pointer("dc-interconnect-2"),
				Prefixes:                 []string{"2.3.4.0/24"},
				Type:                     apiv2.NetworkType_NETWORK_TYPE_SUPER,
				DefaultChildPrefixLength: &apiv2.ChildPrefixLength{Ipv4: pointer.Pointer(uint32(28))},
				Vrf:                      pointer.Pointer(uint32(11)),
			},
			want: &adminv2.NetworkServiceCreateResponse{
				Network: &apiv2.Network{
					Id:                       "dc-interconnect",
					Meta:                     &apiv2.Meta{},
					Prefixes:                 []string{"2.3.4.0/24"},
					DefaultChildPrefixLength: &apiv2.ChildPrefixLength{Ipv4: pointer.Pointer(uint32(28))},
					Vrf:                      pointer.Pointer(uint32(91)),
					Type:                     apiv2.NetworkType_NETWORK_TYPE_SUPER.Enum(),
				}},
			wantErr: nil,
		},
		{
			name: "dc-interconnect",
			rq: &adminv2.NetworkServiceCreateRequest{
				Id:                       pointer.Pointer("dc-interconnect"),
				Prefixes:                 []string{"1.2.3.0/24"},
				Type:                     apiv2.NetworkType_NETWORK_TYPE_SUPER,
				DefaultChildPrefixLength: &apiv2.ChildPrefixLength{Ipv4: pointer.Pointer(uint32(28))},
				Vrf:                      pointer.Pointer(uint32(91)),
			},
			want: &adminv2.NetworkServiceCreateResponse{
				Network: &apiv2.Network{
					Id:                       "dc-interconnect",
					Meta:                     &apiv2.Meta{},
					Prefixes:                 []string{"1.2.3.0/24"},
					DefaultChildPrefixLength: &apiv2.ChildPrefixLength{Ipv4: pointer.Pointer(uint32(28))},
					Vrf:                      pointer.Pointer(uint32(91)),
					Type:                     apiv2.NetworkType_NETWORK_TYPE_SUPER.Enum(),
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

			defer func() {
				test.DeleteNetworks(t, testStore)
			}()

			if tt.preparefn != nil {
				tt.preparefn(t)
			}

			got, err := n.Create(ctx, connect.NewRequest(tt.rq))
			if diff := cmp.Diff(err, tt.wantErr, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("diff = %s", diff)
			}

			if diff := cmp.Diff(
				tt.want, pointer.SafeDeref(got).Msg,
				protocmp.Transform(),
				protocmp.IgnoreFields(
					&apiv2.Network{}, "consumption", "id", "vrf",
				),
				protocmp.IgnoreFields(
					&apiv2.Meta{}, "created_at", "updated_at",
				),
			); diff != "" {
				t.Errorf("networkServiceServer.Create() = %v, want %vņdiff: %s", pointer.SafeDeref(got).Msg, tt.want, diff)
			}
		})
	}
}

func Test_networkServiceServer_CreateUnderlay(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	testStore, closer := test.StartRepositoryWithCleanup(t, log)
	defer closer()
	repo := testStore.Store

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintln(w, "a image")
	}))
	defer ts.Close()

	validURL := ts.URL

	ctx := t.Context()

	test.CreateTenants(t, repo, tenants)
	test.CreateProjects(t, repo, projects)

	test.CreatePartitions(t, repo, []*adminv2.PartitionServiceCreateRequest{
		{Partition: &apiv2.Partition{Id: "partition-one", BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}}},
		{Partition: &apiv2.Partition{Id: "partition-two", BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}}},
	})

	tests := []struct {
		name      string
		preparefn func(t *testing.T)
		rq        *adminv2.NetworkServiceCreateRequest
		want      *adminv2.NetworkServiceCreateResponse
		wantErr   error
	}{
		{
			name: "underlay already exists",
			preparefn: func(t *testing.T) {
				test.CreateNetworks(t, repo, []*adminv2.NetworkServiceCreateRequest{
					{
						Id:        pointer.Pointer("underlay"),
						Prefixes:  []string{"10.0.0.0/24"},
						Type:      apiv2.NetworkType_NETWORK_TYPE_UNDERLAY,
						Partition: pointer.Pointer("partition-one"),
					},
				})
			},
			rq: &adminv2.NetworkServiceCreateRequest{
				Id:        pointer.Pointer("underlay"),
				Prefixes:  []string{"11.0.0.0/24"},
				Type:      apiv2.NetworkType_NETWORK_TYPE_UNDERLAY,
				Partition: pointer.Pointer("partition-one"),
			},
			want:    nil,
			wantErr: errorutil.Conflict("network with id:underlay already exists"),
		},
		{
			name: "underlay project given",
			rq: &adminv2.NetworkServiceCreateRequest{
				Id:        pointer.Pointer("underlay"),
				Prefixes:  []string{"1.2.3.0/24"},
				Type:      apiv2.NetworkType_NETWORK_TYPE_UNDERLAY,
				Partition: pointer.Pointer("partition-one"),
				Project:   pointer.Pointer("p1"),
			},
			want:    nil,
			wantErr: errorutil.InvalidArgument(`project must be nil`),
		},
		{
			name: "ipv6 prefixes are not allowed",
			rq: &adminv2.NetworkServiceCreateRequest{
				Id:        pointer.Pointer("underlay"),
				Prefixes:  []string{"1.2.3.0/24", "2001:db8::/96"},
				Type:      apiv2.NetworkType_NETWORK_TYPE_UNDERLAY,
				Partition: pointer.Pointer("partition-one"),
			},
			want:    nil,
			wantErr: errorutil.InvalidArgument(`underlay can only contain ipv4 prefixes`),
		},
		{
			name: "underlay",
			rq: &adminv2.NetworkServiceCreateRequest{
				Id:        pointer.Pointer("underlay"),
				Prefixes:  []string{"1.2.3.0/24"},
				Type:      apiv2.NetworkType_NETWORK_TYPE_UNDERLAY,
				Partition: pointer.Pointer("partition-one"),
			},
			want: &adminv2.NetworkServiceCreateResponse{
				Network: &apiv2.Network{
					Id:        "underlay",
					Meta:      &apiv2.Meta{},
					Prefixes:  []string{"1.2.3.0/24"},
					Type:      apiv2.NetworkType_NETWORK_TYPE_UNDERLAY.Enum(),
					Partition: pointer.Pointer("partition-one"),
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

			defer func() {
				test.DeleteNetworks(t, testStore)
			}()

			if tt.preparefn != nil {
				tt.preparefn(t)
			}

			got, err := n.Create(ctx, connect.NewRequest(tt.rq))
			if diff := cmp.Diff(err, tt.wantErr, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("diff = %s", diff)
			}

			if diff := cmp.Diff(
				tt.want, pointer.SafeDeref(got).Msg,
				protocmp.Transform(),
				protocmp.IgnoreFields(
					&apiv2.Network{}, "consumption", "id", "vrf",
				),
				protocmp.IgnoreFields(
					&apiv2.Meta{}, "created_at", "updated_at",
				),
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

	test.CreateTenants(t, repo, tenants)
	test.CreateProjects(t, repo, projects)

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
			Type:                     apiv2.NetworkType_NETWORK_TYPE_SUPER,
			Partition:                pointer.Pointer("partition-one"),
		},
		{
			Id:                       pointer.Pointer("tenant-super-network-v6"),
			Prefixes:                 []string{"2001:db8::/96"},
			DefaultChildPrefixLength: &apiv2.ChildPrefixLength{Ipv6: pointer.Pointer(uint32(112))},
			Type:                     apiv2.NetworkType_NETWORK_TYPE_SUPER,
			Partition:                pointer.Pointer("partition-two"),
		},
		{
			Id:                       pointer.Pointer("tenant-super-network-dualstack"),
			Prefixes:                 []string{"2001:dc8::/96", "10.200.0.0/14"},
			DefaultChildPrefixLength: &apiv2.ChildPrefixLength{Ipv4: pointer.Pointer(uint32(22)), Ipv6: pointer.Pointer(uint32(112))},
			Type:                     apiv2.NetworkType_NETWORK_TYPE_SUPER,
			Partition:                pointer.Pointer("partition-three"),
		},
		{
			Id:        pointer.Pointer("underlay"),
			Name:      pointer.Pointer("Underlay Network"),
			Prefixes:  []string{"10.0.0.0/24"},
			Partition: pointer.Pointer("partition-one"),
			Type:      apiv2.NetworkType_NETWORK_TYPE_UNDERLAY,
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
					Type:            apiv2.NetworkType_NETWORK_TYPE_CHILD.Enum(),
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

				protocmp.Transform(),
				protocmp.IgnoreFields(
					&apiv2.Network{}, "consumption", "id", "vrf",
				),
				protocmp.IgnoreFields(
					&apiv2.Meta{}, "created_at", "updated_at",
				),
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
			Type:                     apiv2.NetworkType_NETWORK_TYPE_SUPER,
			Partition:                pointer.Pointer("partition-one"),
		},
		{
			Id:                       pointer.Pointer("tenant-super-network-v6"),
			Prefixes:                 []string{"2001:db8::/96"},
			DefaultChildPrefixLength: &apiv2.ChildPrefixLength{Ipv6: pointer.Pointer(uint32(112))},
			Type:                     apiv2.NetworkType_NETWORK_TYPE_SUPER,
			Partition:                pointer.Pointer("partition-two"),
		},
		{
			Id:                       pointer.Pointer("tenant-super-network-dualstack"),
			Prefixes:                 []string{"2001:dc8::/96", "10.200.0.0/14"},
			DefaultChildPrefixLength: &apiv2.ChildPrefixLength{Ipv4: pointer.Pointer(uint32(22)), Ipv6: pointer.Pointer(uint32(112))},
			Type:                     apiv2.NetworkType_NETWORK_TYPE_SUPER,
			Partition:                pointer.Pointer("partition-three"),
		},
		{
			Id:        pointer.Pointer("underlay"),
			Name:      pointer.Pointer("Underlay Network"),
			Prefixes:  []string{"10.0.0.0/24"},
			Partition: pointer.Pointer("partition-one"),
			Type:      apiv2.NetworkType_NETWORK_TYPE_UNDERLAY,
		},
		{
			Id:                  pointer.Pointer("internet"),
			Prefixes:            []string{"20.0.0.0/24"},
			DestinationPrefixes: []string{"0.0.0.0/0"},
			Vrf:                 pointer.Pointer(uint32(1)),
			Type:                apiv2.NetworkType_NETWORK_TYPE_EXTERNAL,
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
						Vrf:             pointer.Pointer(uint32(30)),
						ParentNetworkId: pointer.Pointer("tenant-super-network"),
						Type:            apiv2.NetworkType_NETWORK_TYPE_CHILD.Enum(),
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
						Id:        "underlay",
						Meta:      &apiv2.Meta{},
						Name:      pointer.Pointer("Underlay Network"),
						Partition: pointer.Pointer("partition-one"),
						Prefixes:  []string{"10.0.0.0/24"},
						Type:      apiv2.NetworkType_NETWORK_TYPE_UNDERLAY.Enum(),
						Vrf:       pointer.Pointer(uint32(20)),
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "super tenant in partition-one",
			rq: &adminv2.NetworkServiceListRequest{
				Query: &apiv2.NetworkQuery{Partition: pointer.Pointer("partition-one"), Type: apiv2.NetworkType_NETWORK_TYPE_SUPER.Enum()},
			},
			want: &adminv2.NetworkServiceListResponse{
				Networks: []*apiv2.Network{
					{
						Id:                       "tenant-super-network",
						Meta:                     &apiv2.Meta{},
						Partition:                pointer.Pointer("partition-one"),
						Prefixes:                 []string{"10.100.0.0/14"},
						Type:                     apiv2.NetworkType_NETWORK_TYPE_SUPER.Enum(),
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
						Type:                     apiv2.NetworkType_NETWORK_TYPE_SUPER.Enum(),
						DefaultChildPrefixLength: &apiv2.ChildPrefixLength{Ipv4: pointer.Pointer(uint32(22)), Ipv6: pointer.Pointer(uint32(112))},
					},
					{
						Id:                       "tenant-super-network-v6",
						Meta:                     &apiv2.Meta{},
						Partition:                pointer.Pointer("partition-two"),
						Prefixes:                 []string{"2001:db8::/96"},
						Type:                     apiv2.NetworkType_NETWORK_TYPE_SUPER.Enum(),
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
						Type:                     apiv2.NetworkType_NETWORK_TYPE_SUPER.Enum(),
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
						Type:                apiv2.NetworkType_NETWORK_TYPE_EXTERNAL.Enum(),
						Vrf:                 pointer.Pointer(uint32(1)),
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
						Vrf:             pointer.Pointer(uint32(35)),
						ParentNetworkId: pointer.Pointer("tenant-super-network"),
						Type:            apiv2.NetworkType_NETWORK_TYPE_CHILD.Enum(),
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
				protocmp.Transform(),
				protocmp.IgnoreFields(
					&apiv2.Network{}, "consumption",
				),
				protocmp.IgnoreFields(
					&apiv2.Meta{}, "created_at", "updated_at",
				),
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
			Type:                     apiv2.NetworkType_NETWORK_TYPE_SUPER,
			Partition:                pointer.Pointer("partition-one"),
		},
		{
			Id:                       pointer.Pointer("tenant-super-network-v6"),
			Prefixes:                 []string{"2001:db8::/96"},
			DefaultChildPrefixLength: &apiv2.ChildPrefixLength{Ipv6: pointer.Pointer(uint32(112))},
			Type:                     apiv2.NetworkType_NETWORK_TYPE_SUPER,
			Partition:                pointer.Pointer("partition-two"),
		},
		{
			Id:                       pointer.Pointer("tenant-super-network-dualstack"),
			Prefixes:                 []string{"2001:dc8::/96", "10.200.0.0/14"},
			DefaultChildPrefixLength: &apiv2.ChildPrefixLength{Ipv4: pointer.Pointer(uint32(22)), Ipv6: pointer.Pointer(uint32(112))},
			Type:                     apiv2.NetworkType_NETWORK_TYPE_SUPER,
			Partition:                pointer.Pointer("partition-three"),
		},
		{
			Id:        pointer.Pointer("underlay"),
			Name:      pointer.Pointer("Underlay Network"),
			Prefixes:  []string{"10.0.0.0/24"},
			Partition: pointer.Pointer("partition-one"),
			Type:      apiv2.NetworkType_NETWORK_TYPE_UNDERLAY,
		},
		{
			Id:                  pointer.Pointer("internet"),
			Prefixes:            []string{"20.0.0.0/24"},
			DestinationPrefixes: []string{"0.0.0.0/0"},
			Type:                apiv2.NetworkType_NETWORK_TYPE_EXTERNAL,
			Vrf:                 pointer.Pointer(uint32(1)),
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
				Labels: &apiv2.UpdateLabels{Update: &apiv2.Labels{Labels: map[string]string{"color": "red", "size": "large"}}},
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
					Vrf:             pointer.Pointer(uint32(30)),
					ParentNetworkId: pointer.Pointer("tenant-super-network"),
					Type:            apiv2.NetworkType_NETWORK_TYPE_CHILD.Enum(),
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
					Type:                     apiv2.NetworkType_NETWORK_TYPE_SUPER.Enum(),
					DefaultChildPrefixLength: &apiv2.ChildPrefixLength{Ipv4: pointer.Pointer(uint32(22))},
				},
			},
			wantErr: nil,
		},
		{
			name: "change nattype of tenant super network",
			rq: &adminv2.NetworkServiceUpdateRequest{
				Id:      "tenant-super-network",
				NatType: apiv2.NATType_NAT_TYPE_IPV4_MASQUERADE.Enum(),
			},
			want: &adminv2.NetworkServiceUpdateResponse{
				Network: &apiv2.Network{
					Id:                       "tenant-super-network",
					Meta:                     &apiv2.Meta{},
					Partition:                pointer.Pointer("partition-one"),
					Prefixes:                 []string{"10.100.0.0/14", "10.101.0.0/14"},
					Type:                     apiv2.NetworkType_NETWORK_TYPE_SUPER.Enum(),
					DefaultChildPrefixLength: &apiv2.ChildPrefixLength{Ipv4: pointer.Pointer(uint32(22))},
					NatType:                  apiv2.NATType_NAT_TYPE_IPV4_MASQUERADE.Enum(),
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
				protocmp.Transform(),
				protocmp.IgnoreFields(
					&apiv2.Network{}, "consumption",
				),
				protocmp.IgnoreFields(
					&apiv2.Meta{}, "created_at", "updated_at",
				),
			); diff != "" {
				t.Errorf("networkServiceServer.Update() = %v, want %vņdiff: %s", pointer.SafeDeref(got).Msg, tt.want, diff)
			}
		})
	}
}
