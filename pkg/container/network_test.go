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
		// Docker: "Error response from daemon: network atmos-local already exists".
		require.NoError(t, networkCreateResult(err, "Error response from daemon: network X already exists"))
	})

	t.Run("genuine failure propagates", func(t *testing.T) {
		err := errors.New("exit status 125")
		got := networkCreateResult(err, "permission denied")
		require.Error(t, got)
		assert.Contains(t, got.Error(), "permission denied")
	})
}

func TestNetworkConnectResult(t *testing.T) {
	t.Run("nil error is success", func(t *testing.T) {
		require.NoError(t, networkConnectResult(nil, ""))
	})

	t.Run("docker already-exists output is idempotent success", func(t *testing.T) {
		err := errors.New("exit status 1")
		// Docker: "Error response from daemon: endpoint with name X already exists in network Y".
		require.NoError(t, networkConnectResult(err, "Error response from daemon: endpoint with name atmos-job already exists in network atmos-fixtures"))
	})

	t.Run("podman already-connected output is idempotent success", func(t *testing.T) {
		err := errors.New("exit status 125")
		// Podman: "Error: <container> is already connected to network <network>: network is already connected".
		require.NoError(t, networkConnectResult(err, "Error: abc123 is already connected to network atmos-fixtures: network is already connected"))
	})

	t.Run("genuine failure propagates", func(t *testing.T) {
		err := errors.New("exit status 1")
		got := networkConnectResult(err, "no such container")
		require.Error(t, got)
		assert.Contains(t, got.Error(), "no such container")
	})
}
