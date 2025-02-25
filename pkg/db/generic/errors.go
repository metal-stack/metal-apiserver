package generic

import (
	"errors"
	"fmt"
)

// FIXME decide if this should go into pkg/errorutil

var (
	errNotFound        = errors.New("NotFound")
	errConflict        = errors.New("Conflict")
	errInternal        = errors.New("Internal")
	errInvalidArgument = errors.New("InvalidArgument")
)

// NotFound creates a new notfound error with a given error message.
func NotFound(format string, args ...interface{}) error {
	return fmt.Errorf("%w %s", errNotFound, fmt.Sprintf(format, args...))
}

// IsNotFound checks if an error is a notfound error.
func IsNotFound(e error) bool {
	return errors.Is(e, errNotFound)
}

// Conflict creates a new conflict error with a given error message.
func Conflict(format string, args ...interface{}) error {
	return fmt.Errorf("%w %s", errConflict, fmt.Sprintf(format, args...))
}

// IsConflict checks if an error is a conflict error.
func IsConflict(e error) bool {
	return errors.Is(e, errConflict)
}

// Internal creates a new Internal error with a given error message and the original error.
func Internal(format string, args ...interface{}) error {
	return fmt.Errorf("%w %s", errInternal, fmt.Sprintf(format, args...))
}

// IsInternal checks if an error is a Internal error.
func IsInternal(e error) bool {
	return errors.Is(e, errInternal)
}

// InvalidArgument creates a new InvalidArgument error with a given error message and the original error.
func InvalidArgument(format string, args ...interface{}) error {
	return fmt.Errorf("%w %s", errInvalidArgument, fmt.Sprintf(format, args...))
}

// IsInvalidArgument checks if an error is a InvalidArgument error.
func IsInvalidArgument(e error) bool {
	return errors.Is(e, errInvalidArgument)
}
