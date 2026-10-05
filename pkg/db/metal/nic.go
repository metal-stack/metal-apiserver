package metal

import "github.com/samber/lo"

type (
	Nic struct {
		MacAddress   string              `rethinkdb:"macAddress" json:"mac_address"`
		Name         string              `rethinkdb:"name" json:"name"`
		Identifier   string              `rethinkdb:"identifier" json:"identifier"`
		Vrf          string              `rethinkdb:"vrf" json:"vrf"`
		Neighbors    Nics                `rethinkdb:"neighbors" json:"neighbors"`
		Hostname     string              `rethinkdb:"hostname" json:"hostname"`
		State        *NicState           `rethinkdb:"state" json:"state"`
		BGPPortState *SwitchBGPPortState `rethinkdb:"bgpPortState" json:"bgp_port_state"`
	}

	Nics []Nic

	NicMap map[string]*Nic

	NicState struct {
		Desired *SwitchPortStatus `rethinkdb:"desired" json:"desired"`
		Actual  SwitchPortStatus  `rethinkdb:"actual" json:"actual"`
	}

	BGPState         string
	SwitchPortStatus string
)

const (
	BGPStateIdle        = BGPState("Idle")
	BGPStateConnect     = BGPState("Connect")
	BGPStateActive      = BGPState("Active")
	BGPStateOpenSent    = BGPState("OpenSent")
	BGPStateOpenConfirm = BGPState("OpenConfirm")
	BGPStateEstablished = BGPState("Established")
)

const (
	SwitchPortStatusUnknown SwitchPortStatus = "UNKNOWN"
	SwitchPortStatusUp      SwitchPortStatus = "UP"
	SwitchPortStatusDown    SwitchPortStatus = "DOWN"
)

func (nics Nics) MapByIdentifier() NicMap {
	nicMap := make(NicMap)
	for _, nic := range nics {
		if nic.Identifier == "" {
			continue
		}
		nicMap[nic.Identifier] = &nic
	}
	return nicMap
}

func (nics Nics) MapByName() NicMap {
	nicMap := make(NicMap)
	for _, nic := range nics {
		if nic.Name == "" {
			continue
		}
		nicMap[nic.Name] = &nic
	}
	return nicMap
}

func (nics Nics) FilterByHostname(hostname string) Nics {
	if hostname == "" {
		return nics
	}
	return lo.Filter(nics, func(nic Nic, _ int) bool {
		return nic.Hostname == hostname
	})
}
