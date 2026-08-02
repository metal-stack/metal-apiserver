package queries

import (
	"fmt"
	"strings"

	"github.com/metal-stack/api/go/enum"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/postgres/cond"
)

// MachineFilter builds a JSONB query condition for Machine entities from the given query.
func MachineFilter(rq *apiv2.MachineQuery) *cond.Where {
	if rq == nil {
		return nil
	}
	var conds []*cond.Where

	if rq.Uuid != nil {
		conds = append(conds, cond.FieldEq("ID", *rq.Uuid))
	}
	if rq.Name != nil {
		conds = append(conds, cond.FieldEq("Name", *rq.Name))
	}
	if rq.Partition != nil {
		conds = append(conds, cond.FieldEq("PartitionID", *rq.Partition))
	}
	if rq.Size != nil {
		conds = append(conds, cond.FieldEq("SizeID", *rq.Size))
	}
	if rq.Rack != nil {
		conds = append(conds, cond.FieldEq("RackID", *rq.Rack))
	}
	if rq.Room != nil {
		conds = append(conds, cond.FieldEq("RoomID", *rq.Room))
	}
	if rq.Labels != nil {
		for key, value := range rq.Labels.Labels {
			tagStr := fmt.Sprintf("%s=%s", key, value)
			conds = append(conds, cond.TagInSlice(tagStr))
		}
	}
	if rq.Waiting != nil {
		val := "true"
		if !*rq.Waiting {
			val = "false"
		}
		conds = append(conds, &cond.Where{SQL: fmt.Sprintf("data->>'Waiting' = '%s'", val)})
	}
	if rq.Preallocated != nil {
		val := "true"
		if !*rq.Preallocated {
			val = "false"
		}
		conds = append(conds, &cond.Where{SQL: fmt.Sprintf("data->>'PreAllocated' = '%s'", val)})
	}
	if rq.NotAllocated != nil && *rq.NotAllocated {
		conds = append(conds, &cond.Where{SQL: "(data->'Allocation' IS NULL OR data->'Allocation' = 'null'::jsonb)"})
	}
	if rq.Allocation != nil {
		alloc := rq.Allocation

		noSubFilters := alloc.Project == nil && alloc.Uuid == nil && alloc.Name == nil && alloc.Image == nil &&
			alloc.Hostname == nil && alloc.AllocationType == nil && alloc.FilesystemLayout == nil &&
			alloc.Labels == nil && alloc.Vpn == nil
		if noSubFilters {
			conds = append(conds, &cond.Where{SQL: "(data ? 'Allocation' AND data->'Allocation' IS NOT NULL AND data->'Allocation' != 'null'::jsonb)"})
		}

		if alloc.Project != nil {
			conds = append(conds, nestedFieldEq("Allocation", "Project", *alloc.Project))
		}
		if alloc.Uuid != nil {
			conds = append(conds, nestedFieldEq("Allocation", "UUID", *alloc.Uuid))
		}
		if alloc.Name != nil {
			conds = append(conds, nestedFieldEq("Allocation", "Name", *alloc.Name))
		}
		if alloc.Image != nil {
			conds = append(conds, nestedFieldEq("Allocation", "ImageID", *alloc.Image))
		}
		if alloc.Hostname != nil {
			conds = append(conds, nestedFieldEq("Allocation", "Hostname", *alloc.Hostname))
		}
		if alloc.AllocationType != nil {
				rolePtr, err := enum.GetStringValue(*alloc.AllocationType)
				if err == nil && rolePtr != nil && *rolePtr != "" {
					conds = append(conds, nestedFieldEq("Allocation", "Role", *rolePtr))
				}
			}
		if alloc.FilesystemLayout != nil {
			conds = append(conds, &cond.Where{
				SQL:  fmt.Sprintf("data->'Allocation'->'FilesystemLayout'->>'ID' = $%d", 1),
				Args: []any{*alloc.FilesystemLayout},
			})
		}
		if alloc.Labels != nil {
			for key, value := range alloc.Labels.Labels {
				conds = append(conds, &cond.Where{
					SQL:  fmt.Sprintf("data->'Allocation'->'Labels'->>'%s' = $%d", key, 1),
					Args: []any{value},
				})
			}
		}
		if alloc.Vpn != nil {
			conds = append(conds, &cond.Where{
				SQL: "(data->'Allocation' ? 'VPN' AND data->'Allocation'->'VPN' IS NOT NULL AND data->'Allocation'->'VPN' != 'null'::jsonb)",
			})
		}
	}
	if rq.Network != nil {
		nw := rq.Network
		for _, id := range nw.Networks {
			conds = append(conds, &cond.Where{
				SQL:  fmt.Sprintf("EXISTS (SELECT 1 FROM jsonb_array_elements(data->'Allocation'->'MachineNetworks') elem WHERE elem->>'NetworkID' = $%d)", 1),
				Args: []any{id},
			})
		}
		for _, prefix := range nw.Prefixes {
			conds = append(conds, &cond.Where{
				SQL:  fmt.Sprintf("EXISTS (SELECT 1 FROM jsonb_array_elements(data->'Allocation'->'MachineNetworks') elem WHERE elem @> $%d::jsonb)", 1),
				Args: []any{fmt.Sprintf(`{"Prefixes": ["%s"]}`, prefix)},
			})
		}
		for _, ip := range nw.Ips {
			conds = append(conds, &cond.Where{
				SQL:  fmt.Sprintf("EXISTS (SELECT 1 FROM jsonb_array_elements(data->'Allocation'->'MachineNetworks') elem WHERE elem @> $%d::jsonb)", 1),
				Args: []any{fmt.Sprintf(`{"IPs": ["%s"]}`, ip)},
			})
		}
		for _, destPrefix := range nw.DestinationPrefixes {
			conds = append(conds, &cond.Where{
				SQL:  fmt.Sprintf("EXISTS (SELECT 1 FROM jsonb_array_elements(data->'Allocation'->'MachineNetworks') elem WHERE elem @> $%d::jsonb)", 1),
				Args: []any{fmt.Sprintf(`{"DestinationPrefixes": ["%s"]}`, destPrefix)},
			})
		}
		for _, vrf := range nw.Vrfs {
			conds = append(conds, &cond.Where{
				SQL:  fmt.Sprintf("EXISTS (SELECT 1 FROM jsonb_array_elements(data->'Allocation'->'MachineNetworks') elem WHERE (elem->>'Vrf')::int = $%d)", 1),
				Args: []any{vrf},
			})
		}
		for _, asn := range nw.Asns {
			conds = append(conds, &cond.Where{
				SQL:  fmt.Sprintf("EXISTS (SELECT 1 FROM jsonb_array_elements(data->'Allocation'->'MachineNetworks') elem WHERE (elem->>'ASN')::int = $%d)", 1),
				Args: []any{asn},
			})
		}
	}
	if rq.Hardware != nil {
		hw := rq.Hardware
		if hw.Memory != nil {
			conds = append(conds, &cond.Where{
				SQL:  fmt.Sprintf("(data->'Hardware'->>'Memory')::float8 = $%d", 1),
				Args: []any{float64(*hw.Memory)},
			})
		}
		if hw.CpuCores != nil {
			conds = append(conds, &cond.Where{
				SQL:  fmt.Sprintf("(SELECT COALESCE(SUM((elem->>'Cores')::int), 0) FROM jsonb_array_elements(data->'Hardware'->'MetalCPUs') elem) = $%d", 1),
				Args: []any{*hw.CpuCores},
			})
		}
	}
	if rq.Nic != nil {
		nic := rq.Nic
		for _, mac := range nic.Macs {
			conds = append(conds, arrayElemFieldEq("Hardware", "Nics", "MacAddress", mac))
		}
		for _, name := range nic.Names {
			conds = append(conds, arrayElemFieldEq("Hardware", "Nics", "Name", name))
		}
		for _, mac := range nic.NeighborMacs {
			conds = append(conds, nestedArrayElemFieldEq("Hardware", "Nics", "Neighbors", "MacAddress", mac))
		}
		for _, name := range nic.NeighborNames {
			conds = append(conds, nestedArrayElemFieldEq("Hardware", "Nics", "Neighbors", "Name", name))
		}
	}
	if rq.Disk != nil {
		disk := rq.Disk
		for _, name := range disk.Names {
			conds = append(conds, arrayElemFieldEq("Hardware", "Disks", "Name", name))
		}
		for _, size := range disk.Sizes {
			conds = append(conds, &cond.Where{
				SQL:  fmt.Sprintf("EXISTS (SELECT 1 FROM jsonb_array_elements(data->'Hardware'->'Disks') elem WHERE (elem->>'Size')::int = $%d)", 1),
				Args: []any{size},
			})
		}
	}
	if rq.State != nil {
			statePtr, err := enum.GetStringValue(rq.State)
			if err != nil {
				return cond.And(conds...)
			}
			stateStr := ""
			if statePtr != nil {
				stateStr = *statePtr
			}
			if *rq.State == apiv2.MachineState_MACHINE_STATE_AVAILABLE {
				stateStr = ""
			}
			if stateStr == "" {
				conds = append(conds, stateEqAvailable())
			} else {
				conds = append(conds, &cond.Where{
					SQL:  fmt.Sprintf("UPPER(data->'State'->>'Value') = $%d", 1),
					Args: []any{strings.ToUpper(stateStr)},
				})
			}
	}
	if rq.Bmc != nil {
		bmc := rq.Bmc
		if bmc.Address != nil {
			conds = append(conds, nestedFieldEq("IPMI", "Address", *bmc.Address))
		}
		if bmc.Mac != nil {
			conds = append(conds, nestedFieldEq("IPMI", "MacAddress", *bmc.Mac))
		}
		if bmc.User != nil {
			conds = append(conds, nestedFieldEq("IPMI", "User", *bmc.User))
		}
		if bmc.Interface != nil {
			conds = append(conds, nestedFieldEq("IPMI", "Interface", *bmc.Interface))
		}
	}
	if rq.Fru != nil {
		fru := rq.Fru
		if fru.ChassisPartNumber != nil {
			conds = append(conds, fruFieldEq("ChassisPartNumber", *fru.ChassisPartNumber))
		}
		if fru.ChassisPartSerial != nil {
			conds = append(conds, fruFieldEq("ChassisPartSerial", *fru.ChassisPartSerial))
		}
		if fru.BoardMfg != nil {
			conds = append(conds, fruFieldEq("BoardMfg", *fru.BoardMfg))
		}
		if fru.BoardSerial != nil {
			conds = append(conds, fruFieldEq("BoardMfgSerial", *fru.BoardSerial))
		}
		if fru.BoardPartNumber != nil {
			conds = append(conds, fruFieldEq("BoardPartNumber", *fru.BoardPartNumber))
		}
		if fru.ProductManufacturer != nil {
			conds = append(conds, fruFieldEq("ProductManufacturer", *fru.ProductManufacturer))
		}
		if fru.ProductPartNumber != nil {
			conds = append(conds, fruFieldEq("ProductPartNumber", *fru.ProductPartNumber))
		}
		if fru.ProductSerial != nil {
			conds = append(conds, fruFieldEq("ProductSerial", *fru.ProductSerial))
		}
	}

	return cond.And(conds...)
}

func nestedFieldEq(parent, field, value string) *cond.Where {
	return &cond.Where{
		SQL:  fmt.Sprintf("data->'%s'->>'%s' = $%d", parent, field, 1),
		Args: []any{value},
	}
}

func fruFieldEq(field, value string) *cond.Where {
	return &cond.Where{
		SQL:  fmt.Sprintf("data->'IPMI'->'Fru'->>'%s' = $%d", field, 1),
		Args: []any{value},
	}
}

func arrayElemFieldEq(parent, array, field, value string) *cond.Where {
	return &cond.Where{
		SQL:  fmt.Sprintf("EXISTS (SELECT 1 FROM jsonb_array_elements(data->'%s'->'%s') elem WHERE elem->>'%s' = $%d)", parent, array, field, 1),
		Args: []any{value},
	}
}

func nestedArrayElemFieldEq(parent, array, nestedArray, field, value string) *cond.Where {
	return &cond.Where{
		SQL: fmt.Sprintf("EXISTS (SELECT 1 FROM jsonb_array_elements(data->'%s'->'%s') elem, jsonb_array_elements(elem->'%s') nelem WHERE nelem->>'%s' = $%d)", parent, array, nestedArray, field, 1),
		Args: []any{value},
	}
}

func stateEqAvailable() *cond.Where {
	return &cond.Where{
		SQL: "(data->'State' IS NULL OR data->'State'->>'Value' = '' OR data->'State'->>'Value' IS NULL)",
	}
}
