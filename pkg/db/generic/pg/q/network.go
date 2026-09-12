package q

import (
	"net/netip"
	"strconv"

	"github.com/metal-stack/api/go/enum"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/generic/pg"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
)

// networkPaths precomputes the JSON paths into a network so the same strings
// are reused across queries instead of being recomputed with reflection.
var networkPaths = struct {
	ID                  string
	Name                string
	Description         string
	PartitionID         string
	ProjectID           string
	Namespace           string
	ParentNetworkID     string
	Vrf                 string
	Labels              string
	NetworkType         string
	NATType             string
	Prefixes            string
	PrefixIP            string
	DestinationPrefixes string
}{
	ID:                  pg.SelectorPath[*metal.Network]("ID"),
	Name:                pg.SelectorPath[*metal.Network]("Name"),
	Description:         pg.SelectorPath[*metal.Network]("Description"),
	PartitionID:         pg.SelectorPath[*metal.Network]("PartitionID"),
	ProjectID:           pg.SelectorPath[*metal.Network]("ProjectID"),
	Namespace:           pg.SelectorPath[*metal.Network]("Namespace"),
	ParentNetworkID:     pg.SelectorPath[*metal.Network]("ParentNetworkID"),
	Vrf:                 pg.SelectorPath[*metal.Network]("Vrf"),
	Labels:              pg.SelectorPath[*metal.Network]("Labels"),
	NetworkType:         pg.SelectorPath[*metal.Network]("NetworkType"),
	NATType:             pg.SelectorPath[*metal.Network]("NATType"),
	Prefixes:            pg.SelectorPath[*metal.Network]("Prefixes"),
	PrefixIP:            "Prefixes.IP",
	DestinationPrefixes: pg.SelectorPath[*metal.Network]("DestinationPrefixes"),
}

// prefixWrap wraps a prefix element object into the Prefixes array for `@>` matching.
func prefixWrap(element map[string]any) map[string]any {
	return containsJSON(networkPaths.Prefixes, []any{element})
}

// destPrefixWrap wraps a prefix element object into the DestinationPrefixes array for `@>` matching.
func destPrefixWrap(element map[string]any) map[string]any {
	return containsJSON(networkPaths.DestinationPrefixes, []any{element})
}

// NetworkFilter converts the given network query into a list of postgres query
// filters. It returns nil when the query is nil.
//
// The filters produced here mirror the RethinkDB NetworkFilter (see the sibling
// `queries` package). Scalar field equality uses the `=` operator; membership
// inside the prefixes/destination-prefixes arrays uses the `@>` containment
// operator; the address-family check uses `ARRAY_ELEM_LIKE` to match a prefix
// whose IP looks like IPv4 (contains a dot) or IPv6 (contains a colon).
func NetworkFilter(rq *apiv2.NetworkQuery) []pg.QueryFilter {
	if rq == nil {
		return nil
	}

	var filters []pg.QueryFilter

	if rq.Project != nil {
		filters = append(filters, pg.QueryFilter{Path: networkPaths.ProjectID, Op: "=", Value: *rq.Project})
	}
	if rq.Id != nil {
		filters = append(filters, pg.QueryFilter{Path: networkPaths.ID, Op: "=", Value: *rq.Id})
	}
	if rq.Name != nil {
		filters = append(filters, pg.QueryFilter{Path: networkPaths.Name, Op: "=", Value: *rq.Name})
	}
	if rq.Namespace != nil {
		filters = append(filters, pg.QueryFilter{Path: networkPaths.Namespace, Op: "=", Value: *rq.Namespace})
	}
	if rq.Description != nil {
		filters = append(filters, pg.QueryFilter{Path: networkPaths.Description, Op: "=", Value: *rq.Description})
	}
	if rq.Partition != nil {
		filters = append(filters, pg.QueryFilter{Path: networkPaths.PartitionID, Op: "=", Value: *rq.Partition})
	}
	if rq.ParentNetwork != nil {
		filters = append(filters, pg.QueryFilter{Path: networkPaths.ParentNetworkID, Op: "=", Value: *rq.ParentNetwork})
	}
	if rq.Vrf != nil {
		filters = append(filters, pg.QueryFilter{Path: networkPaths.Vrf, Op: "=", Value: *rq.Vrf})
	}
	if rq.Labels != nil {
		for key, value := range rq.Labels.Labels {
			filters = append(filters, pg.QueryFilter{Path: networkPaths.Labels, Op: "@>", Value: containsJSON(networkPaths.Labels, map[string]string{key: value})})
		}
	}

	if rq.Type != nil {
		stringValue, err := enum.GetStringValue(rq.Type)
		if err == nil {
			filters = append(filters, pg.QueryFilter{Path: networkPaths.NetworkType, Op: "=", Value: stringValue})
		}
	}

	if rq.NatType != nil {
		nt, err := metal.ToNATType(*rq.NatType)
		if err == nil {
			filters = append(filters, pg.QueryFilter{Path: networkPaths.NATType, Op: "=", Value: string(nt)})
		}
	}

	for _, prefix := range rq.Prefixes {
		pfx := netip.MustParsePrefix(prefix)
		ip := pfx.Addr().String()
		length := strconv.Itoa(pfx.Bits())

		filters = append(filters, pg.QueryFilter{Path: networkPaths.Prefixes, Op: "@>", Value: prefixWrap(map[string]any{"IP": ip})})
		filters = append(filters, pg.QueryFilter{Path: networkPaths.Prefixes, Op: "@>", Value: prefixWrap(map[string]any{"Length": length})})
	}

	for _, destPrefix := range rq.DestinationPrefixes {
		pfx := netip.MustParsePrefix(destPrefix)
		ip := pfx.Addr().String()
		length := strconv.Itoa(pfx.Bits())

		filters = append(filters, pg.QueryFilter{Path: networkPaths.DestinationPrefixes, Op: "@>", Value: destPrefixWrap(map[string]any{"IP": ip})})
		filters = append(filters, pg.QueryFilter{Path: networkPaths.DestinationPrefixes, Op: "@>", Value: destPrefixWrap(map[string]any{"Length": length})})
	}

	if rq.AddressFamily != nil {
		switch rq.AddressFamily.String() {
		case apiv2.NetworkAddressFamily_NETWORK_ADDRESS_FAMILY_V4.String():
			filters = append(filters, pg.QueryFilter{Path: networkPaths.PrefixIP, Op: "ARRAY_ELEM_LIKE", Value: "%.%"})
		case apiv2.NetworkAddressFamily_NETWORK_ADDRESS_FAMILY_V6.String():
			filters = append(filters, pg.QueryFilter{Path: networkPaths.PrefixIP, Op: "ARRAY_ELEM_LIKE", Value: "%:%"})
		case apiv2.NetworkAddressFamily_NETWORK_ADDRESS_FAMILY_DUAL_STACK.String():
			filters = append(filters, pg.QueryFilter{Path: networkPaths.PrefixIP, Op: "ARRAY_ELEM_LIKE", Value: "%.%"})
			filters = append(filters, pg.QueryFilter{Path: networkPaths.PrefixIP, Op: "ARRAY_ELEM_LIKE", Value: "%:%"})
		}
	}

	return filters
}
