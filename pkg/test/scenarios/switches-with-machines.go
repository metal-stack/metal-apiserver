package scenarios

import (
	"time"

	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-apiserver/pkg/repository/api"
	"google.golang.org/protobuf/types/known/durationpb"
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
		SwitchStatuses: []*api.SwitchStatus{
			{
				ID: P01Rack01Switch1,
				LastSync: &apiv2.SwitchSync{
					Duration: &durationpb.Duration{},
				},
				LastSyncError: &apiv2.SwitchSync{
					Error: new("previous error"),
				},
			},
			{
				ID:       P01Rack02Switch1,
				LastSync: &apiv2.SwitchSync{},
				LastSyncError: &apiv2.SwitchSync{
					Duration: durationpb.New(time.Second),
					Error:    new("sync failed"),
				},
			},
			{
				ID: P02Rack01Switch2,
				LastSync: &apiv2.SwitchSync{
					Duration: durationpb.New(time.Second),
				},
			},
		},
		Switches: []*apiv2.Switch{
			SwitchFunc(P01Rack01Switch1, Partition1, P01Rack01, []string{"Ethernet0", "Ethernet1"}, SwitchOSSonic2021, apiv2.SwitchReplaceMode_SWITCH_REPLACE_MODE_OPERATIONAL, Machine1),
			SwitchFunc(P01Rack01Switch2, Partition1, P01Rack01, []string{"Ethernet0", "Ethernet1"}, SwitchOSSonic2021, apiv2.SwitchReplaceMode_SWITCH_REPLACE_MODE_OPERATIONAL, Machine1),
			SwitchFunc(P01Rack01Switch2_1, Partition1, P01Rack01, []string{"Ethernet0", "Ethernet1"}, SwitchOSSonic2021, apiv2.SwitchReplaceMode_SWITCH_REPLACE_MODE_REPLACE),

			SwitchFunc(P01Rack02Switch1, Partition1, P01Rack02, []string{"swp1s0", "swp1s1"}, SwitchOSCumulus, apiv2.SwitchReplaceMode_SWITCH_REPLACE_MODE_OPERATIONAL, Machine2),
			SwitchFunc(P01Rack02Switch1_1, Partition1, P01Rack02, []string{"Ethernet0", "Ethernet1"}, SwitchOSSonic2021, apiv2.SwitchReplaceMode_SWITCH_REPLACE_MODE_OPERATIONAL),
			SwitchFunc(P01Rack02Switch2, Partition1, P01Rack02, []string{"Ethernet0", "Ethernet1"}, SwitchOSSonic2021, apiv2.SwitchReplaceMode_SWITCH_REPLACE_MODE_OPERATIONAL, Machine2),

			SwitchFunc(P01Rack03Switch1, Partition1, P01Rack03, []string{"Ethernet0", "Ethernet1"}, SwitchOSSonic2022, apiv2.SwitchReplaceMode_SWITCH_REPLACE_MODE_OPERATIONAL),
			SwitchFunc(P01Rack03Switch2, Partition1, P01Rack03, []string{"swp1s0", "swp1s1"}, SwitchOSCumulus, apiv2.SwitchReplaceMode_SWITCH_REPLACE_MODE_OPERATIONAL),

			SwitchFunc(P01Rack04Switch1, Partition1, P01Rack04, []string{"Ethernet0"}, SwitchOSSonic2021, apiv2.SwitchReplaceMode_SWITCH_REPLACE_MODE_REPLACE),

			SwitchFunc(P02Rack01Switch1, Partition2, P02Rack01, []string{"Ethernet0", "Ethernet1"}, SwitchOSSonic2021, apiv2.SwitchReplaceMode_SWITCH_REPLACE_MODE_OPERATIONAL, Machine3),
			SwitchFunc(P02Rack01Switch2, Partition2, P02Rack01, []string{"Ethernet0", "Ethernet1"}, SwitchOSSonic2021, apiv2.SwitchReplaceMode_SWITCH_REPLACE_MODE_OPERATIONAL, Machine3),
			SwitchFunc(P02Rack01Switch2_1, Partition2, P02Rack01, []string{"swp1s0", "swp1s1"}, SwitchOSCumulus, apiv2.SwitchReplaceMode_SWITCH_REPLACE_MODE_OPERATIONAL),

			SwitchFunc(P02Rack02Switch1, Partition2, P02Rack02, []string{"Ethernet0", "Ethernet1"}, SwitchOSSonic2021, apiv2.SwitchReplaceMode_SWITCH_REPLACE_MODE_OPERATIONAL, Machine4, Machine5),
			SwitchFunc(P02Rack02Switch2, Partition2, P02Rack02, []string{"Ethernet0", "Ethernet1"}, SwitchOSSonic2021, apiv2.SwitchReplaceMode_SWITCH_REPLACE_MODE_OPERATIONAL, Machine4, Machine5),
			SwitchFunc(P02Rack02Switch2_1, Partition2, P02Rack02, []string{"Ethernet0", "Ethernet1"}, SwitchOSSonic2021, apiv2.SwitchReplaceMode_SWITCH_REPLACE_MODE_OPERATIONAL, Machine6),

			SwitchFunc(P02Rack03Switch1, Partition2, P02Rack03, []string{"Ethernet0", "Ethernet1"}, SwitchOSSonic2021, apiv2.SwitchReplaceMode_SWITCH_REPLACE_MODE_OPERATIONAL, Machine7),
			SwitchFunc(P02Rack03Switch2, Partition2, P02Rack03, []string{"swp1s0", "swp1s1"}, SwitchOSCumulus, apiv2.SwitchReplaceMode_SWITCH_REPLACE_MODE_REPLACE, Machine7),
		},
		Machines: []*MachineWithLiveliness{
			MachineFunc(Machine1, Partition1, SizeC1Large, "", "", metal.MachineLivelinessAlive, false),
			MachineFunc(Machine2, Partition1, SizeC1Large, "", "", metal.MachineLivelinessAlive, false),
			MachineFunc(Machine3, Partition2, SizeC1Large, "", "", metal.MachineLivelinessAlive, false),
			MachineFunc(Machine4, Partition2, SizeC1Large, "", "", metal.MachineLivelinessAlive, false),
			MachineFunc(Machine5, Partition2, SizeC1Large, "", "", metal.MachineLivelinessAlive, false),
			MachineFunc(Machine6, Partition2, SizeC1Large, "", "", metal.MachineLivelinessAlive, false),
			MachineFunc(Machine7, Partition2, SizeC1Large, "", "", metal.MachineLivelinessAlive, false),
			MachineFunc(Machine8, Partition2, SizeC1Large, "", "", metal.MachineLivelinessAlive, false),
		},
	}
)
