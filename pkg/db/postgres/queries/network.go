package queries

import (
	"fmt"
	"net/netip"
	"strconv"

	"github.com/metal-stack/api/go/enum"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/postgres/cond"
)

// NetworkFilter builds a JSONB query condition for Network entities from the given query.
func NetworkFilter(rq *apiv2.NetworkQuery) *cond.Where {
	if rq == nil {
		return nil
	}
	var conds []*cond.Where

	if rq.Project != nil {
		conds = append(conds, cond.FieldEq("ProjectID", *rq.Project))
	}
	if rq.Id != nil {
		conds = append(conds, cond.FieldEq("ID", *rq.Id))
	}
	if rq.Name != nil {
		conds = append(conds, cond.FieldEq("Name", *rq.Name))
	}
	if rq.Namespace != nil {
		conds = append(conds, &cond.Where{
			SQL:  fmt.Sprintf("data->>'Namespace' = $%d", 1),
			Args: []any{*rq.Namespace},
		})
	}
	if rq.Description != nil {
		conds = append(conds, cond.FieldEq("Description", *rq.Description))
	}
	if rq.Partition != nil {
		conds = append(conds, cond.FieldEq("PartitionID", *rq.Partition))
	}
	if rq.ParentNetwork != nil {
		conds = append(conds, cond.FieldEq("ParentNetworkID", *rq.ParentNetwork))
	}
	if rq.Vrf != nil {
		conds = append(conds, cond.FieldEqInt("Vrf", int(*rq.Vrf)))
	}
	if rq.Labels != nil {
		for key, value := range rq.Labels.Labels {
			conds = append(conds, &cond.Where{
				SQL:  fmt.Sprintf("data->'Labels'->>'%s' = $%d", key, 1),
				Args: []any{value},
			})
		}
	}
	if rq.Type != nil {
		typePtr, err := enum.GetStringValue(rq.Type)
		if err == nil && typePtr != nil {
			conds = append(conds, cond.FieldEq("NetworkType", *typePtr))
		}
	}
	if rq.NatType != nil {
		typePtr, err := enum.GetStringValue(rq.NatType)
		if err == nil && typePtr != nil {
			conds = append(conds, cond.FieldEq("NATType", *typePtr))
		}
	}
	for _, prefix := range rq.Prefixes {
		pfx := netip.MustParsePrefix(prefix)
		ip := pfx.Addr().String()
		length := strconv.Itoa(pfx.Bits())
		conds = append(conds, &cond.Where{
			SQL: fmt.Sprintf("EXISTS (SELECT 1 FROM jsonb_array_elements(data->'Prefixes') elem WHERE elem->>'IP' = $%d AND elem->>'Length' = $%d)", 1, 2),
			Args: []any{ip, length},
		})
	}
	for _, destPrefix := range rq.DestinationPrefixes {
		pfx := netip.MustParsePrefix(destPrefix)
		ip := pfx.Addr().String()
		length := strconv.Itoa(pfx.Bits())
		conds = append(conds, &cond.Where{
			SQL: fmt.Sprintf("EXISTS (SELECT 1 FROM jsonb_array_elements(data->'DestinationPrefixes') elem WHERE elem->>'IP' = $%d AND elem->>'Length' = $%d)", 1, 2),
			Args: []any{ip, length},
		})
	}
	if rq.AddressFamily != nil {
		switch rq.AddressFamily.String() {
		case apiv2.NetworkAddressFamily_NETWORK_ADDRESS_FAMILY_V4.String():
			conds = append(conds, &cond.Where{SQL: "EXISTS (SELECT 1 FROM jsonb_array_elements(data->'Prefixes') elem WHERE elem->>'IP' ~ '\\.')"})
		case apiv2.NetworkAddressFamily_NETWORK_ADDRESS_FAMILY_V6.String():
			conds = append(conds, &cond.Where{SQL: "EXISTS (SELECT 1 FROM jsonb_array_elements(data->'Prefixes') elem WHERE elem->>'IP' ~ ':')"})
		case apiv2.NetworkAddressFamily_NETWORK_ADDRESS_FAMILY_DUAL_STACK.String():
			conds = append(conds, &cond.Where{SQL: "EXISTS (SELECT 1 FROM jsonb_array_elements(data->'Prefixes') elem WHERE elem->>'IP' ~ '\\.') AND EXISTS (SELECT 1 FROM jsonb_array_elements(data->'Prefixes') elem WHERE elem->>'IP' ~ ':')"})
		}
	}

	return cond.And(conds...)
}
