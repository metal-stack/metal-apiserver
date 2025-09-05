package errorutil

import (
	"errors"
	"fmt"
	"strings"

	"connectrpc.com/connect"
	"google.golang.org/grpc/status"

	"github.com/google/go-cmp/cmp"
)

// Convert compares the error and maps it to a appropriate connect.Error
// if there is no backend-specific wrapped error given to this function, it will just return an internal error.
// if there are multiple errors wrapped this function only cares about the first found error in the error tree.
func Convert(err error) *connect.Error {
	// Ipam or other connect errors
	var connectErr *connect.Error
	if errors.As(err, &connectErr) {
		// when the connect error is wrapped deeper a tree, connect.Error() calls the string function on
		// the error and adds things like "internal: ..."
		// so we replace the wrapped error message with the direct message
		cleaned := strings.Replace(err.Error(), connectErr.Error(), connectErr.Message(), 1)
		return connect.NewError(connectErr.Code(), errors.New(cleaned))
	}

	// for masterdata-api or other pure grpc apis
	if _, ok := status.FromError(err); ok {
		// when the grpc error is wrapped deeper a tree, status.FromError calls the string function on
		// the error and adds "rpc error: ..."
		// so we unwrap the grpc status on our own and replace the error message with the direct message
		type grpcstatus interface{ GRPCStatus() *status.Status }
		var (
			iterErr = err
		)

		for {
			st, ok := iterErr.(grpcstatus)
			if ok {
				cleaned := strings.Replace(err.Error(), st.GRPCStatus().String(), st.GRPCStatus().Message(), 1)
				return connect.NewError(connect.Code(st.GRPCStatus().Code()), errors.New(cleaned))
			}

			iterErr = errors.Unwrap(iterErr)
			if iterErr == nil {
				// just a theoretical case
				return connect.NewError(connect.CodeUnknown, err)
			}
		}
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

// FailedPrecondition creates a new FailedPrecondition error with a given error message and the original error.
func FailedPrecondition(format string, args ...any) error {
	return connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf(format, args...))
}

// Unauthenticated creates a new Unauthenticated error with a given error message and the original error.
func Unauthenticated(format string, args ...any) error {
	return connect.NewError(connect.CodeUnauthenticated, fmt.Errorf(format, args...))
}

// ResourceExhausted creates a new ResourceExhausted error with a given error message and the original error.
func ResourceExhausted(format string, args ...any) error {
	return connect.NewError(connect.CodeResourceExhausted, fmt.Errorf(format, args...))
}

// PermissionDenied creates a new PermissionDenied error with a given error message and the original error.
func PermissionDenied(format string, args ...any) error {
	return connect.NewError(connect.CodePermissionDenied, fmt.Errorf(format, args...))
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

// NewFailedPrecondition creates a new FailedPrecondition error with a given error message and the original error.
func NewFailedPrecondition(err error) error {
	return connect.NewError(connect.CodeFailedPrecondition, err)
}

// NewUnauthenticated creates a new Unauthenticated error with a given error message and the original error.
func NewUnauthenticated(err error) error {
	return connect.NewError(connect.CodeUnauthenticated, err)
}

// NewResourceExhausted creates a new ResourceExhausted error with a given error message and the original error.
func NewResourceExhausted(err error) error {
	return connect.NewError(connect.CodeResourceExhausted, err)
}

// NewPermissionDenied creates a new PermissionDenied error with a given error message and the original error.
func NewPermissionDenied(err error) error {
	return connect.NewError(connect.CodePermissionDenied, err)
}

func IsNotFound(err error) bool {
	connectErr := Convert(err)
	return connectErr.Code() == connect.CodeNotFound
}

func IsConflict(err error) bool {
	connectErr := Convert(err)
	return connectErr.Code() == connect.CodeAlreadyExists
}

func IsInternal(err error) bool {
	connectErr := Convert(err)
	return connectErr.Code() == connect.CodeInternal
}

func IsInvalidArgument(err error) bool {
	connectErr := Convert(err)
	return connectErr.Code() == connect.CodeInvalidArgument
}

func IsFailedPrecondition(err error) bool {
	connectErr := Convert(err)
	return connectErr.Code() == connect.CodeFailedPrecondition
}

func IsUnauthenticated(err error) bool {
	connectErr := Convert(err)
	return connectErr.Code() == connect.CodeUnauthenticated
}

func IsResourceExhausted(err error) bool {
	connectErr := Convert(err)
	return connectErr.Code() == connect.CodeResourceExhausted
}
func IsPermissionDenied(err error) bool {
	connectErr := Convert(err)
	return connectErr.Code() == connect.CodePermissionDenied
}

func ErrorComparer() cmp.Option {
	return cmp.Comparer(func(x, y error) bool {
		return errorsAreEqual(x, y)
	})
}

func ConnectErrorComparer() cmp.Option {
	return cmp.Comparer(func(x, y *connect.Error) bool {
		return errorsAreEqual(x, y) && x.Code() == y.Code()
	})
}

func errorsAreEqual(x, y error) bool {
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
	return true
}
