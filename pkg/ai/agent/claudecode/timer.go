package claudecode

import (
	"sync"
	"time"
)

// pausableTimer fires a callback after a duration of un-paused time. Time spent paused
// (for example while a person answers an approval prompt) does not count toward the
// duration. A nil *pausableTimer is valid and does nothing.
type pausableTimer struct {
	mu        sync.Mutex
	timer     *time.Timer
	onFire    func()
	remaining time.Duration
	startedAt time.Time
	running   bool
	fired     bool
	stopped   bool
}

// newPausableTimer starts a timer that calls onFire after d. A non-positive d disables
// the timer and returns nil.
func newPausableTimer(d time.Duration, onFire func()) *pausableTimer {
	if d <= 0 {
		return nil
	}
	p := &pausableTimer{onFire: onFire, remaining: d}
	p.startLocked()
	return p
}

// startLocked arms the timer with the remaining duration. The caller must hold p.mu or own p exclusively.
func (p *pausableTimer) startLocked() {
	p.startedAt = time.Now()
	p.running = true
	p.timer = time.AfterFunc(p.remaining, p.fire)
}

// fire runs when the timer expires.
func (p *pausableTimer) fire() {
	p.mu.Lock()
	if !p.running {
		p.mu.Unlock()
		return
	}
	p.running = false
	p.fired = true
	p.mu.Unlock()
	p.onFire()
}

// pause stops the countdown and remembers the remaining duration.
func (p *pausableTimer) pause() {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.running {
		return
	}
	p.timer.Stop()
	p.running = false
	p.remaining -= time.Since(p.startedAt)
	if p.remaining < 0 {
		p.remaining = 0
	}
}

// resume restarts the countdown with the remaining duration. It does nothing after the timer fired or was stopped.
func (p *pausableTimer) resume() {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.running || p.fired || p.stopped {
		return
	}
	p.startLocked()
}

// stop cancels the timer for good.
func (p *pausableTimer) stop() {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.timer != nil {
		p.timer.Stop()
	}
	p.running = false
	p.stopped = true
}

// didFire reports whether the timer expired.
func (p *pausableTimer) didFire() bool {
	if p == nil {
		return false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.fired
}
