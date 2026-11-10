package signals

import (
	"context"
	"os"
	"sync"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func resetShutdownForTest(t *testing.T) {
	t.Helper()
	shutdownMu.Lock()
	old := committedSignal
	committedSignal = nil
	shutdownMu.Unlock()
	t.Cleanup(func() {
		shutdownMu.Lock()
		defer shutdownMu.Unlock()
		assert.Empty(t, shutdownOwners, "operations must release shutdown ownership")
		committedSignal = old
	})
}

func TestShutdownCancelsAllOwnersWithOriginalSignal(t *testing.T) {
	resetShutdownForTest(t)
	first, releaseFirst := WithShutdownContext(context.Background())
	defer releaseFirst()
	second, releaseSecond := WithShutdownContext(context.Background())
	defer releaseSecond()

	require.True(t, HandleShutdownSignal(os.Interrupt))
	require.ErrorIs(t, first.Err(), context.Canceled)
	require.ErrorIs(t, second.Err(), context.Canceled)
	assert.Equal(t, os.Interrupt, CancellationSignal(first))
	require.True(t, HandleShutdownSignal(syscall.SIGTERM))
	assert.Equal(t, os.Interrupt, CancellationSignal(second), "the first signal determines shutdown")
}

func TestShutdownReleaseDoesNotRemoveSuccessor(t *testing.T) {
	resetShutdownForTest(t)
	_, releaseFirst := WithShutdownContext(context.Background())
	releaseFirst()
	ctx, releaseSecond := WithShutdownContext(context.Background())
	defer releaseSecond()
	releaseFirst()

	require.True(t, HandleShutdownSignal(syscall.SIGTERM))
	assert.Equal(t, syscall.SIGTERM, CancellationSignal(ctx))
}

func TestShutdownCommittedBeforeRegistration(t *testing.T) {
	resetShutdownForTest(t)
	require.False(t, HandleShutdownSignal(syscall.SIGTERM))

	ctx, release := WithShutdownContext(context.Background())
	defer release()

	require.ErrorIs(t, ctx.Err(), context.Canceled)
	assert.Equal(t, syscall.SIGTERM, CancellationSignal(ctx))
}

func TestShutdownRegistrationRace(t *testing.T) {
	for range 20 {
		t.Run("registration races signal", func(t *testing.T) {
			resetShutdownForTest(t)
			start := make(chan struct{})
			var ctx context.Context
			var release func()
			var wg sync.WaitGroup
			wg.Add(2)
			go func() {
				defer wg.Done()
				<-start
				ctx, release = WithShutdownContext(context.Background())
			}()
			go func() {
				defer wg.Done()
				<-start
				HandleShutdownSignal(os.Interrupt)
			}()
			close(start)
			wg.Wait()
			defer release()

			require.ErrorIs(t, ctx.Err(), context.Canceled)
			assert.Equal(t, os.Interrupt, CancellationSignal(ctx))
		})
	}
}

func TestShutdownCauseAndExitCodes(t *testing.T) {
	resetShutdownForTest(t)
	ctx, release := WithShutdownContext(context.Background())
	assert.Nil(t, CancellationSignal(ctx))
	release()
	assert.Nil(t, CancellationSignal(ctx), "normal release is not a shutdown signal")
	assert.Equal(t, 130, ExitCodeForSignal(os.Interrupt))
	assert.Equal(t, 143, ExitCodeForSignal(syscall.SIGTERM))
	assert.Equal(t, 130, ExitCodeForSignal(nil))
}
