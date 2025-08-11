package queries_test

import (
	"fmt"
	"log/slog"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-apiserver/pkg/db/queries"
	"github.com/metal-stack/metal-apiserver/pkg/test"
	"github.com/metal-stack/metal-lib/pkg/pointer"
	"github.com/stretchr/testify/require"
)

var (
	m1 = &metal.Machine{
		Base: metal.Base{ID: "m1", Name: "m1"},
		Allocation: &metal.MachineAllocation{
			Creator:     "",
			Created:     time.Time{},
			Name:        "shoot-worker-1",
			Description: "",
			Project:     "p1",
			ImageID:     "debian-12",
			FilesystemLayout: &metal.FilesystemLayout{
				Base:           metal.Base{},
				Filesystems:    []metal.Filesystem{},
				Disks:          []metal.Disk{},
				Raid:           []metal.Raid{},
				VolumeGroups:   []metal.VolumeGroup{},
				LogicalVolumes: metal.LogicalVolumes{},
				Constraints: metal.FilesystemLayoutConstraints{
					Sizes:  []string{},
					Images: map[string]string{},
				},
			},
			MachineNetworks: []*metal.MachineNetwork{},
			Hostname:        "shoot-worker-1",
			SSHPubKeys:      []string{},
			UserData:        "",
			ConsolePassword: "",
			Succeeded:       true,
			Reinstall:       false,
			MachineSetup:    &metal.MachineSetup{},
			Role:            metal.RoleMachine,
			VPN:             &metal.MachineVPN{},
			UUID:            "alloc-m1",
			FirewallRules: &metal.FirewallRules{
				Egress:  []metal.EgressRule{},
				Ingress: []metal.IngressRule{},
			},
			DNSServers: metal.DNSServers{},
			NTPServers: metal.NTPServers{},
		},
		PartitionID:  "p1",
		SizeID:       "c1-medium",
		RackID:       "rack-1",
		Waiting:      false,
		PreAllocated: false,
		Hardware: metal.MachineHardware{
			Memory:    0,
			Nics:      metal.Nics{},
			Disks:     []metal.BlockDevice{},
			MetalCPUs: []metal.MetalCPU{},
			MetalGPUs: []metal.MetalGPU{},
		},
		State:    metal.MachineState{},
		LEDState: metal.ChassisIdentifyLEDState{},
		Tags:     []string{"color=red"},
		IPMI: metal.IPMI{
			Address:       "",
			MacAddress:    "",
			User:          "",
			Password:      "",
			Interface:     "",
			Fru:           metal.Fru{},
			BMCVersion:    "",
			PowerState:    "",
			PowerMetric:   &metal.PowerMetric{},
			PowerSupplies: metal.PowerSupplies{},
			LastUpdated:   time.Time{},
		},
		BIOS: metal.BIOS{},
	}
	m2 = &metal.Machine{
		Base: metal.Base{ID: "m2", Name: "m2"},
		Allocation: &metal.MachineAllocation{
			Creator:     "",
			Created:     time.Time{},
			Name:        "shoot-fw-m2",
			Description: "",
			Project:     "p2",
			ImageID:     "firewall-ubuntu-3",
			FilesystemLayout: &metal.FilesystemLayout{
				Base:           metal.Base{},
				Filesystems:    []metal.Filesystem{},
				Disks:          []metal.Disk{},
				Raid:           []metal.Raid{},
				VolumeGroups:   []metal.VolumeGroup{},
				LogicalVolumes: metal.LogicalVolumes{},
				Constraints: metal.FilesystemLayoutConstraints{
					Sizes:  []string{},
					Images: map[string]string{},
				},
			},
			MachineNetworks: []*metal.MachineNetwork{},
			Hostname:        "shoot-fw-m2",
			SSHPubKeys:      []string{},
			UserData:        "",
			ConsolePassword: "",
			Succeeded:       true,
			Reinstall:       false,
			MachineSetup:    &metal.MachineSetup{},
			Role:            metal.RoleFirewall,
			VPN:             &metal.MachineVPN{},
			UUID:            "alloc-m2",
			FirewallRules: &metal.FirewallRules{
				Egress:  []metal.EgressRule{},
				Ingress: []metal.IngressRule{},
			},
			DNSServers: metal.DNSServers{},
			NTPServers: metal.NTPServers{},
		},
		PartitionID:  "p2",
		SizeID:       "n1-medium",
		RackID:       "rack-2",
		Waiting:      false,
		PreAllocated: false,
		Hardware: metal.MachineHardware{
			Memory:    0,
			Nics:      metal.Nics{},
			Disks:     []metal.BlockDevice{},
			MetalCPUs: []metal.MetalCPU{},
			MetalGPUs: []metal.MetalGPU{},
		},
		State:    metal.MachineState{},
		LEDState: metal.ChassisIdentifyLEDState{},
		Tags:     []string{"size=medium"},
		IPMI: metal.IPMI{
			Address:       "",
			MacAddress:    "",
			User:          "",
			Password:      "",
			Interface:     "",
			Fru:           metal.Fru{},
			BMCVersion:    "",
			PowerState:    "",
			PowerMetric:   &metal.PowerMetric{},
			PowerSupplies: metal.PowerSupplies{},
			LastUpdated:   time.Time{},
		},
		BIOS: metal.BIOS{},
	}
	m3 = &metal.Machine{
		Base: metal.Base{ID: "m3", Name: "m3"},
		Allocation: &metal.MachineAllocation{
			Creator:     "",
			Created:     time.Time{},
			Name:        "",
			Description: "",
			Project:     "",
			ImageID:     "",
			FilesystemLayout: &metal.FilesystemLayout{
				Base:           metal.Base{},
				Filesystems:    []metal.Filesystem{},
				Disks:          []metal.Disk{},
				Raid:           []metal.Raid{},
				VolumeGroups:   []metal.VolumeGroup{},
				LogicalVolumes: metal.LogicalVolumes{},
				Constraints: metal.FilesystemLayoutConstraints{
					Sizes:  []string{},
					Images: map[string]string{},
				},
			},
			MachineNetworks: []*metal.MachineNetwork{},
			Hostname:        "",
			SSHPubKeys:      []string{},
			UserData:        "",
			ConsolePassword: "",
			Succeeded:       false,
			Reinstall:       false,
			MachineSetup:    &metal.MachineSetup{},
			Role:            "",
			VPN:             &metal.MachineVPN{},
			UUID:            "",
			FirewallRules: &metal.FirewallRules{
				Egress:  []metal.EgressRule{},
				Ingress: []metal.IngressRule{},
			},
			DNSServers: metal.DNSServers{},
			NTPServers: metal.NTPServers{},
		},
		PartitionID:  "",
		SizeID:       "",
		RackID:       "",
		Waiting:      false,
		PreAllocated: false,
		Hardware: metal.MachineHardware{
			Memory:    0,
			Nics:      metal.Nics{},
			Disks:     []metal.BlockDevice{},
			MetalCPUs: []metal.MetalCPU{},
			MetalGPUs: []metal.MetalGPU{},
		},
		State:    metal.MachineState{},
		LEDState: metal.ChassisIdentifyLEDState{},
		Tags:     []string{},
		IPMI: metal.IPMI{
			Address:       "",
			MacAddress:    "",
			User:          "",
			Password:      "",
			Interface:     "",
			Fru:           metal.Fru{},
			BMCVersion:    "",
			PowerState:    "",
			PowerMetric:   &metal.PowerMetric{},
			PowerSupplies: metal.PowerSupplies{},
			LastUpdated:   time.Time{},
		},
		BIOS: metal.BIOS{},
	}
	machines = []*metal.Machine{m1, m2, m3}
)

func TestMachineFilter(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	ds, _, rethinkCloser := test.StartRethink(t, log)
	defer func() {
		rethinkCloser()
	}()

	ctx := t.Context()

	for _, m := range machines {
		createdMachine, err := ds.Machine().Create(ctx, m)
		require.NoError(t, err)
		require.NotNil(t, createdMachine)
		require.Equal(t, m.ID, createdMachine.ID)
	}

	tests := []struct {
		name string
		rq   *apiv2.MachineQuery
		want []*metal.Machine
	}{
		{
			name: "empty request returns unfiltered",
			rq:   nil,
			want: []*metal.Machine{m1, m2, m3},
		},
		{
			name: "by id",
			rq:   &apiv2.MachineQuery{Uuid: &m1.ID},
			want: []*metal.Machine{m1},
		},
		{
			name: "by id 2",
			rq:   &apiv2.MachineQuery{Uuid: &m2.ID},
			want: []*metal.Machine{m2},
		},
		{
			name: "by name",
			rq:   &apiv2.MachineQuery{Name: &m1.Name},
			want: []*metal.Machine{m1},
		},
		{
			name: "by label",
			rq:   &apiv2.MachineQuery{Labels: &apiv2.Labels{Labels: map[string]string{"color": "red"}}},
			want: []*metal.Machine{m1},
		},
		{
			name: "by label 2",
			rq:   &apiv2.MachineQuery{Labels: &apiv2.Labels{Labels: map[string]string{"size": "medium"}}},
			want: []*metal.Machine{m2},
		},
		{
			name: "by project",
			rq:   &apiv2.MachineQuery{Allocation: &apiv2.MachineAllocationQuery{Project: pointer.Pointer("p1")}},
			want: []*metal.Machine{m1},
		},
		{
			name: "by partition",
			rq:   &apiv2.MachineQuery{Partition: &m1.PartitionID},
			want: []*metal.Machine{m1},
		},
		{
			name: "by role",
			rq:   &apiv2.MachineQuery{Allocation: &apiv2.MachineAllocationQuery{AllocationType: apiv2.MachineAllocationType_MACHINE_ALLOCATION_TYPE_MACHINE.Enum()}},
			want: []*metal.Machine{m1},
		},
		{
			name: "by role",
			rq:   &apiv2.MachineQuery{Allocation: &apiv2.MachineAllocationQuery{AllocationType: apiv2.MachineAllocationType_MACHINE_ALLOCATION_TYPE_FIREWALL.Enum()}},
			want: []*metal.Machine{m2},
		},
		{
			name: "by image",
			rq:   &apiv2.MachineQuery{Allocation: &apiv2.MachineAllocationQuery{Image: pointer.Pointer("debian-12")}},
			want: []*metal.Machine{m1},
		},
		{
			name: "by size",
			rq:   &apiv2.MachineQuery{Size: pointer.Pointer("n1-medium")},
			want: []*metal.Machine{m2},
		},

		// FIXME tests for all query parameters
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ds.Machine().List(ctx, queries.MachineFilter(tt.rq))
			require.NoError(t, err)

			slices.SortFunc(got, func(a, b *metal.Machine) int {
				return strings.Compare(a.ID, b.ID)
			})

			fmt.Print(got)
			if diff := cmp.Diff(
				tt.want, got,
				cmpopts.IgnoreFields(
					metal.Machine{}, "Created", "Changed",
				),
			); diff != "" {
				t.Errorf("machineServiceServer.List() = %v, want %vņdiff: %s", got, tt.want, diff)
			}

		})
	}
}
