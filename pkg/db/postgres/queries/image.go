package queries

import (
	"github.com/metal-stack/api/go/enum"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
)

// ImageFilter builds an in-memory filter for Image entities from the given query.
func ImageFilter(rq *apiv2.ImageQuery) func(map[string]any) bool {
	if rq == nil {
		return nil
	}
	return func(data map[string]any) bool {
		if rq.Id != nil {
			if data["ID"] != *rq.Id {
				return false
			}
		}
		if rq.Os != nil {
			if data["OS"] != *rq.Os {
				return false
			}
		}
		if rq.Version != nil {
			if data["Version"] != *rq.Version {
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
		if rq.Feature != nil {
			featurePtr, err := enum.GetStringValue(*rq.Feature)
			if err == nil && featurePtr != nil {
				features, _ := data["Features"].(map[string]any)
				if features == nil || features[*featurePtr] == nil {
					return false
				}
			}
		}
		if rq.Classification != nil {
			classPtr, err := enum.GetStringValue(*rq.Classification)
			if err == nil && classPtr != nil && data["Classification"] != *classPtr {
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
