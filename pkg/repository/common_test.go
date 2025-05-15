package repository

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_validate(t *testing.T) {

	var errs []error
	errs = validate(errs, false, "condition is false")

	require.Len(t, errs, 1)
	require.EqualError(t, errors.Join(errs...), "condition is false")

	var errs2 []error
	errs2 = validate(errs2, false, "condition 1 is false")
	errs2 = validate(errs2, false, "condition 2 is false")

	require.Len(t, errs2, 2)
	require.EqualError(t, errors.Join(errs2...), "condition 1 is false\ncondition 2 is false")
}
