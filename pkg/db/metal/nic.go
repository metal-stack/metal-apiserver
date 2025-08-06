package metal

// A MacAddress is the type for mac addresses. When using a
// custom type, we cannot use strings directly.
type MacAddress string

// Nic information.
type Nic struct {
	MacAddress   MacAddress          `rethinkdb:"macAddress"`
	Name         string              `rethinkdb:"name"`
	Identifier   string              `rethinkdb:"identifier"`
	Vrf          string              `rethinkdb:"vrf"`
	Neighbors    Nics                `rethinkdb:"neighbors"`
	Hostname     string              `rethinkdb:"hostname"`
	State        *NicState           `rethinkdb:"state"`
	BGPPortState *SwitchBGPPortState `rethinkdb:"bgpPortState"`
}

// Nics is a list of nics.
type Nics []Nic

// NicState represents the desired and actual state of a network interface
// controller (NIC). The Desired field indicates the intended state of the
// NIC, while Actual indicates its current operational state. The Desired
// state will be removed when the actual state is equal to the desired state.
type NicState struct {
	Desired *SwitchPortStatus `rethinkdb:"desired"`
	Actual  SwitchPortStatus  `rethinkdb:"actual"`
}

type SwitchBGPPortState struct {
	Neighbor              string
	PeerGroup             string
	VrfName               string
	BgpState              string
	BgpTimerUpEstablished int64
	SentPrefixCounter     int64
	AcceptedPrefixCounter int64
}

// SwitchPortStatus is a type alias for a string that represents the status of a switch port.
// Valid values are defined as constants in this package.
type SwitchPortStatus string

// SwitchPortStatus defines the possible statuses for a switch port.
// UNKNOWN indicates the status is not known.
// UP indicates the port is up and operational.
// DOWN indicates the port is down and not operational.
const (
	SwitchPortStatusUnknown SwitchPortStatus = "UNKNOWN"
	SwitchPortStatusUp      SwitchPortStatus = "UP"
	SwitchPortStatusDown    SwitchPortStatus = "DOWN"
)
