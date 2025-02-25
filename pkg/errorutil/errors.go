package errorutil

import (
	"errors"

	"connectrpc.com/connect"
	"github.com/metal-stack/api-server/pkg/db/generic"

	mdcv1 "github.com/metal-stack/masterdata-api/api/v1"
)

// IsNotFound compares the given error if it is a NotFound and returns true, otherwise false
func IsNotFound(err error) bool {

	// Ipam or other connect error
	var connectErr *connect.Error
	if errors.As(err, &connectErr) {
		if connectErr.Code() == connect.CodeNotFound {
			return true
		}
	}

	// RethinkDB Error
	if generic.IsNotFound(err) {
		return true
	}

	// Masterdata Error
	if mdcv1.IsNotFound(err) {
		return true
	}

	return false
}

// IsConflict compares the given error if it is a Conflict and returns true, otherwise false
func IsConflict(err error) bool {

	// Ipam or other connect error
	var connectErr *connect.Error
	if errors.As(err, &connectErr) {
		if connectErr.Code() == connect.CodeAlreadyExists {
			return true
		}
	}

	// RethinkDB Error
	if generic.IsConflict(err) {
		return true
	}

	// Masterdata Error
	if mdcv1.IsConflict(err) {
		return true
	}

	return false
}

// IsInternal compares the given error if it is a InternalServer and returns true, otherwise false
func IsInternal(err error) bool {

	// Ipam or other connect error
	var connectErr *connect.Error
	if errors.As(err, &connectErr) {
		if connectErr.Code() == connect.CodeInternal {
			return true
		}
	}

	// RethinkDB Error
	if generic.IsInternal(err) {
		return true
	}

	// Masterdata Error
	if mdcv1.IsInternal(err) {
		return true
	}

	return false
}

// IsInvalidArgument compares the given error if it is a InvalidArgument and returns true, otherwise false
func IsInvalidArgument(err error) bool {

	// Ipam or other connect error
	var connectErr *connect.Error
	if errors.As(err, &connectErr) {
		if connectErr.Code() == connect.CodeInvalidArgument {
			return true
		}
	}

	//RethinkDB error
	if generic.IsInvalidArgument(err) {
		return true
	}

	// Masterdata Error
	if mdcv1.IsOptimistickLockError(err) {
		return true
	}

	return false
}
