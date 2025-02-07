package repository

import (
	"context"

	"github.com/metal-stack/api-server/pkg/db/generic"
	mdcv1 "github.com/metal-stack/masterdata-api/api/v1"
	mdm "github.com/metal-stack/masterdata-api/pkg/client"
)

type projectRepository struct {
	r   *Repository
	mdc mdm.Client
}

func (r *projectRepository) Get(ctx context.Context, id string) (*mdcv1.Project, error) {
	resp, err := r.mdc.Project().Get(ctx, &mdcv1.ProjectGetRequest{Id: id})
	if err != nil {
		// FIXME check for notfound
		return nil, err
	}
	if resp.Project == nil || resp.Project.Meta == nil {
		return nil, generic.NotFound("error retrieving project %q", id)
	}
	return resp.Project, nil
}
