package queries_test

import (
	"log/slog"
	"os"
	"sort"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/stretchr/testify/require"

	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-apiserver/pkg/db/queries"
	"github.com/metal-stack/metal-apiserver/pkg/test"
)

var (
	testProjectID = "project-1"
	testNamespace = "namespace-1"

	i1 = &metal.IP{
		IPAddress:        "10.0.0.1",
		AllocationUUID:   "uuid-1",
		Namespace:        nil,
		ParentPrefixCidr: "10.0.0.0/8",
		Name:             "ip-one",
		Description:      "IP 1",
		ProjectID:        testProjectID,
		NetworkID:        "n1",
		Type:             metal.Ephemeral,
		Tags:             []string{"color=red", "size=small"},
		Created:          time.Time{},
		Changed:          time.Time{},
	}
	i2 = &metal.IP{
		IPAddress:        "10.0.0.2",
		AllocationUUID:   "uuid-2",
		Namespace:        nil,
		ParentPrefixCidr: "10.0.0.0/8",
		Name:             "ip-two",
		Description:      "IP 2",
		ProjectID:        testProjectID,
		NetworkID:        "n2",
		Type:             metal.Static,
		Tags:             []string{"color=green", "size=medium"},
		Created:          time.Time{},
		Changed:          time.Time{},
	}
	i3 = &metal.IP{
		IPAddress:        "10.0.0.3",
		AllocationUUID:   "uuid-3",
		Namespace:        &testNamespace,
		ParentPrefixCidr: "10.0.1.0/24",
		Name:             "ip-three",
		Description:      "IP 3",
		ProjectID:        testProjectID,
		NetworkID:        "n3",
		Type:             metal.Ephemeral,
		Tags:             []string{"color=blue"},
		Created:          time.Time{},
		Changed:          time.Time{},
	}
	i4 = &metal.IP{
		IPAddress:        "10.0.0.4",
		AllocationUUID:   "uuid-4",
		Namespace:        nil,
		ParentPrefixCidr: "10.0.0.0/8",
		Name:             "ip-four",
		Description:      "IP 4",
		ProjectID:        "project-2",
		NetworkID:        "n1",
		Type:             metal.Static,
		Tags:             []string{"color=yellow"},
		Created:          time.Time{},
		Changed:          time.Time{},
	}
	i5 = &metal.IP{
		IPAddress:        "fd12:34:56:78::1",
		AllocationUUID:   "uuid-5",
		Namespace:        nil,
		ParentPrefixCidr: "fd12:34:56:78::/64",
		Name:             "ip-v6",
		Description:      "IPv6 IP",
		ProjectID:        testProjectID,
		NetworkID:        "n4",
		Type:             metal.Ephemeral,
		Tags:             []string{},
		Created:          time.Time{},
		Changed:          time.Time{},
	}
	ips = []*metal.IP{i1, i2, i3, i4, i5}
)

func TestIPFilter(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	ds, _, rethinkCloser := test.StartRethink(t, log)
	defer func() {
		rethinkCloser()
	}()

	ctx := t.Context()

	for _, ip := range ips {
		_, err := ds.IP().Create(ctx, ip)
		require.NoError(t, err)
	}

	tests := []struct {
		name string
		rq   *apiv2.IPQuery
		want []*metal.IP
	}{
		{
			name: "empty request returns unfiltered",
			rq:   nil,
			want: []*metal.IP{i1, i2, i3, i4, i5},
		},
		{
			name: "by ip",
			rq:   &apiv2.IPQuery{Ip: &i1.IPAddress},
			want: []*metal.IP{i1},
		},
		{
			name: "by ip 2",
			rq:   &apiv2.IPQuery{Ip: &i2.IPAddress},
			want: []*metal.IP{i2},
		},
		{
			name: "by uuid",
			rq:   &apiv2.IPQuery{Uuid: &i1.AllocationUUID},
			want: []*metal.IP{i1},
		},
		{
			name: "by name",
			rq:   &apiv2.IPQuery{Name: &i1.Name},
			want: []*metal.IP{i1},
		},
		{
			name: "by name 2",
			rq:   &apiv2.IPQuery{Name: &i5.Name},
			want: []*metal.IP{i5},
		},
		{
			name: "by project",
			rq:   &apiv2.IPQuery{Project: &i1.ProjectID},
			want: []*metal.IP{i1, i2, i3, i5},
		},
		{
			name: "by project different",
			rq:   &apiv2.IPQuery{Project: &i4.ProjectID},
			want: []*metal.IP{i4},
		},
		{
			name: "by namespace",
			rq:   &apiv2.IPQuery{Namespace: i3.Namespace},
			want: []*metal.IP{i3},
		},
		{
			name: "by network",
			rq:   &apiv2.IPQuery{Network: &i1.NetworkID},
			want: []*metal.IP{i1, i4},
		},
		{
			name: "by network 2",
			rq:   &apiv2.IPQuery{Network: &i3.NetworkID},
			want: []*metal.IP{i3},
		},
		{
			name: "by parent prefix cidr",
			rq:   &apiv2.IPQuery{ParentPrefixCidr: &i1.ParentPrefixCidr},
			want: []*metal.IP{i1, i2, i4},
		},
		{
			name: "by parent prefix cidr 2",
			rq:   &apiv2.IPQuery{ParentPrefixCidr: &i3.ParentPrefixCidr},
			want: []*metal.IP{i3},
		},
		{
			name: "by label",
			rq:   &apiv2.IPQuery{Labels: &apiv2.Labels{Labels: map[string]string{"color": "red"}}},
			want: []*metal.IP{i1},
		},
		{
			name: "by label 2",
			rq:   &apiv2.IPQuery{Labels: &apiv2.Labels{Labels: map[string]string{"size": "small"}}},
			want: []*metal.IP{i1},
		},
		{
			name: "by label 3",
			rq:   &apiv2.IPQuery{Labels: &apiv2.Labels{Labels: map[string]string{"color": "green"}}},
			want: []*metal.IP{i2},
		},
		{
			name: "by type ephemeral",
			rq:   &apiv2.IPQuery{Type: apiv2.IPType_IP_TYPE_EPHEMERAL.Enum()},
			want: []*metal.IP{i1, i3, i5},
		},
		{
			name: "by type static",
			rq:   &apiv2.IPQuery{Type: apiv2.IPType_IP_TYPE_STATIC.Enum()},
			want: []*metal.IP{i2, i4},
		},
		{
			name: "by address family v4",
			rq:   &apiv2.IPQuery{AddressFamily: apiv2.IPAddressFamily_IP_ADDRESS_FAMILY_V4.Enum()},
			want: []*metal.IP{i1, i2, i3, i4},
		},
		{
			name: "by address family v6",
			rq:   &apiv2.IPQuery{AddressFamily: apiv2.IPAddressFamily_IP_ADDRESS_FAMILY_V6.Enum()},
			want: []*metal.IP{i5},
		},
		{
			name: "by wrong address family",
			rq:   &apiv2.IPQuery{AddressFamily: apiv2.IPAddressFamily_IP_ADDRESS_FAMILY_V4.Enum()},
			want: []*metal.IP{i1, i2, i3, i4},
		},
		{
			name: "by machine empty result",
			rq:   &apiv2.IPQuery{Machine: new("nonexistent-machine")},
			want: nil,
		},
		{
			name: "combined by project and type",
			rq: &apiv2.IPQuery{
				Project: &i1.ProjectID,
				Type:    apiv2.IPType_IP_TYPE_STATIC.Enum(),
			},
			want: []*metal.IP{i2},
		},
		{
			name: "combined by ip and project",
			rq: &apiv2.IPQuery{
				Ip:      &i1.IPAddress,
				Project: &i1.ProjectID,
			},
			want: []*metal.IP{i1},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ds.IP().List(ctx, queries.IpFilter(tt.rq))
			require.NoError(t, err)

			sort.Slice(got, func(i, j int) bool {
				return got[i].IPAddress < got[j].IPAddress
			})
			sort.Slice(tt.want, func(i, j int) bool {
				return tt.want[i].IPAddress < tt.want[j].IPAddress
			})

			if diff := cmp.Diff(
				tt.want, got,
				cmpopts.IgnoreFields(
					metal.IP{}, "Created", "Changed",
				),
				cmpopts.SortSlices(func(a, b *metal.IP) bool {
					return a.IPAddress < b.IPAddress
				}),
			); diff != "" {
				t.Errorf("ipServiceServer.List() = %v, want %v\n diff: %s", got, tt.want, diff)
			}

		})
	}
}

func TestIpProjectScoped(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	ds, _, rethinkCloser := test.StartRethink(t, log)
	defer func() {
		rethinkCloser()
	}()

	ctx := t.Context()

	for _, ip := range ips {
		_, err := ds.IP().Create(ctx, ip)
		require.NoError(t, err)
	}

	tests := []struct {
		name     string
		project  string
		expected []string
	}{
		{
			name:     "project-1 returns matching ips",
			project:  "project-1",
			expected: []string{i1.IPAddress, i2.IPAddress, i3.IPAddress, i5.IPAddress},
		},
		{
			name:     "project-2 returns matching ips",
			project:  "project-2",
			expected: []string{i4.IPAddress},
		},
		{
			name:     "nonexistent project returns empty",
			project:  "project-nonexistent",
			expected: []string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ds.IP().List(ctx, queries.IpProjectScoped(tt.project))
			require.NoError(t, err)

			var gotIDs []string
			for _, ip := range got {
				gotIDs = append(gotIDs, ip.IPAddress)
			}

			require.ElementsMatch(t, tt.expected, gotIDs)
		})
	}
}
