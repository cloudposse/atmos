package claudecode

import (
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
