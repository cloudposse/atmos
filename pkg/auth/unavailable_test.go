package auth

import (
	"fmt"
	"testing"

	cockroacherrors "github.com/cockroachdb/errors"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestNormalizeAuthenticationError(t *testing.T) {
	require.Nil(t, NormalizeAuthenticationError(nil))
	for _, fatal := range []error{errUtils.ErrInvalidAuthConfig, errUtils.ErrIdentityNotFound, errUtils.ErrProviderNotFound, errUtils.ErrInvalidIdentityKind, errUtils.ErrInvalidIdentityConfig, errUtils.ErrCircularDependency, errUtils.ErrUserAborted} {
		err := fmt.Errorf("%w: %w", errUtils.ErrAuthenticationFailed, fatal)
		require.Same(t, err, NormalizeAuthenticationError(err))
		require.NotErrorIs(t, NormalizeAuthenticationError(err), errUtils.ErrAuthenticationUnavailable)
	}
	for _, err := range []error{errUtils.ErrAuthenticationFailed, errUtils.ErrIdentityAuthFailed, errUtils.ErrNoCredentialsFound, errUtils.ErrExpiredCredentials, errUtils.ErrCredentialsInvalid, errUtils.ErrIdentityCredentialsNone} {
		normalized := NormalizeAuthenticationError(err)
		require.ErrorIs(t, normalized, errUtils.ErrAuthenticationUnavailable)
		require.ErrorIs(t, normalized, err)
		require.Same(t, normalized, NormalizeAuthenticationError(normalized))
	}
}

func TestNormalizeAuthenticationErrorPreservesHints(t *testing.T) {
	original := cockroacherrors.WithHint(errUtils.ErrAuthenticationFailed, "Start the configured emulator and try again.")
	normalized := NormalizeAuthenticationError(original)
	require.ErrorIs(t, normalized, errUtils.ErrAuthenticationUnavailable)
	require.ErrorIs(t, normalized, errUtils.ErrAuthenticationFailed)
	require.Equal(t, cockroacherrors.GetAllHints(original), cockroacherrors.GetAllHints(normalized))
}
