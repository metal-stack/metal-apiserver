package queries

import (
	"fmt"

	"github.com/metal-stack/api/go/enum"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/api/go/tag"
	"github.com/metal-stack/metal-apiserver/pkg/db/postgres/cond"
)

// IpFilter builds a JSONB query condition for IP entities from the given query.
func IpFilter(rq *apiv2.IPQuery) *cond.Where {
	if rq == nil {
		return nil
	}
	var conds []*cond.Where

	if rq.Project != nil {
		conds = append(conds, cond.FieldEq("ProjectID", *rq.Project))
	}
	if rq.Ip != nil {
		conds = append(conds, cond.FieldEq("IPAddress", *rq.Ip))
	}
	if rq.Uuid != nil {
		conds = append(conds, cond.FieldEq("AllocationUUID", *rq.Uuid))
	}
	if rq.Name != nil {
		conds = append(conds, cond.FieldEq("Name", *rq.Name))
	}
	if rq.Namespace != nil {
		conds = append(conds, cond.NestedFieldEq("", "Namespace", *rq.Namespace))
	}
	if rq.Network != nil {
		conds = append(conds, cond.FieldEq("NetworkID", *rq.Network))
	}
	if rq.ParentPrefixCidr != nil {
		conds = append(conds, cond.FieldEq("ParentPrefixCidr", *rq.ParentPrefixCidr))
	}
	if rq.Machine != nil {
		tagStr := fmt.Sprintf("%s=%s", tag.MachineID, *rq.Machine)
		conds = append(conds, cond.TagInSlice(tagStr))
	}
	if rq.Labels != nil {
		for _, value := range rq.Labels.Labels {
			_ = value
		}
		for key, value := range rq.Labels.Labels {
			tagStr := fmt.Sprintf("%s=%s", key, value)
			conds = append(conds, cond.TagInSlice(tagStr))
		}
	}
	if rq.Type != nil {
			typePtr, err := enum.GetStringValue(*rq.Type)
			if err == nil && typePtr != nil {
				conds = append(conds, cond.FieldEq("Type", *typePtr))
			}
		}
	if rq.AddressFamily != nil {
		switch rq.AddressFamily.String() {
		case apiv2.IPAddressFamily_IP_ADDRESS_FAMILY_V4.String():
			conds = append(conds, &cond.Where{
				SQL:  "data->>'IPAddress' ~ '\\\\.'",
				Args: nil,
			})
		case apiv2.IPAddressFamily_IP_ADDRESS_FAMILY_V6.String():
			conds = append(conds, &cond.Where{
				SQL:  "data->>'IPAddress' ~ ':'",
				Args: nil,
			})
		}
	}

	return cond.And(conds...)
}
