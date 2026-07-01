package container

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNetworkCreateResult(t *testing.T) {
	t.Run("nil error is success", func(t *testing.T) {
		require.NoError(t, networkCreateResult(nil, "abc123"))
	})

	t.Run("already-exists output is idempotent success", func(t *testing.T) {
		err := errors.New("exit status 125")
		// Docker: "Error response from daemon: network atmos-emulator-local already exists".
		require.NoError(t, networkCreateResult(err, "Error response from daemon: network X already exists"))
	})

	t.Run("genuine failure propagates", func(t *testing.T) {
		err := errors.New("exit status 125")
		got := networkCreateResult(err, "permission denied")
		require.Error(t, got)
		assert.Contains(t, got.Error(), "permission denied")
	})
}
