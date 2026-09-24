package auth

import (
	"fmt"
	"testing"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/stretchr/testify/require"
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
