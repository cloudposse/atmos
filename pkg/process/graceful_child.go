package process

import (
	"context"
	"os"
	"os/exec"

	"github.com/cloudposse/atmos/pkg/perf"
)

// GracefulChild is a subprocess that Atmos creates and waits on itself, outside Run,
// and that must stop gracefully on the first SIGINT/SIGTERM, such as Terraform under
// the streaming terminal UI.
//
// Unlike a child started by Run, it runs while Atmos may own the terminal in raw mode.
// A Ctrl-C keypress then reaches Atmos as input, not as a signal, so the terminal
// delivers nothing to the child and Atmos must interrupt it itself: a child that shares
// the terminal receives every forwarded signal directly.
//
// Use it in this order: NewGracefulChild, create cmd with Context, Start, cmd.Wait, Finish.
type GracefulChild struct {
	ctx    context.Context
	cancel context.CancelFunc
	cmd    *exec.Cmd
	group  *childGroup
	leave  func()
}

// NewGracefulChild returns a child whose command must be created with Context. It refuses
// with ErrProcessInterrupted when Atmos is already stopping after an interrupt, so an
// interrupted run starts no new Terraform command.
func NewGracefulChild(ctx context.Context) (*GracefulChild, error) {
	defer perf.Track(nil, "process.NewGracefulChild")()

	if ctx == nil {
		ctx = context.Background()
	}
	if err := gracefulStartError(ctx, InterruptGraceful); err != nil {
		return nil, err
	}
	executionCtx, cancel := executionContext(ctx, InterruptGraceful)
	return &GracefulChild{ctx: executionCtx, cancel: cancel, leave: func() {}}, nil
}

// Context is the context the child's command must be created with. A signal does not
// cancel it; a second signal, Kill, or any other cancellation of the parent context does.
//
//nolint:lintroller // Trivial getter.
func (c *GracefulChild) Context() context.Context { return c.ctx }

// Start starts cmd, which must have been created with Context and must have its stdin
// assigned, and joins the graceful stop. When notify is set, it is called after the first
// signal has been forwarded to the child, so a caller can show that Atmos is waiting.
func (c *GracefulChild) Start(cmd *exec.Cmd, notify func(os.Signal)) error {
	defer perf.Track(nil, "process.GracefulChild.Start")()

	c.cmd = cmd
	c.group = newChildGroup(cmd)
	if err := cmd.Start(); err != nil {
		c.cancel()
		return err
	}
	c.group.started(c.cancel)
	pid, grouped := cmd.Process.Pid, c.group.grouped
	c.leave = gracefulStops.Register(commandDisplayName(cmd.Path), func(sig os.Signal) {
		forwardTerminalInterrupt(pid, grouped, sig)
		if notify != nil {
			notify(sig)
		}
	})
	return nil
}

// Interrupt asks the child to stop gracefully, as a Ctrl-C keypress that the terminal UI
// received would. It returns an error when the platform cannot deliver an interrupt,
// as on Windows, and the caller should then Kill the child instead.
func (c *GracefulChild) Interrupt() error {
	defer perf.Track(nil, "process.GracefulChild.Interrupt")()

	if c.cmd == nil || c.cmd.Process == nil {
		return os.ErrProcessDone
	}
	return interruptTerminalChild(c.cmd.Process, c.group.grouped)
}

// Kill terminates the child's process tree immediately, as the second interrupt does.
func (c *GracefulChild) Kill() {
	defer perf.Track(nil, "process.GracefulChild.Kill")()

	c.cancel()
}

// Finish leaves the graceful stop and the exit-cleanup registry. It must be called once
// cmd.Wait has returned; it is idempotent.
func (c *GracefulChild) Finish() {
	defer perf.Track(nil, "process.GracefulChild.Finish")()

	c.leave()
	if c.group != nil {
		c.group.finished()
	}
	c.cancel()
}
