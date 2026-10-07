package process

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"

	"golang.org/x/term"

	"github.com/cloudposse/atmos/pkg/signals"
)

// childShutdownGrace is how long a signalled child process tree gets to exit
// after SIGTERM before it is killed outright. It is used both as exec.Cmd's
// WaitDelay and the context-cancellation grace period, including cancellation
// from the exit cleanup when Atmos receives SIGINT/SIGTERM.
const childShutdownGrace = 2 * time.Second

// childPollInterval is how often cancellation re-checks whether signalled
// children are gone while waiting out childShutdownGrace.
const childPollInterval = 25 * time.Millisecond

// childExitCleanupWait covers cancellation escalation, output-pipe WaitDelay,
// and scheduling headroom without letting shutdown wait indefinitely.
const childExitCleanupWait = 2*childShutdownGrace + time.Second

// stdinIsTerminal reports whether the stdin handed to a child is a real
// terminal. It is a variable so tests can stub the check.
var stdinIsTerminal = func(r io.Reader) bool {
	f, ok := r.(*os.File)
	if !ok {
		return false
	}
	return term.IsTerminal(int(f.Fd()))
}

// childGroup tracks one spawned child so that the whole process tree it leads
// is terminated on context cancellation or when Atmos is signalled.
//
// On Unix a child normally becomes the leader of its own process group
// (Setpgid), which lets Atmos signal the child and every grandchild at once.
//
// Exception: when the child inherits a terminal on stdin it is NOT moved into
// a new process group. A process in a background group that reads from, or
// reconfigures, its controlling terminal is stopped by SIGTTIN/SIGTTOU, so
// interactive children (editors, prompts, browser login flows) must stay in
// the terminal's foreground group. Those children are still tracked, but only
// the direct child is signalled.
type childGroup struct {
	cmd     *exec.Cmd
	grouped bool
	once    sync.Once
	child   *childProc
}

// newChildGroup prepares cmd (before Start) so its process tree can be
// terminated as a unit. It must be called after cmd.Stdin is assigned,
// because the terminal exception inspects it.
func newChildGroup(cmd *exec.Cmd) *childGroup {
	g := &childGroup{cmd: cmd}
	if stdinIsTerminal(cmd.Stdin) {
		return g
	}
	g.grouped = enableProcessGroup(cmd)
	if g.grouped {
		// Bound how long Wait lingers for output pipes held open by
		// grandchildren, and give the group time to exit gracefully.
		cmd.WaitDelay = childShutdownGrace
	}
	return g
}

// started registers the running child with the exit-cleanup registry.
// It must be called after a successful cmd.Start().
func (g *childGroup) started(cancel context.CancelFunc) {
	if g.cmd.Process == nil {
		return
	}
	g.child = registerChild(g.cmd.Process.Pid, g.grouped, cancel)
}

// finished unregisters the child after its cancellation cleanup has completed.
// It must be called once cmd.Wait() has returned; it is idempotent.
func (g *childGroup) finished() {
	g.once.Do(func() {
		if g.child != nil {
			unregisterChild(g.child)
		}
	})
}

// tolerateWaitDelay clears exec.ErrWaitDelay when the child itself exited
// successfully and the context was not canceled. That error only means a
// backgrounded grandchild kept an output pipe open past the WaitDelay; the
// command's own outcome is success.
func (g *childGroup) tolerateWaitDelay(ctx context.Context, err error) error {
	if !g.grouped || err == nil || !errors.Is(err, exec.ErrWaitDelay) {
		return err
	}
	if ctx.Err() == nil && g.cmd.ProcessState != nil && g.cmd.ProcessState.Success() {
		return nil
	}
	return err
}

// childProc identifies one invocation, independently of its reusable process ID.
// Cancellation belongs to that invocation; done closes after Wait and cleanup finish.
type childProc struct {
	pid     int
	grouped bool
	cancel  context.CancelFunc
	done    chan struct{}
}

var (
	childrenMu  sync.Mutex
	children    = map[*childProc]struct{}{}
	cleanupOnce sync.Once
)

// registerChild records a live invocation and installs the exit cleanup once.
func registerChild(pid int, grouped bool, cancel context.CancelFunc) *childProc {
	cleanupOnce.Do(func() {
		// The cleanup lives for the whole process, so the deregister function is unused.
		_ = signals.RegisterExitCleanup(killAllChildren)
	})
	child := &childProc{pid: pid, grouped: grouped, cancel: cancel, done: make(chan struct{})}
	childrenMu.Lock()
	children[child] = struct{}{}
	childrenMu.Unlock()
	return child
}

// unregisterChild completes the exact invocation that has been waited on.
func unregisterChild(child *childProc) {
	childrenMu.Lock()
	defer childrenMu.Unlock()
	if _, ok := children[child]; ok {
		delete(children, child)
		close(child.done)
	}
}

// liveChildren returns a snapshot of the registered invocations.
func liveChildren() []*childProc {
	childrenMu.Lock()
	defer childrenMu.Unlock()
	out := make([]*childProc, 0, len(children))
	for child := range children {
		out = append(out, child)
	}
	return out
}

// killAllChildren runs the same cancellation path as a caller's context deadline.
// Snapshots contain invocation callbacks instead of PID-based signaling operations.
func killAllChildren() {
	cancelChildren(liveChildren(), childExitCleanupWait)
}

// cancelChildren cancels every invocation before waiting, using one shared budget.
// Cmd.Cancel owns group escalation, so a reaped leader cannot drop its descendants.
func cancelChildren(procs []*childProc, wait time.Duration) {
	if len(procs) == 0 {
		return
	}
	for _, child := range procs {
		child.cancel()
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	for _, child := range procs {
		select {
		case <-child.done:
		case <-timer.C:
			return
		}
	}
}
