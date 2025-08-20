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
				Base: metal.Base{
					ID: "c1-medium-fsl",
				},
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
				Base: metal.Base{
					ID: "n1-medium-fsl",
				},
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
			MachineNetworks: []*metal.MachineNetwork{
				{
					NetworkID:           "internet",
					Prefixes:            []string{"1.2.3.0/24", "2.3.4.0/24"},
					IPs:                 []string{"1.2.3.4", "2.3.4.5"},
					DestinationPrefixes: []string{"0.0.0.0/0"},
					Vrf:                 104009,
					ASN:                 4009,
				},
			},
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
				Base: metal.Base{
					ID: "c1-large-fsl",
				},
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
		SizeID:       "c1-large-x86",
		RackID:       "",
		Waiting:      false,
		PreAllocated: false,
		Hardware: metal.MachineHardware{
			Memory: 2048,
			Nics: metal.Nics{
				{
					MacAddress: "aa:bb",
					Name:       "eth0",
					Vrf:        "vrf104009",
					Neighbors: metal.Nics{
						{
							MacAddress: "cc:dd",
							Name:       "swp1",
							Vrf:        "vrf104009",
							Neighbors:  metal.Nics{},
						},
					},
				},
			},
			Disks: []metal.BlockDevice{
				{
					Name: "/dev/sda",
					Size: 4096,
				},
			},
			MetalCPUs: []metal.MetalCPU{
				{Cores: 4},
				{Cores: 6},
			},
			MetalGPUs: []metal.MetalGPU{},
		},
		State: metal.MachineState{
			Value: metal.LockedState,
		},
		LEDState: metal.ChassisIdentifyLEDState{},
		Tags:     []string{},
		IPMI: metal.IPMI{
			Address:    "192.168.0.1",
			MacAddress: "ee:ff",
			User:       "admin",
			Password:   "",
			Interface:  "eth1",
			Fru: metal.Fru{
				ChassisPartNumber:   "chass-1",
				ChassisPartSerial:   "chass-serial-1",
				BoardMfg:            "board-mfg-1",
				BoardMfgSerial:      "board-serial-1",
				BoardPartNumber:     "board-1",
				ProductManufacturer: "vendor-a",
				ProductPartNumber:   "vendor-a-1",
				ProductSerial:       "vendor-serial-1",
			},
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
			name: "by partition",
			rq:   &apiv2.MachineQuery{Partition: &m1.PartitionID},
			want: []*metal.Machine{m1},
		},
		{
			name: "by size",
			rq:   &apiv2.MachineQuery{Size: pointer.Pointer("n1-medium")},
			want: []*metal.Machine{m2},
		},
		{
			name: "by rack",
			rq:   &apiv2.MachineQuery{Rack: pointer.Pointer("rack-2")},
			want: []*metal.Machine{m2},
		},
		{
			name: "by state",
			rq:   &apiv2.MachineQuery{State: apiv2.MachineState_MACHINE_STATE_LOCKED.Enum()},
			want: []*metal.Machine{m3},
		},
		// Allocation Queries
		{
			name: "by allocation name",
			rq:   &apiv2.MachineQuery{Allocation: &apiv2.MachineAllocationQuery{Name: pointer.Pointer("shoot-worker-1")}},
			want: []*metal.Machine{m1},
		},
		{
			name: "by allocation hostname",
			rq:   &apiv2.MachineQuery{Allocation: &apiv2.MachineAllocationQuery{Hostname: pointer.Pointer("shoot-worker-1")}},
			want: []*metal.Machine{m1},
		},
		{
			name: "by project",
			rq:   &apiv2.MachineQuery{Allocation: &apiv2.MachineAllocationQuery{Project: pointer.Pointer("p1")}},
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
			name: "by fsl",
			rq:   &apiv2.MachineQuery{Allocation: &apiv2.MachineAllocationQuery{FilesystemLayout: pointer.Pointer("n1-medium-fsl")}},
			want: []*metal.Machine{m2},
		},

		// Network Queries
		{
			name: "by network id",
			rq:   &apiv2.MachineQuery{Network: &apiv2.MachineNetworkQuery{Networks: []string{"internet"}}},
			want: []*metal.Machine{m2},
		},
		{
			name: "by network prefixes",
			rq:   &apiv2.MachineQuery{Network: &apiv2.MachineNetworkQuery{Prefixes: []string{"1.2.3.0/24"}}},
			want: []*metal.Machine{m2},
		},
		{
			name: "by network destinationprefixes",
			rq:   &apiv2.MachineQuery{Network: &apiv2.MachineNetworkQuery{DestinationPrefixes: []string{"0.0.0.0/0"}}},
			want: []*metal.Machine{m2},
		},
		{
			name: "by network ips",
			rq:   &apiv2.MachineQuery{Network: &apiv2.MachineNetworkQuery{Ips: []string{"1.2.3.4"}}},
			want: []*metal.Machine{m2},
		},
		{
			name: "by network vrf",
			rq:   &apiv2.MachineQuery{Network: &apiv2.MachineNetworkQuery{Vrfs: []uint64{104009}}},
			want: []*metal.Machine{m2},
		},
		{
			name: "by network asn",
			rq:   &apiv2.MachineQuery{Network: &apiv2.MachineNetworkQuery{Asns: []uint32{4009}}},
			want: []*metal.Machine{m2},
		},

		// Hardware Queries
		{
			name: "by hardware memory",
			rq:   &apiv2.MachineQuery{Hardware: &apiv2.MachineHardwareQuery{Memory: pointer.Pointer(uint64(2048))}},
			want: []*metal.Machine{m3},
		},
		{
			name: "by hardware cpus",
			rq:   &apiv2.MachineQuery{Hardware: &apiv2.MachineHardwareQuery{CpuCores: pointer.Pointer(uint32(10))}},
			want: []*metal.Machine{m3},
		},

		// Nic Queries
		{
			name: "by nic mac",
			rq:   &apiv2.MachineQuery{Nic: &apiv2.MachineNicQuery{Macs: []string{"aa:bb"}}},
			want: []*metal.Machine{m3},
		},
		{
			name: "by nic name",
			rq:   &apiv2.MachineQuery{Nic: &apiv2.MachineNicQuery{Names: []string{"eth0"}}},
			want: []*metal.Machine{m3},
		},
		// FIXME is a string in the backend and not populated
		// {
		// 	name: "by nic vrf",
		// 	rq:   &apiv2.MachineQuery{Nic: &apiv2.MachineNicQuery{Vrfs: []uint64{4009}}},
		// 	want: []*metal.Machine{m3},
		// },
		{
			name: "by nic neighbor mac",
			rq:   &apiv2.MachineQuery{Nic: &apiv2.MachineNicQuery{NeighborMacs: []string{"cc:dd"}}},
			want: []*metal.Machine{m3},
		},
		{
			name: "by nic neighbor name",
			rq:   &apiv2.MachineQuery{Nic: &apiv2.MachineNicQuery{NeighborNames: []string{"swp1"}}},
			want: []*metal.Machine{m3},
		},
		// FIXME is a string in the backend and not populated
		// {
		// 	name: "by nic neighbor vrf",
		// 	rq:   &apiv2.MachineQuery{Nic: &apiv2.MachineNicQuery{NeighborVrfs: []uint64{4009}}},
		// 	want: []*metal.Machine{m3},
		// },

		// Disk Queries
		{
			name: "by disk name",
			rq:   &apiv2.MachineQuery{Disk: &apiv2.MachineDiskQuery{Names: []string{"/dev/sda"}}},
			want: []*metal.Machine{m3},
		},
		{
			name: "by disk size",
			rq:   &apiv2.MachineQuery{Disk: &apiv2.MachineDiskQuery{Sizes: []uint64{4096}}},
			want: []*metal.Machine{m3},
		},

		// IPMI Queries
		{
			name: "by ipmi address",
			rq:   &apiv2.MachineQuery{Ipmi: &apiv2.MachineIPMIQuery{Address: pointer.Pointer("192.168.0.1")}},
			want: []*metal.Machine{m3},
		},
		{
			name: "by ipmi mac",
			rq:   &apiv2.MachineQuery{Ipmi: &apiv2.MachineIPMIQuery{Mac: pointer.Pointer("ee:ff")}},
			want: []*metal.Machine{m3},
		},
		{
			name: "by ipmi user",
			rq:   &apiv2.MachineQuery{Ipmi: &apiv2.MachineIPMIQuery{User: pointer.Pointer("admin")}},
			want: []*metal.Machine{m3},
		},
		{
			name: "by ipmi interface",
			rq:   &apiv2.MachineQuery{Ipmi: &apiv2.MachineIPMIQuery{Interface: pointer.Pointer("eth1")}},
			want: []*metal.Machine{m3},
		},

		// FRU Queries
		{
			name: "by fru chassispartnumber",
			rq:   &apiv2.MachineQuery{Fru: &apiv2.MachineFRUQuery{ChassisPartNumber: pointer.Pointer("chass-1")}},
			want: []*metal.Machine{m3},
		},
		{
			name: "by fru chassispartserial",
			rq:   &apiv2.MachineQuery{Fru: &apiv2.MachineFRUQuery{ChassisPartSerial: pointer.Pointer("chass-serial-1")}},
			want: []*metal.Machine{m3},
		},
		{
			name: "by fru boardmfg",
			rq:   &apiv2.MachineQuery{Fru: &apiv2.MachineFRUQuery{BoardMfg: pointer.Pointer("board-mfg-1")}},
			want: []*metal.Machine{m3},
		},
		{
			name: "by fru boardserial",
			rq:   &apiv2.MachineQuery{Fru: &apiv2.MachineFRUQuery{BoardSerial: pointer.Pointer("board-serial-1")}},
			want: []*metal.Machine{m3},
		},
		{
			name: "by fru boardpartnumber",
			rq:   &apiv2.MachineQuery{Fru: &apiv2.MachineFRUQuery{BoardPartNumber: pointer.Pointer("board-1")}},
			want: []*metal.Machine{m3},
		},
		{
			name: "by fru productmanufacturer",
			rq:   &apiv2.MachineQuery{Fru: &apiv2.MachineFRUQuery{ProductManufacturer: pointer.Pointer("vendor-a")}},
			want: []*metal.Machine{m3},
		},
		{
			name: "by fru productpartnumber",
			rq:   &apiv2.MachineQuery{Fru: &apiv2.MachineFRUQuery{ProductPartNumber: pointer.Pointer("vendor-a-1")}},
			want: []*metal.Machine{m3},
		},
		{
			name: "by fru productserial",
			rq:   &apiv2.MachineQuery{Fru: &apiv2.MachineFRUQuery{ProductSerial: pointer.Pointer("vendor-serial-1")}},
			want: []*metal.Machine{m3},
		},
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
