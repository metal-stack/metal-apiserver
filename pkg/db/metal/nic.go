package metal

// Nic information.
// This is used for machine nics and switch nics as backing store
type Nic struct {
	MacAddress   string              `rethinkdb:"macAddress"`
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

type NicMap map[string]*Nic

// NicState represents the desired and actual state of a network interface
// controller (NIC). The Desired field indicates the intended state of the
// NIC, while Actual indicates its current operational state. The Desired
// state will be removed when the actual state is equal to the desired state.
type NicState struct {
	Desired SwitchPortStatus `rethinkdb:"desired"`
	Actual  SwitchPortStatus `rethinkdb:"actual"`
}

type SwitchBGPPortState struct {
	// FIXME add rethinkdb annotations, check against existing database entries
	Neighbor              string
	PeerGroup             string
	VrfName               string
	BgpState              BGPState
	BgpTimerUpEstablished uint64
	SentPrefixCounter     uint64
	AcceptedPrefixCounter uint64
}

type BGPState string

const (
	BGPStateIdle        = BGPState("idle")
	BGPStateConnect     = BGPState("connect")
	BGPStateActive      = BGPState("active")
	BGPStateOpenSent    = BGPState("open-sent")
	BGPStateOpenConfirm = BGPState("open-confirm")
	BGPStateEstablished = BGPState("established")
)

// SwitchPortStatus is a type alias for a string that represents the status of a switch port.
// Valid values are defined as constants in this package.
type SwitchPortStatus string

// SwitchPortStatus defines the possible statuses for a switch port.
// UNKNOWN indicates the status is not known.
// UP indicates the port is up and operational.
// DOWN indicates the port is down and not operational.
const (
	SwitchPortStatusUp   SwitchPortStatus = "up"
	SwitchPortStatusDown SwitchPortStatus = "down"
)

func (nics Nics) MapByIdentifier() NicMap {
	nicMap := make(NicMap)
	for _, nic := range nics {
		nicMap[nic.Identifier] = &nic
	}
	return nicMap
}
