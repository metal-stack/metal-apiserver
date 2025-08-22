package metal

import (
	"fmt"
	"time"

	"github.com/metal-stack/api/go/enum"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
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

// FIXME: Nic is part of machine service PR. Remove once it is merged.
type Nic struct {
	Mac          string        `rethinkdb:"macAddress"`
	Name         string        `rethinkdb:"name"`
	Identifier   string        `rethinkdb:"identifier"`
	Vrf          *string       `rethinkdb:"vrf"`
	Neighbors    Nics          `rethinkdb:"neighbors"`
	Hostname     string        `rethinkdb:"hostname"`
	State        *NicState     `rethinkdb:"state"`
	BGPPortState *BGPPortState `rethinkdb:"bgpPortState"`
}

type Nics []Nic

type NicState struct {
	Desired SwitchPortStatus `rethinkdb:"desired"`
	Actual  SwitchPortStatus `rethinkdb:"actual"`
}

type BGPPortState struct {
	Neighbor              string        `rethinkdb:"neighbor"`
	PeerGroup             string        `rethinkdb:"peergroup"`
	VrfName               string        `rethinkdb:"vrfname"`
	State                 BGPState      `rethinkdb:"state"`
	BGPTimerUpEstablished time.Duration `rethinkdb:"bgptimerupestablished"`
	SentPrefixCounter     uint64        `rethinkdb:"sentprefixcounter"`
	AcceptedPrefixCounter uint64        `rethinkdb:"acceptedprefixcounter"`
}

type SwitchReplaceMode string
type SwitchPortStatus string
type SwitchOSVendor string
type BGPState string

const (
	ReplaceModeReplace     = SwitchReplaceMode("replace")
	ReplaceModeOperational = SwitchReplaceMode("operational")

	SwitchOSVendorCumulus = SwitchOSVendor("Cumulus")
	SwitchOSVendorSonic   = SwitchOSVendor("SONiC")

	SwitchPortStatusUp   = SwitchPortStatus("up")
	SwitchPortStatusDown = SwitchPortStatus("down")

	BGPStateIdle        = BGPState("idle")
	BGPStateConnect     = BGPState("connect")
	BGPStateActive      = BGPState("active")
	BGPStateOpenSent    = BGPState("open-sent")
	BGPStateOpenConfirm = BGPState("open-confirm")
	BGPStateEstablished = BGPState("established")
)

func ToReplaceMode(mode apiv2.SwitchReplaceMode) (SwitchReplaceMode, error) {
	switch mode {
	case apiv2.SwitchReplaceMode_SWITCH_REPLACE_MODE_OPERATIONAL:
		return ReplaceModeOperational, nil
	case apiv2.SwitchReplaceMode_SWITCH_REPLACE_MODE_REPLACE:
		return ReplaceModeReplace, nil
	case apiv2.SwitchReplaceMode_SWITCH_REPLACE_MODE_UNSPECIFIED:
		fallthrough
	default:
		return SwitchReplaceMode(""), fmt.Errorf("switch replace mode:%q is invalid", mode)
	}
}

func FromReplaceMode(mode SwitchReplaceMode) (apiv2.SwitchReplaceMode, error) {
	apiv2ReplaceMode, err := enum.GetEnum[apiv2.SwitchReplaceMode](string(mode))
	if err != nil {
		return apiv2.SwitchReplaceMode_SWITCH_REPLACE_MODE_UNSPECIFIED, fmt.Errorf("switch replace mode:%q is invalid", mode)
	}
	return apiv2ReplaceMode, nil
}

func ToSwitchOSVendor(vendor apiv2.SwitchOSVendor) (SwitchOSVendor, error) {
	switch vendor {
	case apiv2.SwitchOSVendor_SWITCH_OS_VENDOR_CUMULUS:
		return SwitchOSVendorCumulus, nil
	case apiv2.SwitchOSVendor_SWITCH_OS_VENDOR_SONIC:
		return SwitchOSVendorSonic, nil
	case apiv2.SwitchOSVendor_SWITCH_OS_VENDOR_UNSPECIFIED:
		fallthrough
	default:
		return SwitchOSVendor(""), fmt.Errorf("switch os vendor:%q is invalid", vendor)
	}
}

func FromSwitchOSVendor(vendor SwitchOSVendor) (apiv2.SwitchOSVendor, error) {
	apiv2Vendor, err := enum.GetEnum[apiv2.SwitchOSVendor](string(vendor))
	if err != nil {
		return apiv2.SwitchOSVendor_SWITCH_OS_VENDOR_UNSPECIFIED, fmt.Errorf("switch os vendor:%q is invalid", vendor)
	}
	return apiv2Vendor, nil
}

func ToSwitchPortStatus(status apiv2.SwitchPortStatus) (SwitchPortStatus, error) {
	switch status {
	case apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_DOWN:
		return SwitchPortStatusDown, nil
	case apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP:
		return SwitchPortStatusUp, nil
	case apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UNSPECIFIED:
		fallthrough
	default:
		return SwitchPortStatus(""), fmt.Errorf("switch port status:%q is invalid", status)
	}
}

func FromSwitchPortStatus(status SwitchPortStatus) (apiv2.SwitchPortStatus, error) {
	apiv2Status, err := enum.GetEnum[apiv2.SwitchPortStatus](string(status))
	if err != nil {
		return apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UNSPECIFIED, fmt.Errorf("switch port status:%q is invalid", status)
	}
	return apiv2Status, nil
}

func ToBGPState(state apiv2.BGPState) (BGPState, error) {
	switch state {
	case apiv2.BGPState_BGP_STATE_IDLE:
		return BGPStateIdle, nil
	case apiv2.BGPState_BGP_STATE_CONNECT:
		return BGPStateConnect, nil
	case apiv2.BGPState_BGP_STATE_ACTIVE:
		return BGPStateActive, nil
	case apiv2.BGPState_BGP_STATE_OPEN_SENT:
		return BGPStateOpenSent, nil
	case apiv2.BGPState_BGP_STATE_OPEN_CONFIRM:
		return BGPStateOpenConfirm, nil
	case apiv2.BGPState_BGP_STATE_ESTABLISHED:
		return BGPStateEstablished, nil
	case apiv2.BGPState_BGP_STATE_UNSPECIFIED:
		fallthrough
	default:
		return BGPState(""), fmt.Errorf("bgp state:%q is invalid", state)
	}
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

	for _, nic := range switchNics {
		if nic == nil {
			continue
		}

		var bgpPortState *BGPPortState
		if nic.BgpPortState != nil {
			bgpState, err := ToBGPState(nic.BgpPortState.BgpState)
			if err != nil {
				return nil, err
			}

			bgpPortState = &BGPPortState{
				Neighbor:              nic.BgpPortState.Neighbor,
				PeerGroup:             nic.BgpPortState.PeerGroup,
				VrfName:               nic.BgpPortState.VrfName,
				State:                 bgpState,
				BGPTimerUpEstablished: nic.BgpPortState.BgpTimerUpEstablished.AsDuration(),
				SentPrefixCounter:     nic.BgpPortState.SentPrefixCounter,
				AcceptedPrefixCounter: nic.BgpPortState.AcceptedPrefixCounter,
			}
		}

		desiredState, err := ToSwitchPortStatus(nic.State.Desired)
		if err != nil {
			return nil, err
		}
		actualState, err := ToSwitchPortStatus(nic.State.Actual)
		if err != nil {
			return nil, err
		}

		nics = append(nics, Nic{
			Name:       nic.Name,
			Identifier: nic.Identifier,
			Mac:        nic.Mac,
			Vrf:        nic.Vrf,
			State: &NicState{
				Desired: desiredState,
				Actual:  actualState,
			},
			BGPPortState: bgpPortState,
		})
	}

	return nics, nil
}
