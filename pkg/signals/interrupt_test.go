package signals

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSuspendInterruptExit_SuspendAndRelease(t *testing.T) {
	assert.False(t, InterruptExitSuspended())

	release := SuspendInterruptExit()
	assert.True(t, InterruptExitSuspended())

	release()
	assert.False(t, InterruptExitSuspended())
}

func TestSuspendInterruptExit_Nesting(t *testing.T) {
	releaseOuter := SuspendInterruptExit()
	releaseInner := SuspendInterruptExit()
	assert.True(t, InterruptExitSuspended())

	releaseInner()
	assert.True(t, InterruptExitSuspended(), "outer suspension must still be active")

	releaseOuter()
	assert.False(t, InterruptExitSuspended())
}

func TestSuspendInterruptExit_ReleaseIsIdempotent(t *testing.T) {
	release := SuspendInterruptExit()
	release()
	release()
	release()
	assert.False(t, InterruptExitSuspended(), "double release must not underflow the counter")

	// A new suspension still works after redundant releases.
	release2 := SuspendInterruptExit()
	assert.True(t, InterruptExitSuspended())
	release2()
	assert.False(t, InterruptExitSuspended())
}

func TestSuspendInterruptExit_Concurrent(t *testing.T) {
	const goroutines = 50

	var wg sync.WaitGroup
	for range goroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			release := SuspendInterruptExit()
			assert.True(t, InterruptExitSuspended())
			release()
		}()
	}
	wg.Wait()

	assert.False(t, InterruptExitSuspended())
}
