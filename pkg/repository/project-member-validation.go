package repository

import (
	"context"
	"fmt"

	"github.com/metal-stack/api/go/errorutil"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/repository/api"
)

func (t *projectMemberRepository) validateCreate(ctx context.Context, req *apiv2.ProjectMemberServiceCreateRequest) error {
	if req.Role == apiv2.ProjectRole_PROJECT_ROLE_UNSPECIFIED {
		return fmt.Errorf("role must not be %s", req.Role)
	}

	// TODO: this should actually be checked by the tenant-apiserver? if this goes wrong then the state is corrupted and users cannot lookup members anymore
	existing, err := t.get(ctx, req.Member)
	if err != nil && !errorutil.IsNotFound(err) {
		return errorutil.Internal("error looking up tenant member: %w", err)
	}
	if existing != nil {
		return errorutil.Conflict("%q is already member of project %q", req.Member, req.Project)
	}

	return nil
}

func (t *projectMemberRepository) validateUpdate(ctx context.Context, req *apiv2.ProjectMemberServiceUpdateRequest, membership *projectMemberEntity) error {
	// TODO: currently the API defines that only owners can update members so there is no possibility to elevate permissions
	// probably, we should still check that no elevation of permissions is possible in case we later change the API

	if req.Role != nil {
		lastOwner, err := t.checkIfMemberIsLastOwner(ctx, membership)
		if err != nil {
			return errorutil.NewFailedPrecondition(err)
		}

		if lastOwner && *req.Role != apiv2.ProjectRole_PROJECT_ROLE_OWNER {
			return errorutil.FailedPrecondition("cannot demote last owner's permissions")
		}
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

func (t *projectMemberRepository) checkIfMemberIsLastOwner(ctx context.Context, membership *projectMemberEntity) (bool, error) {
	isOwner := membership.Meta.Annotations[api.ProjectRoleAnnotation] == apiv2.ProjectRole_PROJECT_ROLE_OWNER.String()
	if !isOwner {
		return false, nil
	}

	memberships, err := t.list(ctx, &apiv2.ProjectMemberQuery{
		Role: apiv2.ProjectRole_PROJECT_ROLE_OWNER.Enum(),
	})
	if err != nil {
		return false, err
	}

	return len(memberships) < 2, nil
}
