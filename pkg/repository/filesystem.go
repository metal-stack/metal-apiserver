package repository

import (
	"context"

	"github.com/metal-stack/api-server/pkg/db/metal"
	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
)

type filesystemRepository struct {
	r *Repository
}

func (r *filesystemRepository) Get(ctx context.Context, id string) (*metal.FilesystemLayout, error) {
	fsl, err := r.r.ds.FilesystemLayout().Get(ctx, id)
	if err != nil {
		return nil, err
	}

	return fsl, nil
}

func (r *filesystemRepository) Create(ctx context.Context, rq *metal.FilesystemLayout) (*metal.FilesystemLayout, error) {
	fsl, err := r.r.ds.FilesystemLayout().Create(ctx, rq)
	if err != nil {
		return nil, err
	}

	return fsl, nil
}

func (r *filesystemRepository) Update(ctx context.Context, rq *adminv2.FilesystemServiceUpdateRequest) (*metal.FilesystemLayout, error) {
	old, err := r.Get(ctx, rq.FilesystemLayout.Id)
	if err != nil {
		return nil, err
	}

	new := *old

	// FIXME implement update logic

	err = r.r.ds.FilesystemLayout().Update(ctx, &new, old)
	if err != nil {
		return nil, err
	}

	return &new, nil
}

func (r *filesystemRepository) Delete(ctx context.Context, id string) (*metal.FilesystemLayout, error) {
	fsl, err := r.Get(ctx, id)
	if err != nil {
		return nil, err
	}

	err = r.r.ds.FilesystemLayout().Delete(ctx, fsl)
	if err != nil {
		return nil, err
	}

	return fsl, nil
}

func (r *filesystemRepository) List(ctx context.Context, rq *apiv2.IPServiceListRequest) ([]*metal.FilesystemLayout, error) {
	ip, err := r.r.ds.FilesystemLayout().List(ctx)
	if err != nil {
		return nil, err
	}

	return ip, nil
}
