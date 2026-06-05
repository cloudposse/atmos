package toolchain

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/github"
)

func TestHandleRefResolveError_NotFound(t *testing.T) {
	// A ref-not-found error maps to a friendly ErrToolNotFound.
	cause := fmt.Errorf("%w: 'does-not-exist' in cloudposse/atmos", github.ErrRefNotFound)

	err := handleRefResolveError(cause, "does-not-exist")

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrToolNotFound, "ref-not-found should surface as ErrToolNotFound")
	assert.NotErrorIs(t, err, errUtils.ErrToolInstall, "should not be classified as an install failure")
}

func TestHandleRefResolveError_Generic(t *testing.T) {
	// A non-not-found error (e.g. rate limit / network) maps to ErrToolInstall and preserves the cause.
	cause := errors.New("rate limit exceeded")

	err := handleRefResolveError(cause, "main")

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrToolInstall, "generic resolve failures should surface as ErrToolInstall")
	assert.ErrorIs(t, err, cause, "the underlying cause should be preserved in the chain")
}
