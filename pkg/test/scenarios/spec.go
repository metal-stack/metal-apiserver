package scenarios

import (
	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
)

const (
	Tenant1 = "john.doe"
	// Project UUIDs are generated be counting the first digit for every tenant, last digit for every project of this tenant
	Tenant1Project1 = "10000000-0000-0000-0000-000000000001"
	Tenant1Project2 = "10000000-0000-0000-0000-000000000002"

	ImageDebian13    = "debian-13.0.20260131"
	ImageDebian12    = "debian-12.0.20251220"
	ImageDebian11    = "debian-11.0.20241220"
	ImageFirewall3_0 = "firewall-ubuntu-3.0.20260201"

	SizeN1Medium = "n1-medium-x86"
	SizeC1Large  = "c1-large-x86"

	Partition1 = "partition-1"
	Partition2 = "partition-2"

	NetworkInternet              = "internet"
	NetworkUnderlayPartition1    = "underlay-partition-1"
	NetworkTenantSuperNamespaced = "tenant-super-namespaced"
	NetworkTenantSuperPartition1 = "tenant-super-partition-1"

	P01Rack01 = "p01-rack01"
	P01Rack02 = "p01-rack02"
	P01Rack03 = "p01-rack03"
	P01Rack04 = "p01-rack04"
	P02Rack01 = "p02-rack01"
	P02Rack02 = "p02-rack02"
	P02Rack03 = "p02-rack03"

	Machine1 = "00000000-0000-0000-0000-000000000001"
	Machine2 = "00000000-0000-0000-0000-000000000002"
	Machine3 = "00000000-0000-0000-0000-000000000003"
	Machine4 = "00000000-0000-0000-0000-000000000004"
	Machine5 = "00000000-0000-0000-0000-000000000005"
	Machine6 = "00000000-0000-0000-0000-000000000006"
	Machine7 = "00000000-0000-0000-0000-000000000007"
	Machine8 = "00000000-0000-0000-0000-000000000008"

	P01Rack01Switch1   = "p01-r01leaf01"
	P01Rack01Switch2   = "p01-r01leaf02"
	P01Rack01Switch2_1 = "p01-r01leaf02-1"
	P01Rack02Switch1   = "p01-r02leaf01"
	P01Rack02Switch1_1 = "p01-r02leaf01-1"
	P01Rack02Switch2   = "p01-r02leaf02"
	P01Rack03Switch1   = "p01-r03leaf01"
	P01Rack03Switch2   = "p01-r03leaf02"
	P01Rack04Switch1   = "p01-r04leaf01"
	P02Rack01Switch1   = "p02-r01leaf01"
	P02Rack01Switch2   = "p02-r01leaf02"
	P02Rack01Switch2_1 = "p02-r01leaf02-1"
	P02Rack02Switch1   = "p02-r02leaf01"
	P02Rack02Switch2   = "p02-r02leaf02"
	P02Rack02Switch2_1 = "p02-r02leaf02-1"
	P02Rack03Switch1   = "p02-r03leaf01"
	P02Rack03Switch2   = "p02-r03leaf02"
)

var (
	SwitchOSSonic2021 = &apiv2.SwitchOS{
		Vendor:  apiv2.SwitchOSVendor_SWITCH_OS_VENDOR_SONIC,
		Version: "2021",
	}

	SwitchOSSonic2022 = &apiv2.SwitchOS{
		Vendor:  apiv2.SwitchOSVendor_SWITCH_OS_VENDOR_SONIC,
		Version: "2022",
	}

	SwitchOSCumulus = &apiv2.SwitchOS{
		Vendor:  apiv2.SwitchOSVendor_SWITCH_OS_VENDOR_CUMULUS,
		Version: "v9.0.0",
	}
)

type (
	MachineWithLiveliness struct {
		Liveliness metal.MachineLiveliness
		Machine    *metal.Machine
	}

	DatacenterSpec struct {
		Partitions           []string
		Tenants              []string
		ProjectsPerTenant    int
		Images               map[string]apiv2.ImageFeature
		FilesystemLayouts    []*adminv2.FilesystemServiceCreateRequest
		Sizes                []*apiv2.Size
		SizeReservations     []*adminv2.SizeReservationServiceCreateRequest
		SizeImageConstraints []*adminv2.SizeImageConstraintServiceCreateRequest
		Networks             []*adminv2.NetworkServiceCreateRequest
		IPs                  []*apiv2.IPServiceCreateRequest
		Switches             []*apiv2.Switch
		Machines             []*MachineWithLiveliness
		ReservedMachines     []string // TODO
	}
)
