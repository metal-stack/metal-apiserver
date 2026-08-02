package queries

import (
	"fmt"
	"strings"

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
		conds = append(conds, cond.FieldIsNull("Allocation"))
	}
	if rq.Allocation != nil {
		alloc := rq.Allocation
		if alloc.Project != nil {
			conds = append(conds, cond.NestedFieldEq("Allocation", "Project", *alloc.Project))
		}
		if alloc.Uuid != nil {
			conds = append(conds, cond.NestedFieldEq("Allocation", "UUID", *alloc.Uuid))
		}
		if alloc.Name != nil {
			conds = append(conds, cond.NestedFieldEq("Allocation", "Name", *alloc.Name))
		}
		if alloc.Image != nil {
			conds = append(conds, cond.NestedFieldEq("Allocation", "ImageID", *alloc.Image))
		}
		if alloc.Hostname != nil {
			conds = append(conds, cond.NestedFieldEq("Allocation", "Hostname", *alloc.Hostname))
		}
		if alloc.AllocationType != nil {
			roleStr, err := enumGetStringValue(*alloc.AllocationType)
			if err == nil && roleStr != "" {
				conds = append(conds, cond.NestedFieldEq("Allocation", "Role", roleStr))
			}
		}
		if alloc.FilesystemLayout != nil {
			conds = append(conds, cond.NestedFieldEq("Allocation", "FilesystemLayout", *alloc.FilesystemLayout))
		}
		if alloc.Labels != nil {
			for key, value := range alloc.Labels.Labels {
				conds = append(conds, &cond.Where{
					SQL:  fmt.Sprintf("data->'Allocation'->'Labels'->>'%s' = $%d", escapeJSONKey(key), 1),
					Args: []any{value},
				})
			}
		}
		if alloc.Vpn != nil {
			conds = append(conds, cond.NestedFieldHasKey("Allocation", "VPN"))
		}
	}
	if rq.Network != nil {
		nw := rq.Network
		for _, id := range nw.Networks {
			conds = append(conds, cond.NestedArrayFieldEq("Allocation", "Networks", id))
		}
		for _, prefix := range nw.Prefixes {
			conds = append(conds, cond.NestedArrayCheck("Allocation", fmt.Sprintf(`{"Networks": [{"Prefixes": ["%s"]}]}`, escapeJSONString(prefix))))
		}
		for _, ip := range nw.Ips {
			conds = append(conds, cond.NestedArrayCheck("Allocation", fmt.Sprintf(`{"Networks": [{"IPs": ["%s"]}]}`, escapeJSONString(ip))))
		}
		for _, destPrefix := range nw.DestinationPrefixes {
			conds = append(conds, cond.NestedArrayCheck("Allocation", fmt.Sprintf(`{"Networks": [{"DestinationPrefixes": ["%s"]}]}`, escapeJSONString(destPrefix))))
		}
		for _, vrf := range nw.Vrfs {
			conds = append(conds, &cond.Where{
				SQL:  fmt.Sprintf("EXISTS (SELECT 1 FROM jsonb_array_elements(data->'Allocation'->'Networks') elem WHERE elem->>'Vrf' = $%d::text)", 1),
				Args: []any{fmt.Sprintf("%d", vrf)},
			})
		}
		for _, asn := range nw.Asns {
			conds = append(conds, &cond.Where{
				SQL:  fmt.Sprintf("EXISTS (SELECT 1 FROM jsonb_array_elements(data->'Allocation'->'Networks') elem WHERE (elem->>'ASN')::int = $%d)", 1),
				Args: []any{asn},
			})
		}
	}
	if rq.Hardware != nil {
		hw := rq.Hardware
		if hw.Memory != nil {
			conds = append(conds, cond.FieldEqFloat("Memory", float64(*hw.Memory)))
		}
		if hw.CpuCores != nil {
			conds = append(conds, &cond.Where{
				SQL:  fmt.Sprintf("(SELECT SUM((elem->>'Cores')::int) FROM jsonb_array_elements(data->'Hardware'->'Cpus') elem) = $%d", 1),
				Args: []any{*hw.CpuCores},
			})
		}
	}
	if rq.Nic != nil {
		nic := rq.Nic
		for _, mac := range nic.Macs {
			conds = append(conds, &cond.Where{
				SQL:  fmt.Sprintf("EXISTS (SELECT 1 FROM jsonb_array_elements(data->'Hardware'->'Nics') elem WHERE elem->>'MacAddress' = $%d)", 1),
				Args: []any{mac},
			})
		}
		for _, name := range nic.Names {
			conds = append(conds, &cond.Where{
				SQL:  fmt.Sprintf("EXISTS (SELECT 1 FROM jsonb_array_elements(data->'Hardware'->'Nics') elem WHERE elem->>'Name' = $%d)", 1),
				Args: []any{name},
			})
		}
		for _, mac := range nic.NeighborMacs {
			conds = append(conds, &cond.Where{
				SQL:  fmt.Sprintf("EXISTS (SELECT 1 FROM jsonb_array_elements(data->'Hardware'->'Nics') nic, jsonb_array_elements(nic->'Neighbors') neigh WHERE neigh->>'MacAddress' = $%d)", 1),
				Args: []any{mac},
			})
		}
		for _, name := range nic.NeighborNames {
			conds = append(conds, &cond.Where{
				SQL:  fmt.Sprintf("EXISTS (SELECT 1 FROM jsonb_array_elements(data->'Hardware'->'Nics') nic, jsonb_array_elements(nic->'Neighbors') neigh WHERE neigh->>'Name' = $%d)", 1),
				Args: []any{name},
			})
		}
	}
	if rq.Disk != nil {
		disk := rq.Disk
		for _, name := range disk.Names {
			conds = append(conds, &cond.Where{
				SQL:  fmt.Sprintf("EXISTS (SELECT 1 FROM jsonb_array_elements(data->'Hardware'->'Disks') elem WHERE elem->>'Name' = $%d)", 1),
				Args: []any{name},
			})
		}
		for _, size := range disk.Sizes {
			conds = append(conds, &cond.Where{
				SQL:  fmt.Sprintf("EXISTS (SELECT 1 FROM jsonb_array_elements(data->'Hardware'->'Disks') elem WHERE (elem->>'Size')::int = $%d)", 1),
				Args: []any{size},
			})
		}
	}
	if rq.State != nil {
		stateStr, err := enumGetStringValue(*rq.State)
		if err == nil && stateStr != "" {
			if *rq.State == apiv2.MachineState_MACHINE_STATE_AVAILABLE {
				conds = append(conds, &cond.Where{SQL: "(data->'State' IS NULL OR data->'State'->>'Value' = '' OR data->'State'->>'Value' IS NULL)"})
			} else {
				conds = append(conds, &cond.Where{
					SQL:  fmt.Sprintf("UPPER(data->'State'->>'Value') = $%d", 1),
					Args: []any{strings.ToUpper(stateStr)},
				})
			}
		}
	}
	if rq.Bmc != nil {
		bmc := rq.Bmc
		if bmc.Address != nil {
			conds = append(conds, cond.NestedFieldEq("IPMI", "Address", *bmc.Address))
		}
		if bmc.Mac != nil {
			conds = append(conds, cond.NestedFieldEq("IPMI", "MacAddress", *bmc.Mac))
		}
		if bmc.User != nil {
			conds = append(conds, cond.NestedFieldEq("IPMI", "User", *bmc.User))
		}
		if bmc.Interface != nil {
			conds = append(conds, cond.NestedFieldEq("IPMI", "Interface", *bmc.Interface))
		}
	}
	if rq.Fru != nil {
		fru := rq.Fru
		if fru.ChassisPartNumber != nil {
			conds = append(conds, &cond.Where{
				SQL:  fmt.Sprintf("data->'IPMI'->'Fru'->>'ChassisPartNumber' = $%d", 1),
				Args: []any{*fru.ChassisPartNumber},
			})
		}
		if fru.ChassisPartSerial != nil {
			conds = append(conds, &cond.Where{
				SQL:  fmt.Sprintf("data->'IPMI'->'Fru'->>'ChassisPartSerial' = $%d", 1),
				Args: []any{*fru.ChassisPartSerial},
			})
		}
		if fru.BoardMfg != nil {
			conds = append(conds, &cond.Where{
				SQL:  fmt.Sprintf("data->'IPMI'->'Fru'->>'BoardMfg' = $%d", 1),
				Args: []any{*fru.BoardMfg},
			})
		}
		if fru.BoardSerial != nil {
			conds = append(conds, &cond.Where{
				SQL:  fmt.Sprintf("data->'IPMI'->'Fru'->>'BoardMfgSerial' = $%d", 1),
				Args: []any{*fru.BoardSerial},
			})
		}
		if fru.BoardPartNumber != nil {
			conds = append(conds, &cond.Where{
				SQL:  fmt.Sprintf("data->'IPMI'->'Fru'->>'BoardPartNumber' = $%d", 1),
				Args: []any{*fru.BoardPartNumber},
			})
		}
		if fru.ProductManufacturer != nil {
			conds = append(conds, &cond.Where{
				SQL:  fmt.Sprintf("data->'IPMI'->'Fru'->>'ProductManufacturer' = $%d", 1),
				Args: []any{*fru.ProductManufacturer},
			})
		}
		if fru.ProductPartNumber != nil {
			conds = append(conds, &cond.Where{
				SQL:  fmt.Sprintf("data->'IPMI'->'Fru'->>'ProductPartNumber' = $%d", 1),
				Args: []any{*fru.ProductPartNumber},
			})
		}
		if fru.ProductSerial != nil {
			conds = append(conds, &cond.Where{
				SQL:  fmt.Sprintf("data->'IPMI'->'Fru'->>'ProductSerial' = $%d", 1),
				Args: []any{*fru.ProductSerial},
			})
		}
	}

	return cond.And(conds...)
}
