package native

import (
	"math"
	"sync"
	"time"

	"github.com/cloudposse/atmos/pkg/perf"
)

// DefaultInteractionLimit is how long an interactive invocation may wait for the user, on top of
// its call timeout. It matches the longest device-code lifetime common identity providers issue.
const DefaultInteractionLimit = 15 * time.Minute

// saturatingDurationAdd adds two non-negative durations, capping the result at the largest
// time.Duration. The call timeout has no upper bound, so adding the interaction limit to a very
// large one would otherwise wrap to a negative duration and drop the call's hard deadline.
func saturatingDurationAdd(a, b time.Duration) time.Duration {
	if b > 0 && a > time.Duration(math.MaxInt64)-b {
		return time.Duration(math.MaxInt64)
	}
	return a + b
}

// WithInteractionLimit bounds the time an interactive invocation may spend waiting for the user
// to complete a sign-in. That wait does not count against the call timeout, but the call's hard
// deadline is the call timeout plus this limit. Zero or negative keeps DefaultInteractionLimit.
//
//nolint:lintroller // Factory option; exempt under repository tracking policy.
func WithInteractionLimit(limit time.Duration) Option {
	return func(o *options) {
		if limit > 0 {
			o.interactionLimit = limit
		}
	}
}

// WithPackageAlias names the package's key under plugins.packages so errors can point at the
// configuration that controls them, such as plugins.packages.<alias>.interaction_timeout.
//
//nolint:lintroller // Trivial accessor, pure lookup, factory option, or delegation; exempt under repository tracking policy.
func WithPackageAlias(alias string) Option { return func(o *options) { o.packageAlias = alias } }

// AwaitUser marks the invocation as waiting for the user, such as after a device-code prompt
// was shown, and returns the function that ends the wait. While any wait is open the call
// timeout does not run; the hard deadline still applies. The returned function is idempotent,
// and both are no-ops for an unknown invocation or one without a call timeout.
func (h *Handle) AwaitUser(invocationID string) (resume func()) {
	defer perf.Track(nil, "native.Handle.AwaitUser")()

	if value, ok := h.budgets.Load(invocationID); ok {
		return value.(*budget).await()
	}
	return func() {}
}

// budget enforces a call timeout that pauses while the user is being waited on. The work the
// plugin does stays bounded by the timeout, but a person signing in is not cut off by it. The
// time spent waiting for the user is bounded separately by the interaction limit.
type budget struct {
	mu        sync.Mutex
	remaining time.Duration
	since     time.Time
	waiting   int
	timer     *time.Timer
	// generation invalidates a timer that fired while its budget was being paused or resumed.
	generation uint64
	finished   bool
	// expire runs once, outside the lock, when the budget ends. It reports whether the user wait
	// ran out, as opposed to the plugin's own work.
	expire func(waitExhausted bool)
	// deadline is the invocation's hard deadline: the call timeout plus the interaction limit.
	deadline time.Time
	// waitLimit bounds the cumulative time spent waiting for the user; zero or negative means unbounded.
	waitLimit time.Duration
	// waited is the user wait already closed; waitSince starts the open wait; waitTimer ends it at the limit.
	waited          time.Duration
	waitSince       time.Time
	waitTimer       *time.Timer
	exhaustedByWait bool
}

// newBudget starts counting immediately and calls expire once the work time or the user wait
// time runs out. The user wait is not counted against timeout but against waitLimit.
func newBudget(timeout, waitLimit time.Duration, expire func(waitExhausted bool)) *budget {
	b := &budget{remaining: timeout, since: time.Now(), expire: expire, waitLimit: waitLimit}
	b.timer = b.schedule()
	return b
}

// schedule arms a timer for the remaining time; the caller holds the lock or owns b.
func (b *budget) schedule() *time.Timer {
	generation := b.generation
	return time.AfterFunc(max(b.remaining, 0), func() { b.fire(generation, false) })
}

// scheduleWait arms a timer for the user wait time left; the caller holds the lock.
func (b *budget) scheduleWait() *time.Timer {
	generation := b.generation
	return time.AfterFunc(max(b.waitLimit-b.waited, 0), func() { b.fire(generation, true) })
}

// fire ends the budget unless it was paused, resumed, or finished since the timer was armed.
// A work timer must not end a paused budget; a wait timer must not end a running one.
func (b *budget) fire(generation uint64, wait bool) {
	b.mu.Lock()
	if b.finished || (b.waiting > 0) != wait || generation != b.generation {
		b.mu.Unlock()
		return
	}
	b.finished = true
	b.exhaustedByWait = wait
	b.mu.Unlock()
	b.expire(wait)
}

// userWaitExhausted reports whether the budget ended because the user wait ran out.
func (b *budget) userWaitExhausted() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.exhaustedByWait
}

// await pauses the work budget until every returned resume function has been called, and starts
// counting the user wait against the wait limit. If the work allowance is already spent, the
// budget ends instead of pausing, so an exhausted budget is never left paused.
func (b *budget) await() func() {
	b.mu.Lock()
	if b.finished {
		b.mu.Unlock()
		return func() {}
	}
	if b.waiting == 0 {
		b.timer.Stop()
		b.generation++
		b.remaining -= time.Since(b.since)
		if b.remaining <= 0 {
			// A timer callback that already started cannot be cancelled, and it would now see a
			// stale generation, so expire here; the callback runs outside the lock.
			b.finished = true
			b.mu.Unlock()
			b.expire(false)
			return func() {}
		}
		if b.waitLimit > 0 {
			b.waitSince = time.Now()
			b.waitTimer = b.scheduleWait()
		}
	}
	b.waiting++
	b.mu.Unlock()
	var once sync.Once
	return func() { once.Do(b.resume) }
}

func (b *budget) resume() {
	b.mu.Lock()
	b.waiting--
	if b.waiting > 0 || b.finished {
		b.mu.Unlock()
		return
	}
	if b.waitTimer != nil {
		b.waitTimer.Stop()
		b.waited += time.Since(b.waitSince)
		if b.waited >= b.waitLimit {
			// The wait timer may be due but blocked on the lock, and restarting work would make it
			// stale, so expire here; the callback runs outside the lock.
			b.finished = true
			b.exhaustedByWait = true
			b.mu.Unlock()
			b.expire(true)
			return
		}
	}
	b.generation++
	b.since = time.Now()
	b.timer = b.schedule()
	b.mu.Unlock()
}

// stop releases the timers when the invocation ends.
func (b *budget) stop() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.finished = true
	b.timer.Stop()
	if b.waitTimer != nil {
		b.waitTimer.Stop()
	}
}
