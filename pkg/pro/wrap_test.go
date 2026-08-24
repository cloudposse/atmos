package pro

import (
	"errors"
	"testing"

	cockroachErrors "github.com/cockroachdb/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

// TestWrapErr_NilCauseReturnsSentinel verifies that a nil cause short-circuits
// to the sentinel itself, avoiding an unnecessary builder allocation.
func TestWrapErr_NilCauseReturnsSentinel(t *testing.T) {
	got := wrapErr(errUtils.ErrFailedToMakeRequest, nil)
	require.Error(t, got)
	assert.Same(t, errUtils.ErrFailedToMakeRequest, got)
}

// TestWrapErr_NilSentinelReturnsCause verifies that when no sentinel is
// supplied the cause is returned untouched, so callers can pass through
// errors without losing the chain.
func TestWrapErr_NilSentinelReturnsCause(t *testing.T) {
	cause := errors.New("boom")
	got := wrapErr(nil, cause)
	require.Error(t, got)
	assert.Same(t, cause, got)
}

// TestWrapErr_BothNonNil verifies that errors.Is matches the sentinel and the
// cause, and that any cockroach hints attached to the cause are surfaced on
// the outer wrapper so the CLI renderer can show them.
func TestWrapErr_BothNonNil(t *testing.T) {
	cause := cockroachErrors.WithHint(errors.New("inner"), "try again later")
	got := wrapErr(errUtils.ErrFailedToMakeRequest, cause)

	require.Error(t, got)
	assert.True(t, errors.Is(got, errUtils.ErrFailedToMakeRequest), "outer sentinel must match")
	assert.True(t, errors.Is(got, cause), "inner cause must remain in chain")
	assert.Contains(t, cockroachErrors.GetAllHints(got), "try again later")
}
