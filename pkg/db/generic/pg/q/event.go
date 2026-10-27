package q

import (
	"github.com/metal-stack/metal-apiserver/pkg/db/generic/pg"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
)

// eventPathID is the JSON path to a provisioning event container's id, which is
// the machine id it belongs to.
var eventPathID = pg.SelectorPath[*metal.ProvisioningEventContainer]("ID")

// EventFilter converts a machine id into a list of postgres query filters. It
// returns nil when the machine id is empty.
//
// It mirrors the RethinkDB EventFilter (see the sibling `queries` package): the
// event container is keyed by the machine id.
func EventFilter(machineID string) []pg.QueryFilter {
	if machineID == "" {
		return nil
	}

	return []pg.QueryFilter{{Path: eventPathID, Op: "=", Value: machineID}}
}
