package repository

import (
	"context"

	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
)

func (t *projectMemberRepository) validateCreate(ctx context.Context, req *ProjectMemberCreateRequest) error {
	return nil
}

func (t *projectMemberRepository) validateUpdate(ctx context.Context, req *ProjectMemberUpdateRequest, membership *projectMemberEntity) error {
	// TODO: currently the API defines that only owners can update members so there is no possibility to elevate permissions
	// probably, we should still check that no elevation of permissions is possible in case we later change the API

	lastOwner, err := t.checkIfMemberIsLastOwner(ctx, membership)
	if err != nil {
		return errorutil.NewFailedPrecondition(err)
	}

	if lastOwner && req.Role != apiv2.ProjectRole_PROJECT_ROLE_OWNER {
		return errorutil.FailedPrecondition("cannot demote last owner's permissions")
	}

	return nil
}

func (t *projectMemberRepository) validateDelete(ctx context.Context, req *projectMemberEntity) error {
	lastOwner, err := t.checkIfMemberIsLastOwner(ctx, req)
	if err != nil {
		return errorutil.NewFailedPrecondition(err)
	}
	if lastOwner {
		return errorutil.FailedPrecondition("cannot remove last owner of a project")
	}

	return nil
}
