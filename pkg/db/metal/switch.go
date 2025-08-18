package metal

import "time"

type Switch struct {
	Base
	Hostname       string            `rethinkdb:"hostname"`
	Description    string            `rethinkdb:"description"`
	RackID         string            `rethinkdb:"rackid"`
	Partition      string            `rethinkdb:"partition"`
	ReplaceMode    SwitchReplaceMode `rethinkdb:"replacemode"`
	ManagementIP   string            `rethinkdb:"managementip"`
	ManagementUser string            `rethinkdb:"managementuser"`
	OS             SwitchOS          `rethinkdb:"os"`
	Nics           Nics              `rethinkdb:"nics"`
}

type Switches []Switch

type SwitchOS struct {
	Vendor           SwitchOSVendor `rethinkdb:"vendor"`
	Version          string         `rethinkdb:"version"`
	MetalCoreVersion string         `rethinkdb:"metalcoreversion"`
}

type Nic struct {
	Name         string        `rethinkdb:"name"`
	Identifier   string        `rethinkdb:"identifier"`
	Mac          string        `rethinkdb:"mac"`
	Vrf          *string       `rethinkdb:"vrf"`
	State        *NicState     `rethinkdb:"state"`
	BGPPortState *BGPPortState `rethinkdb:"bgpportstate"`
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
