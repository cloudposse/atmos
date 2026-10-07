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
// WaitDelay (context cancellation, e.g. a step timeout) and as the grace period
// of the exit cleanup that runs when Atmos itself receives SIGINT/SIGTERM.
const childShutdownGrace = 2 * time.Second

// childPollInterval is how often the exit cleanup re-checks whether signalled
// children are gone while waiting out childShutdownGrace.
const childPollInterval = 25 * time.Millisecond

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
	pid     int
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
func (g *childGroup) started() {
	if g.cmd.Process == nil {
		return
	}
	g.pid = g.cmd.Process.Pid
	registerChild(g.pid, g.grouped)
}

// finished unregisters the child after its cancellation cleanup has completed.
// It must be called once cmd.Wait() has returned; it is idempotent.
func (g *childGroup) finished() {
	g.once.Do(func() {
		if g.pid != 0 {
			unregisterChild(g.pid)
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

// childProc identifies a live child in the registry.
type childProc struct {
	pid     int
	grouped bool
}

var (
	childrenMu  sync.Mutex
	children    = map[int]childProc{}
	cleanupOnce sync.Once
)

// registerChild records a live child and installs the exit cleanup once.
func registerChild(pid int, grouped bool) {
	cleanupOnce.Do(func() {
		// The cleanup lives for the whole process, so the deregister function is unused.
		_ = signals.RegisterExitCleanup(killAllChildren)
	})
	childrenMu.Lock()
	children[pid] = childProc{pid: pid, grouped: grouped}
	childrenMu.Unlock()
}

// unregisterChild removes a child that has been waited on.
func unregisterChild(pid int) {
	childrenMu.Lock()
	delete(children, pid)
	childrenMu.Unlock()
}

// liveChildren returns a snapshot of the registered children.
func liveChildren() []childProc {
	childrenMu.Lock()
	defer childrenMu.Unlock()
	out := make([]childProc, 0, len(children))
	for _, c := range children {
		out = append(out, c)
	}
	return out
}

// killAllChildren is the exit cleanup run by the main signal handler before
// the process exits: it SIGTERMs every live child tree, waits up to
// childShutdownGrace for them to go away, then SIGKILLs the survivors.
func killAllChildren() {
	procs := liveChildren()
	if len(procs) == 0 {
		return
	}
	for _, c := range procs {
		_ = terminateChild(c.pid, c.grouped)
	}
	deadline := time.Now().Add(childShutdownGrace)
	for time.Now().Before(deadline) && anyChildAlive(procs) {
		time.Sleep(childPollInterval)
	}
	for _, c := range procs {
		if childAlive(c.pid, c.grouped) {
			_ = killChild(c.pid, c.grouped)
		}
	}
}

func anyChildAlive(procs []childProc) bool {
	for _, c := range procs {
		if childAlive(c.pid, c.grouped) {
			return true
		}
	}
	return false
}
