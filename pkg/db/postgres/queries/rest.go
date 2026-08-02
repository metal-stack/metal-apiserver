package queries

import (
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
)

// EventFilter builds an in-memory filter for ProvisioningEventContainer entities from the given query.
func EventFilter(machineID string) func(map[string]any) bool {
	if machineID == "" {
		return nil
	}
	return func(data map[string]any) bool {
		return data["ID"] == machineID
	}
}

// PartitionFilter builds an in-memory filter for Partition entities from the given query.
func PartitionFilter(rq *apiv2.PartitionQuery) func(map[string]any) bool {
	if rq == nil {
		return nil
	}
	return func(data map[string]any) bool {
		if rq.Id != nil {
			if data["ID"] != *rq.Id {
				return false
			}
		}
		return true
	}
}

// FileSystemLayoutFilter builds an in-memory filter for FilesystemLayout entities from the given query.
func FileSystemLayoutFilter(rq *apiv2.FilesystemServiceListRequest) func(map[string]any) bool {
	if rq == nil {
		return nil
	}
	return func(data map[string]any) bool {
		if rq.Id != nil {
			if data["ID"] != *rq.Id {
				return false
			}
		}
		return true
	}
}

// SizeImageConstraintFilter builds an in-memory filter for SizeImageConstraint entities from the given query.
func SizeImageConstraintFilter(rq *apiv2.SizeImageConstraintQuery) func(map[string]any) bool {
	if rq == nil {
		return nil
	}
	return func(data map[string]any) bool {
		if rq.Size != nil {
			if data["ID"] != *rq.Size {
				return false
			}
		}
		if rq.Name != nil {
			if data["Name"] != *rq.Name {
				return false
			}
		}
		if rq.Description != nil {
			if data["Description"] != *rq.Description {
				return false
			}
		}
		return true
	}
}

// SizeReservationFilter builds an in-memory filter for SizeReservation entities from the given query.
func SizeReservationFilter(rq *apiv2.SizeReservationQuery) func(map[string]any) bool {
	if rq == nil {
		return nil
	}
	return func(data map[string]any) bool {
		if rq.Id != nil {
			if data["ID"] != *rq.Id {
				return false
			}
		}
		if rq.Size != nil {
			if data["SizeID"] != *rq.Size {
				return false
			}
		}
		if rq.Name != nil {
			if data["Name"] != *rq.Name {
				return false
			}
		}
		if rq.Description != nil {
			if data["Description"] != *rq.Description {
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
		if rq.Project != nil {
			if data["ProjectID"] != *rq.Project {
				return false
			}
		}
		if rq.Partition != nil {
			partitionIDs, _ := data["PartitionIDs"].([]any)
			if !containsAny(partitionIDs, *rq.Partition) {
				return false
			}
		}
		return true
	}
}
