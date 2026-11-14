package artifact

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

// TestSentinelsAreDistinct guards against two sentinels aliasing each other, which would make
// errors.Is match the wrong failure class.
func TestSentinelsAreDistinct(t *testing.T) {
	sentinels := []error{
		errUtils.ErrArtifactConfigInvalid,
		errUtils.ErrArtifactRefInvalid,
		errUtils.ErrArtifactConflict,
		errUtils.ErrArtifactNotFound,
		errUtils.ErrArtifactPublicationUnavailable,
		errUtils.ErrArtifactUnsupportedCapability,
		errUtils.ErrArtifactIntegrityFailed,
		errUtils.ErrArtifactDocumentMissing,
		errUtils.ErrArtifactPackagingInvalid,
		errUtils.ErrArtifactUnsafePath,
		errUtils.ErrArtifactRecordInvalid,
		errUtils.ErrArtifactTooLarge,
	}
	for i, sentinel := range sentinels {
		wrapped := fmt.Errorf("%w: context", sentinel)
		for j, other := range sentinels {
			assert.Equal(t, i == j, errors.Is(wrapped, other), "%v vs %v", sentinel, other)
		}
	}
}

func TestCapabilityError(t *testing.T) {
	t.Run("unsupported capability wraps the sentinel", func(t *testing.T) {
		err := NewUnsupportedCapability("shared", "lock", "lock/repository", "OCI tags cannot provide distributed exclusion")
		require.ErrorIs(t, err, errUtils.ErrArtifactUnsupportedCapability)
		assert.NotErrorIs(t, err, errUtils.ErrArtifactPublicationUnavailable)
		assert.Contains(t, err.Error(), "shared")
		assert.Contains(t, err.Error(), "lock/repository")
	})

	t.Run("survives further wrapping and is extractable", func(t *testing.T) {
		wrapped := fmt.Errorf("%w: publishing vpc:1.0", NewUnsupportedCapability("shared", "lock", "lock/repository", ""))
		require.ErrorIs(t, wrapped, errUtils.ErrArtifactUnsupportedCapability)
		capability, ok := AsCapabilityError(wrapped)
		require.True(t, ok)
		assert.Equal(t, "lock/repository", capability.Capability)
		_, ok = AsCapabilityError(errors.New("plain"))
		assert.False(t, ok)
	})

	t.Run("publication unavailable keeps its cause", func(t *testing.T) {
		cause := errors.New("sdk: 503")
		err := NewPublicationUnavailable("backups", "publish", "pending operation op-1", cause)
		require.ErrorIs(t, err, errUtils.ErrArtifactPublicationUnavailable)
		require.ErrorIs(t, err, cause, "backend causes stay available for diagnostics")
		assert.NotErrorIs(t, err, errUtils.ErrArtifactUnsupportedCapability)
	})

	t.Run("zero value defaults to unsupported", func(t *testing.T) {
		require.ErrorIs(t, &CapabilityError{}, errUtils.ErrArtifactUnsupportedCapability)
	})
}
