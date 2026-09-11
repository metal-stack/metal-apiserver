// Package pg provides Postgres query helpers for the generic Postgres datastore.
//
// The helpers mirror the RethinkDB query filters (see the sibling `queries`
// package) but produce the `pg.QueryFilter` format used by
// `pg.GenericRepository.Query`.
package pg

import (
	"strings"

	"github.com/metal-stack/api/go/enum"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/generic/pg"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
)

// machinePaths precomputes the JSON paths into a machine so the same strings
// are reused across queries instead of being recomputed with reflection.
var machinePaths = struct {
	ID             string
	Name           string
	PartitionID    string
	SizeID         string
	RackID         string
	RoomID         string
	Waiting        string
	PreAllocated   string
	Allocation     string
	StateValue     string
	Tags           string
	HardwareMemory string
	IPMIAddress    string
	IPMIMacAddress string
	IPMIUser       string
	IPMIInterface  string
	FRUChassisPN   string
	FRUChassisPS   string
	FRUBoardMfg    string
	FRUBoardMfgSer string
	FRUBoardPN     string
	FRUProductMfg  string
	FRUProductPN   string
	FRUProductSer  string
	AllocProject   string
	AllocUUID      string
	AllocName      string
	AllocImageID   string
	AllocHostname  string
	AllocRole      string
}{
	ID:             pg.SelectorPath[*metal.Machine]("ID"),
	Name:           pg.SelectorPath[*metal.Machine]("Name"),
	PartitionID:    pg.SelectorPath[*metal.Machine]("PartitionID"),
	SizeID:         pg.SelectorPath[*metal.Machine]("SizeID"),
	RackID:         pg.SelectorPath[*metal.Machine]("RackID"),
	RoomID:         pg.SelectorPath[*metal.Machine]("RoomID"),
	Waiting:        pg.SelectorPath[*metal.Machine]("Waiting"),
	PreAllocated:   pg.SelectorPath[*metal.Machine]("PreAllocated"),
	Allocation:     pg.SelectorPath[*metal.Machine]("Allocation"),
	StateValue:     pg.SelectorPath[*metal.Machine]("State", "Value"),
	Tags:           pg.SelectorPath[*metal.Machine]("Tags"),
	HardwareMemory: pg.SelectorPath[*metal.Machine]("Hardware", "Memory"),
	IPMIAddress:    pg.SelectorPath[*metal.Machine]("IPMI", "Address"),
	IPMIMacAddress: pg.SelectorPath[*metal.Machine]("IPMI", "MacAddress"),
	IPMIUser:       pg.SelectorPath[*metal.Machine]("IPMI", "User"),
	IPMIInterface:  pg.SelectorPath[*metal.Machine]("IPMI", "Interface"),
	FRUChassisPN:   pg.SelectorPath[*metal.Machine]("IPMI", "Fru", "ChassisPartNumber"),
	FRUChassisPS:   pg.SelectorPath[*metal.Machine]("IPMI", "Fru", "ChassisPartSerial"),
	FRUBoardMfg:    pg.SelectorPath[*metal.Machine]("IPMI", "Fru", "BoardMfg"),
	FRUBoardMfgSer: pg.SelectorPath[*metal.Machine]("IPMI", "Fru", "BoardMfgSerial"),
	FRUBoardPN:     pg.SelectorPath[*metal.Machine]("IPMI", "Fru", "BoardPartNumber"),
	FRUProductMfg:  pg.SelectorPath[*metal.Machine]("IPMI", "Fru", "ProductManufacturer"),
	FRUProductPN:   pg.SelectorPath[*metal.Machine]("IPMI", "Fru", "ProductPartNumber"),
	FRUProductSer:  pg.SelectorPath[*metal.Machine]("IPMI", "Fru", "ProductSerial"),
	AllocProject:   pg.SelectorPath[*metal.Machine]("Allocation", "Project"),
	AllocUUID:      pg.SelectorPath[*metal.Machine]("Allocation", "UUID"),
	AllocName:      pg.SelectorPath[*metal.Machine]("Allocation", "Name"),
	AllocImageID:   pg.SelectorPath[*metal.Machine]("Allocation", "ImageID"),
	AllocHostname:  pg.SelectorPath[*metal.Machine]("Allocation", "Hostname"),
	AllocRole:      pg.SelectorPath[*metal.Machine]("Allocation", "Role"),
}

// MachineFilter converts the given machine query into a list of postgres
// query filters. It returns nil when the query is nil.
//
// The filters produced here mirror the subset of the RethinkDB MachineFilter
// that maps cleanly onto the `pg.QueryFilter` equality model. Filters that
// depend on negation (e.g. NotAllocated) or array-element membership across a
// list of nested objects are not expressible with a single scalar query filter
// and are therefore not produced.
func MachineFilter(rq *apiv2.MachineQuery) []pg.QueryFilter {
	if rq == nil {
		return nil
	}

	var filters []pg.QueryFilter

	if rq.Uuid != nil {
		filters = append(filters, pg.QueryFilter{Path: machinePaths.ID, Op: "=", Value: *rq.Uuid})
	}
	if rq.Name != nil {
		filters = append(filters, pg.QueryFilter{Path: machinePaths.Name, Op: "=", Value: *rq.Name})
	}
	if rq.Partition != nil {
		filters = append(filters, pg.QueryFilter{Path: machinePaths.PartitionID, Op: "=", Value: *rq.Partition})
	}
	if rq.Size != nil {
		filters = append(filters, pg.QueryFilter{Path: machinePaths.SizeID, Op: "=", Value: *rq.Size})
	}
	if rq.Rack != nil {
		filters = append(filters, pg.QueryFilter{Path: machinePaths.RackID, Op: "=", Value: *rq.Rack})
	}
	if rq.Room != nil {
		filters = append(filters, pg.QueryFilter{Path: machinePaths.RoomID, Op: "=", Value: *rq.Room})
	}
	if rq.Waiting != nil {
		filters = append(filters, pg.QueryFilter{Path: machinePaths.Waiting, Op: "=", Value: *rq.Waiting})
	}
	if rq.Preallocated != nil {
		filters = append(filters, pg.QueryFilter{Path: machinePaths.PreAllocated, Op: "=", Value: *rq.Preallocated})
	}

	if rq.State != nil {
		stateString, err := enum.GetStringValue(*rq.State)
		if err == nil {
			if *rq.State == apiv2.MachineState_MACHINE_STATE_AVAILABLE {
				stateString = new("")
			}
			filters = append(filters, pg.QueryFilter{Path: machinePaths.StateValue, Op: "=", Value: strings.ToUpper(*stateString)})
		}
	}

	if rq.Allocation != nil {
		alloc := rq.Allocation
		if alloc.Project != nil {
			filters = append(filters, pg.QueryFilter{Path: machinePaths.AllocProject, Op: "=", Value: *alloc.Project})
		}
		if alloc.Uuid != nil {
			filters = append(filters, pg.QueryFilter{Path: machinePaths.AllocUUID, Op: "=", Value: *alloc.Uuid})
		}
		if alloc.Name != nil {
			filters = append(filters, pg.QueryFilter{Path: machinePaths.AllocName, Op: "=", Value: *alloc.Name})
		}
		if alloc.Image != nil {
			filters = append(filters, pg.QueryFilter{Path: machinePaths.AllocImageID, Op: "=", Value: *alloc.Image})
		}
		if alloc.Hostname != nil {
			filters = append(filters, pg.QueryFilter{Path: machinePaths.AllocHostname, Op: "=", Value: *alloc.Hostname})
		}
		if alloc.AllocationType != nil {
			roleString, err := enum.GetStringValue(*alloc.AllocationType)
			if err == nil {
				filters = append(filters, pg.QueryFilter{Path: machinePaths.AllocRole, Op: "=", Value: *roleString})
			}
		}
	}

	if rq.Hardware != nil && rq.Hardware.Memory != nil {
		filters = append(filters, pg.QueryFilter{Path: machinePaths.HardwareMemory, Op: "=", Value: *rq.Hardware.Memory})
	}

	if rq.Bmc != nil {
		bmc := rq.Bmc
		if bmc.Address != nil {
			filters = append(filters, pg.QueryFilter{Path: machinePaths.IPMIAddress, Op: "=", Value: *bmc.Address})
		}
		if bmc.Mac != nil {
			filters = append(filters, pg.QueryFilter{Path: machinePaths.IPMIMacAddress, Op: "=", Value: *bmc.Mac})
		}
		if bmc.User != nil {
			filters = append(filters, pg.QueryFilter{Path: machinePaths.IPMIUser, Op: "=", Value: *bmc.User})
		}
		if bmc.Interface != nil {
			filters = append(filters, pg.QueryFilter{Path: machinePaths.IPMIInterface, Op: "=", Value: *bmc.Interface})
		}
	}

	if rq.Fru != nil {
		fru := rq.Fru
		if fru.ChassisPartNumber != nil {
			filters = append(filters, pg.QueryFilter{Path: machinePaths.FRUChassisPN, Op: "=", Value: *fru.ChassisPartNumber})
		}
		if fru.ChassisPartSerial != nil {
			filters = append(filters, pg.QueryFilter{Path: machinePaths.FRUChassisPS, Op: "=", Value: *fru.ChassisPartSerial})
		}
		if fru.BoardMfg != nil {
			filters = append(filters, pg.QueryFilter{Path: machinePaths.FRUBoardMfg, Op: "=", Value: *fru.BoardMfg})
		}
		if fru.BoardSerial != nil {
			filters = append(filters, pg.QueryFilter{Path: machinePaths.FRUBoardMfgSer, Op: "=", Value: *fru.BoardSerial})
		}
		if fru.BoardPartNumber != nil {
			filters = append(filters, pg.QueryFilter{Path: machinePaths.FRUBoardPN, Op: "=", Value: *fru.BoardPartNumber})
		}
		if fru.ProductManufacturer != nil {
			filters = append(filters, pg.QueryFilter{Path: machinePaths.FRUProductMfg, Op: "=", Value: *fru.ProductManufacturer})
		}
		if fru.ProductPartNumber != nil {
			filters = append(filters, pg.QueryFilter{Path: machinePaths.FRUProductPN, Op: "=", Value: *fru.ProductPartNumber})
		}
		if fru.ProductSerial != nil {
			filters = append(filters, pg.QueryFilter{Path: machinePaths.FRUProductSer, Op: "=", Value: *fru.ProductSerial})
		}
	}

	return filters
}
