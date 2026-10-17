package q

import (
	"github.com/metal-stack/api/go/enum"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/generic/pg"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
)

// switchPaths precomputes the JSON paths into a switch.
var switchPaths = struct {
	ID                 string
	Rack               string
	Room               string
	Partition          string
	OSVendor           string
	OSVersion          string
	MachineConnections string
}{
	ID:                 pg.SelectorPath[*metal.Switch]("ID"),
	Rack:               pg.SelectorPath[*metal.Switch]("Rack"),
	Room:               pg.SelectorPath[*metal.Switch]("Room"),
	Partition:          pg.SelectorPath[*metal.Switch]("Partition"),
	OSVendor:           pg.SelectorPath[*metal.Switch]("OS", "Vendor"),
	OSVersion:          pg.SelectorPath[*metal.Switch]("OS", "Version"),
	MachineConnections: pg.SelectorPath[*metal.Switch]("MachineConnections"),
}

// SwitchFilter converts the given switch query into a list of postgres query
// filters. It returns nil when the query is nil.
//
// It mirrors the RethinkDB SwitchFilter (see the sibling `queries` package).
// Scalar field equality uses the `=` operator; the connected-machine check
// matches a switch whose `machine_connections` map has an entry (key) for the
// given machine id, expressed as a presence check on that nested key.
func SwitchFilter(rq *apiv2.SwitchQuery) []pg.QueryFilter {
	if rq == nil {
		return nil
	}

	var filters []pg.QueryFilter

	if rq.Id != nil {
		filters = append(filters, pg.QueryFilter{Path: switchPaths.ID, Op: "=", Value: *rq.Id})
	}
	if rq.Partition != nil {
		filters = append(filters, pg.QueryFilter{Path: switchPaths.Partition, Op: "=", Value: *rq.Partition})
	}
	if rq.Rack != nil {
		filters = append(filters, pg.QueryFilter{Path: switchPaths.Rack, Op: "=", Value: *rq.Rack})
	}
	if rq.Room != nil {
		filters = append(filters, pg.QueryFilter{Path: switchPaths.Room, Op: "=", Value: *rq.Room})
	}

	if rq.Os != nil {
		if rq.Os.Vendor != nil {
			vendor, err := enum.GetStringValue(rq.Os.Vendor)
			if err == nil {
				filters = append(filters, pg.QueryFilter{Path: switchPaths.OSVendor, Op: "=", Value: *vendor})
			}
		}
		if rq.Os.Version != nil {
			filters = append(filters, pg.QueryFilter{Path: switchPaths.OSVersion, Op: "=", Value: *rq.Os.Version})
		}
	}

	if rq.ConnectedMachineId != nil {
		filters = append(filters, pg.QueryFilter{Path: switchPaths.MachineConnections + "." + *rq.ConnectedMachineId, Op: "IS NOT NULL"})
	}

	return filters
}
