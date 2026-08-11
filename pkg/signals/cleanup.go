package signals

import (
	"sort"
	"sync"

	"github.com/cloudposse/atmos/pkg/perf"
)

// exitCleanups holds cleanup functions that must run before the process exits
// through the signal handler. Because os.Exit skips deferred functions, state that
// must be undone on signal exit (e.g. terminal raw mode during a TTY session)
// is registered here and run by the main signal handler.
var (
	exitCleanupsMu sync.Mutex
	exitCleanups   = map[int]func(){}
	exitCleanupSeq int
)

// RegisterExitCleanup registers a cleanup to run if the process exits through
// the signal handler. The returned deregister function removes it (idempotent)
// and must be called when the cleanup has been performed normally.
func RegisterExitCleanup(cleanup func()) (deregister func()) {
	defer perf.Track(nil, "signals.RegisterExitCleanup")()

	exitCleanupsMu.Lock()
	exitCleanupSeq++
	id := exitCleanupSeq
	exitCleanups[id] = cleanup
	exitCleanupsMu.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() {
			exitCleanupsMu.Lock()
			delete(exitCleanups, id)
			exitCleanupsMu.Unlock()
		})
	}
}

// RunExitCleanups runs all registered exit cleanups (most recent first).
// Called by the main signal handler before exiting the process.
func RunExitCleanups() {
	defer perf.Track(nil, "signals.RunExitCleanups")()

	exitCleanupsMu.Lock()
	ids := make([]int, 0, len(exitCleanups))
	for id := range exitCleanups {
		ids = append(ids, id)
	}
	// Most recent first (higher id = registered later), like defers.
	sort.Sort(sort.Reverse(sort.IntSlice(ids)))
	fns := make([]func(), 0, len(ids))
	for _, id := range ids {
		fns = append(fns, exitCleanups[id])
	}
	exitCleanupsMu.Unlock()

	for _, fn := range fns {
		fn()
	}
}
