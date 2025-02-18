package repository

import (
	"context"

	"github.com/metal-stack/api-server/pkg/db/generic"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	mdcv1 "github.com/metal-stack/masterdata-api/api/v1"
)

type projectRepository struct {
	r *Repostore
}

func (r *projectRepository) Get(ctx context.Context, id string) (*mdcv1.Project, error) {
	resp, err := r.r.mdc.Project().Get(ctx, &mdcv1.ProjectGetRequest{Id: id})
	if err != nil {
		// FIXME check for notfound
		return nil, err
	}
	if resp.Project == nil || resp.Project.Meta == nil {
		return nil, generic.NotFound("error retrieving project %q", id)
	}
	return resp.Project, nil
}

func (r *projectRepository) Create(ctx context.Context, e *apiv2.ProjectServiceCreateRequest) (*mdcv1.Project, error) {
	panic("unimplemented")
}
func (r *projectRepository) Update(ctx context.Context, msg *apiv2.ProjectServiceUpdateRequest) (*mdcv1.Project, error) {
	panic("unimplemented")
}
func (r *projectRepository) Delete(ctx context.Context, e *mdcv1.Project) (*mdcv1.Project, error) {
	panic("unimplemented")
}
func (r *projectRepository) Find(ctx context.Context, query *apiv2.ProjectServiceListRequest) (*mdcv1.Project, error) {
	panic("unimplemented")
}
func (r *projectRepository) List(ctx context.Context, query *apiv2.ProjectServiceListRequest) ([]*mdcv1.Project, error) {
	panic("unimplemented")
}
func (r *projectRepository) ConvertToInternal(msg *apiv2.Project) (*mdcv1.Project, error) {
	panic("unimplemented")
}
func (r *projectRepository) ConvertToProto(e *mdcv1.Project) (*apiv2.Project, error) {
	panic("unimplemented")
}
