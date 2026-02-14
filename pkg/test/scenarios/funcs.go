package scenarios

import (
	"fmt"
	"time"

	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
)

var (
	SwitchPairFunc = func(partition, rack string, ports int) []*apiv2.Switch {
		return []*apiv2.Switch{
			{
				Id:          fmt.Sprintf("sw1-%s-%s", partition, rack),
				Meta:        &apiv2.Meta{},
				Partition:   partition,
				Rack:        new(rack),
				ReplaceMode: apiv2.SwitchReplaceMode_SWITCH_REPLACE_MODE_OPERATIONAL,
				Nics:        switchNicsFunc(ports),
				Os: &apiv2.SwitchOS{
					Vendor: apiv2.SwitchOSVendor_SWITCH_OS_VENDOR_SONIC,
				},
			},
			{
				Id:          fmt.Sprintf("sw2-%s-%s", partition, rack),
				Meta:        &apiv2.Meta{},
				Partition:   partition,
				Rack:        new(rack),
				ReplaceMode: apiv2.SwitchReplaceMode_SWITCH_REPLACE_MODE_OPERATIONAL,
				Nics:        switchNicsFunc(ports),
				Os: &apiv2.SwitchOS{
					Vendor: apiv2.SwitchOSVendor_SWITCH_OS_VENDOR_SONIC,
				},
			},
		}
	}

	switchNicsFunc = func(ports int) []*apiv2.SwitchNic {
		var nics []*apiv2.SwitchNic
		for i := range ports {
			nics = append(nics, &apiv2.SwitchNic{
				Name:       fmt.Sprintf("Ethernet%d", i),
				Identifier: fmt.Sprintf("Eth%d/%d", i+1, i+1), // TODO configure breakout
				State: &apiv2.NicState{
					Actual: apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP,
				},
			})
		}
		return nics
	}

	MachineFunc = func(id, partition, size, project string, liveliness metal.MachineLiveliness) *MachineWithLiveliness[metal.MachineLiveliness, *metal.Machine] {
		m := &metal.Machine{
			Base:        metal.Base{ID: id},
			PartitionID: partition,
			SizeID:      size,
			IPMI: metal.IPMI{ // required for healthy machine state
				Address:     "1.2.3." + id,
				MacAddress:  "aa:bb:0" + id,
				LastUpdated: time.Now().Add(-1 * time.Minute),
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
		return &MachineWithLiveliness[metal.MachineLiveliness, *metal.Machine]{
			Liveliness: liveliness,
			Machine:    m,
		}
	}
)
