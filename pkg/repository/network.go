package repository

import (
	"context"

	"github.com/metal-stack/api-server/pkg/db/generic"
	"github.com/metal-stack/api-server/pkg/db/metal"
)

type networkRepository struct {
	r     *Repository
	scope ProjectScope
}

func (r *networkRepository) Get(ctx context.Context, id string) (*metal.Network, error) {
	nw, err := r.r.ds.Network().Get(ctx, id)
	if err != nil {
		return nil, err
	}

	if r.scope != ProjectScope(nw.ProjectID) {
		return nil, generic.NotFound("network with id:%s not found", id)
	}

	return nw, nil
}

func (r *networkRepository) Delete(ctx context.Context, id string) (*metal.Network, error) {
	nw, err := r.Get(ctx, id)
	if err != nil {
		return nil, err
	}

	// FIXME delete in ipam with the help of Tx

	err = r.r.ds.Network().Delete(ctx, nw)
	if err != nil {
		return nil, err
	}

	return nw, nil
}
