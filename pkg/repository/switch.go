package repository

import (
	"context"

	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	infrav2 "github.com/metal-stack/api/go/metalstack/infra/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
)

type switchRepository struct {
	s *Store
}

func (r *switchRepository) get(ctx context.Context, id string) (*metal.Switch, error) {
	panic("unimplemented")
}

func (r *switchRepository) validateCreate(ctx context.Context, req *infrav2.SwitchServiceCreateRequest) error {
	panic("unimplemented")
}

func (r *switchRepository) create(ctx context.Context, req *infrav2.SwitchServiceCreateRequest) (*metal.Switch, error) {
	panic("unimplemented")
}

func (r *switchRepository) validateUpdate(ctx context.Context, req *adminv2.SwitchServiceUpdateRequest, oldSwitch *metal.Switch) error {
	panic("unimplemented")
}

func (r *switchRepository) update(ctx context.Context, oldSwitch *metal.Switch, req *adminv2.SwitchServiceUpdateRequest) (*metal.Switch, error) {
	panic("unimplemented")
}

func (r *switchRepository) validateDelete(ctx context.Context, sw *metal.Switch) error {
	panic("unimplemented")
}

func (r *switchRepository) delete(ctx context.Context, sw *metal.Switch) error {
	panic("unimplemented")
}

func (r *switchRepository) find(ctx context.Context, query *apiv2.SwitchQuery) (*metal.Switch, error) {
	panic("unimplemented")
}

func (r *switchRepository) list(ctx context.Context, query *apiv2.SwitchQuery) ([]*metal.Switch, error) {
	panic("unimplemented")
}

func (r *switchRepository) convertToInternal(sw *apiv2.Switch) (*metal.Switch, error) {
	panic("unimplemented")
}

func (r *switchRepository) convertToProto(sw *metal.Switch) (*apiv2.Switch, error) {
	panic("unimplemented")
}

func (r *switchRepository) matchScope(sw *metal.Switch) bool {
	panic("unimplemented")
}
