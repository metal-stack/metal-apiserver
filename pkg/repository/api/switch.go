package api

import apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"

type (
	SwitchServiceCreateRequest struct {
		Switch *apiv2.Switch
	}

	SwitchStatus struct {
		ID            string
		LastSync      *apiv2.SwitchSync
		LastSyncError *apiv2.SwitchSync
	}
)

func (s *SwitchStatus) GetID() string {
	return s.ID
}
