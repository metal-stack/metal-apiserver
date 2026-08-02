package queries

import (
	"fmt"

	"github.com/metal-stack/api/go/enum"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/postgres/cond"
)

// SwitchFilter builds a JSONB query condition for Switch entities from the given query.
func SwitchFilter(query *apiv2.SwitchQuery) *cond.Where {
	if query == nil {
		return nil
	}
	var conds []*cond.Where

	if query.Id != nil {
		conds = append(conds, cond.FieldEq("ID", *query.Id))
	}
	if query.Partition != nil {
		conds = append(conds, cond.FieldEq("Partition", *query.Partition))
	}
	if query.Rack != nil {
		conds = append(conds, cond.FieldEq("Rack", *query.Rack))
	}
	if query.Room != nil {
		conds = append(conds, cond.FieldEq("Room", *query.Room))
	}
	if query.Os != nil {
		if query.Os.Vendor != nil {
			vendorPtr, err := enum.GetStringValue(query.Os.Vendor)
			if err == nil && vendorPtr != nil {
				conds = append(conds, &cond.Where{
					SQL:  fmt.Sprintf("data->'OS'->>'Vendor' = $%d", 1),
					Args: []any{*vendorPtr},
				})
			}
		}
		if query.Os.Version != nil {
			conds = append(conds, &cond.Where{
				SQL:  fmt.Sprintf("data->'OS'->>'Version' = $%d", 1),
				Args: []any{*query.Os.Version},
			})
		}
	}
	if query.ConnectedMachineId != nil {
		conds = append(conds, &cond.Where{
			SQL: fmt.Sprintf("data->'MachineConnections' ? '%s'", *query.ConnectedMachineId),
		})
	}

	return cond.And(conds...)
}
