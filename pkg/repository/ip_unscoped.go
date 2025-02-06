package repository

import (
	"context"

	"github.com/metal-stack/api-server/pkg/db/metal"
)

type ipUnscopedRepository struct {
	r *Repository
}

func (ur *ipUnscopedRepository) List(ctx context.Context) ([]*metal.IP, error) {
	return ur.r.ds.IP().List(ctx)
}
