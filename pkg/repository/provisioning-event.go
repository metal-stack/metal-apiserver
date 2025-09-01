package repository

import (
	"context"

	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	infrav2 "github.com/metal-stack/api/go/metalstack/infra/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
)

type provisioningEventRepository struct {
	s *Store
}

func (r *provisioningEventRepository) create(ctx context.Context, c *infrav2.EventServiceSendRequest) (*metal.ProvisioningEventContainer, error) {
	panic("unimplemented")
}
func (r *provisioningEventRepository) get(ctx context.Context, id string) (*metal.ProvisioningEventContainer, error) {
	panic("unimplemented")
}
func (r *provisioningEventRepository) update(ctx context.Context, id string, u *infrav2.EventServiceSendRequest) (*metal.ProvisioningEventContainer, error) {
	panic("unimplemented")
}
func (r *provisioningEventRepository) delete(ctx context.Context, id string) (*metal.ProvisioningEventContainer, error) {
	panic("unimplemented")
}
func (r *provisioningEventRepository) find(ctx context.Context, query any) (*metal.ProvisioningEventContainer, error) {
	panic("unimplemented")
}
func (r *provisioningEventRepository) list(ctx context.Context, query any) ([]*metal.ProvisioningEventContainer, error) {
	panic("unimplemented")
}
func (r *provisioningEventRepository) additionalMethods() *provisioningEventRepository {
	panic("unimplemented")
}
func (r *provisioningEventRepository) convertToInternal(msg *apiv2.MachineProvisioningEvent) (*metal.ProvisioningEventContainer, error) {
	panic("unimplemented")
}
func (r *provisioningEventRepository) convertToProto(e *metal.ProvisioningEventContainer) (*apiv2.MachineProvisioningEvent, error) {
	panic("unimplemented")
}
