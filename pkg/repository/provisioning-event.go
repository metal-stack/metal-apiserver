package repository

import (
	"context"

	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	infrav2 "github.com/metal-stack/api/go/metalstack/infra/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
)

type (
	provisioningEventRepository struct {
		s *Store
	}
	ProvisioningEventQuery struct {
		MachineId *string
	}

	EventServiceSendRequest struct {
		*infrav2.EventServiceSendRequest
	}
)

func (*EventServiceSendRequest) GetUpdateMeta() *apiv2.UpdateMeta {
	return &apiv2.UpdateMeta{}
}

func (r *provisioningEventRepository) validateCreate(ctx context.Context, req *infrav2.EventServiceSendRequest) error {
	panic("unimplemented")
}

func (r *provisioningEventRepository) validateUpdate(ctx context.Context, req *EventServiceSendRequest, e *metal.ProvisioningEventContainer) error {
	panic("unimplemented")
}

func (r *provisioningEventRepository) validateDelete(ctx context.Context, req *metal.ProvisioningEventContainer) error {
	panic("unimplemented")
}

// Create implements ProvisioningEvent.
func (r *provisioningEventRepository) create(ctx context.Context, c *infrav2.EventServiceSendRequest) (*metal.ProvisioningEventContainer, error) {
	panic("unimplemented")
}

// Update implements ProvisioningEvent.
func (r *provisioningEventRepository) update(ctx context.Context, e *metal.ProvisioningEventContainer, u *EventServiceSendRequest) (*metal.ProvisioningEventContainer, error) {
	panic("unimplemented")
}

// Get implements ProvisioningEvent.
func (r *provisioningEventRepository) get(ctx context.Context, id string) (*metal.ProvisioningEventContainer, error) {
	panic("unimplemented")
}

// Delete implements ProvisioningEvent.
func (r *provisioningEventRepository) delete(ctx context.Context, e *metal.ProvisioningEventContainer) error {
	panic("unimplemented")
}

// Find implements ProvisioningEvent.
func (r *provisioningEventRepository) find(ctx context.Context, query *ProvisioningEventQuery) (*metal.ProvisioningEventContainer, error) {
	panic("unimplemented")
}

// List implements ProvisioningEvent.
func (r *provisioningEventRepository) list(ctx context.Context, query *ProvisioningEventQuery) ([]*metal.ProvisioningEventContainer, error) {
	panic("unimplemented")
}

func (r *provisioningEventRepository) matchScope(_ *metal.ProvisioningEventContainer) bool {
	// Is not project scoped
	return true
}

// ConvertToInternal implements ProvisioningEvent.
func (r *provisioningEventRepository) convertToInternal(ctx context.Context, msg *apiv2.MachineProvisioningEvent) (*metal.ProvisioningEventContainer, error) {
	panic("unimplemented")
}

// ConvertToProto implements ProvisioningEvent.
func (r *provisioningEventRepository) convertToProto(ctx context.Context, e *metal.ProvisioningEventContainer) (*apiv2.MachineProvisioningEvent, error) {
	panic("unimplemented")
}

// AdditionalMethods implements ProvisioningEvent.
func (p *provisioningEventRepository) AdditionalMethods() *provisioningEventRepository {
	panic("unimplemented")
}
