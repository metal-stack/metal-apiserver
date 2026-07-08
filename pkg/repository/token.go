package repository

import (
	"context"

	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/repository/api"
)

type (
	tokenRepository struct {
		s     *Store
		scope *UserScope
		patg  api.ProjectsAndTenantsGetter
	}
)

func (t *tokenRepository) get(ctx context.Context, id string) (*api.TokenWithSecret, error) {
	panic("unimplemented")
}

func (t *tokenRepository) list(ctx context.Context, query *apiv2.TokenQuery) ([]*api.TokenWithSecret, error) {
	panic("unimplemented")
}

func (t *tokenRepository) find(ctx context.Context, query *apiv2.TokenQuery) (*api.TokenWithSecret, error) {
	panic("unimplemented")
}

func (t *tokenRepository) create(ctx context.Context, c *adminv2.TokenServiceCreateRequest) (*api.TokenWithSecret, error) {
	panic("unimplemented")
}

func (t *tokenRepository) delete(ctx context.Context, e *api.TokenWithSecret) (*deleteInfo, error) {
	panic("unimplemented")
}

func (t *tokenRepository) update(ctx context.Context, tok *api.TokenWithSecret, req *apiv2.TokenServiceUpdateRequest) (*api.TokenWithSecret, error) {
	panic("unimplemented")
}

func (t *tokenRepository) matchScope(e *api.TokenWithSecret) bool {
	panic("unimplemented")
}

func (t *tokenRepository) convertToInternal(ctx context.Context, msg *api.TokenWithSecret) (*api.TokenWithSecret, error) {
	return msg, nil
}

func (t *tokenRepository) convertToProto(ctx context.Context, e *api.TokenWithSecret) (*api.TokenWithSecret, error) {
	return e, nil
}
