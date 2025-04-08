package errorutil

import (
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"google.golang.org/grpc/status"

	"github.com/google/go-cmp/cmp"
	mdcv1 "github.com/metal-stack/masterdata-api/api/v1"
)

// TODO: find a more generic impl.

// Convert compares the error and maps it to a appropriate connect.Error
func Convert(err error) *connect.Error {
	if err := notFound(err); err != nil {
		return err
	}
	if err := conflict(err); err != nil {
		return err
	}
	if err := invalidArgument(err); err != nil {
		return err
	}
	if err := internal(err); err != nil {
		return err
	}
	if err := unauthenticated(err); err != nil {
		return err
	}

	return connect.NewError(connect.CodeInternal, err)
}

// NotFound creates a new notfound error with a given error message.
func NotFound(format string, args ...any) error {
	return connect.NewError(connect.CodeNotFound, fmt.Errorf(format, args...))
}

// Conflict creates a new conflict error with a given error message.
func Conflict(format string, args ...any) error {
	return connect.NewError(connect.CodeAlreadyExists, fmt.Errorf(format, args...))
}

// Internal creates a new Internal error with a given error message and the original error.
func Internal(format string, args ...any) error {
	return connect.NewError(connect.CodeInternal, fmt.Errorf(format, args...))
}

// InvalidArgument creates a new InvalidArgument error with a given error message and the original error.
func InvalidArgument(format string, args ...any) error {
	return connect.NewError(connect.CodeInvalidArgument, fmt.Errorf(format, args...))
}

// Unauthenticated creates a new Unauthenticated error with a given error message and the original error.
func Unauthenticated(format string, args ...any) error {
	return connect.NewError(connect.CodeUnauthenticated, fmt.Errorf(format, args...))
}

// NewNotFound creates a new notfound error with a given error message.
func NewNotFound(err error) error {
	return connect.NewError(connect.CodeNotFound, err)
}

// NewConflict creates a new conflict error with a given error message.
func NewConflict(err error) error {
	return connect.NewError(connect.CodeAlreadyExists, err)
}

// NewInternal creates a new Internal error with a given error message and the original error.
func NewInternal(err error) error {
	return connect.NewError(connect.CodeInternal, err)
}

// NewInvalidArgument creates a new InvalidArgument error with a given error message and the original error.
func NewInvalidArgument(err error) error {
	return connect.NewError(connect.CodeInvalidArgument, err)
}

// NewUnauthenticated creates a new Unauthenticated error with a given error message and the original error.
func NewUnauthenticated(err error) error {
	return connect.NewError(connect.CodeUnauthenticated, err)
}

func NewFailedPrecondition(err error) error {
	return connect.NewError(connect.CodeFailedPrecondition, err)
}

func IsNotFound(err error) bool {
	e := notFound(err)
	return e != nil
}
func IsConflict(err error) bool {
	e := conflict(err)
	return e != nil
}
func IsInternal(err error) bool {
	e := internal(err)
	return e != nil
}
func IsInvalidArgument(err error) bool {
	e := invalidArgument(err)
	return e != nil
}
func IsUnauthenticated(err error) bool {
	e := unauthenticated(err)
	return e != nil
}

// IsNotFound compares the given error if it is a NotFound and returns true, otherwise false
func notFound(err error) *connect.Error {

	// Ipam or other connect error
	var connectErr *connect.Error
	if errors.As(err, &connectErr) {
		if connectErr.Code() == connect.CodeNotFound {
			return connectErr
		}
	}

	// Masterdata Error
	if mdcv1.IsNotFound(err) {
		st, ok := status.FromError(err)
		if ok {
			return connect.NewError(connect.CodeNotFound, errors.New(st.Message()))
		}
		return connect.NewError(connect.CodeNotFound, err)
	}

	return nil
}

// IsConflict compares the given error if it is a Conflict and returns true, otherwise false
func conflict(err error) *connect.Error {

	// Ipam or other connect error
	var connectErr *connect.Error
	if errors.As(err, &connectErr) {
		if connectErr.Code() == connect.CodeAlreadyExists {
			return connectErr
		}
	}

	// Masterdata Error
	if mdcv1.IsConflict(err) {
		st, ok := status.FromError(err)
		if ok {
			return connect.NewError(connect.CodeAlreadyExists, errors.New(st.Message()))
		}
		return connect.NewError(connect.CodeAlreadyExists, err)
	}

	return nil
}

// IsInternal compares the given error if it is a InternalServer and returns true, otherwise false
func internal(err error) *connect.Error {

	// Ipam or other connect error
	var connectErr *connect.Error
	if errors.As(err, &connectErr) {
		if connectErr.Code() == connect.CodeInternal {
			return connectErr
		}
	}

	// Masterdata Error
	if mdcv1.IsInternal(err) {
		st, ok := status.FromError(err)
		if ok {
			return connect.NewError(connect.CodeInternal, errors.New(st.Message()))
		}
		return connect.NewError(connect.CodeInternal, err)
	}

	return nil
}

// IsInvalidArgument compares the given error if it is a InvalidArgument and returns true, otherwise false
func invalidArgument(err error) *connect.Error {

	// Ipam or other connect error
	var connectErr *connect.Error
	if errors.As(err, &connectErr) {
		if connectErr.Code() == connect.CodeInvalidArgument {
			return connectErr
		}
	}

	// Masterdata Error
	if mdcv1.IsOptimistickLockError(err) {
		st, ok := status.FromError(err)
		if ok {
			return connect.NewError(connect.CodeInvalidArgument, errors.New(st.Message()))
		}
		return connect.NewError(connect.CodeInvalidArgument, err)
	}

	return nil
}

// unauthenticated compares the given error if it is a unauthenticated and returns true, otherwise false
func unauthenticated(err error) *connect.Error {

	// Ipam or other connect error
	var connectErr *connect.Error
	if errors.As(err, &connectErr) {
		if connectErr.Code() == connect.CodeUnauthenticated {
			return connectErr
		}
	}

	return nil
}

func ConnectErrorComparer() cmp.Option {
	return cmp.Comparer(func(x, y *connect.Error) bool {
		if x == nil && y == nil {
			return true
		}
		if x == nil && y != nil {
			return false
		}
		if x != nil && y == nil {
			return false
		}
		if x.Error() != y.Error() {
			return false
		}
		if x.Code() != y.Code() {
			return false
		}
		return true
	})
}
