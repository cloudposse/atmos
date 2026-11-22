package native

import (
	"sync"
	"time"

	"github.com/cloudposse/atmos/pkg/perf"
)

// DefaultInteractionLimit is how long an interactive invocation may wait for the user, on top of
// its call timeout. It matches the longest device-code lifetime common identity providers issue.
const DefaultInteractionLimit = 15 * time.Minute

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
// plugin does stays bounded by the timeout, but a person signing in is not cut off by it.
type budget struct {
	mu        sync.Mutex
	remaining time.Duration
	since     time.Time
	waiting   int
	timer     *time.Timer
	// generation invalidates a timer that fired while its budget was being paused or resumed.
	generation uint64
	finished   bool
	expire     func()
	// deadline is the invocation's hard deadline: the call timeout plus the interaction limit.
	deadline time.Time
}

// newBudget starts counting immediately and calls expire once the time runs out.
func newBudget(timeout time.Duration, expire func()) *budget {
	b := &budget{remaining: timeout, since: time.Now(), expire: expire}
	b.timer = b.schedule()
	return b
}

// schedule arms a timer for the remaining time; the caller holds the lock or owns b.
func (b *budget) schedule() *time.Timer {
	generation := b.generation
	return time.AfterFunc(max(b.remaining, 0), func() { b.fire(generation) })
}

func (b *budget) fire(generation uint64) {
	b.mu.Lock()
	if b.finished || b.waiting > 0 || generation != b.generation {
		b.mu.Unlock()
		return
	}
	b.finished = true
	b.mu.Unlock()
	b.expire()
}

// await pauses the budget until every returned resume function has been called.
func (b *budget) await() func() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.finished {
		return func() {}
	}
	if b.waiting == 0 {
		b.timer.Stop()
		b.generation++
		b.remaining -= time.Since(b.since)
	}
	b.waiting++
	var once sync.Once
	return func() { once.Do(b.resume) }
}

func (b *budget) resume() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.waiting--
	if b.waiting > 0 || b.finished {
		return
	}
	b.since = time.Now()
	b.timer = b.schedule()
}

// stop releases the timer when the invocation ends.
func (b *budget) stop() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.finished = true
	b.timer.Stop()
}
