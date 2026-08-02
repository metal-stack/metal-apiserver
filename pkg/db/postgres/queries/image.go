package queries

import (
	"fmt"

	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/postgres/cond"
)

// ImageFilter builds a JSONB query condition for Image entities from the given query.
func ImageFilter(rq *apiv2.ImageQuery) *cond.Where {
	if rq == nil {
		return nil
	}
	var conds []*cond.Where

	if rq.Id != nil {
		conds = append(conds, cond.FieldEq("ID", *rq.Id))
	}
	if rq.Os != nil {
		conds = append(conds, cond.FieldEq("OS", *rq.Os))
	}
	if rq.Version != nil {
		conds = append(conds, cond.FieldEq("Version", *rq.Version))
	}
	if rq.Name != nil {
		conds = append(conds, cond.FieldEq("Name", *rq.Name))
	}
	if rq.Description != nil {
		conds = append(conds, cond.FieldEq("Description", *rq.Description))
	}
	if rq.Feature != nil {
		featureStr, err := enumGetStringValue(*rq.Feature)
		if err == nil && featureStr != "" {
			conds = append(conds, &cond.Where{
				SQL:  fmt.Sprintf("data->'Features' ? '%s'", escapeJSONKey(featureStr)),
				Args: nil,
			})
		}
	}
	if rq.Classification != nil {
		classStr, err := enumGetStringValue(*rq.Classification)
		if err == nil && classStr != "" {
			conds = append(conds, cond.FieldEq("Classification", classStr))
		}
	}
	if rq.Labels != nil {
		for key, value := range rq.Labels.Labels {
			conds = append(conds, &cond.Where{
				SQL:  fmt.Sprintf("data->'Labels'->>'%s' = $%d", escapeJSONKey(key), 1),
				Args: []any{value},
			})
		}
	}

	return cond.And(conds...)
}
