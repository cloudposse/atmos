package signals

import (
	"os"
	"sort"
	"sync"

	"github.com/cloudposse/atmos/pkg/perf"
)

// GracefulStops coordinates child processes that must stop on their own when
// Atmos is interrupted, such as Terraform saving state and releasing its lock.
//
// The first SIGINT/SIGTERM is forwarded once to every registered child and the
// exit is deferred until they have all exited. A second signal forces the exit:
// the caller then runs the usual exit cleanups, which terminate the children.
type GracefulStops struct {
	mu           sync.Mutex
	next         uint64
	participants map[uint64]gracefulParticipant
	// First signal of a graceful stop; nil until one begins.
	signal   os.Signal
	forced   chan struct{}
	isForced bool
	drained  chan struct{}
	onBegin  func(os.Signal)
	onDrain  func()
}

// gracefulParticipant is one running child that stops gracefully.
type gracefulParticipant struct {
	name    string
	forward func(os.Signal)
}

// defaultGraceful is the registry the main signal handler consults.
var defaultGraceful = NewGracefulStops(RequestExit, restartCleanupBudget)

// NewGracefulStops returns an empty registry. The optional onBegin runs once
// when a graceful stop begins, before the signal is forwarded; onDrain runs
// once when the last child of a begun stop exits.
func NewGracefulStops(onBegin func(os.Signal), onDrain func()) *GracefulStops {
	defer perf.Track(nil, "signals.NewGracefulStops")()

	return &GracefulStops{
		participants: map[uint64]gracefulParticipant{},
		forced:       make(chan struct{}),
		onBegin:      onBegin,
		onDrain:      onDrain,
	}
}

// DefaultGracefulStops returns the registry used by the Atmos signal handler.
//
//nolint:lintroller // Trivial getter of the process-wide registry.
func DefaultGracefulStops() *GracefulStops { return defaultGraceful }

// BeginGracefulStop handles sig with the process-wide registry; see GracefulStops.Begin.
func BeginGracefulStop(sig os.Signal) ([]string, bool) {
	defer perf.Track(nil, "signals.BeginGracefulStop")()

	return defaultGraceful.Begin(sig)
}

// Register records a running child under a display name. The forward function
// delivers the first signal to the child; it is called at most once. A child
// registered after a stop began receives that signal immediately. The returned
// release function must be called once the child has exited; it is idempotent.
func (g *GracefulStops) Register(name string, forward func(os.Signal)) (release func()) {
	defer perf.Track(nil, "signals.GracefulStops.Register")()

	g.mu.Lock()
	if len(g.participants) == 0 {
		g.drained = make(chan struct{})
	}
	g.next++
	id := g.next
	g.participants[id] = gracefulParticipant{name: name, forward: forward}
	pending := g.signal
	g.mu.Unlock()

	if pending != nil {
		forward(pending)
	}
	var once sync.Once
	return func() {
		once.Do(func() { g.release(id) })
	}
}

// release removes one participant and finishes the stop after the last one.
func (g *GracefulStops) release(id uint64) {
	g.mu.Lock()
	delete(g.participants, id)
	if len(g.participants) != 0 {
		g.mu.Unlock()
		return
	}
	drained := g.drained
	finishing := g.signal != nil
	g.mu.Unlock()
	// Publish the drain only after onDrain renewed the cleanup budget, so a woken waiter
	// that goes on to wait for credential cleanup already sees the renewed budget.
	if finishing && g.onDrain != nil {
		g.onDrain()
	}
	close(drained)
}

// Begin handles a SIGINT/SIGTERM that would otherwise exit Atmos. When children
// are registered and no stop is in progress, it retains sig through onBegin,
// forwards sig once to every child, and returns their sorted display names with
// true: the caller must then keep running and wait. Otherwise it returns false
// and the caller exits; a signal received during a stop also forces Wait to return.
func (g *GracefulStops) Begin(sig os.Signal) ([]string, bool) {
	defer perf.Track(nil, "signals.GracefulStops.Begin")()

	g.mu.Lock()
	if g.signal != nil {
		if !g.isForced {
			g.isForced = true
			close(g.forced)
		}
		g.mu.Unlock()
		return nil, false
	}
	if len(g.participants) == 0 {
		g.mu.Unlock()
		return nil, false
	}
	g.signal = sig
	snapshot := make([]gracefulParticipant, 0, len(g.participants))
	for _, participant := range g.participants {
		snapshot = append(snapshot, participant)
	}
	g.mu.Unlock()

	// Retain the exit request first, so no new graceful child starts after the forward.
	if g.onBegin != nil {
		g.onBegin(sig)
	}
	names := make([]string, 0, len(snapshot))
	for _, participant := range snapshot {
		participant.forward(sig)
		names = append(names, participant.name)
	}
	sort.Strings(names)
	return names, true
}

// Stopping reports whether a graceful stop has begun. New graceful children
// must not start once it has.
func (g *GracefulStops) Stopping() bool {
	defer perf.Track(nil, "signals.GracefulStops.Stopping")()

	g.mu.Lock()
	defer g.mu.Unlock()
	return g.signal != nil
}

// Wait blocks until every registered child has exited or a second signal
// forced the exit. It is deliberately unbounded, like Terraform's own wait
// after the first interrupt: an in-flight resource may take minutes to finish,
// and the second signal is the escape hatch.
func (g *GracefulStops) Wait() {
	defer perf.Track(nil, "signals.GracefulStops.Wait")()

	for {
		g.mu.Lock()
		if len(g.participants) == 0 {
			g.mu.Unlock()
			return
		}
		drained, forced := g.drained, g.forced
		g.mu.Unlock()
		select {
		case <-drained:
			// A child may have registered after the last one left; wait for it too.
		case <-forced:
			return
		}
	}
}
