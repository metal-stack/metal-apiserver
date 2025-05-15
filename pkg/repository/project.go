package repository

import (
	"context"
	"errors"
	"strconv"

	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	mdcv1 "github.com/metal-stack/masterdata-api/api/v1"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
)

// FIXME completely untested and incomplete

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
		return nil, errorutil.Convert(err)
	}
	if resp.Project == nil || resp.Project.Meta == nil {
		return nil, errorutil.NotFound("error retrieving project %q", id)
	}
	err = r.MatchScope(resp.Project)
	if err != nil {
		return nil, errorutil.Convert(err)
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

	// FIXME howto set the avatarurl during create ??
	project := &mdcv1.Project{
		Meta: &mdcv1.Meta{
			Id: e.message.Name,
		},
		Name:        e.message.Name,
		Description: e.message.Description,
		TenantId:    e.message.Login,
	}

	resp, err := r.r.mdc.Project().Create(ctx, &mdcv1.ProjectCreateRequest{Project: project})
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return resp.Project, nil
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
func (r *projectRepository) ConvertToInternal(p *apiv2.Project) (*mdcv1.Project, error) {
	meta := &mdcv1.Meta{
		Id:          p.Uuid,
		CreatedTime: p.Meta.CreatedAt,
		UpdatedTime: p.Meta.UpdatedAt,
	}
	if p.AvatarUrl != nil {
		meta.Annotations["avatarUrl"] = *p.AvatarUrl
	}
	return &mdcv1.Project{
		Meta:        meta,
		Name:        p.Name,
		Description: p.Description,
		TenantId:    p.Tenant,
	}, nil
}

// FIXME copied over from pkg/project/project.go
// remove there once all services are converted to repo
const (
	defaultProjectAnnotation = "metal-stack.io/default-project"
	avatarURLAnnotation      = "avatarUrl"
)

func (r *projectRepository) ConvertToProto(p *mdcv1.Project) (*apiv2.Project, error) {
	if p.Meta == nil {
		return nil, errors.New("project meta is nil")
	}
	avatarUrl := p.Meta.Annotations[avatarURLAnnotation]

	return &apiv2.Project{
		Uuid:        p.Meta.Id,
		Name:        p.Name,
		Description: p.Description,
		Tenant:      p.TenantId,
		Meta: &apiv2.Meta{
			CreatedAt: p.Meta.CreatedTime,
			UpdatedAt: p.Meta.UpdatedTime,
		},
		AvatarUrl: &avatarUrl,
	}, nil

}
func isDefaultProject(p *mdcv1.Project) bool {
	value, ok := p.Meta.Annotations[defaultProjectAnnotation]
	if !ok {
		return false
	}

	res, err := strconv.ParseBool(value)
	if err != nil {
		return false
	}

	return res
}
