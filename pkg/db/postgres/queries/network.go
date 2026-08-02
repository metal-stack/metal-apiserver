package queries

import (
	"net/netip"
	"strconv"
	"strings"

	"github.com/metal-stack/api/go/enum"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
)

// NetworkFilter builds an in-memory filter for Network entities from the given query.
func NetworkFilter(rq *apiv2.NetworkQuery) func(map[string]any) bool {
	if rq == nil {
		return nil
	}
	return func(data map[string]any) bool {
		if rq.Project != nil {
			if data["ProjectID"] != *rq.Project {
				return false
			}
		}
		if rq.Id != nil {
			if data["ID"] != *rq.Id {
				return false
			}
		}
		if rq.Name != nil {
			if data["Name"] != *rq.Name {
				return false
			}
		}
		if rq.Namespace != nil {
			v, _ := data["Namespace"].(*string)
			if v == nil || *v != *rq.Namespace {
				return false
			}
		}
		if rq.Description != nil {
			if data["Description"] != *rq.Description {
				return false
			}
		}
		if rq.Partition != nil {
			if data["PartitionID"] != *rq.Partition {
				return false
			}
		}
		if rq.ParentNetwork != nil {
			if data["ParentNetworkID"] != *rq.ParentNetwork {
				return false
			}
		}
		if rq.Vrf != nil {
			vrf, ok := data["Vrf"].(float64)
			if !ok || uint32(vrf) != *rq.Vrf {
				return false
			}
		}
		if rq.Labels != nil {
			labels, _ := data["Labels"].(map[string]any)
			for key, value := range rq.Labels.Labels {
				if labels == nil || labels[key] != value {
					return false
				}
			}
		}
		if rq.Type != nil {
			stringPtr, err := enum.GetStringValue(rq.Type)
			if err == nil && stringPtr != nil && data["NetworkType"] != *stringPtr {
				return false
			}
		}
		if rq.NatType != nil {
			nt, err := metal.ToNATType(*rq.NatType)
			if err == nil && data["NATType"] != string(nt) {
				return false
			}
		}
		for _, prefix := range rq.Prefixes {
			pfx := netip.MustParsePrefix(prefix)
			ip := pfx.Addr().String()
			length := strconv.Itoa(pfx.Bits())

			prefixes, _ := data["Prefixes"].([]any)
			if !hasPrefix(prefixes, ip, length) {
				return false
			}
		}
		for _, destPrefix := range rq.DestinationPrefixes {
			pfx := netip.MustParsePrefix(destPrefix)
			ip := pfx.Addr().String()
			length := strconv.Itoa(pfx.Bits())

			destPrefixes, _ := data["DestinationPrefixes"].([]any)
			if !hasPrefix(destPrefixes, ip, length) {
				return false
			}
		}
		if rq.AddressFamily != nil {
			prefixes, _ := data["Prefixes"].([]any)
			switch rq.AddressFamily.String() {
			case apiv2.NetworkAddressFamily_NETWORK_ADDRESS_FAMILY_V4.String():
				if !hasPrefixWithSeparator(prefixes, ".") {
					return false
				}
			case apiv2.NetworkAddressFamily_NETWORK_ADDRESS_FAMILY_V6.String():
				if !hasPrefixWithSeparator(prefixes, ":") {
					return false
				}
			case apiv2.NetworkAddressFamily_NETWORK_ADDRESS_FAMILY_DUAL_STACK.String():
				if !hasPrefixWithSeparator(prefixes, ".") || !hasPrefixWithSeparator(prefixes, ":") {
					return false
				}
			}
		}
		return true
	}
}

func hasPrefix(prefixes []any, ip, length string) bool {
	for _, p := range prefixes {
		if pm, ok := p.(map[string]any); ok {
			if toString(pm["IP"]) == ip && toString(pm["Length"]) == length {
				return true
			}
		}
	}
	return false
}

func hasPrefixWithSeparator(prefixes []any, sep string) bool {
	for _, p := range prefixes {
		if pm, ok := p.(map[string]any); ok {
			if ip, ok := pm["IP"].(string); ok && strings.Contains(ip, sep) {
				return true
			}
		}
	}
	return false
}
