package api

import (
	"time"

	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
)

type (
	ComponentServiceCreateRequest struct {
		*apiv2.Component
		Expiration time.Duration
	}

	ComponentServiceUpdateRequest struct {
	}
)

func (*ComponentServiceUpdateRequest) GetUpdateMeta() *apiv2.UpdateMeta {
	return &apiv2.UpdateMeta{}
}
