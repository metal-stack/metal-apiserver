package test_test

import (
	"log/slog"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/test"
	"github.com/metal-stack/metal-apiserver/pkg/test/scenarios"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/testing/protocmp"
)

// TODO: is this test still needed?
func Test_partitionUpdateWithDatacenter(t *testing.T) {
	t.Parallel()

	dc := test.NewDatacenter(t, slog.Default())
	dc.Create(&scenarios.DefaultDatacenter)
	defer dc.Close()

	dc.Dump()
	dc.CleanUp()
}

func TestCopy(t *testing.T) {
	tests := []struct {
		name string
		dc   *test.Datacenter
		want *test.Datacenter
	}{
		{
			name: "copy datacenter entities",
			dc: &test.Datacenter{
				Tenants:    []string{"t1"},
				Projects:   map[string]string{"project-1": "00000000-0000-0000-0000-000000000001"},
				Partitions: map[string]*apiv2.Partition{"partition-1": {Id: "partition-1"}},
				Sizes:      map[string]*apiv2.Size{"s1": {Id: "s1"}},
				Networks:   map[string]*apiv2.Network{"n1": {Id: "n1"}},
				IPs:        map[string]*apiv2.IP{"1.1.1.1": {Ip: "1.1.1.1"}},
				Images:     map[string]*apiv2.Image{"i1": {Id: "i1"}},
				Switches:   map[string]*apiv2.Switch{"sw1": {Id: "sw1"}},
				Machines:   map[string]*apiv2.Machine{"m1": {Uuid: "00000000-0000-0000-0000-000000000011"}},
			},
			want: &test.Datacenter{
				Tenants:    []string{"t1"},
				Projects:   map[string]string{"project-1": "00000000-0000-0000-0000-000000000001"},
				Partitions: map[string]*apiv2.Partition{"partition-1": {Id: "partition-1"}},
				Sizes:      map[string]*apiv2.Size{"s1": {Id: "s1"}},
				Networks:   map[string]*apiv2.Network{"n1": {Id: "n1"}},
				IPs:        map[string]*apiv2.IP{"1.1.1.1": {Ip: "1.1.1.1"}},
				Images:     map[string]*apiv2.Image{"i1": {Id: "i1"}},
				Switches:   map[string]*apiv2.Switch{"sw1": {Id: "sw1"}},
				Machines:   map[string]*apiv2.Machine{"m1": {Uuid: "00000000-0000-0000-0000-000000000011"}},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := test.Copy(tt.dc)
			require.NoError(t, err)

			// change some field in the original to ensure the copy isn't affected
			tt.dc.Projects["project-1"] = "10000000-0000-0000-0000-000000000001"

			if diff := cmp.Diff(tt.want, got, protocmp.Transform(), cmpopts.IgnoreFields(
				test.Datacenter{}, "TestStore", "t", "closers",
			)); diff != "" {
				t.Errorf("Copy() diff = %s", diff)
			}
		})
	}
}

func TestAssert(t *testing.T) {
	tests := []struct {
		name    string
		prev    *test.Datacenter
		current *test.Datacenter
		modify  func(*test.Datacenter)
		wantErr bool
	}{
		{
			name: "no modification, both equal",
			prev: &test.Datacenter{
				Tenants:    []string{"t1"},
				Projects:   map[string]string{"project-1": "00000000-0000-0000-0000-000000000001"},
				Partitions: map[string]*apiv2.Partition{"partition-1": {Id: "partition-1"}},
				Sizes:      map[string]*apiv2.Size{"s1": {Id: "s1"}},
				Networks:   map[string]*apiv2.Network{"n1": {Id: "n1"}},
				IPs:        map[string]*apiv2.IP{"1.1.1.1": {Ip: "1.1.1.1"}},
				Images:     map[string]*apiv2.Image{"i1": {Id: "i1"}},
				Switches:   map[string]*apiv2.Switch{"sw1": {Id: "sw1"}},
				Machines:   map[string]*apiv2.Machine{"m1": {Uuid: "00000000-0000-0000-0000-000000000011"}},
			},
			current: &test.Datacenter{
				Tenants:    []string{"t1"},
				Projects:   map[string]string{"project-1": "00000000-0000-0000-0000-000000000001"},
				Partitions: map[string]*apiv2.Partition{"partition-1": {Id: "partition-1"}},
				Sizes:      map[string]*apiv2.Size{"s1": {Id: "s1"}},
				Networks:   map[string]*apiv2.Network{"n1": {Id: "n1"}},
				IPs:        map[string]*apiv2.IP{"1.1.1.1": {Ip: "1.1.1.1"}},
				Images:     map[string]*apiv2.Image{"i1": {Id: "i1"}},
				Switches:   map[string]*apiv2.Switch{"sw1": {Id: "sw1"}},
				Machines:   map[string]*apiv2.Machine{"m1": {Uuid: "00000000-0000-0000-0000-000000000011"}},
			},
			modify:  nil,
			wantErr: false,
		},
		{
			name: "no modification, but datacenters differ",
			prev: &test.Datacenter{
				Tenants:    []string{"t1"},
				Projects:   map[string]string{"project-1": "00000000-0000-0000-0000-000000000001"},
				Partitions: map[string]*apiv2.Partition{"partition-1": {Id: "partition-1"}},
				Sizes:      map[string]*apiv2.Size{"s1": {Id: "s1"}},
				Networks:   map[string]*apiv2.Network{"n1": {Id: "n1"}},
				IPs:        map[string]*apiv2.IP{"1.1.1.1": {Ip: "1.1.1.1"}},
				Images:     map[string]*apiv2.Image{"i1": {Id: "i1"}},
				Switches:   map[string]*apiv2.Switch{"sw1": {Id: "sw1"}},
				Machines:   map[string]*apiv2.Machine{"m1": {Uuid: "00000000-0000-0000-0000-000000000011"}},
			},
			current: &test.Datacenter{
				Tenants:    []string{"t1"},
				Projects:   map[string]string{"project-1": "00000000-0000-0000-0000-000000000001", "project-2": "00000000-0000-0000-0000-000000000002"},
				Partitions: map[string]*apiv2.Partition{"partition-1": {Id: "partition-1"}},
				Sizes:      map[string]*apiv2.Size{"s1": {Id: "s1"}},
				Networks:   map[string]*apiv2.Network{"n1": {Id: "n1"}},
				IPs:        map[string]*apiv2.IP{"1.1.1.1": {Ip: "1.1.1.1"}},
				Images:     map[string]*apiv2.Image{"i1": {Id: "i1"}},
				Switches:   map[string]*apiv2.Switch{"sw1": {Id: "sw1"}},
				Machines:   map[string]*apiv2.Machine{"m1": {Uuid: "00000000-0000-0000-0000-000000000011"}},
			},
			modify:  nil,
			wantErr: true,
		},
		{
			name: "apply correct modification",
			prev: &test.Datacenter{
				Tenants:    []string{"t1"},
				Projects:   map[string]string{"project-1": "00000000-0000-0000-0000-000000000001"},
				Partitions: map[string]*apiv2.Partition{"partition-1": {Id: "partition-1"}},
				Sizes:      map[string]*apiv2.Size{"s1": {Id: "s1"}},
				Networks:   map[string]*apiv2.Network{"n1": {Id: "n1"}},
				IPs:        map[string]*apiv2.IP{"1.1.1.1": {Ip: "1.1.1.1"}},
				Images:     map[string]*apiv2.Image{"i1": {Id: "i1"}},
				Switches:   map[string]*apiv2.Switch{"sw1": {Id: "sw1"}},
				Machines:   map[string]*apiv2.Machine{"m1": {Uuid: "00000000-0000-0000-0000-000000000011"}},
			},
			current: &test.Datacenter{
				Tenants:    []string{"t1"},
				Projects:   map[string]string{"project-1": "00000000-0000-0000-0000-000000000001", "project-2": "00000000-0000-0000-0000-000000000002"},
				Partitions: map[string]*apiv2.Partition{"partition-1": {Id: "partition-1"}},
				Sizes:      map[string]*apiv2.Size{"s1": {Id: "s1"}},
				Networks:   map[string]*apiv2.Network{"n1": {Id: "n1"}},
				IPs:        map[string]*apiv2.IP{"1.1.1.1": {Ip: "1.1.1.1"}},
				Images:     map[string]*apiv2.Image{"i1": {Id: "i1"}},
				Switches:   map[string]*apiv2.Switch{"sw1": {Id: "sw1"}},
				Machines:   map[string]*apiv2.Machine{"m1": {Uuid: "00000000-0000-0000-0000-000000000011"}},
			},
			modify: func(d *test.Datacenter) {
				d.Projects["project-2"] = "00000000-0000-0000-0000-000000000002"
			},
			wantErr: false,
		},
		{
			name: "apply wrong modification",
			prev: &test.Datacenter{
				Tenants:    []string{"t1"},
				Projects:   map[string]string{"project-1": "00000000-0000-0000-0000-000000000001"},
				Partitions: map[string]*apiv2.Partition{"partition-1": {Id: "partition-1"}},
				Sizes:      map[string]*apiv2.Size{"s1": {Id: "s1"}},
				Networks:   map[string]*apiv2.Network{"n1": {Id: "n1"}},
				IPs:        map[string]*apiv2.IP{"1.1.1.1": {Ip: "1.1.1.1"}},
				Images:     map[string]*apiv2.Image{"i1": {Id: "i1"}},
				Switches:   map[string]*apiv2.Switch{"sw1": {Id: "sw1"}},
				Machines:   map[string]*apiv2.Machine{"m1": {Uuid: "00000000-0000-0000-0000-000000000011"}},
			},
			current: &test.Datacenter{
				Tenants:    []string{"t1"},
				Projects:   map[string]string{"project-1": "00000000-0000-0000-0000-000000000001", "project-2": "00000000-0000-0000-0000-000000000002"},
				Partitions: map[string]*apiv2.Partition{"partition-1": {Id: "partition-1"}},
				Sizes:      map[string]*apiv2.Size{"s1": {Id: "s1"}},
				Networks:   map[string]*apiv2.Network{"n1": {Id: "n1"}},
				IPs:        map[string]*apiv2.IP{"1.1.1.1": {Ip: "1.1.1.1"}},
				Images:     map[string]*apiv2.Image{"i1": {Id: "i1"}},
				Switches:   map[string]*apiv2.Switch{"sw1": {Id: "sw1"}},
				Machines:   map[string]*apiv2.Machine{"m1": {Uuid: "00000000-0000-0000-0000-000000000011"}},
			},
			modify: func(d *test.Datacenter) {
				d.Projects["project-3"] = "00000000-0000-0000-0000-000000000003"
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := test.Assert(tt.prev, tt.current, tt.modify); (err != nil) != tt.wantErr {
				t.Errorf("Assert() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
