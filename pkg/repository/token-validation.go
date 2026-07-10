package repository

import (
	"context"

	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/repository/api"
)

func (t *tokenRepository) validateCreate(ctx context.Context, req *adminv2.TokenServiceCreateRequest) error {
	panic("unimplemented")
}

func (t *tokenRepository) validateUpdate(ctx context.Context, req *apiv2.TokenServiceUpdateRequest, tokenToUpdate *api.TokenWithSecret) error {
	panic("unimplemented")
}

func (t *tokenRepository) validateDelete(ctx context.Context, req *api.TokenWithSecret) error {
	// token scope match is already checked before this func
	// apart from this a token can always be revoked
	return nil
}
