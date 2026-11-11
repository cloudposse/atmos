package signals

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCleanupOwnsBoundedCancellationAndReleasesExitDeferral(t *testing.T) {
	parent, cancel := context.WithCancel(t.Context())
	operation, release := DeferExitForCleanup(parent)
	t.Cleanup(release)
	require.True(t, CleanupExitDeferred())
	owned, stop := OwnedCleanupContext(operation, time.Second)
	defer stop()
	cancel()
	require.ErrorIs(t, operation.Err(), context.Canceled)
	require.NoError(t, owned.Err(), "cleanup must survive caller cancellation")
	require.True(t, OwnsCleanup(owned))
	require.False(t, OwnsCleanup(parent))
	_, bounded := owned.Deadline()
	require.True(t, bounded)
	release()
	release()
	require.False(t, CleanupExitDeferred())
}
