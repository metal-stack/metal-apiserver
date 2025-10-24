package metal

import (
	"fmt"
	"strings"
	"time"

	"github.com/metal-stack/api/go/enum"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
)

type (
	Switch struct {
		Base
		Rack               string            `rethinkdb:"rackid"`
		Partition          string            `rethinkdb:"partitionid"`
		ReplaceMode        SwitchReplaceMode `rethinkdb:"mode"`
		ManagementIP       string            `rethinkdb:"management_ip"`
		ManagementUser     string            `rethinkdb:"management_user"`
		ConsoleCommand     string            `rethinkdb:"console_command"`
		OS                 SwitchOS          `rethinkdb:"os"`
		Nics               Nics              `rethinkdb:"network_interfaces"`
		MachineConnections ConnectionMap     `rethinkdb:"machineconnections"`
	}
	Switches []Switch

	SwitchStatus struct {
		Base
		LastSync      *SwitchSync `rethinkdb:"last_sync"`
		LastSyncError *SwitchSync `rethinkdb:"last_sync_error"`
	}

	SwitchBGPPortState struct {
		// FIXME add rethinkdb annotations, check against existing database entries
		Neighbor              string
		PeerGroup             string
		VrfName               string
		BgpState              BGPState
		BgpTimerUpEstablished uint64
		SentPrefixCounter     uint64
		AcceptedPrefixCounter uint64
	}

	SwitchSync struct {
		Time     time.Time     `rethinkdb:"time"`
		Duration time.Duration `rethinkdb:"duration"`
		Error    *string       `rethinkdb:"error"`
	}

	Connection struct {
		Nic       Nic    `rethinkdb:"nic"`
		MachineID string `rethinkdb:"machineid"`
	}
	Connections []Connection

	// ConnectionMap maps machine ids to connections
	ConnectionMap map[string]Connections

	SwitchOS struct {
		Vendor           SwitchOSVendor `rethinkdb:"vendor"`
		Version          string         `rethinkdb:"version"`
		MetalCoreVersion string         `rethinkdb:"metal_core_version"`
	}

	SwitchReplaceMode string
	SwitchOSVendor    string
)

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
		return apiv2.SwitchReplaceMode_SWITCH_REPLACE_MODE_UNSPECIFIED, fmt.Errorf("switch replace mode %q is invalid", mode)
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
		return apiv2.SwitchOSVendor_SWITCH_OS_VENDOR_UNSPECIFIED, fmt.Errorf("switch os vendor %q is invalid", vendor)
	}
	return apiv2Vendor, nil
}

func ToSwitchPortStatus(status apiv2.SwitchPortStatus) (SwitchPortStatus, error) {
	strVal, err := enum.GetStringValue(status)
	if err != nil {
		return SwitchPortStatus(""), err
	}
	return SwitchPortStatus(strings.ToUpper(*strVal)), nil
}

func FromSwitchPortStatus(status *SwitchPortStatus) (apiv2.SwitchPortStatus, error) {
	if status == nil {
		return apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UNSPECIFIED, nil
	}

	apiv2Status, err := enum.GetEnum[apiv2.SwitchPortStatus](strings.ToLower(string(*status)))
	if err != nil {
		return apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UNSPECIFIED, fmt.Errorf("switch port status %q is invalid", *status)
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
		return apiv2.BGPState_BGP_STATE_UNSPECIFIED, fmt.Errorf("bgp state %q is invalid", state)
	}
	return apiv2State, nil
}
