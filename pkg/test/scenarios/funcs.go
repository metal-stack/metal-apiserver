package scenarios

import (
	"fmt"
	"time"

	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/samber/lo"
)

var (
	SwitchFunc = func(id, partition, rack string, ports []string, os *apiv2.SwitchOS, replaceMode apiv2.SwitchReplaceMode, machines ...string) *apiv2.Switch {
		var (
			nics []*apiv2.SwitchNic
			cons []*apiv2.MachineConnection
		)

		for i, p := range ports {
			nic := &apiv2.SwitchNic{
				Name:       p,
				Identifier: p,
				State: &apiv2.NicState{
					Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
				},
			}
			nics = append(nics, nic)
			if i < len(machines) {
				cons = append(cons, &apiv2.MachineConnection{
					MachineId: machines[i],
					Nic:       nic,
				})
			}
		}

		return &apiv2.Switch{
			Id:                 id,
			Rack:               new(rack),
			Partition:          partition,
			Nics:               nics,
			Os:                 os,
			ReplaceMode:        replaceMode,
			MachineConnections: cons,
		}
	}

	MachineFunc = func(id, partition, size, project, image string, liveliness metal.MachineLiveliness, waiting bool) *MachineWithLiveliness {
		machineNumber := lo.Substring(id, -1, 1)
		m := &metal.Machine{
			Base:        metal.Base{ID: id},
			PartitionID: partition,
			SizeID:      size,
			IPMI: metal.IPMI{ // required for healthy machine state
				Address:     fmt.Sprintf("1.2.3.%s:623", machineNumber),
				MacAddress:  "aa:bb:0" + machineNumber,
				LastUpdated: time.Now().Add(-1 * time.Minute),
				Fru: metal.Fru{
					ProductSerial: "PS" + machineNumber,
				},
			},
			State: metal.MachineState{
				Value: metal.AvailableState,
			},
			Waiting: waiting,
		}
		if project != "" && image != "" {
			m.Allocation = &metal.MachineAllocation{
				Project: project,
				ImageID: image,
			}
		}
		return &MachineWithLiveliness{
			Liveliness: liveliness,
			Machine:    m,
		}
	}
)
