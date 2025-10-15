package network

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	"github.com/metal-stack/metal-apiserver/pkg/test"
	"github.com/metal-stack/metal-lib/pkg/pointer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var (
	p0 = "00000000-0000-0000-0000-000000000000"
	p1 = "00000000-0000-0000-0000-000000000001"
	p2 = "00000000-0000-0000-0000-000000000002"
	p3 = "00000000-0000-0000-0000-000000000003"

	tenants  = []*apiv2.TenantServiceCreateRequest{{Name: "t1"}, {Name: "t0"}}
	projects = []*apiv2.ProjectServiceCreateRequest{{Name: p0, Login: "t0"}, {Name: p1, Login: "t1"}, {Name: p2, Login: "t1"}, {Name: p3, Login: "t1"}}
)

func Test_networkServiceServer_Get(t *testing.T) {
	t.Parallel()

	log := slog.Default()

	testStore, closer := test.StartRepositoryWithCleanup(t, log)
	defer closer()
	repo := testStore.Store

	ctx := t.Context()

	test.CreateTenants(t, testStore, tenants)
	test.CreateProjects(t, repo, projects)
	test.CreateNetworks(t, repo, []*adminv2.NetworkServiceCreateRequest{
		{Id: pointer.Pointer("internet"), Prefixes: []string{"1.2.3.0/24"}, Vrf: pointer.Pointer(uint32(9)), Type: apiv2.NetworkType_NETWORK_TYPE_EXTERNAL},
	})

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
					Type:     apiv2.NetworkType_NETWORK_TYPE_EXTERNAL.Enum(),
					Vrf:      pointer.Pointer(uint32(9)),
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
			if tt.wantErr == nil {
				// Execute proto based validation
				test.Validate(t, tt.rq)
			}
			got, err := n.Get(ctx, tt.rq)
			if diff := cmp.Diff(err, tt.wantErr, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("diff = %s", diff)
			}

			if diff := cmp.Diff(
				tt.want, got,
				protocmp.Transform(),
				protocmp.IgnoreFields(
					&apiv2.Network{}, "consumption",
				),
				protocmp.IgnoreFields(
					&apiv2.Meta{}, "created_at", "updated_at",
				),
			); diff != "" {
				t.Errorf("networkServiceServer.Get() = %v, want %vņdiff: %s", got, tt.want, diff)
			}
		})
	}
}

func Test_networkServiceServer_List(t *testing.T) {
	t.Parallel()

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	testStore, closer := test.StartRepositoryWithCleanup(t, log)
	defer closer()
	repo := testStore.Store

	ctx := t.Context()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintln(w, "a image")
	}))
	defer ts.Close()

	validURL := ts.URL

	test.CreateTenants(t, testStore, tenants)
	test.CreateProjects(t, repo, projects)
	test.CreatePartitions(t, repo, []*adminv2.PartitionServiceCreateRequest{
		{Partition: &apiv2.Partition{Id: "partition-one", BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}}},
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
			Id:        pointer.Pointer("underlay"),
			Name:      pointer.Pointer("Underlay Network"),
			Partition: pointer.Pointer("partition-one"),
			Prefixes:  []string{"10.0.0.0/24"},
			Type:      apiv2.NetworkType_NETWORK_TYPE_UNDERLAY,
		},
		{
			Id:                  pointer.Pointer("internet"),
			Prefixes:            []string{"1.2.3.0/24"},
			DestinationPrefixes: []string{"0.0.0.0/0"},
			Vrf:                 pointer.Pointer(uint32(11)),
			Type:                apiv2.NetworkType_NETWORK_TYPE_EXTERNAL,
		},
	})

	networkMap := test.AllocateNetworks(t, repo, []*apiv2.NetworkServiceCreateRequest{
		{Name: pointer.Pointer("p1-network-a"), Project: p1, Partition: pointer.Pointer("partition-one")},
		{Name: pointer.Pointer("p1-network-b"), Project: p1, Partition: pointer.Pointer("partition-one")},
		{Name: pointer.Pointer("p2-network-a"), Project: p2, Partition: pointer.Pointer("partition-one")},
		{Name: pointer.Pointer("p2-network-b"), Project: p2, Partition: pointer.Pointer("partition-one")},
		{Name: pointer.Pointer("p3-network-a"), Project: p3, Partition: pointer.Pointer("partition-one")},
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
					{Id: "internet", Meta: &apiv2.Meta{}, Prefixes: []string{"1.2.3.0/24"}, DestinationPrefixes: []string{"0.0.0.0/0"}, Vrf: pointer.Pointer(uint32(11)), Type: apiv2.NetworkType_NETWORK_TYPE_EXTERNAL.Enum()},
				},
			},
			wantErr: nil,
		},
		{
			name: "list by project",
			rq:   &apiv2.NetworkServiceListRequest{Project: p1, Query: &apiv2.NetworkQuery{Project: pointer.Pointer(p1)}},
			want: &apiv2.NetworkServiceListResponse{
				Networks: []*apiv2.Network{
					{
						Id:            networkMap["p1-network-a"].ID,
						Meta:          &apiv2.Meta{},
						Name:          pointer.Pointer("p1-network-a"),
						ParentNetwork: pointer.Pointer("tenant-super-network"),
						Project:       pointer.Pointer(p1),
						Partition:     pointer.Pointer("partition-one"),
						Vrf:           pointer.Pointer(uint32(20)),
						Type:          apiv2.NetworkType_NETWORK_TYPE_CHILD.Enum(),
					},
					{
						Id:            networkMap["p1-network-b"].ID,
						Meta:          &apiv2.Meta{},
						Name:          pointer.Pointer("p1-network-b"),
						ParentNetwork: pointer.Pointer("tenant-super-network"),
						Project:       pointer.Pointer(p1),
						Partition:     pointer.Pointer("partition-one"),
						Vrf:           pointer.Pointer(uint32(30)),
						Type:          apiv2.NetworkType_NETWORK_TYPE_CHILD.Enum(),
					},
				},
			},
			wantErr: nil,
		},
		{
			name:    "list by invalid (not owned) project",
			rq:      &apiv2.NetworkServiceListRequest{Project: p1, Query: &apiv2.NetworkQuery{Project: pointer.Pointer(p0)}},
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
			if tt.wantErr == nil {
				// Execute proto based validation
				test.Validate(t, tt.rq)
			}
			got, err := n.List(ctx, tt.rq)
			if diff := cmp.Diff(err, tt.wantErr, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("diff = %s", diff)
			}

			if diff := cmp.Diff(
				tt.want, got,
				protocmp.Transform(),
				protocmp.IgnoreFields(
					&apiv2.Network{}, "consumption", "prefixes",
				),
				protocmp.IgnoreFields(
					&apiv2.Meta{}, "created_at", "updated_at",
				),
			); diff != "" {
				t.Errorf("networkServiceServer.List() = %v, want %vņdiff: %s", got, tt.want, diff)
			}
		})
	}
}

func Test_networkServiceServer_ListBaseNetworks(t *testing.T) {
	t.Parallel()

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

	test.CreateTenants(t, testStore, tenants)
	test.CreateProjects(t, repo, projects)
	test.CreatePartitions(t, repo, []*adminv2.PartitionServiceCreateRequest{
		{Partition: &apiv2.Partition{Id: "partition-one", BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}}},
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
			Name:      pointer.Pointer("Shared Storage Network"),
			Project:   pointer.Pointer(p3),
			Partition: pointer.Pointer("partition-one"),
			Type:      apiv2.NetworkType_NETWORK_TYPE_CHILD_SHARED,
		},
		{
			Id:        pointer.Pointer("underlay"),
			Name:      pointer.Pointer("Underlay Network"),
			Partition: pointer.Pointer("partition-one"),
			Prefixes:  []string{"10.0.0.0/24"},
			Type:      apiv2.NetworkType_NETWORK_TYPE_UNDERLAY,
		},
		{
			Id:                  pointer.Pointer("0-internet"),
			Prefixes:            []string{"1.2.3.0/24"},
			DestinationPrefixes: []string{"0.0.0.0/0"},
			Vrf:                 pointer.Pointer(uint32(11)),
			Type:                apiv2.NetworkType_NETWORK_TYPE_EXTERNAL,
		},
	})

	sharedNetwork, err := repo.UnscopedNetwork().Find(ctx, &apiv2.NetworkQuery{Name: pointer.Pointer("Shared Storage Network")})
	require.NoError(t, err)

	tests := []struct {
		name    string
		rq      *apiv2.NetworkServiceListBaseNetworksRequest
		want    *apiv2.NetworkServiceListBaseNetworksResponse
		wantErr error
	}{
		{
			name: "list without project",
			rq:   &apiv2.NetworkServiceListBaseNetworksRequest{},
			want: &apiv2.NetworkServiceListBaseNetworksResponse{
				Networks: []*apiv2.Network{
					{
						Id:                  "0-internet",
						Meta:                &apiv2.Meta{},
						Prefixes:            []string{"1.2.3.0/24"},
						DestinationPrefixes: []string{"0.0.0.0/0"},
						Vrf:                 pointer.Pointer(uint32(11)),
						Type:                apiv2.NetworkType_NETWORK_TYPE_EXTERNAL.Enum(),
					},
					{
						Id:                       "tenant-super-network",
						Meta:                     &apiv2.Meta{},
						Prefixes:                 []string{"10.100.0.0/14"},
						DefaultChildPrefixLength: &apiv2.ChildPrefixLength{Ipv4: pointer.Pointer(uint32(22))},
						Type:                     apiv2.NetworkType_NETWORK_TYPE_SUPER.Enum(),
						Partition:                pointer.Pointer("partition-one"),
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "list by project",
			rq:   &apiv2.NetworkServiceListBaseNetworksRequest{Project: p3},
			want: &apiv2.NetworkServiceListBaseNetworksResponse{
				Networks: []*apiv2.Network{
					{
						Id:            sharedNetwork.ID,
						Meta:          &apiv2.Meta{},
						Name:          pointer.Pointer("Shared Storage Network"),
						Project:       pointer.Pointer(p3),
						Partition:     pointer.Pointer("partition-one"),
						ParentNetwork: pointer.Pointer("tenant-super-network"),
						Prefixes:      []string{"10.100.0.0/22"},
						Vrf:           pointer.Pointer(uint32(20)),
						Type:          apiv2.NetworkType_NETWORK_TYPE_CHILD_SHARED.Enum(),
					},
					{
						Id:                  "0-internet",
						Meta:                &apiv2.Meta{},
						Prefixes:            []string{"1.2.3.0/24"},
						DestinationPrefixes: []string{"0.0.0.0/0"},
						Vrf:                 pointer.Pointer(uint32(11)),
						Type:                apiv2.NetworkType_NETWORK_TYPE_EXTERNAL.Enum(),
					},
					{
						Id:                       "tenant-super-network",
						Meta:                     &apiv2.Meta{},
						Prefixes:                 []string{"10.100.0.0/14"},
						DefaultChildPrefixLength: &apiv2.ChildPrefixLength{Ipv4: pointer.Pointer(uint32(22))},
						Type:                     apiv2.NetworkType_NETWORK_TYPE_SUPER.Enum(),
						Partition:                pointer.Pointer("partition-one"),
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
			if tt.wantErr == nil {
				// Execute proto based validation
				test.Validate(t, tt.rq)
			}
			got, err := n.ListBaseNetworks(ctx, tt.rq)
			if diff := cmp.Diff(err, tt.wantErr, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("diff = %s", diff)
			}

			if diff := cmp.Diff(
				tt.want, got,
				protocmp.Transform(),
				protocmp.IgnoreFields(
					&apiv2.Network{}, "consumption", "id",
				),
				protocmp.IgnoreFields(
					&apiv2.Meta{}, "created_at", "updated_at",
				),
			); diff != "" {
				t.Errorf("networkServiceServer.ListBaseNetworks() = %v, want %vņdiff: %s", got, tt.want, diff)
			}
		})
	}
}

func Test_networkServiceServer_Update(t *testing.T) {
	t.Parallel()

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	testStore, closer := test.StartRepositoryWithCleanup(t, log)
	defer closer()
	repo := testStore.Store

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintln(w, "a image")
	}))
	defer ts.Close()

	validURL := ts.URL

	test.CreateTenants(t, testStore, tenants)
	test.CreateProjects(t, repo, projects)
	test.CreatePartitions(t, repo, []*adminv2.PartitionServiceCreateRequest{
		{Partition: &apiv2.Partition{Id: "partition-one", BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}}},
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
			Id:        pointer.Pointer("underlay"),
			Name:      pointer.Pointer("Underlay Network"),
			Partition: pointer.Pointer("partition-one"),
			Prefixes:  []string{"10.0.0.0/24"},
			Type:      apiv2.NetworkType_NETWORK_TYPE_UNDERLAY,
		},
		{
			Id:                  pointer.Pointer("internet"),
			Prefixes:            []string{"1.2.3.0/24"},
			DestinationPrefixes: []string{"0.0.0.0/0"},
			Vrf:                 pointer.Pointer(uint32(11)),
			Type:                apiv2.NetworkType_NETWORK_TYPE_EXTERNAL,
		},
	})

	networkMap := test.AllocateNetworks(t, repo, []*apiv2.NetworkServiceCreateRequest{
		{Name: pointer.Pointer("p1-network-a"), Project: p1, Partition: pointer.Pointer("partition-one")},
		{Name: pointer.Pointer("p1-network-b"), Project: p1, Partition: pointer.Pointer("partition-one")},
		{Name: pointer.Pointer("p2-network-a"), Project: p2, Partition: pointer.Pointer("partition-one")},
		{Name: pointer.Pointer("p2-network-b"), Project: p2, Partition: pointer.Pointer("partition-one"), Labels: &apiv2.Labels{Labels: map[string]string{"a": "b"}}},
		{Name: pointer.Pointer("p3-network-a"), Project: p3, Partition: pointer.Pointer("partition-one")},
	})

	tests := []struct {
		name    string
		rq      *apiv2.NetworkServiceUpdateRequest
		want    *apiv2.NetworkServiceUpdateResponse
		wantErr error
	}{
		{
			name: "update name",
			rq: &apiv2.NetworkServiceUpdateRequest{
				Id: networkMap["p1-network-a"].ID,
				UpdateMeta: &apiv2.UpdateMeta{
					UpdatedAt: timestamppb.New(networkMap["p1-network-a"].Changed),
				},
				Project: p1,
				Name:    pointer.Pointer("P1 Updated Network")},
			want: &apiv2.NetworkServiceUpdateResponse{
				Network: &apiv2.Network{
					Id:            networkMap["p1-network-a"].ID,
					Meta:          &apiv2.Meta{Generation: 1},
					Name:          pointer.Pointer("P1 Updated Network"),
					ParentNetwork: pointer.Pointer("tenant-super-network"),
					Project:       pointer.Pointer(p1),
					Partition:     pointer.Pointer("partition-one"),
					Vrf:           pointer.Pointer(uint32(20)),
					Prefixes:      []string{"10.100.0.0/22"},
					Type:          apiv2.NetworkType_NETWORK_TYPE_CHILD.Enum(),
				},
			},
			wantErr: nil,
		},
		{
			name: "update description",
			rq: &apiv2.NetworkServiceUpdateRequest{
				Id: networkMap["p1-network-b"].ID,
				UpdateMeta: &apiv2.UpdateMeta{
					UpdatedAt: timestamppb.New(networkMap["p1-network-b"].Changed),
				},
				Project:     p1,
				Description: pointer.Pointer("P1 Description")},
			want: &apiv2.NetworkServiceUpdateResponse{
				Network: &apiv2.Network{
					Id:            networkMap["p1-network-b"].ID,
					Meta:          &apiv2.Meta{Generation: 1},
					Name:          pointer.Pointer("p1-network-b"),
					Description:   pointer.Pointer("P1 Description"),
					ParentNetwork: pointer.Pointer("tenant-super-network"),
					Project:       pointer.Pointer(p1),
					Partition:     pointer.Pointer("partition-one"),
					Prefixes:      []string{"10.100.4.0/22"},
					Vrf:           pointer.Pointer(uint32(30)),
					Type:          apiv2.NetworkType_NETWORK_TYPE_CHILD.Enum(),
				},
			},
			wantErr: nil,
		},
		{
			name: "update labels",
			rq: &apiv2.NetworkServiceUpdateRequest{
				Id: networkMap["p3-network-a"].ID,
				UpdateMeta: &apiv2.UpdateMeta{
					UpdatedAt: timestamppb.New(networkMap["p3-network-a"].Changed),
				},
				Project: p3,
				Labels:  &apiv2.UpdateLabels{Update: &apiv2.Labels{Labels: map[string]string{"size": "small"}}},
			},
			want: &apiv2.NetworkServiceUpdateResponse{
				Network: &apiv2.Network{
					Id: networkMap["p3-network-a"].ID,
					Meta: &apiv2.Meta{
						Labels:     &apiv2.Labels{Labels: map[string]string{"size": "small"}},
						Generation: 1,
					},
					Name:          pointer.Pointer("p3-network-a"),
					ParentNetwork: pointer.Pointer("tenant-super-network"),
					Project:       pointer.Pointer(p3),
					Partition:     pointer.Pointer("partition-one"),
					Prefixes:      []string{"10.100.16.0/22"},
					Vrf:           pointer.Pointer(uint32(44)),
					Type:          apiv2.NetworkType_NETWORK_TYPE_CHILD.Enum(),
				},
			},
			wantErr: nil,
		},
		{
			name: "remove labels",
			rq: &apiv2.NetworkServiceUpdateRequest{
				Id: networkMap["p2-network-b"].ID,
				UpdateMeta: &apiv2.UpdateMeta{
					UpdatedAt: timestamppb.New(networkMap["p2-network-b"].Changed),
				},
				Project: p2,
				Labels:  &apiv2.UpdateLabels{Remove: []string{"a"}},
			},
			want: &apiv2.NetworkServiceUpdateResponse{
				Network: &apiv2.Network{
					Id: networkMap["p2-network-b"].ID,
					Meta: &apiv2.Meta{
						Labels:     &apiv2.Labels{},
						Generation: 1,
					},
					Name:          pointer.Pointer("p2-network-b"),
					ParentNetwork: pointer.Pointer("tenant-super-network"),
					Project:       pointer.Pointer(p2),
					Partition:     pointer.Pointer("partition-one"),
					Prefixes:      []string{"10.100.12.0/22"},
					Vrf:           pointer.Pointer(uint32(42)),
					Type:          apiv2.NetworkType_NETWORK_TYPE_CHILD.Enum(),
				},
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
			rq:      &apiv2.NetworkServiceUpdateRequest{Id: networkMap["p3-network-a"].ID, Project: "p4"},
			want:    nil,
			wantErr: errorutil.NotFound("*metal.Network with id %q not found", networkMap["p3-network-a"].ID),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := &networkServiceServer{
				log:  log,
				repo: repo,
			}
			if tt.wantErr == nil {
				// Execute proto based validation
				test.Validate(t, tt.rq)
			}
			got, err := n.Update(t.Context(), tt.rq)
			if diff := cmp.Diff(err, tt.wantErr, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("diff = %s", diff)
			}

			if diff := cmp.Diff(
				tt.want, got,
				protocmp.Transform(),
				protocmp.IgnoreFields(
					&apiv2.Network{}, "consumption",
				),
				protocmp.IgnoreFields(
					&apiv2.Meta{}, "created_at", "updated_at",
				),
			); diff != "" {
				t.Errorf("networkServiceServer.Update() = %v, want %vņdiff: %s", got, tt.want, diff)
			}
		})
	}
}

func Test_networkServiceServer_Create(t *testing.T) {
	t.Parallel()

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

	test.CreateTenants(t, testStore, tenants)
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
			Id:                       pointer.Pointer("super-with-project"),
			Project:                  pointer.Pointer(p1),
			Prefixes:                 []string{"192.168.2.0/24"},
			DefaultChildPrefixLength: &apiv2.ChildPrefixLength{Ipv4: pointer.Pointer(uint32(28))},
			Type:                     apiv2.NetworkType_NETWORK_TYPE_SUPER,
			Partition:                pointer.Pointer("partition-four"),
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
			Prefixes:            []string{"1.2.3.0/24"},
			DestinationPrefixes: []string{"0.0.0.0/0"},
			Vrf:                 pointer.Pointer(uint32(90)),
			Type:                apiv2.NetworkType_NETWORK_TYPE_EXTERNAL,
		},
	})

	tests := []struct {
		name    string
		rq      *apiv2.NetworkServiceCreateRequest
		want    *apiv2.NetworkServiceCreateResponse
		wantErr error
	}{
		{
			name: "simple network defaults to v4",
			rq:   &apiv2.NetworkServiceCreateRequest{Project: p1, Name: pointer.Pointer("My Machine Network"), Partition: pointer.Pointer("partition-one")},
			want: &apiv2.NetworkServiceCreateResponse{
				Network: &apiv2.Network{
					Name:          pointer.Pointer("My Machine Network"),
					Meta:          &apiv2.Meta{},
					ParentNetwork: pointer.Pointer("tenant-super-network"),
					Partition:     pointer.Pointer("partition-one"),
					Project:       pointer.Pointer(p1),
					Prefixes:      []string{"10.100.0.0/22"},
					Type:          apiv2.NetworkType_NETWORK_TYPE_CHILD.Enum(),
				}},
			wantErr: nil,
		},
		{
			name: "simple network defaults to v4 with empty length",
			rq:   &apiv2.NetworkServiceCreateRequest{Project: p1, Name: pointer.Pointer("My Machine Network"), Length: &apiv2.ChildPrefixLength{}, Partition: pointer.Pointer("partition-one")},
			want: &apiv2.NetworkServiceCreateResponse{
				Network: &apiv2.Network{
					Name:          pointer.Pointer("My Machine Network"),
					Meta:          &apiv2.Meta{},
					ParentNetwork: pointer.Pointer("tenant-super-network"),
					Partition:     pointer.Pointer("partition-one"),
					Project:       pointer.Pointer(p1),
					Prefixes:      []string{"10.100.4.0/22"},
					Type:          apiv2.NetworkType_NETWORK_TYPE_CHILD.Enum(),
				}},
			wantErr: nil,
		},
		{
			name: "simple network defaults to v4 with different length",
			rq:   &apiv2.NetworkServiceCreateRequest{Project: p1, Name: pointer.Pointer("My Machine Network"), Length: &apiv2.ChildPrefixLength{Ipv4: pointer.Pointer(uint32(28))}, Partition: pointer.Pointer("partition-one")},
			want: &apiv2.NetworkServiceCreateResponse{
				Network: &apiv2.Network{
					Name:          pointer.Pointer("My Machine Network"),
					Meta:          &apiv2.Meta{},
					ParentNetwork: pointer.Pointer("tenant-super-network"),
					Partition:     pointer.Pointer("partition-one"),
					Project:       pointer.Pointer(p1),
					Prefixes:      []string{"10.100.8.0/28"},
					Type:          apiv2.NetworkType_NETWORK_TYPE_CHILD.Enum(),
				}},
			wantErr: nil,
		},
		{
			name: "simple network defaults to v6",
			rq:   &apiv2.NetworkServiceCreateRequest{Project: p1, Name: pointer.Pointer("My Machine Network"), Partition: pointer.Pointer("partition-two")},
			want: &apiv2.NetworkServiceCreateResponse{
				Network: &apiv2.Network{
					Name:          pointer.Pointer("My Machine Network"),
					Meta:          &apiv2.Meta{},
					ParentNetwork: pointer.Pointer("tenant-super-network-v6"),
					Partition:     pointer.Pointer("partition-two"),
					Project:       pointer.Pointer(p1),
					Prefixes:      []string{"2001:db8::/112"},
					Type:          apiv2.NetworkType_NETWORK_TYPE_CHILD.Enum(),
				}},
			wantErr: nil,
		},
		{
			name: "simple network defaults to v4 with different length",
			rq:   &apiv2.NetworkServiceCreateRequest{Project: p1, Name: pointer.Pointer("My Machine Network"), Length: &apiv2.ChildPrefixLength{Ipv6: pointer.Pointer(uint32(120))}, Partition: pointer.Pointer("partition-two")},
			want: &apiv2.NetworkServiceCreateResponse{
				Network: &apiv2.Network{
					Name:          pointer.Pointer("My Machine Network"),
					Meta:          &apiv2.Meta{},
					ParentNetwork: pointer.Pointer("tenant-super-network-v6"),
					Partition:     pointer.Pointer("partition-two"),
					Project:       pointer.Pointer(p1),
					Prefixes:      []string{"2001:db8::1:0/120"},
					Type:          apiv2.NetworkType_NETWORK_TYPE_CHILD.Enum(),
				}},
			wantErr: nil,
		},
		{
			name: "simple network defaults to dualstack",
			rq:   &apiv2.NetworkServiceCreateRequest{Project: p1, Name: pointer.Pointer("My Machine Network"), Partition: pointer.Pointer("partition-three")},
			want: &apiv2.NetworkServiceCreateResponse{
				Network: &apiv2.Network{
					Name:          pointer.Pointer("My Machine Network"),
					Meta:          &apiv2.Meta{},
					ParentNetwork: pointer.Pointer("tenant-super-network-dualstack"),
					Partition:     pointer.Pointer("partition-three"),
					Project:       pointer.Pointer(p1),
					Prefixes:      []string{"10.200.0.0/22", "2001:dc8::/112"},
					Type:          apiv2.NetworkType_NETWORK_TYPE_CHILD.Enum(),
				}},
			wantErr: nil,
		},
		{
			name: "simple network force v6 by af",
			rq: &apiv2.NetworkServiceCreateRequest{
				Project:       p2,
				Name:          pointer.Pointer("My Machine Network"),
				Partition:     pointer.Pointer("partition-three"),
				AddressFamily: apiv2.NetworkAddressFamily_NETWORK_ADDRESS_FAMILY_V6.Enum(),
			},
			want: &apiv2.NetworkServiceCreateResponse{
				Network: &apiv2.Network{
					Name:          pointer.Pointer("My Machine Network"),
					Meta:          &apiv2.Meta{},
					ParentNetwork: pointer.Pointer("tenant-super-network-dualstack"),
					Partition:     pointer.Pointer("partition-three"),
					Project:       pointer.Pointer(p2),
					Prefixes:      []string{"2001:dc8::1:0/112"},
					Type:          apiv2.NetworkType_NETWORK_TYPE_CHILD.Enum(),
				}},
			wantErr: nil,
		},
		{
			name: "simple network force v4 by af",
			rq: &apiv2.NetworkServiceCreateRequest{
				Project:       p2,
				Name:          pointer.Pointer("My Machine Network"),
				Partition:     pointer.Pointer("partition-three"),
				AddressFamily: apiv2.NetworkAddressFamily_NETWORK_ADDRESS_FAMILY_V4.Enum(),
			},
			want: &apiv2.NetworkServiceCreateResponse{
				Network: &apiv2.Network{
					Name:          pointer.Pointer("My Machine Network"),
					Meta:          &apiv2.Meta{},
					ParentNetwork: pointer.Pointer("tenant-super-network-dualstack"),
					Partition:     pointer.Pointer("partition-three"),
					Project:       pointer.Pointer(p2),
					Prefixes:      []string{"10.200.4.0/22"},
					Type:          apiv2.NetworkType_NETWORK_TYPE_CHILD.Enum(),
				}},
			wantErr: nil,
		},
		{
			name: "simple network dualstack but v6 with different length",
			rq: &apiv2.NetworkServiceCreateRequest{
				Project:   p2,
				Name:      pointer.Pointer("My Machine Network"),
				Partition: pointer.Pointer("partition-three"),
				Length:    &apiv2.ChildPrefixLength{Ipv6: pointer.Pointer(uint32(116))},
			},
			want: &apiv2.NetworkServiceCreateResponse{
				Network: &apiv2.Network{
					Name:          pointer.Pointer("My Machine Network"),
					Meta:          &apiv2.Meta{},
					ParentNetwork: pointer.Pointer("tenant-super-network-dualstack"),
					Partition:     pointer.Pointer("partition-three"),
					Project:       pointer.Pointer(p2),
					Prefixes:      []string{"2001:dc8::2:0/116"},
					Type:          apiv2.NetworkType_NETWORK_TYPE_CHILD.Enum(),
				}},
			wantErr: nil,
		},
		{
			name: "disallow child network creation for super network that is scoped on a different project",
			rq: &apiv2.NetworkServiceCreateRequest{
				Project:       p2,
				Name:          pointer.Pointer("My Machine Network"),
				ParentNetwork: pointer.Pointer("super-with-project"),
			},
			want:    nil,
			wantErr: errorutil.InvalidArgument("not allowed to create child network with project %s in network super-with-project scoped to project %s", p2, p1),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := &networkServiceServer{
				log:  log,
				repo: repo,
			}
			if tt.wantErr == nil {
				// Execute proto based validation
				test.Validate(t, tt.rq)
			}
			got, err := n.Create(ctx, tt.rq)
			if diff := cmp.Diff(err, tt.wantErr, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("diff = %s", diff)
			}

			if diff := cmp.Diff(
				tt.want, got,
				protocmp.Transform(),
				protocmp.IgnoreFields(
					&apiv2.Network{}, "consumption", "id", "vrf",
				),
				protocmp.IgnoreFields(
					&apiv2.Meta{}, "created_at", "updated_at",
				),
			); diff != "" {
				t.Errorf("networkServiceServer.Create() = %v, want %vņdiff: %s", got, tt.want, diff)
			}
		})
	}
}

func Test_networkServiceServer_Delete(t *testing.T) {
	t.Parallel()

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	testStore, closer := test.StartRepositoryWithCleanup(t, log)
	defer closer()
	repo := testStore.Store

	ctx := t.Context()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintln(w, "a image")
	}))
	defer ts.Close()

	validURL := ts.URL

	test.CreateTenants(t, testStore, tenants)
	test.CreateProjects(t, repo, projects)
	test.CreatePartitions(t, repo, []*adminv2.PartitionServiceCreateRequest{
		{Partition: &apiv2.Partition{Id: "partition-one", BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}}},
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
			Id:        pointer.Pointer("underlay"),
			Name:      pointer.Pointer("Underlay Network"),
			Partition: pointer.Pointer("partition-one"),
			Prefixes:  []string{"10.0.0.0/24"},
			Type:      apiv2.NetworkType_NETWORK_TYPE_UNDERLAY,
		},
		{
			Id:                  pointer.Pointer("internet"),
			Prefixes:            []string{"1.2.3.0/24"},
			DestinationPrefixes: []string{"0.0.0.0/0"},
			Vrf:                 pointer.Pointer(uint32(11)),
			Type:                apiv2.NetworkType_NETWORK_TYPE_EXTERNAL,
		},
	})

	networkMap := test.AllocateNetworks(t, repo, []*apiv2.NetworkServiceCreateRequest{
		{Name: pointer.Pointer("p1-network-a"), Project: p1, Partition: pointer.Pointer("partition-one")},
		{Name: pointer.Pointer("p1-network-b"), Project: p1, Partition: pointer.Pointer("partition-one")},
		{Name: pointer.Pointer("p2-network-a"), Project: p2, Partition: pointer.Pointer("partition-one")},
		{Name: pointer.Pointer("p2-network-b"), Project: p2, Partition: pointer.Pointer("partition-one")},
		{Name: pointer.Pointer("p3-network-a"), Project: p3, Partition: pointer.Pointer("partition-one")},
	})

	test.CreateMachines(t, testStore, []*metal.Machine{
		{
			Base: metal.Base{ID: "m1"}, PartitionID: "partition-one",
			Allocation: &metal.MachineAllocation{
				MachineNetworks: []*metal.MachineNetwork{
					{
						NetworkID: networkMap["p3-network-a"].ID,
					},
				},
			},
		},
	})

	tests := []struct {
		name    string
		rq      *apiv2.NetworkServiceDeleteRequest
		want    *apiv2.NetworkServiceDeleteResponse
		wantErr error
	}{
		{
			name: "delete existing",
			rq:   &apiv2.NetworkServiceDeleteRequest{Id: networkMap["p1-network-a"].ID, Project: p1},
			want: &apiv2.NetworkServiceDeleteResponse{
				Network: &apiv2.Network{
					Id:            networkMap["p1-network-a"].ID,
					Meta:          &apiv2.Meta{},
					Name:          pointer.Pointer("p1-network-a"),
					ParentNetwork: pointer.Pointer("tenant-super-network"),
					Project:       pointer.Pointer(p1),
					Partition:     pointer.Pointer("partition-one"),
					Prefixes:      []string{"10.100.0.0/22"},
					Vrf:           pointer.Pointer(uint32(35)),
					Type:          apiv2.NetworkType_NETWORK_TYPE_CHILD.Enum(),
				},
			},
			wantErr: nil,
		},
		{
			name:    "project does not match",
			rq:      &apiv2.NetworkServiceDeleteRequest{Id: networkMap["p1-network-b"].ID, Project: p2},
			want:    nil,
			wantErr: errorutil.NotFound("*metal.Network with id %q not found", networkMap["p1-network-b"].ID),
		},
		{
			name:    "network does not exist anymore",
			rq:      &apiv2.NetworkServiceDeleteRequest{Id: networkMap["p1-network-a"].ID, Project: "pa"},
			want:    nil,
			wantErr: errorutil.NotFound(`no network with id %q found`, networkMap["p1-network-a"].ID),
		},
		{
			name:    "network has machine allocation",
			rq:      &apiv2.NetworkServiceDeleteRequest{Id: networkMap["p3-network-a"].ID, Project: p3},
			want:    nil,
			wantErr: errorutil.InvalidArgument(`cannot remove network with existing machine allocations`),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := &networkServiceServer{
				log:  log,
				repo: repo,
			}
			if tt.wantErr == nil {
				// Execute proto based validation
				test.Validate(t, tt.rq)
			}
			got, err := n.Delete(ctx, tt.rq)
			if diff := cmp.Diff(err, tt.wantErr, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("diff = %s", diff)
			}
			if diff := cmp.Diff(
				tt.want, got,
				protocmp.Transform(),
				protocmp.IgnoreFields(
					&apiv2.Network{}, "consumption", "id", "vrf",
				),
				protocmp.IgnoreFields(
					&apiv2.Meta{}, "created_at", "updated_at",
				),
			); diff != "" {
				t.Errorf("networkServiceServer.Create() = %v, want %vņdiff: %s", got, tt.want, diff)
			}

			if tt.want == nil {
				return
			}
			assert.EventuallyWithT(t, func(collect *assert.CollectT) {
				_, err := repo.UnscopedNetwork().Get(ctx, tt.rq.Id)
				t.Logf("check for network:%q being deleted:%v", tt.rq.Id, err)
				assert.True(collect, errorutil.IsNotFound(err))
			}, 5*time.Second, 100*time.Millisecond)

		})
	}
}
