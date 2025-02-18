package repository

import (
	"context"

	"github.com/metal-stack/api-server/pkg/db/metal"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
)

type ipUnscopedRepository struct {
	r *Repostore
}

func (ur *ipUnscopedRepository) Get(ctx context.Context, id string) (*metal.IP, error) {
	panic("unimplemented")
}
func (ur *ipUnscopedRepository) Create(ctx context.Context, e *apiv2.IPServiceCreateRequest) (*metal.IP, error) {
	panic("unimplemented")
}
func (ur *ipUnscopedRepository) Update(ctx context.Context, msg *apiv2.IPServiceUpdateRequest) (*metal.IP, error) {
	panic("unimplemented")
}
func (ur *ipUnscopedRepository) Delete(ctx context.Context, e *metal.IP) (*metal.IP, error) {
	panic("unimplemented")
}
func (ur *ipUnscopedRepository) Find(ctx context.Context, query *apiv2.IPServiceListRequest) (*metal.IP, error) {
	panic("unimplemented")
}
func (ur *ipUnscopedRepository) List(ctx context.Context, query *apiv2.IPServiceListRequest) ([]*metal.IP, error) {
	return ur.r.ds.IP().List(ctx)
}
func (ur *ipUnscopedRepository) ConvertToInternal(msg *apiv2.IP) (*metal.IP, error) {
	return ur.r.IP(ProjectScope("")).ConvertToInternal(msg)
}
func (ur *ipUnscopedRepository) ConvertToProto(e *metal.IP) (*apiv2.IP, error) {
	return ur.r.IP(ProjectScope("")).ConvertToProto(e)
}
