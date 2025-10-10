package metal

import (
	"fmt"
	"slices"
	"strings"

	"github.com/metal-stack/api/go/enum"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	"github.com/metal-stack/metal-lib/pkg/pointer"
)

type Switch struct {
	Base
	RackID             string            `rethinkdb:"rackid"`
	Partition          string            `rethinkdb:"partitionid"`
	ReplaceMode        SwitchReplaceMode `rethinkdb:"mode"`
	ManagementIP       string            `rethinkdb:"management_ip"`
	ManagementUser     string            `rethinkdb:"management_user"`
	ConsoleCommand     string            `rethinkdb:"console_command"`
	OS                 SwitchOS          `rethinkdb:"os"`
	Nics               Nics              `rethinkdb:"network_interfaces"`
	MachineConnections ConnectionMap     `rethinkdb:"machineconnections"`
}

type Switches []Switch

type Connection struct {
	Nic       Nic    `rethinkdb:"nic"`
	MachineID string `rethinkdb:"machineid"`
}

type Connections []Connection

// ConnectionMap maps machine ids to connections
type ConnectionMap map[string]Connections

type SwitchOS struct {
	Vendor           SwitchOSVendor `rethinkdb:"vendor"`
	Version          string         `rethinkdb:"version"`
	MetalCoreVersion string         `rethinkdb:"metal_core_version"`
}

type SwitchReplaceMode string
type SwitchOSVendor string

const (
	SwitchReplaceModeReplace     = SwitchReplaceMode("replace")
	SwitchReplaceModeOperational = SwitchReplaceMode("operational")

	SwitchOSVendorCumulus = SwitchOSVendor("Cumulus")
	SwitchOSVendorSonic   = SwitchOSVendor("SONiC")
)

func ToReplaceMode(mode apiv2.SwitchReplaceMode) (SwitchReplaceMode, error) {
	strVal, err := enum.GetStringValue(mode)
	if err != nil {
		return SwitchReplaceMode(""), err
	}
	return SwitchReplaceMode(*strVal), nil
}

func FromReplaceMode(mode SwitchReplaceMode) (apiv2.SwitchReplaceMode, error) {
	apiv2ReplaceMode, err := enum.GetEnum[apiv2.SwitchReplaceMode](string(mode))
	if err != nil {
		return apiv2.SwitchReplaceMode_SWITCH_REPLACE_MODE_UNSPECIFIED, fmt.Errorf("switch replace mode:%q is invalid", mode)
	}
	return apiv2ReplaceMode, nil
}

func ToSwitchOSVendor(vendor apiv2.SwitchOSVendor) (SwitchOSVendor, error) {
	strVal, err := enum.GetStringValue(vendor)
	if err != nil {
		return SwitchOSVendor(""), err
	}
	return SwitchOSVendor(*strVal), nil
}

func FromSwitchOSVendor(vendor SwitchOSVendor) (apiv2.SwitchOSVendor, error) {
	apiv2Vendor, err := enum.GetEnum[apiv2.SwitchOSVendor](string(vendor))
	if err != nil {
		return apiv2.SwitchOSVendor_SWITCH_OS_VENDOR_UNSPECIFIED, fmt.Errorf("switch os vendor:%q is invalid", vendor)
	}
	return apiv2Vendor, nil
}

func ToSwitchPortStatus(status apiv2.SwitchPortStatus) (SwitchPortStatus, error) {
	strVal, err := enum.GetStringValue(status)
	if err != nil {
		return SwitchPortStatus(""), err
	}
	return SwitchPortStatus(*strVal), nil
}

func FromSwitchPortStatus(status SwitchPortStatus) (apiv2.SwitchPortStatus, error) {
	apiv2Status, err := enum.GetEnum[apiv2.SwitchPortStatus](strings.ToLower(string(status)))
	if err != nil {
		return apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UNSPECIFIED, fmt.Errorf("switch port status:%q is invalid", status)
	}
	return apiv2Status, nil
}

func ToBGPState(state apiv2.BGPState) (BGPState, error) {
	strVal, err := enum.GetStringValue(state)
	if err != nil {
		return BGPState(""), err
	}
	return BGPState(*strVal), nil
}

func FromBGPState(state BGPState) (apiv2.BGPState, error) {
	apiv2State, err := enum.GetEnum[apiv2.BGPState](string(state))
	if err != nil {
		return apiv2.BGPState_BGP_STATE_UNSPECIFIED, fmt.Errorf("bgp state:%q is invalid", state)
	}
	return apiv2State, nil
}

func ToMetalNics(switchNics []*apiv2.SwitchNic) (Nics, error) {
	var nics Nics

	for _, switchNic := range switchNics {
		if switchNic == nil {
			continue
		}

		nic, err := toMetalNic(switchNic)
		if err != nil {
			return nil, err
		}

		nics = append(nics, *nic)
	}

	return nics, nil
}

func toMetalNic(switchNic *apiv2.SwitchNic) (*Nic, error) {
	var (
		bgpPortState *SwitchBGPPortState
		nicState     *NicState
	)

	if switchNic.BgpPortState != nil {
		bgpState, err := ToBGPState(switchNic.BgpPortState.BgpState)
		if err != nil {
			return nil, err
		}

		bgpPortState = &SwitchBGPPortState{
			Neighbor:              switchNic.BgpPortState.Neighbor,
			PeerGroup:             switchNic.BgpPortState.PeerGroup,
			VrfName:               switchNic.BgpPortState.VrfName,
			BgpState:              bgpState,
			BgpTimerUpEstablished: uint64(switchNic.BgpPortState.BgpTimerUpEstablished.Seconds),
			SentPrefixCounter:     switchNic.BgpPortState.SentPrefixCounter,
			AcceptedPrefixCounter: switchNic.BgpPortState.AcceptedPrefixCounter,
		}
	}

	if switchNic.State != nil {
		desiredState, err := ToSwitchPortStatus(switchNic.State.Desired)
		if err != nil {
			return nil, err
		}
		actualState, err := ToSwitchPortStatus(switchNic.State.Actual)
		if err != nil {
			return nil, err
		}
		nicState = &NicState{
			Desired: desiredState,
			Actual:  actualState,
		}
	}

	return &Nic{
		// TODO: what about hostname and neighbors?
		Name:         switchNic.Name,
		Identifier:   switchNic.Identifier,
		MacAddress:   switchNic.Mac,
		Vrf:          pointer.SafeDeref(switchNic.Vrf),
		State:        nicState,
		BGPPortState: bgpPortState,
	}, nil
}

func ToMachineConnections(connections []*apiv2.MachineConnection) (ConnectionMap, error) {
	var (
		machineConnections = make(ConnectionMap)
		connectedNics      []string
		duplicateNics      []string
	)

	for _, con := range connections {
		nic, err := toMetalNic(con.Nic)
		if err != nil {
			return nil, err
		}

		metalCons := machineConnections[con.MachineId]
		metalCons = append(metalCons, Connection{
			Nic:       *nic,
			MachineID: con.MachineId,
		})

		machineConnections[con.MachineId] = metalCons

		if slices.Contains(connectedNics, nic.Identifier) {
			duplicateNics = append(duplicateNics, nic.Identifier)
		}
		connectedNics = append(connectedNics, nic.Identifier)
	}

	if len(duplicateNics) > 0 {
		return nil, errorutil.InvalidArgument("found multiple connections for nics %v", duplicateNics)
	}

	return machineConnections, nil
}
