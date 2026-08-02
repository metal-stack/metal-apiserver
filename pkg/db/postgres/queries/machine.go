package queries

import (
	"fmt"
	"strings"

	"github.com/metal-stack/api/go/enum"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
)

// MachineFilter builds an in-memory filter for Machine entities from the given query.
func MachineFilter(rq *apiv2.MachineQuery) func(map[string]any) bool {
	if rq == nil {
		return nil
	}
	return func(data map[string]any) bool {
		if rq.Uuid != nil {
			if data["ID"] != *rq.Uuid {
				return false
			}
		}
		if rq.Name != nil {
			if data["Name"] != *rq.Name {
				return false
			}
		}
		if rq.Partition != nil {
			if data["PartitionID"] != *rq.Partition {
				return false
			}
		}
		if rq.Size != nil {
			if data["SizeID"] != *rq.Size {
				return false
			}
		}
		if rq.Rack != nil {
			if data["RackID"] != *rq.Rack {
				return false
			}
		}
		if rq.Room != nil {
			if data["RoomID"] != *rq.Room {
				return false
			}
		}
		if rq.Labels != nil {
			tags, _ := data["Tags"].([]any)
			for key, value := range rq.Labels.Labels {
				tagStr := fmt.Sprintf("%s=%s", key, value)
				if !containsAny(tags, tagStr) {
					return false
				}
			}
		}
		if rq.Waiting != nil {
			if data["Waiting"] != *rq.Waiting {
				return false
			}
		}
		if rq.Preallocated != nil {
			if data["PreAllocated"] != *rq.Preallocated {
				return false
			}
		}
		if rq.NotAllocated != nil && *rq.NotAllocated {
			alloc, has := data["Allocation"]
			if !has || alloc != nil {
				return false
			}
		}
		if rq.Allocation != nil {
			alloc := rq.Allocation
			allocMap, _ := data["Allocation"].(map[string]any)
			if allocMap == nil {
				return false
			}

			if alloc.Project != nil {
				if allocMap["Project"] != *alloc.Project {
					return false
				}
			}
			if alloc.Uuid != nil {
				if allocMap["UUID"] != *alloc.Uuid {
					return false
				}
			}
			if alloc.Name != nil {
				if allocMap["Name"] != *alloc.Name {
					return false
				}
			}
			if alloc.Image != nil {
				if allocMap["ImageID"] != *alloc.Image {
					return false
				}
			}
			if alloc.Hostname != nil {
				if allocMap["Hostname"] != *alloc.Hostname {
					return false
				}
			}
			if alloc.AllocationType != nil {
				rolePtr, err := enum.GetStringValue(*alloc.AllocationType)
				if err == nil && rolePtr != nil && allocMap["Role"] != *rolePtr {
					return false
				}
			}
			if alloc.FilesystemLayout != nil {
				fsl, _ := allocMap["FilesystemLayout"].(map[string]any)
				if fsl == nil || fsl["ID"] != *alloc.FilesystemLayout {
					return false
				}
			}
			if alloc.Labels != nil {
				labels, _ := allocMap["Labels"].(map[string]any)
				for key, value := range alloc.Labels.Labels {
					if labels == nil || labels[key] != value {
						return false
					}
				}
			}
			if alloc.Vpn != nil {
				_, has := allocMap["VPN"]
				if !has {
					return false
				}
			}
		}
		if rq.Network != nil {
			nw := rq.Network
			allocation, _ := data["Allocation"].(map[string]any)
			networks, _ := allocation["Networks"].([]any)

			for _, id := range nw.Networks {
				if !hasNetworkStrField(networks, "NetworkID", id) {
					return false
				}
			}
			for _, prefix := range nw.Prefixes {
				if !networkContainsAny(networks, "Prefixes", prefix) {
					return false
				}
			}
			for _, ip := range nw.Ips {
				if !networkContainsAny(networks, "IPs", ip) {
					return false
				}
			}
			for _, destPrefix := range nw.DestinationPrefixes {
				if !networkContainsAny(networks, "DestinationPrefixes", destPrefix) {
					return false
				}
			}
			for _, vrf := range nw.Vrfs {
				if !hasNetworkFloatField(networks, "Vrf", float64(vrf)) {
					return false
				}
			}
			for _, asn := range nw.Asns {
				if !hasNetworkFloatField(networks, "ASN", float64(asn)) {
					return false
				}
			}
		}
		if rq.Hardware != nil {
			hw := rq.Hardware
			hwMap, _ := data["Hardware"].(map[string]any)
			if hwMap == nil {
				if hw.Memory != nil || hw.CpuCores != nil {
					return false
				}
			} else {
				if hw.Memory != nil {
					mem, ok := hwMap["Memory"].(float64)
					if !ok || uint64(mem) != *hw.Memory {
						return false
					}
				}
				if hw.CpuCores != nil {
					cpus, _ := hwMap["Cpus"].([]any)
					total := uint64(0)
					for _, cpu := range cpus {
						if cpuMap, ok := cpu.(map[string]any); ok {
							if cores, ok := cpuMap["Cores"].(float64); ok {
								total += uint64(cores)
							}
						}
					}
					if uint32(total) != *hw.CpuCores {
						return false
					}
				}
			}
		}
		if rq.Nic != nil {
			nic := rq.Nic
			hwMap, _ := data["Hardware"].(map[string]any)
			nics, _ := hwMap["Nics"].([]any)

			for _, mac := range nic.Macs {
				if !nicHasField(nics, "MacAddress", mac) {
					return false
				}
			}
			for _, name := range nic.Names {
				if !nicHasField(nics, "Name", name) {
					return false
				}
			}
			for _, mac := range nic.NeighborMacs {
				if !nicHasNeighborField(nics, "MacAddress", mac) {
					return false
				}
			}
			for _, name := range nic.NeighborNames {
				if !nicHasNeighborField(nics, "Name", name) {
					return false
				}
			}
		}
		if rq.Disk != nil {
			disk := rq.Disk
			hwMap, _ := data["Hardware"].(map[string]any)
			blockDevices, _ := hwMap["Disks"].([]any)

			for _, name := range disk.Names {
				if !blockDeviceHasStrField(blockDevices, "Name", name) {
					return false
				}
			}
			for _, size := range disk.Sizes {
				if !blockDeviceHasFloatField(blockDevices, "Size", float64(size)) {
					return false
				}
			}
		}
		if rq.State != nil {
			statePtr, err := enum.GetStringValue(rq.State)
			if err != nil {
				return false
			}
			stateString := ""
			if statePtr != nil {
				stateString = *statePtr
			}
			if *rq.State == apiv2.MachineState_MACHINE_STATE_AVAILABLE {
				stateString = ""
			}
			stateMap, _ := data["State"].(map[string]any)
			if stateString == "" {
				if stateMap != nil {
					if toString(stateMap["Value"]) != "" {
						return false
					}
				}
			} else {
				if stateMap == nil {
					return false
				}
				if !strings.EqualFold(toString(stateMap["Value"]), strings.ToUpper(stateString)) {
					return false
				}
			}
		}
		if rq.Bmc != nil {
			bmc := rq.Bmc
			ipmi, _ := data["IPMI"].(map[string]any)
			if ipmi == nil {
				return false
			}
			if bmc.Address != nil && ipmi["Address"] != *bmc.Address {
				return false
			}
			if bmc.Mac != nil && ipmi["MacAddress"] != *bmc.Mac {
				return false
			}
			if bmc.User != nil && ipmi["User"] != *bmc.User {
				return false
			}
			if bmc.Interface != nil && ipmi["Interface"] != *bmc.Interface {
				return false
			}
		}
		if rq.Fru != nil {
			fru := rq.Fru
			ipmi, _ := data["IPMI"].(map[string]any)
			if ipmi == nil {
				return false
			}
			fruMap, _ := ipmi["Fru"].(map[string]any)
			if fruMap == nil {
				return false
			}
			if fru.ChassisPartNumber != nil && fruMap["ChassisPartNumber"] != *fru.ChassisPartNumber {
				return false
			}
			if fru.ChassisPartSerial != nil && fruMap["ChassisPartSerial"] != *fru.ChassisPartSerial {
				return false
			}
			if fru.BoardMfg != nil && fruMap["BoardMfg"] != *fru.BoardMfg {
				return false
			}
			if fru.BoardSerial != nil && fruMap["BoardMfgSerial"] != *fru.BoardSerial {
				return false
			}
			if fru.BoardPartNumber != nil && fruMap["BoardPartNumber"] != *fru.BoardPartNumber {
				return false
			}
			if fru.ProductManufacturer != nil && fruMap["ProductManufacturer"] != *fru.ProductManufacturer {
				return false
			}
			if fru.ProductPartNumber != nil && fruMap["ProductPartNumber"] != *fru.ProductPartNumber {
				return false
			}
			if fru.ProductSerial != nil && fruMap["ProductSerial"] != *fru.ProductSerial {
				return false
			}
		}
		return true
	}
}

func hasNetworkStrField(networks []any, field, val string) bool {
	for _, n := range networks {
		if nm, ok := n.(map[string]any); ok {
			if toString(nm[field]) == val {
				return true
			}
		}
	}
	return false
}

func hasNetworkFloatField(networks []any, field string, val float64) bool {
	for _, n := range networks {
		if nm, ok := n.(map[string]any); ok {
			if v, ok := nm[field].(float64); ok && v == val {
				return true
			}
		}
	}
	return false
}

func networkContainsAny(networks []any, field, val string) bool {
	for _, n := range networks {
		if nm, ok := n.(map[string]any); ok {
			items, _ := nm[field].([]any)
			for _, item := range items {
				if toString(item) == val {
					return true
				}
			}
		}
	}
	return false
}

func nicHasField(nics []any, field, val string) bool {
	for _, n := range nics {
		if nm, ok := n.(map[string]any); ok {
			if toString(nm[field]) == val {
				return true
			}
		}
	}
	return false
}

func nicHasNeighborField(nics []any, field, val string) bool {
	for _, n := range nics {
		if nm, ok := n.(map[string]any); ok {
			neighbors, _ := nm["Neighbors"].([]any)
			for _, neigh := range neighbors {
				if ne, ok := neigh.(map[string]any); ok {
					if toString(ne[field]) == val {
						return true
					}
				}
			}
		}
	}
	return false
}

func blockDeviceHasStrField(devices []any, field, val string) bool {
	for _, d := range devices {
		if dm, ok := d.(map[string]any); ok {
			if toString(dm[field]) == val {
				return true
			}
		}
	}
	return false
}

func blockDeviceHasFloatField(devices []any, field string, val float64) bool {
	for _, d := range devices {
		if dm, ok := d.(map[string]any); ok {
			if v, ok := dm[field].(float64); ok && v == val {
				return true
			}
		}
	}
	return false
}
