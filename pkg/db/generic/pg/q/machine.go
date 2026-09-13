// Package q provides Postgres query helpers for the generic Postgres datastore.
//
// The helpers mirror the RethinkDB query filters (see the sibling `queries`
// package) but produce the `pg.QueryFilter` format used by
// `pg.GenericRepository.Query`.
package q

import (
	"fmt"
	"slices"
	"strings"

	"github.com/metal-stack/api/go/enum"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/generic/pg"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
)

// machinePaths precomputes the JSON paths into a machine so the same strings
// are reused across queries instead of being recomputed with reflection.
var machinePaths = struct {
	ID              string
	Name            string
	PartitionID     string
	SizeID          string
	RackID          string
	RoomID          string
	Waiting         string
	PreAllocated    string
	Allocation      string
	StateValue      string
	Tags            string
	HardwareMemory  string
	HardwareCPUsSum string
	IPMIAddress     string
	IPMIMacAddress  string
	IPMIUser        string
	IPMIInterface   string
	FRUChassisPN    string
	FRUChassisPS    string
	FRUBoardMfg     string
	FRUBoardMfgSer  string
	FRUBoardPN      string
	FRUProductMfg   string
	FRUProductPN    string
	FRUProductSer   string
	AllocProject    string
	AllocUUID       string
	AllocName       string
	AllocImageID    string
	AllocHostname   string
	AllocRole       string
	AllocFSLID      string
	AllocVPN        string
	AllocLabels     string
	MachineNetworks string
	Nics            string
	Disks           string
}{
	ID:              pg.SelectorPath[*metal.Machine]("ID"),
	Name:            pg.SelectorPath[*metal.Machine]("Name"),
	PartitionID:     pg.SelectorPath[*metal.Machine]("PartitionID"),
	SizeID:          pg.SelectorPath[*metal.Machine]("SizeID"),
	RackID:          pg.SelectorPath[*metal.Machine]("RackID"),
	RoomID:          pg.SelectorPath[*metal.Machine]("RoomID"),
	Waiting:         pg.SelectorPath[*metal.Machine]("Waiting"),
	PreAllocated:    pg.SelectorPath[*metal.Machine]("PreAllocated"),
	Allocation:      pg.SelectorPath[*metal.Machine]("Allocation"),
	StateValue:      pg.SelectorPath[*metal.Machine]("State", "Value"),
	Tags:            pg.SelectorPath[*metal.Machine]("Tags"),
	HardwareMemory:  pg.SelectorPath[*metal.Machine]("Hardware", "Memory"),
	HardwareCPUsSum: "Hardware.MetalCPUs.Cores",
	IPMIAddress:     pg.SelectorPath[*metal.Machine]("IPMI", "Address"),
	IPMIMacAddress:  pg.SelectorPath[*metal.Machine]("IPMI", "MacAddress"),
	IPMIUser:        pg.SelectorPath[*metal.Machine]("IPMI", "User"),
	IPMIInterface:   pg.SelectorPath[*metal.Machine]("IPMI", "Interface"),
	FRUChassisPN:    pg.SelectorPath[*metal.Machine]("IPMI", "Fru", "ChassisPartNumber"),
	FRUChassisPS:    pg.SelectorPath[*metal.Machine]("IPMI", "Fru", "ChassisPartSerial"),
	FRUBoardMfg:     pg.SelectorPath[*metal.Machine]("IPMI", "Fru", "BoardMfg"),
	FRUBoardMfgSer:  pg.SelectorPath[*metal.Machine]("IPMI", "Fru", "BoardMfgSerial"),
	FRUBoardPN:      pg.SelectorPath[*metal.Machine]("IPMI", "Fru", "BoardPartNumber"),
	FRUProductMfg:   pg.SelectorPath[*metal.Machine]("IPMI", "Fru", "ProductManufacturer"),
	FRUProductPN:    pg.SelectorPath[*metal.Machine]("IPMI", "Fru", "ProductPartNumber"),
	FRUProductSer:   pg.SelectorPath[*metal.Machine]("IPMI", "Fru", "ProductSerial"),
	AllocProject:    pg.SelectorPath[*metal.Machine]("Allocation", "Project"),
	AllocUUID:       pg.SelectorPath[*metal.Machine]("Allocation", "UUID"),
	AllocName:       pg.SelectorPath[*metal.Machine]("Allocation", "Name"),
	AllocImageID:    pg.SelectorPath[*metal.Machine]("Allocation", "ImageID"),
	AllocHostname:   pg.SelectorPath[*metal.Machine]("Allocation", "Hostname"),
	AllocRole:       pg.SelectorPath[*metal.Machine]("Allocation", "Role"),
	AllocFSLID:      pg.SelectorPath[*metal.Machine]("Allocation", "FilesystemLayout", "ID"),
	AllocVPN:        pg.SelectorPath[*metal.Machine]("Allocation", "VPN"),
	AllocLabels:     pg.SelectorPath[*metal.Machine]("Allocation", "Labels"),
	MachineNetworks: pg.SelectorPath[*metal.Machine]("Allocation", "MachineNetworks"),
	Nics:            pg.SelectorPath[*metal.Machine]("Hardware", "Nics"),
	Disks:           pg.SelectorPath[*metal.Machine]("Hardware", "Disks"),
}

// containsJSON builds a structured JSON fragment for the `@>` containment
// operator from a dotted path and a value used verbatim at that path. It mirrors
// the shape of the machine so that Postgres matches any machine whose nested
// value contains the fragment.
//
// Examples:
//   - containsJSON("Tags", []string{"color=red"}) -> {"Tags":["color=red"]}
//   - containsJSON("Allocation.Labels", map[string]string{"color":"red"}) ->
//     {"Allocation":{"Labels":{"color":"red"}}}
//   - containsJSON("Hardware.Nics", []any{element}) ->
//     {"Hardware":{"Nics":[element]}}
func containsJSON(path string, value any) map[string]any {
	parts := strings.Split(path, ".")
	var cur = value
	for _, part := range slices.Backward(parts) {
		cur = map[string]any{part: cur}
	}
	return cur.(map[string]any)
}

// netwWrap wraps an element object into the MachineNetworks array for `@>` matching.
func netwWrap(element map[string]any) map[string]any {
	return containsJSON(machinePaths.MachineNetworks, []any{element})
}

// nicsWrap wraps an element object into the Hardware.Nics array for `@>` matching.
func nicsWrap(element map[string]any) map[string]any {
	return containsJSON(machinePaths.Nics, []any{element})
}

// disksWrap wraps an element object into the Hardware.Disks array for `@>` matching.
func disksWrap(element map[string]any) map[string]any {
	return containsJSON(machinePaths.Disks, []any{element})
}

// MachineFilter converts the given machine query into a list of postgres
// query filters. It returns nil when the query is nil.
//
// The filters produced here mirror the RethinkDB MachineFilter (see the sibling
// `queries` package). Scalar field equality uses the `=` operator; membership
// inside nested arrays and maps (tags, labels, networks, nics, disks) uses the
// `@>` containment operator; field presence uses `IS NULL`/`IS NOT NULL`; and
// the hardware cpu-core sum uses `SUM_EQ`.
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
	if rq.Labels != nil {
		for key, value := range rq.Labels.Labels {
			tag := fmt.Sprintf("%s=%s", key, value)
			filters = append(filters, pg.QueryFilter{Path: machinePaths.Tags, Op: "@>", Value: containsJSON(machinePaths.Tags, []string{tag})})
		}
	}
	if rq.Waiting != nil {
		filters = append(filters, pg.QueryFilter{Path: machinePaths.Waiting, Op: "=", Value: *rq.Waiting})
	}
	if rq.Preallocated != nil {
		filters = append(filters, pg.QueryFilter{Path: machinePaths.PreAllocated, Op: "=", Value: *rq.Preallocated})
	}
	if rq.NotAllocated != nil && *rq.NotAllocated {
		filters = append(filters, pg.QueryFilter{Path: machinePaths.Allocation, Op: "IS NULL"})
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
		filters = append(filters, pg.QueryFilter{Path: machinePaths.Allocation, Op: "IS NOT NULL"})
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
		if alloc.FilesystemLayout != nil {
			filters = append(filters, pg.QueryFilter{Path: machinePaths.AllocFSLID, Op: "=", Value: *alloc.FilesystemLayout})
		}
		if alloc.Labels != nil {
			for key, value := range alloc.Labels.Labels {
				filters = append(filters, pg.QueryFilter{Path: machinePaths.AllocLabels, Op: "@>", Value: containsJSON(machinePaths.AllocLabels, map[string]string{key: value})})
			}
		}
		if alloc.Vpn != nil {
			filters = append(filters, pg.QueryFilter{Path: machinePaths.AllocVPN, Op: "IS NOT NULL"})
		}
	}

	if rq.Network != nil {
		nw := rq.Network
		for _, id := range nw.Networks {
			filters = append(filters, pg.QueryFilter{Path: machinePaths.MachineNetworks, Op: "@>", Value: netwWrap(map[string]any{"NetworkID": id})})
		}
		for _, prefix := range nw.Prefixes {
			filters = append(filters, pg.QueryFilter{Path: machinePaths.MachineNetworks, Op: "@>", Value: netwWrap(map[string]any{"Prefixes": []string{prefix}})})
		}
		for _, ip := range nw.Ips {
			filters = append(filters, pg.QueryFilter{Path: machinePaths.MachineNetworks, Op: "@>", Value: netwWrap(map[string]any{"IPs": []string{ip}})})
		}
		for _, destPrefix := range nw.DestinationPrefixes {
			filters = append(filters, pg.QueryFilter{Path: machinePaths.MachineNetworks, Op: "@>", Value: netwWrap(map[string]any{"DestinationPrefixes": []string{destPrefix}})})
		}
		for _, vrf := range nw.Vrfs {
			filters = append(filters, pg.QueryFilter{Path: machinePaths.MachineNetworks, Op: "@>", Value: netwWrap(map[string]any{"Vrf": vrf})})
		}
		for _, asn := range nw.Asns {
			filters = append(filters, pg.QueryFilter{Path: machinePaths.MachineNetworks, Op: "@>", Value: netwWrap(map[string]any{"ASN": asn})})
		}
	}

	if rq.Hardware != nil {
		hw := rq.Hardware
		if hw.Memory != nil {
			filters = append(filters, pg.QueryFilter{Path: machinePaths.HardwareMemory, Op: "=", Value: *hw.Memory})
		}
		if hw.CpuCores != nil {
			filters = append(filters, pg.QueryFilter{Path: machinePaths.HardwareCPUsSum, Op: "SUM_EQ", Value: *hw.CpuCores})
		}
	}

	if rq.Nic != nil {
		nic := rq.Nic
		for _, mac := range nic.Macs {
			filters = append(filters, pg.QueryFilter{Path: machinePaths.Nics, Op: "@>", Value: nicsWrap(map[string]any{"MacAddress": mac})})
		}
		for _, name := range nic.Names {
			filters = append(filters, pg.QueryFilter{Path: machinePaths.Nics, Op: "@>", Value: nicsWrap(map[string]any{"Name": name})})
		}
		for _, mac := range nic.NeighborMacs {
			filters = append(filters, pg.QueryFilter{Path: machinePaths.Nics, Op: "@>", Value: nicsWrap(map[string]any{"Neighbors": []any{map[string]any{"MacAddress": mac}}})})
		}
		for _, name := range nic.NeighborNames {
			filters = append(filters, pg.QueryFilter{Path: machinePaths.Nics, Op: "@>", Value: nicsWrap(map[string]any{"Neighbors": []any{map[string]any{"Name": name}}})})
		}
	}

	if rq.Disk != nil {
		disk := rq.Disk
		for _, name := range disk.Names {
			filters = append(filters, pg.QueryFilter{Path: machinePaths.Disks, Op: "@>", Value: disksWrap(map[string]any{"Name": name})})
		}
		for _, size := range disk.Sizes {
			filters = append(filters, pg.QueryFilter{Path: machinePaths.Disks, Op: "@>", Value: disksWrap(map[string]any{"Size": size})})
		}
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
