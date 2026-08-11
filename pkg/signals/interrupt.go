// Package signals coordinates process-wide signal handling behavior.
//
// Atmos installs a global SIGINT/SIGTERM handler in main() that exits the
// process. When a step hands the terminal to a foreground child process
// (interactive or TTY steps), Ctrl-C must be handled by the child, not by
// Atmos: the terminal delivers SIGINT to the whole foreground process group,
// and if Atmos exits first the child is killed by SIGPIPE mid-session.
//
// This package provides a nestable suspension counter that the main signal
// handler consults before exiting on SIGINT.
package signals

import (
	"sync"
	"sync/atomic"

	"github.com/cloudposse/atmos/pkg/perf"
)

// interruptExitSuspensions counts active interrupt-exit suspensions.
// While positive, the main signal handler ignores SIGINT and keeps waiting
// for the foreground child process to handle it.
var interruptExitSuspensions atomic.Int32

// SuspendInterruptExit marks that a foreground child process owns SIGINT
// handling. It returns a release function that must be called when the child
// exits. Suspensions nest (e.g., a workflow TTY step inside a custom command),
// and the release function is idempotent.
func SuspendInterruptExit() (release func()) {
	defer perf.Track(nil, "signals.SuspendInterruptExit")()

	interruptExitSuspensions.Add(1)

	var once sync.Once
	return func() {
		once.Do(func() {
			interruptExitSuspensions.Add(-1)
		})
	}
}

// InterruptExitSuspended reports whether the process-exit-on-SIGINT behavior
// is currently suspended because a foreground child process owns SIGINT.
func InterruptExitSuspended() bool {
	defer perf.Track(nil, "signals.InterruptExitSuspended")()

	return interruptExitSuspensions.Load() > 0
}
