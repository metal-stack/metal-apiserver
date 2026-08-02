package queries

import (
	"fmt"

	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/postgres/cond"
)

// EventFilter builds a JSONB query condition for ProvisioningEventContainer entities.
func EventFilter(machineID string) *cond.Where {
	if machineID == "" {
		return nil
	}
	return cond.FieldEq("ID", machineID)
}

// PartitionFilter builds a JSONB query condition for Partition entities.
func PartitionFilter(rq *apiv2.PartitionQuery) *cond.Where {
	if rq == nil {
		return nil
	}
	var conds []*cond.Where
	if rq.Id != nil {
		conds = append(conds, cond.FieldEq("ID", *rq.Id))
	}
	return cond.And(conds...)
}

// FileSystemLayoutFilter builds a JSONB query condition for FilesystemLayout entities.
func FileSystemLayoutFilter(rq *apiv2.FilesystemServiceListRequest) *cond.Where {
	if rq == nil {
		return nil
	}
	var conds []*cond.Where
	if rq.Id != nil {
		conds = append(conds, cond.FieldEq("ID", *rq.Id))
	}
	return cond.And(conds...)
}

// SizeImageConstraintFilter builds a JSONB query condition for SizeImageConstraint entities.
func SizeImageConstraintFilter(rq *apiv2.SizeImageConstraintQuery) *cond.Where {
	if rq == nil {
		return nil
	}
	var conds []*cond.Where

	if rq.Size != nil {
		conds = append(conds, cond.FieldEq("ID", *rq.Size))
	}
	if rq.Name != nil {
		conds = append(conds, cond.FieldEq("Name", *rq.Name))
	}
	if rq.Description != nil {
		conds = append(conds, cond.FieldEq("Description", *rq.Description))
	}

	return cond.And(conds...)
}

// SizeReservationFilter builds a JSONB query condition for SizeReservation entities.
func SizeReservationFilter(rq *apiv2.SizeReservationQuery) *cond.Where {
	if rq == nil {
		return nil
	}
	var conds []*cond.Where

	if rq.Id != nil {
		conds = append(conds, cond.FieldEq("ID", *rq.Id))
	}
	if rq.Size != nil {
		conds = append(conds, cond.FieldEq("SizeID", *rq.Size))
	}
	if rq.Name != nil {
		conds = append(conds, cond.FieldEq("Name", *rq.Name))
	}
	if rq.Description != nil {
		conds = append(conds, cond.FieldEq("Description", *rq.Description))
	}
	if rq.Labels != nil {
		for key, value := range rq.Labels.Labels {
			conds = append(conds, &cond.Where{
				SQL:  fmt.Sprintf("data->'Labels'->>'%s' = $%d", escapeJSONKey(key), 1),
				Args: []any{value},
			})
		}
	}
	if rq.Project != nil {
		conds = append(conds, cond.FieldEq("ProjectID", *rq.Project))
	}
	if rq.Partition != nil {
		conds = append(conds, &cond.Where{
			SQL:  fmt.Sprintf("EXISTS (SELECT 1 FROM jsonb_array_elements_text(data->'PartitionIDs') elem WHERE elem = $%d)", 1),
			Args: []any{*rq.Partition},
		})
	}

	return cond.And(conds...)
}
