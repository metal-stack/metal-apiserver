package metal

type (
	Nic struct {
		MacAddress   string              `rethinkdb:"macAddress"`
		Name         string              `rethinkdb:"name"`
		Identifier   string              `rethinkdb:"identifier"`
		Vrf          string              `rethinkdb:"vrf"`
		Neighbors    Nics                `rethinkdb:"neighbors"`
		Hostname     string              `rethinkdb:"hostname"`
		State        *NicState           `rethinkdb:"state"`
		BGPPortState *SwitchBGPPortState `rethinkdb:"bgpPortState"`
	}

	Nics []Nic

	NicMap map[string]*Nic

	NicState struct {
		Desired *SwitchPortStatus `rethinkdb:"desired"`
		Actual  SwitchPortStatus  `rethinkdb:"actual"`
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

	BGPState         string
	SwitchPortStatus string
)

const (
	BGPStateIdle        = BGPState("idle")
	BGPStateConnect     = BGPState("connect")
	BGPStateActive      = BGPState("active")
	BGPStateOpenSent    = BGPState("open-sent")
	BGPStateOpenConfirm = BGPState("open-confirm")
	BGPStateEstablished = BGPState("established")
)

const (
	SwitchPortStatusUnknown SwitchPortStatus = "UNKNOWN"
	SwitchPortStatusUp      SwitchPortStatus = "UP"
	SwitchPortStatusDown    SwitchPortStatus = "DOWN"
)

func (nics Nics) MapByIdentifier() NicMap {
	nicMap := make(NicMap)
	for _, nic := range nics {
		nicMap[nic.Identifier] = &nic
	}
	return nicMap
}
