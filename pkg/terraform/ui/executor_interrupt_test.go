package ui

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

// stoppingText is part of the notice the TUI shows while terraform stops gracefully.
const stoppingText = "to stop gracefully. Press Ctrl-C again to force exit."

// unsupportedInterruptCommand is a command that cannot be interrupted, as on Windows.
func unsupportedInterruptCommand() *runningCommand {
	return &runningCommand{cmd: &exec.Cmd{Path: "terraform"}}
}

func TestGracefulModelFallsBackToCancelWhenInterruptIsUnsupported(t *testing.T) {
	m := gracefulModel{
		inner: NewModel("comp", "stack", "apply", strings.NewReader("")),
		run:   unsupportedInterruptCommand(),
	}

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})

	wrapped := updated.(gracefulModel)
	assert.False(t, wrapped.stopping, "an interrupt that cannot be delivered must not wait")
	assert.True(t, modelWasCancelled(wrapped.inner), "the keypress cancels, as before")
	require.NotNil(t, cmd)
	assert.IsType(t, tea.QuitMsg{}, cmd())
}

func TestGracefulModelShowsTheNoticeAfterASignal(t *testing.T) {
	m := gracefulModel{
		inner: NewModel("comp", "stack", "apply", strings.NewReader("")),
		run:   unsupportedInterruptCommand(),
	}
	assert.NotContains(t, m.View(), stoppingText)

	updated, cmd := m.Update(interruptMsg{signal: os.Interrupt})
	assert.Nil(t, cmd, "the TUI keeps running while terraform stops")
	stopping := updated.(gracefulModel)
	assert.True(t, stopping.stopping)
	assert.Contains(t, stopping.View(), "Interrupt received; waiting for terraform "+stoppingText)
	assert.False(t, modelWasCancelled(stopping.inner))

	// Terraform exits: the final view replaces the notice.
	finished, cmd := stopping.Update(doneMsg{exitCode: 1})
	require.NotNil(t, cmd)
	assert.NotContains(t, finished.View(), stoppingText)
	_, outcome := unwrapFinalModel(finished)
	assert.Equal(t, tuiOutcome{interrupted: true}, outcome)
}

func TestGracefulModelNoticeNamesSIGTERM(t *testing.T) {
	m := gracefulModel{
		inner: NewModel("comp", "stack", "apply", strings.NewReader("")),
		run:   unsupportedInterruptCommand(),
	}

	updated, _ := m.Update(interruptMsg{signal: syscall.SIGTERM})

	assert.Contains(t, updated.View(),
		"Termination requested; waiting for terraform to stop gracefully. Send the signal again to force exit.")
}

func TestGracefulModelSecondQuitKeyCancels(t *testing.T) {
	m := gracefulModel{
		inner: NewInitModel("comp", "stack", "init", strings.NewReader("")),
		run:   unsupportedInterruptCommand(),
	}
	stopping, _ := m.Update(interruptMsg{})

	cancelled, cmd := stopping.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})

	require.NotNil(t, cmd)
	assert.IsType(t, tea.QuitMsg{}, cmd())
	assert.NotContains(t, cancelled.View(), stoppingText)
	_, outcome := unwrapFinalModel(cancelled)
	assert.Equal(t, tuiOutcome{cancelled: true}, outcome, "the second keypress kills terraform")
}

func TestGracefulModelDelegatesOtherMessages(t *testing.T) {
	m := gracefulModel{
		inner: NewModel("comp", "stack", "plan", strings.NewReader("")),
		run:   unsupportedInterruptCommand(),
	}
	assert.NotNil(t, m.Init(), "the wrapped model's Init runs")

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	inner, ok := updated.(gracefulModel).inner.(Model)
	require.True(t, ok)
	assert.Equal(t, 120, inner.width)
	assert.Equal(t, m.inner.View(), m.View(), "no notice before an interrupt")
}

func TestUnwrapFinalModel(t *testing.T) {
	plain := NewModel("comp", "stack", "plan", strings.NewReader(""))
	cancelled := Model{cancelled: true}
	tests := []struct {
		name  string
		final tea.Model
		want  tuiOutcome
	}{
		{name: "completed on its own", final: gracefulModel{inner: plain}, want: tuiOutcome{}},
		{
			name: "stopped gracefully", final: gracefulModel{inner: plain, stopping: true},
			want: tuiOutcome{interrupted: true},
		},
		{
			name: "forced after an interrupt", final: gracefulModel{inner: cancelled, stopping: true},
			want: tuiOutcome{cancelled: true},
		},
		{name: "unwrapped cancelled model", final: cancelled, want: tuiOutcome{cancelled: true}},
		{name: "unwrapped model", final: noCancelModel{}, want: tuiOutcome{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inner, outcome := unwrapFinalModel(tt.final)
			assert.Equal(t, tt.want, outcome)
			assert.Equal(t, tt.want.stopped(), outcome.cancelled || outcome.interrupted)
			_, wrapped := inner.(gracefulModel)
			assert.False(t, wrapped, "the caller always receives the wrapped TUI model")
		})
	}
}

func TestRunningCommandNotifiesTheProgram(t *testing.T) {
	received := make(chan tea.Msg, 2)
	send := func(msg tea.Msg) { received <- msg }

	t.Run("a signal before the program starts is delivered on attach", func(t *testing.T) {
		run := unsupportedInterruptCommand()
		run.notifyInterrupt(syscall.SIGTERM)
		assert.Equal(t, syscall.SIGTERM, run.pending)
		run.attachSender(send)
		assert.Equal(t, interruptMsg{signal: syscall.SIGTERM}, receiveMsg(t, received))
		assert.Nil(t, run.pending)
	})
	t.Run("a signal while the program runs is delivered", func(t *testing.T) {
		run := unsupportedInterruptCommand()
		run.attachSender(send)
		run.notifyInterrupt(os.Interrupt)
		assert.Equal(t, interruptMsg{signal: os.Interrupt}, receiveMsg(t, received))
	})
	t.Run("a signal after the program finished is kept, not sent", func(t *testing.T) {
		run := unsupportedInterruptCommand()
		run.attachSender(send)
		run.detach()
		run.notifyInterrupt(os.Interrupt)
		assert.Equal(t, os.Interrupt, run.pending)
		assert.Empty(t, received)
	})
}

// receiveMsg waits for one message sent to the program.
func receiveMsg(t *testing.T, received <-chan tea.Msg) tea.Msg {
	t.Helper()
	select {
	case msg := <-received:
		return msg
	case <-time.After(5 * time.Second):
		t.Fatal("no message reached the program")
		return nil
	}
}

func TestRunningCommandWithoutChild(t *testing.T) {
	run := unsupportedInterruptCommand()
	assert.ErrorIs(t, run.interrupt(), errUtils.ErrInterruptUnsupported)
	run.kill() // Not started: nothing to kill.
	run.finish()
}

func TestCommandName(t *testing.T) {
	assert.Equal(t, "terraform", commandName(&exec.Cmd{Path: "terraform"}))
	assert.Equal(t, "tofu", commandName(&exec.Cmd{Path: "tofu.EXE"}))
	assert.Equal(t, "terraform-1.5.7", commandName(&exec.Cmd{Path: "terraform-1.5.7"}))
}

func TestNewCommandsRefuseACancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	opts := &ExecuteOptions{Command: "terraform", WorkingDir: t.TempDir()}

	run, stdout, err := newStreamingCommand(ctx, opts, nil)
	assert.ErrorIs(t, err, errUtils.ErrCommandStart)
	assert.ErrorIs(t, err, context.Canceled)
	assert.Nil(t, run)
	assert.Nil(t, stdout)

	res, err := newInitCommand(ctx, opts)
	assert.ErrorIs(t, err, errUtils.ErrCommandStart)
	assert.Nil(t, res)
}

// TestRunTUIProgram_InterruptedLeavesProcessRunning verifies a graceful stop does not kill
// terraform: it already exited on its own after the interrupt, or Execute waits for it.
func TestRunTUIProgram_InterruptedLeavesProcessRunning(t *testing.T) {
	origRun := runTeaProgram
	t.Cleanup(func() { runTeaProgram = origRun })
	runTeaProgram = func(p *tea.Program) (tea.Model, error) {
		return gracefulModel{inner: Model{done: true}, stopping: true}, nil
	}

	exePath, err := os.Executable()
	require.NoError(t, err)
	cmd := exec.Command(exePath, "-test.run=^$")
	cmd.Env = append(os.Environ(), "_ATMOS_TEST_SLEEP_SECONDS=1")
	require.NoError(t, cmd.Start())

	model := NewModel("comp", "stack", "apply", strings.NewReader(""))
	finalModel, outcome, runErr := runTUIProgram(model, &runningCommand{cmd: cmd})

	require.NoError(t, runErr)
	assert.Equal(t, tuiOutcome{interrupted: true}, outcome)
	assert.IsType(t, Model{}, finalModel)
	require.NoError(t, cmd.Wait(), "terraform must not be killed after a graceful stop")
}

// fakeStreamingTerminal lets Execute and ExecuteInit run without a real terminal, with the
// TUI replaced by fakeRun.
func fakeStreamingTerminal(t *testing.T, fakeRun func(p *tea.Program) (tea.Model, error)) {
	t.Helper()
	origPreconditions, origRun := streamingUIPreconditions, runTeaProgram
	t.Cleanup(func() { streamingUIPreconditions, runTeaProgram = origPreconditions, origRun })
	streamingUIPreconditions = func() error { return nil }
	runTeaProgram = fakeRun
}

// exitingTerraformOptions runs the test binary as a terraform that exits 0 at once.
func exitingTerraformOptions(t *testing.T) *ExecuteOptions {
	t.Helper()
	exePath, err := os.Executable()
	require.NoError(t, err)
	return &ExecuteOptions{
		Command:    exePath,
		Args:       []string{"-test.run=^$"},
		WorkingDir: t.TempDir(),
		Env:        append(os.Environ(), "_ATMOS_TEST_SLEEP_SECONDS=0"),
		SubCommand: subCommandApply,
	}
}

// TestExecuteReturnsInterruptedAfterAGracefulStop verifies a graceful stop ends the run with
// the interrupted exit code, for both the streaming command and init.
func TestExecuteReturnsInterruptedAfterAGracefulStop(t *testing.T) {
	fakeStreamingTerminal(t, func(p *tea.Program) (tea.Model, error) {
		return gracefulModel{inner: noCancelModel{}, stopping: true, signal: os.Interrupt}, nil
	})

	var exitErr errUtils.ExitCodeError
	require.ErrorAs(t, Execute(context.Background(), exitingTerraformOptions(t)), &exitErr)
	assert.Equal(t, cancelledExitCode, exitErr.Code)

	require.ErrorAs(t, ExecuteInit(context.Background(), exitingTerraformOptions(t)), &exitErr)
	assert.Equal(t, cancelledExitCode, exitErr.Code)
}

// TestExecuteReportsATUIFailure verifies a TUI that fails to run is reported, for both commands.
func TestExecuteReportsATUIFailure(t *testing.T) {
	fakeStreamingTerminal(t, func(p *tea.Program) (tea.Model, error) {
		return nil, errUtils.ErrTUIRun
	})

	assert.ErrorIs(t, Execute(context.Background(), exitingTerraformOptions(t)), errUtils.ErrTUIRun)
	assert.ErrorIs(t, ExecuteInit(context.Background(), exitingTerraformOptions(t)), errUtils.ErrTUIRun)
}

// TestExecuteStartsNothingAfterCancellation verifies an already-cancelled run starts no terraform.
func TestExecuteStartsNothingAfterCancellation(t *testing.T) {
	fakeStreamingTerminal(t, func(p *tea.Program) (tea.Model, error) {
		t.Fatal("the TUI must not run when terraform did not start")
		return nil, nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	assert.ErrorIs(t, Execute(ctx, exitingTerraformOptions(t)), errUtils.ErrCommandStart)
	assert.ErrorIs(t, ExecuteInit(ctx, exitingTerraformOptions(t)), errUtils.ErrCommandStart)
}
