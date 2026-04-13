package scenarios

import (
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
)

var (
	SwitchesWithMachinesDatacenter = DatacenterSpec{
		Partitions:        []string{Partition1, Partition2},
		Tenants:           []string{Tenant1},
		ProjectsPerTenant: 1,
		Sizes: []*apiv2.Size{
			{
				Id:   SizeC1Large,
				Name: new(SizeC1Large),
				Constraints: []*apiv2.SizeConstraint{
					{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_CORES, Min: 4, Max: 4},
					{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_MEMORY, Min: 1024, Max: 1024},
					{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_STORAGE, Min: 1024, Max: 1024},
				},
			},
		},
		Switches: []*apiv2.Switch{
			SwitchFunc(P01Rack01Switch1, Partition1, P01Rack01, []string{"Ethernet0", "Ethernet1"}, SwitchOSSonic2021, Machine1),
			SwitchFunc(P01Rack01Switch2, Partition1, P01Rack01, []string{"Ethernet0", "Ethernet1"}, SwitchOSSonic2021, Machine1),

			SwitchFunc(P01Rack02Switch1, Partition1, P01Rack02, []string{"swp1s0", "swp1s1"}, SwitchOSCumulus, Machine2),
			SwitchFunc(P01Rack02Switch1_1, Partition1, P01Rack02, []string{"Ethernet0", "Ethernet1"}, SwitchOSSonic2021),
			SwitchFunc(P01Rack02Switch2, Partition1, P01Rack02, []string{"Ethernet0", "Ethernet1"}, SwitchOSSonic2021, Machine2),

			SwitchFunc(P01Rack03Switch1, Partition1, P01Rack03, []string{"Ethernet0", "Ethernet1"}, SwitchOSSonic2022),
			SwitchFunc(P01Rack03Switch2, Partition1, P01Rack03, []string{"swp1s0", "swp1s1"}, SwitchOSCumulus),

			SwitchFunc(P02Rack01Switch1, Partition2, P02Rack01, []string{"Ethernet0", "Ethernet1"}, SwitchOSSonic2021, Machine3),
			SwitchFunc(P02Rack01Switch2, Partition2, P02Rack01, []string{"Ethernet0", "Ethernet1"}, SwitchOSSonic2021, Machine3),
			SwitchFunc(P02Rack01Switch2_1, Partition2, P02Rack01, []string{"swp1s0", "swp1s1"}, SwitchOSCumulus),

			SwitchFunc(P02Rack02Switch1, Partition2, P02Rack02, []string{"Ethernet0", "Ethernet1"}, SwitchOSSonic2021, Machine4, Machine5),
			SwitchFunc(P02Rack02Switch2, Partition2, P02Rack02, []string{"Ethernet0", "Ethernet1"}, SwitchOSSonic2021, Machine4, Machine5),
			SwitchFunc(P02Rack02Switch2_1, Partition2, P02Rack02, []string{"Ethernet0", "Ethernet1"}, SwitchOSSonic2021, Machine6),
		},
		Machines: []*MachineWithLiveliness{
			MachineFunc(Machine1, Partition1, SizeC1Large, "", "", metal.MachineLivelinessAlive),
			MachineFunc(Machine2, Partition1, SizeC1Large, "", "", metal.MachineLivelinessAlive),
			MachineFunc(Machine3, Partition2, SizeC1Large, "", "", metal.MachineLivelinessAlive),
			MachineFunc(Machine4, Partition2, SizeC1Large, "", "", metal.MachineLivelinessAlive),
			MachineFunc(Machine5, Partition2, SizeC1Large, "", "", metal.MachineLivelinessAlive),
			MachineFunc(Machine6, Partition2, SizeC1Large, "", "", metal.MachineLivelinessAlive),
		},
	}
)
