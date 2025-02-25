package errorutil

import (
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"google.golang.org/grpc/status"

	mdcv1 "github.com/metal-stack/masterdata-api/api/v1"
)

// TODO: find a more generic impl.

var (
	errNotFound        = errors.New("NotFound")
	errConflict        = errors.New("Conflict")
	errInternal        = errors.New("Internal")
	errInvalidArgument = errors.New("InvalidArgument")
)

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

	return connect.NewError(connect.CodeInternal, err)
}

// NotFound creates a new notfound error with a given error message.
func NotFound(format string, args ...interface{}) error {
	return fmt.Errorf("%w %s", errNotFound, fmt.Sprintf(format, args...))
}

// Conflict creates a new conflict error with a given error message.
func Conflict(format string, args ...interface{}) error {
	return fmt.Errorf("%w %s", errConflict, fmt.Sprintf(format, args...))
}

// Internal creates a new Internal error with a given error message and the original error.
func Internal(format string, args ...interface{}) error {
	return fmt.Errorf("%w %s", errInternal, fmt.Sprintf(format, args...))
}

// InvalidArgument creates a new InvalidArgument error with a given error message and the original error.
func InvalidArgument(format string, args ...interface{}) error {
	return fmt.Errorf("%w %s", errInvalidArgument, fmt.Sprintf(format, args...))
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

// IsNotFound compares the given error if it is a NotFound and returns true, otherwise false
func notFound(err error) *connect.Error {

	// Ipam or other connect error
	var connectErr *connect.Error
	if errors.As(err, &connectErr) {
		if connectErr.Code() == connect.CodeNotFound {
			return connectErr
		}
	}

	// RethinkDB Error
	if errors.Is(err, errNotFound) {
		return connect.NewError(connect.CodeNotFound, err)
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

	// RethinkDB Error
	if errors.Is(err, errConflict) {
		return connect.NewError(connect.CodeAlreadyExists, err)
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

	// RethinkDB Error
	if errors.Is(err, errInternal) {
		return connect.NewError(connect.CodeInternal, err)
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

	//RethinkDB error
	if errors.Is(err, errInvalidArgument) {
		return connect.NewError(connect.CodeInvalidArgument, err)
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
