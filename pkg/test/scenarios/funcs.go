package scenarios

import (
	"fmt"

	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
)

var (
	switchPairFunc = func(partition, rack string, ports int) []*apiv2.Switch {
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
)
