package claudecode

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPausableTimer_NilIsInert(t *testing.T) {
	timer := newPausableTimer(0, func() { t.Error("must not fire") })
	assert.Nil(t, timer)
	assert.NotPanics(t, func() {
		timer.pause()
		timer.resume()
		timer.stop()
	})
	assert.False(t, timer.didFire())
}

func TestPausableTimer_Fires(t *testing.T) {
	fired := make(chan struct{}, 1)
	timer := newPausableTimer(50*time.Millisecond, func() { fired <- struct{}{} })
	defer timer.stop()

	select {
	case <-fired:
	case <-time.After(5 * time.Second):
		require.Fail(t, "timer did not fire")
	}
	assert.True(t, timer.didFire())
}

func TestPausableTimer_PausedTimeDoesNotCount(t *testing.T) {
	fired := make(chan time.Time, 1)
	start := time.Now()
	timer := newPausableTimer(400*time.Millisecond, func() { fired <- time.Now() })
	defer timer.stop()

	time.Sleep(100 * time.Millisecond)
	timer.pause()

	// Paused for far longer than the full duration: must not fire.
	select {
	case <-fired:
		require.Fail(t, "timer fired while paused")
	case <-time.After(700 * time.Millisecond):
	}

	resumedAt := time.Now()
	timer.resume()
	select {
	case at := <-fired:
		// Roughly 300ms of budget remained when it was paused.
		assert.GreaterOrEqual(t, at.Sub(resumedAt), 200*time.Millisecond)
		assert.Greater(t, at.Sub(start), time.Second)
	case <-time.After(5 * time.Second):
		require.Fail(t, "timer did not fire after resume")
	}
}

func TestPausableTimer_StopPreventsFireAndResume(t *testing.T) {
	timer := newPausableTimer(50*time.Millisecond, func() { t.Error("must not fire") })
	timer.stop()
	timer.resume()
	time.Sleep(200 * time.Millisecond)
	assert.False(t, timer.didFire())
}

// TestPausableTimer_FireAfterPauseIsIgnored covers the race where the runtime timer's callback is already
// on its way when pause() takes the lock: the late callback must not fire the user's function.
func TestPausableTimer_FireAfterPauseIsIgnored(t *testing.T) {
	var fired atomic.Int32
	timer := newPausableTimer(time.Hour, func() { fired.Add(1) })
	defer timer.stop()

	timer.pause()
	timer.fire() // The callback that lost the race.

	assert.Zero(t, fired.Load())
	assert.False(t, timer.didFire())

	// The timer is still usable: resuming re-arms it with what remains.
	timer.mu.Lock()
	timer.remaining = 20 * time.Millisecond
	timer.mu.Unlock()
	timer.resume()

	assert.Eventually(t, func() bool { return fired.Load() == 1 }, 5*time.Second, 5*time.Millisecond)
	assert.True(t, timer.didFire())
}

// TestPausableTimer_PauseIsIdempotent checks that pausing twice charges the elapsed time only once.
func TestPausableTimer_PauseIsIdempotent(t *testing.T) {
	timer := newPausableTimer(time.Hour, func() { t.Error("must not fire") })
	defer timer.stop()

	// Pretend a minute has already run. Relying on the wall clock to advance between two calls is
	// not safe: Windows can return the same timestamp for both.
	timer.mu.Lock()
	timer.startedAt = time.Now().Add(-time.Minute)
	timer.mu.Unlock()

	timer.pause()
	timer.mu.Lock()
	afterFirst := timer.remaining
	timer.mu.Unlock()
	require.LessOrEqual(t, afterFirst, 59*time.Minute, "the elapsed minute was charged")

	time.Sleep(20 * time.Millisecond)
	timer.pause()

	timer.mu.Lock()
	afterSecond := timer.remaining
	timer.mu.Unlock()
	assert.Equal(t, afterFirst, afterSecond, "a second pause does not charge more time")
}

// TestPausableTimer_PauseAfterBudgetElapsedFiresOnResume checks the remaining time never goes negative:
// when the whole budget was used up before the pause landed, resuming fires right away.
func TestPausableTimer_PauseAfterBudgetElapsedFiresOnResume(t *testing.T) {
	var fired atomic.Int32
	timer := newPausableTimer(time.Hour, func() { fired.Add(1) })
	defer timer.stop()

	timer.mu.Lock()
	timer.startedAt = time.Now().Add(-2 * time.Hour)
	timer.mu.Unlock()
	timer.pause()

	timer.mu.Lock()
	remaining := timer.remaining
	timer.mu.Unlock()
	assert.Zero(t, remaining, "the remaining time is clamped to zero")
	assert.False(t, timer.didFire(), "pausing does not fire the timer")

	timer.resume()

	assert.Eventually(t, func() bool { return fired.Load() == 1 }, 5*time.Second, 5*time.Millisecond)
	assert.True(t, timer.didFire())
}

// TestPausableTimer_ResumeWhileRunningDoesNotArmASecondTimer checks that an unpaired resume is a no-op.
func TestPausableTimer_ResumeWhileRunningDoesNotArmASecondTimer(t *testing.T) {
	var fired atomic.Int32
	timer := newPausableTimer(30*time.Millisecond, func() { fired.Add(1) })
	defer timer.stop()

	timer.resume()
	timer.resume()

	require.Eventually(t, func() bool { return fired.Load() >= 1 }, 5*time.Second, 5*time.Millisecond)
	time.Sleep(100 * time.Millisecond)
	assert.Equal(t, int32(1), fired.Load(), "the callback runs exactly once")
}
