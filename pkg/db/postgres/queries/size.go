package queries

import (
	"fmt"

	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/postgres/cond"
)

// SizeFilter builds a JSONB query condition for Size entities from the given query.
func SizeFilter(rq *apiv2.SizeQuery) *cond.Where {
	if rq == nil {
		return nil
	}
	var conds []*cond.Where

	if rq.Id != nil {
		conds = append(conds, cond.FieldEq("ID", *rq.Id))
	}
	if rq.Description != nil {
		conds = append(conds, cond.FieldEq("Description", *rq.Description))
	}
	if rq.Name != nil {
		conds = append(conds, cond.FieldEq("Name", *rq.Name))
	}
	if rq.Labels != nil {
		for key, value := range rq.Labels.Labels {
			conds = append(conds, &cond.Where{
				SQL:  fmt.Sprintf("data->'Labels'->>'%s' = $%d", key, 1),
				Args: []any{value},
			})
		}
	}

	return cond.And(conds...)
}
