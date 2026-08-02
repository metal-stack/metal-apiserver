package queries

import (
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
)

// SizeFilter builds an in-memory filter for Size entities from the given query.
func SizeFilter(rq *apiv2.SizeQuery) func(map[string]any) bool {
	if rq == nil {
		return nil
	}
	return func(data map[string]any) bool {
		if rq.Id != nil {
			if data["ID"] != *rq.Id {
				return false
			}
		}
		if rq.Description != nil {
			if data["Description"] != *rq.Description {
				return false
			}
		}
		if rq.Name != nil {
			if data["Name"] != *rq.Name {
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
		return true
	}
}
