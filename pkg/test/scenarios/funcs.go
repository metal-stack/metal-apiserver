package scenarios

import (
	"fmt"
	"time"

	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/samber/lo"
)

var (
	SwitchPairFunc = func(partition, rack string, ports int, machines ...string) []*apiv2.Switch {
		nics, cons := switchNicsFunc(ports, machines)

		return []*apiv2.Switch{
			{
				Id:                 fmt.Sprintf("sw1-%s-%s", partition, rack),
				Meta:               &apiv2.Meta{},
				Partition:          partition,
				Rack:               new(rack),
				ReplaceMode:        apiv2.SwitchReplaceMode_SWITCH_REPLACE_MODE_OPERATIONAL,
				Nics:               nics,
				MachineConnections: cons,
				Os: &apiv2.SwitchOS{
					Vendor: apiv2.SwitchOSVendor_SWITCH_OS_VENDOR_SONIC,
				},
			},
			{
				Id:                 fmt.Sprintf("sw2-%s-%s", partition, rack),
				Meta:               &apiv2.Meta{},
				Partition:          partition,
				Rack:               new(rack),
				ReplaceMode:        apiv2.SwitchReplaceMode_SWITCH_REPLACE_MODE_OPERATIONAL,
				Nics:               nics,
				MachineConnections: cons,
				Os: &apiv2.SwitchOS{
					Vendor: apiv2.SwitchOSVendor_SWITCH_OS_VENDOR_SONIC,
				},
			},
		}
	}

	switchNicsFunc = func(ports int, machines []string) ([]*apiv2.SwitchNic, []*apiv2.MachineConnection) {
		var (
			nics []*apiv2.SwitchNic
			cons []*apiv2.MachineConnection
		)
		for i := range ports {
			nic := &apiv2.SwitchNic{
				Name:       fmt.Sprintf("Ethernet%d", i),
				Identifier: fmt.Sprintf("Eth%d/%d", i+1, i+1), // TODO configure breakout
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
		return nics, cons
	}

	MachineFunc = func(id, partition, size, project string, liveliness metal.MachineLiveliness) *MachineWithLiveliness {
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
		}
		if project != "" {
			m.Allocation = &metal.MachineAllocation{
				Project: project,
			}
		}
		return &MachineWithLiveliness{
			Liveliness: liveliness,
			Machine:    m,
		}
	}
)
