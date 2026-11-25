package ui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	tea "github.com/charmbracelet/bubbletea"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/process"
	"github.com/cloudposse/atmos/pkg/signals"
	"github.com/cloudposse/atmos/pkg/ui"
)

// interruptMsg tells the TUI that Atmos received SIGINT/SIGTERM and forwarded it to terraform.
type interruptMsg struct {
	signal os.Signal
}

// finishedModel is implemented by TUI models that report whether they reached their final view.
type finishedModel interface {
	Finished() bool
}

// runningCommand is a started terraform command that stops gracefully when interrupted:
// the first Ctrl-C (a keypress, because the TUI reads the terminal in raw mode) or the first
// SIGINT/SIGTERM is delivered to terraform so it can save state and release its lock, and
// the TUI keeps streaming until terraform exits. The second one terminates it.
type runningCommand struct {
	cmd *exec.Cmd
	// Child is nil when a test builds the command directly; it then can only be killed.
	child *process.GracefulChild

	mu   sync.Mutex
	send func(tea.Msg)
	// Pending is a signal received before the program could be notified.
	pending os.Signal

	waitOnce sync.Once
	waitErr  error
}

// newGracefulCommand creates the terraform command under a graceful child. The caller
// assigns its streams, then calls start; on a setup error it must call finish.
func newGracefulCommand(ctx context.Context, opts *ExecuteOptions, args []string) (*runningCommand, error) {
	child, err := process.NewGracefulChild(ctx)
	if err != nil {
		return nil, fmt.Errorf(fmtWrapErr, errUtils.ErrCommandStart, err)
	}
	cmd := execCommandContext(child.Context(), opts.Command, args...)
	cmd.Dir = opts.WorkingDir
	cmd.Env = opts.Env
	cmd.Stdin = os.Stdin // Allow interactive terraform commands.
	return &runningCommand{cmd: cmd, child: child}, nil
}

// start starts the command and joins the graceful stop.
func (r *runningCommand) start() error {
	if err := r.child.Start(r.cmd, r.notifyInterrupt); err != nil {
		return fmt.Errorf(fmtWrapErr, errUtils.ErrCommandStart, err)
	}
	return nil
}

// notifyInterrupt runs after the signal handler forwarded a signal to terraform. The message
// is sent asynchronously: tea.Program.Send blocks until the program reads it, and the signal
// handler must not block.
func (r *runningCommand) notifyInterrupt(sig os.Signal) {
	r.mu.Lock()
	send := r.send
	if send == nil {
		r.pending = sig
	}
	r.mu.Unlock()
	if send != nil {
		go send(interruptMsg{signal: sig})
	}
}

// attach routes signal notifications to program, including one received before it existed.
func (r *runningCommand) attach(program *tea.Program) {
	r.attachSender(program.Send)
}

// attachSender routes signal notifications to send; a nil send stops routing them.
func (r *runningCommand) attachSender(send func(tea.Msg)) {
	r.mu.Lock()
	r.send = send
	var pending os.Signal
	if send != nil {
		pending, r.pending = r.pending, nil
	}
	r.mu.Unlock()
	if pending != nil {
		go send(interruptMsg{signal: pending})
	}
}

// detach stops routing signal notifications once the program has finished.
func (r *runningCommand) detach() {
	r.attachSender(nil)
}

// interrupt asks terraform to stop gracefully, as the first Ctrl-C keypress does.
func (r *runningCommand) interrupt() error {
	if r.child == nil {
		return errUtils.ErrInterruptUnsupported
	}
	return r.child.Interrupt()
}

// kill terminates terraform immediately, as the second Ctrl-C keypress does.
func (r *runningCommand) kill() {
	if r.child != nil {
		r.child.Kill()
		return
	}
	if r.cmd.Process != nil {
		_ = r.cmd.Process.Kill()
	}
}

// finish leaves the graceful stop after terraform has been waited on. It is idempotent.
func (r *runningCommand) finish() {
	if r.child != nil {
		r.child.Finish()
	}
}

// wait reaps terraform and leaves the graceful stop. It is safe to call more than once and
// concurrently: init waits in a background goroutine while a failed TUI also waits, and two
// concurrent exec.Cmd.Wait calls can block forever.
func (r *runningCommand) wait() error {
	r.waitOnce.Do(func() {
		r.waitErr = r.cmd.Wait()
		r.finish()
	})
	return r.waitErr
}

// tuiOutcome reports how a TUI run ended besides the command's own result.
type tuiOutcome struct {
	// Cancelled means terraform was killed: a second Ctrl-C, or one the platform could not deliver.
	cancelled bool
	// Interrupted means terraform was asked to stop gracefully and ran until it exited.
	interrupted bool
}

// stopped reports whether the run ended because the user or a signal interrupted it.
func (o tuiOutcome) stopped() bool {
	return o.cancelled || o.interrupted
}

// gracefulModel wraps a TUI model so the first quit key interrupts terraform instead of
// killing it. While terraform stops, the TUI keeps rendering its progress with a notice;
// the second quit key reaches the wrapped model, which cancels, and the caller kills terraform.
type gracefulModel struct {
	inner    tea.Model
	run      *runningCommand
	stopping bool
	// Signal is the one that began the stop; a keypress stands for an interrupt.
	signal os.Signal
}

// Init starts the wrapped model.
//
//nolint:gocritic // bubbletea models must be passed by value
func (m gracefulModel) Init() tea.Cmd {
	return m.inner.Init()
}

// Update intercepts the first quit key and signal notifications, and delegates the rest.
//
//nolint:gocritic // bubbletea models must be passed by value
func (m gracefulModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case interruptMsg:
		m.stopping, m.signal = true, msg.signal
		return m, nil
	case tea.KeyMsg:
		if isQuitKey(msg.String()) && !m.stopping && m.run.interrupt() == nil {
			m.stopping, m.signal = true, os.Interrupt
			return m, nil
		}
	}
	inner, cmd := m.inner.Update(msg)
	m.inner = inner
	return m, cmd
}

// View renders the wrapped model, with the stopping notice until it reaches its final view.
//
//nolint:gocritic // bubbletea models must be passed by value
func (m gracefulModel) View() string {
	view := m.inner.View()
	if !m.stopping {
		return view
	}
	if finished, ok := m.inner.(finishedModel); ok && finished.Finished() {
		return view
	}
	notice := signals.GracefulStopNotice(m.signal, []string{commandName(m.run.cmd)})
	return view + newlineStr + ui.FormatWarning(notice) + newlineStr
}

// commandName names the running command for the notice, e.g. "terraform" or "tofu".
func commandName(cmd *exec.Cmd) string {
	name := filepath.Base(cmd.Path)
	if strings.EqualFold(filepath.Ext(name), ".exe") {
		name = name[:len(name)-len(".exe")]
	}
	return name
}

// unwrapFinalModel returns the wrapped model and how the run ended.
func unwrapFinalModel(finalModel tea.Model) (tea.Model, tuiOutcome) {
	inner, interrupted := finalModel, false
	if wrapped, ok := finalModel.(gracefulModel); ok {
		inner, interrupted = wrapped.inner, wrapped.stopping
	}
	cancelled := modelWasCancelled(inner)
	return inner, tuiOutcome{cancelled: cancelled, interrupted: interrupted && !cancelled}
}
