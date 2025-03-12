package repository

import (
	"context"

	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	mdcv1 "github.com/metal-stack/masterdata-api/api/v1"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
)

type projectRepository struct {
	r     *Store
	scope *ProjectScope
}

func (r *projectRepository) ValidateCreate(ctx context.Context, req *apiv2.ProjectServiceCreateRequest) (*Validated[*apiv2.ProjectServiceCreateRequest], error) {
	return &Validated[*apiv2.ProjectServiceCreateRequest]{
		message: req,
	}, nil
}

func (r *projectRepository) ValidateUpdate(ctx context.Context, req *apiv2.ProjectServiceUpdateRequest) (*Validated[*apiv2.ProjectServiceUpdateRequest], error) {
	return &Validated[*apiv2.ProjectServiceUpdateRequest]{
		message: req,
	}, nil
}

func (r *projectRepository) ValidateDelete(ctx context.Context, req *mdcv1.Project) (*Validated[*mdcv1.Project], error) {
	return &Validated[*mdcv1.Project]{
		message: req,
	}, nil
}

func (r *projectRepository) Get(ctx context.Context, id string) (*mdcv1.Project, error) {
	resp, err := r.r.mdc.Project().Get(ctx, &mdcv1.ProjectGetRequest{Id: id})
	if err != nil {
		// FIXME check for notfound
		return nil, err
	}
	if resp.Project == nil || resp.Project.Meta == nil {
		return nil, errorutil.NotFound("error retrieving project %q", id)
	}
	err = r.MatchScope(resp.Project)
	if err != nil {
		return nil, err
	}

	return resp.Project, nil
}

func (r *projectRepository) MatchScope(p *mdcv1.Project) error {
	if r.scope == nil {
		return nil
	}
	if r.scope.projectID == p.Meta.Id {
		return nil
	}
	return errorutil.NotFound("project:%s not found", p.Meta.Id)
}

func (r *projectRepository) Create(ctx context.Context, e *Validated[*apiv2.ProjectServiceCreateRequest]) (*mdcv1.Project, error) {
	panic("unimplemented")
}
func (r *projectRepository) Update(ctx context.Context, msg *Validated[*apiv2.ProjectServiceUpdateRequest]) (*mdcv1.Project, error) {
	panic("unimplemented")
}
func (r *projectRepository) Delete(ctx context.Context, e *Validated[*mdcv1.Project]) (*mdcv1.Project, error) {
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
