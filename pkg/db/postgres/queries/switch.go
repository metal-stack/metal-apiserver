package queries

import (
	"github.com/metal-stack/api/go/enum"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
)

// SwitchFilter builds an in-memory filter for Switch entities from the given query.
func SwitchFilter(query *apiv2.SwitchQuery) func(map[string]any) bool {
	if query == nil {
		return nil
	}
	return func(data map[string]any) bool {
		if query.Id != nil {
			if data["ID"] != *query.Id {
				return false
			}
		}
		if query.Partition != nil {
			if data["Partition"] != *query.Partition {
				return false
			}
		}
		if query.Rack != nil {
			if data["Rack"] != *query.Rack {
				return false
			}
		}
		if query.Room != nil {
			if data["Room"] != *query.Room {
				return false
			}
		}
		if query.Os != nil {
			osMap, _ := data["OS"].(map[string]any)
			if osMap == nil {
				return false
			}
			if query.Os.Vendor != nil {
				vendorPtr, err := enum.GetStringValue(query.Os.Vendor)
				if err == nil && vendorPtr != nil && osMap["Vendor"] != *vendorPtr {
					return false
				}
			}
			if query.Os.Version != nil {
				if osMap["Version"] != *query.Os.Version {
					return false
				}
			}
		}
		if query.ConnectedMachineId != nil {
			mc, _ := data["MachineConnections"].(map[string]any)
			if mc == nil || mc[*query.ConnectedMachineId] == nil {
				return false
			}
		}
		return true
	}
}
