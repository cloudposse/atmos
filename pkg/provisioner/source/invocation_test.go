package source

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestBeginProvisionPeerWaitHonorsCancellation verifies that a caller waiting for an in-process peer
// stops when its context is cancelled, and that a cancelled caller never reports a reused result.
func TestBeginProvisionPeerWaitHonorsCancellation(t *testing.T) {
	t.Cleanup(BeginInvocation())
	target := filepath.Join(t.TempDir(), "target")
	require.NoError(t, os.MkdirAll(target, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(target, "main.tf"), []byte("# source"), 0o600))

	reused, finishPeer, err := beginProvision(t.Context(), "key", target)
	require.NoError(t, err)
	require.False(t, reused)

	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	reused, finish, err := beginProvision(ctx, "key", target)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.False(t, reused)
	require.Nil(t, finish)

	finishPeer(true)

	// Once the peer finished successfully, a live caller reuses its result...
	reused, _, err = beginProvision(t.Context(), "key", target)
	require.NoError(t, err)
	require.True(t, reused)

	// ...but an already-cancelled caller gets the cancellation, not success.
	cancelled, cancelNow := context.WithCancel(t.Context())
	cancelNow()
	reused, _, err = beginProvision(cancelled, "key", target)
	require.ErrorIs(t, err, context.Canceled)
	require.False(t, reused)
}
