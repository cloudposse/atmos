package reported_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/plugin/reported"
)

var errCause = errors.New("cause")

func TestWrapPreservesIdentityAndSilencesTheRootHandler(t *testing.T) {
	wrapped := reported.Wrap(fmt.Errorf("context: %w", errCause))
	require.ErrorIs(t, wrapped, errCause)
	assert.EqualError(t, wrapped, "context: cause", "the message is unchanged")
	var exit errUtils.ExitCodeError
	require.ErrorAs(t, wrapped, &exit)
	assert.True(t, exit.Silent)
	assert.Equal(t, 1, errUtils.GetExitCode(wrapped))
	assert.True(t, reported.IsReported(wrapped))
	assert.True(t, reported.IsReported(fmt.Errorf("outer: %w", wrapped)))
}

func TestWrapIsIdempotentAndNilSafe(t *testing.T) {
	require.NoError(t, reported.Wrap(nil))
	once := reported.Wrap(errCause)
	assert.Same(t, once, reported.Wrap(once))
	assert.False(t, reported.IsReported(errCause))
	assert.False(t, reported.IsReported(nil))
}
