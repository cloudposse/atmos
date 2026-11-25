package signals

import (
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// waitAsync runs stops.Wait in the background and reports when it returns. It gives the
// waiter time to block on the current participants, so a release happens while it waits.
// The returned value, read after the channel closes, is what observe reported when Wait returned.
func waitAsync(stops *GracefulStops, observe func() bool) (<-chan struct{}, *atomic.Bool) {
	waited := make(chan struct{})
	var observed atomic.Bool
	go func() {
		stops.Wait()
		observed.Store(observe())
		close(waited)
	}()
	time.Sleep(20 * time.Millisecond)
	return waited, &observed
}

func TestGracefulStopWaitReturnsOnlyAfterTheDrainCallback(t *testing.T) {
	var renewed atomic.Bool
	stops := NewGracefulStops(nil, func() {
		// Stands in for restartCleanupBudget: a woken waiter must already see its effect.
		time.Sleep(50 * time.Millisecond)
		renewed.Store(true)
	})
	release := stops.Register("terraform", func(os.Signal) {})
	_, ok := stops.Begin(os.Interrupt)
	require.True(t, ok)
	waited, sawRenewed := waitAsync(stops, renewed.Load)

	release()
	select {
	case <-waited:
	case <-time.After(time.Second):
		t.Fatal("Wait did not return after the last participant exited")
	}
	assert.True(t, sawRenewed.Load(), "Wait returned before the drain callback finished")
}

func TestGracefulStopWaitCoversAChildRegisteredAsTheLastOneLeaves(t *testing.T) {
	var stops *GracefulStops
	lateReleased := make(chan func(), 1)
	var forwarded atomic.Int32
	var joined atomic.Bool
	stops = NewGracefulStops(nil, func() {
		// A late child joins in the window between the last release and the waiter waking up.
		if joined.CompareAndSwap(false, true) {
			lateReleased <- stops.Register("tofu", func(os.Signal) { forwarded.Add(1) })
		}
	})
	release := stops.Register("terraform", func(os.Signal) {})
	_, ok := stops.Begin(os.Interrupt)
	require.True(t, ok)
	waited, _ := waitAsync(stops, func() bool { return true })

	release()
	releaseLate := <-lateReleased
	assert.Equal(t, int32(1), forwarded.Load(), "the late child receives the pending signal")
	select {
	case <-waited:
		t.Fatal("Wait returned while the late child was still running")
	case <-time.After(50 * time.Millisecond):
	}
	releaseLate()
	select {
	case <-waited:
	case <-time.After(time.Second):
		t.Fatal("Wait did not return after the late child exited")
	}
}
