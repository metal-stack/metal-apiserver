package scenarios

import (
	"slices"

	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
)

const (
	SwmTenant = "jane.doe"
	SwmSize   = "c1-large-x86"

	SwmPartition1 = "partition-1"
	SwmPartition2 = "partition-2"

	SwmRack1 = "rack-1"
	SwmRack2 = "rack-2"
	SwmRack3 = "rack-3"

	SwmMachine1 = "00000000-0000-0000-0000-000000000001"
	SwmMachine2 = "00000000-0000-0000-0000-000000000002"
	SwmMachine3 = "00000000-0000-0000-0000-000000000003"
	SwmMachine4 = "00000000-0000-0000-0000-000000000004"
	SwmMachine5 = "00000000-0000-0000-0000-000000000005"
)

var (
	SwitchesWithMachinesDatacenter = DatacenterSpec{
		Partitions:        []string{SwmPartition1, SwmPartition2},
		Tenants:           []string{SwmTenant},
		ProjectsPerTenant: 1,
		Sizes: []*apiv2.Size{
			{
				Id:   SwmSize,
				Name: new(SwmSize),
				Constraints: []*apiv2.SizeConstraint{
					{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_CORES, Min: 4, Max: 4},
					{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_MEMORY, Min: 1024, Max: 1024},
					{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_STORAGE, Min: 1024, Max: 1024},
				},
			},
		},
		Switches: slices.Concat(
			SwitchPairFunc(SwmPartition1, SwmRack1, 2, SwmMachine1),
			SwitchPairFunc(SwmPartition1, SwmRack2, 2, SwmMachine2),
			SwitchPairFunc(SwmPartition2, SwmRack1, 2, SwmMachine3),
			SwitchPairFunc(SwmPartition2, SwmRack2, 2, SwmMachine4, SwmMachine5),
			SwitchPairFunc(SwmPartition1, SwmRack3, 2),
		),
		Machines: []*MachineWithLiveliness{
			MachineFunc(SwmMachine1, SwmPartition1, SwmSize, "", "", metal.MachineLivelinessAlive),
			MachineFunc(SwmMachine2, SwmPartition1, SwmSize, "", "", metal.MachineLivelinessAlive),
			MachineFunc(SwmMachine3, SwmPartition2, SwmSize, "", "", metal.MachineLivelinessAlive),
			MachineFunc(SwmMachine4, SwmPartition2, SwmSize, "", "", metal.MachineLivelinessAlive),
			MachineFunc(SwmMachine5, SwmPartition2, SwmSize, "", "", metal.MachineLivelinessAlive),
		},
	}
)
