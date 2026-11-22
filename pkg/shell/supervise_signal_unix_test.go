//go:build !windows

package shell

import (
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/signals"
)

// forceTerminalDeliversInterrupt pins whether the terminal is treated as having
// delivered Ctrl-C to the child already.
func forceTerminalDeliversInterrupt(t *testing.T, delivers bool) {
	t.Helper()
	orig := terminalDeliversInterruptFn
	t.Cleanup(func() { terminalDeliversInterruptFn = orig })
	terminalDeliversInterruptFn = func() bool { return delivers }
}

// setSupervisionTimings shortens the escalation delay and grace period for the test.
func setSupervisionTimings(t *testing.T, escalation, grace time.Duration) {
	t.Helper()
	origEscalation, origGrace := escalationDelay, terminationGrace
	t.Cleanup(func() { escalationDelay, terminationGrace = origEscalation, origGrace })
	escalationDelay, terminationGrace = escalation, grace
}

// awaitingChild is a running test-binary child waiting for a signal.
type awaitingChild struct {
	report string
	result chan error
}

// startAwaitingChild runs the test binary as a child that waits for a signal, through
// run (RunCommand or StartInteractive), and returns once the child is ready.
func startAwaitingChild(t *testing.T, run func(exe string, env []string) error, ignore ...syscall.Signal) *awaitingChild {
	t.Helper()
	exe, err := os.Executable()
	require.NoError(t, err)

	child := &awaitingChild{report: filepath.Join(t.TempDir(), "signal-report"), result: make(chan error, 1)}
	env := append(os.Environ(), awaitSignalEnv+"="+child.report)
	if len(ignore) > 0 {
		var list string
		for i, sig := range ignore {
			if i > 0 {
				list += ","
			}
			list += strconv.Itoa(int(sig))
		}
		env = append(env, ignoreSignalsEnv+"="+list)
	}
	go func() { child.result <- run(exe, env) }()

	require.Eventually(t, func() bool { return child.reportContent() == awaitSignalReady }, 10*time.Second, 10*time.Millisecond,
		"the child must start and install its signal handlers")
	return child
}

func (c *awaitingChild) reportContent() string {
	raw, err := os.ReadFile(c.report)
	if err != nil {
		return ""
	}
	return string(raw)
}

// wait returns the error of run once the child has exited.
func (c *awaitingChild) wait(t *testing.T) error {
	t.Helper()
	select {
	case err := <-c.result:
		return err
	case <-time.After(10 * time.Second):
		t.Fatal("the child is still running: the signal was not forwarded")
		return nil
	}
}

func runCommandChild(exe string, env []string) error { return RunCommand([]string{exe}, env) }

func startInteractiveChild(exe string, env []string) error { return StartInteractive(exe, nil, env) }

// requireSignalExit asserts that run returned the 128+N exit status for sig and that
// the child reported acting on sig.
func requireSignalExit(t *testing.T, child *awaitingChild, err error, sig syscall.Signal) {
	t.Helper()
	var exitErr errUtils.ExitCodeError
	require.ErrorAs(t, err, &exitErr)
	assert.Equal(t, 128+int(sig), exitErr.Code)
	assert.Equal(t, strconv.Itoa(int(sig)), child.reportContent(), "the child must have received %v", sig)
}

// TestRunCommand_TerminationIsForwardedToChild is the regression for SIGTERM sent only
// to Atmos leaving the `auth exec` child running. The main signal handler runs the exit
// cleanups before it exits (simulated here); the child must receive SIGTERM and Atmos
// must wait for it and report 128+15.
func TestRunCommand_TerminationIsForwardedToChild(t *testing.T) {
	forceTerminalDeliversInterrupt(t, true)
	child := startAwaitingChild(t, runCommandChild)
	assert.True(t, signals.InterruptExitSuspended(), "Atmos must not exit on Ctrl-C while the child owns it")

	signals.RunExitCleanups()

	requireSignalExit(t, child, child.wait(t), syscall.SIGTERM)
	assert.False(t, signals.InterruptExitSuspended(), "the suspension must be released once the child exits")
	// The cleanup is deregistered: running the exit cleanups again does nothing.
	signals.RunExitCleanups()
}

// TestRunCommand_InterruptForwardedWhenTerminalDoesNotDeliverIt verifies that a SIGINT
// sent only to Atmos (no terminal delivered it to the child) is forwarded to the child.
func TestRunCommand_InterruptForwardedWhenTerminalDoesNotDeliverIt(t *testing.T) {
	forceTerminalDeliversInterrupt(t, false)
	child := startAwaitingChild(t, runCommandChild)

	require.NoError(t, syscall.Kill(os.Getpid(), syscall.SIGINT))

	requireSignalExit(t, child, child.wait(t), syscall.SIGINT)
}

// TestRunCommand_InterruptNotForwardedWhenTerminalDeliversIt is the negative path: when
// the terminal already sent Ctrl-C to the child's process group, Atmos must not send a
// second SIGINT (Terraform, for example, treats a second Ctrl-C as a forced abort).
func TestRunCommand_InterruptNotForwardedWhenTerminalDeliversIt(t *testing.T) {
	forceTerminalDeliversInterrupt(t, true)
	child := startAwaitingChild(t, runCommandChild)

	require.NoError(t, syscall.Kill(os.Getpid(), syscall.SIGINT))
	// Give a wrongly forwarded signal time to arrive before ending the child another way.
	time.Sleep(300 * time.Millisecond)
	assert.Equal(t, awaitSignalReady, child.reportContent(), "the child must not have received a second SIGINT")

	signals.RunExitCleanups()
	requireSignalExit(t, child, child.wait(t), syscall.SIGTERM)
}

// TestStartInteractive_TerminationEscalatesToHangup verifies that `auth shell` is not
// left running when Atmos is terminated: an interactive shell ignores SIGTERM, so SIGHUP
// follows after the escalation delay.
func TestStartInteractive_TerminationEscalatesToHangup(t *testing.T) {
	forceTerminalDeliversInterrupt(t, true)
	setSupervisionTimings(t, 100*time.Millisecond, 10*time.Second)
	child := startAwaitingChild(t, startInteractiveChild, syscall.SIGTERM)

	signals.RunExitCleanups()

	requireSignalExit(t, child, child.wait(t), syscall.SIGHUP)
}

// TestStartInteractive_TerminationStopsAtFirstSignal verifies that no SIGHUP is sent
// when the shell already exited on SIGTERM.
func TestStartInteractive_TerminationStopsAtFirstSignal(t *testing.T) {
	forceTerminalDeliversInterrupt(t, true)
	setSupervisionTimings(t, 5*time.Second, 10*time.Second)
	child := startAwaitingChild(t, startInteractiveChild)

	start := time.Now()
	signals.RunExitCleanups()
	assert.Less(t, time.Since(start), 5*time.Second, "the cleanup must return as soon as the shell exits")

	requireSignalExit(t, child, child.wait(t), syscall.SIGTERM)
}

// TestRunCommand_TerminationGivesUpAfterGrace verifies that a child ignoring SIGTERM
// does not hold Atmos past the grace period.
func TestRunCommand_TerminationGivesUpAfterGrace(t *testing.T) {
	forceTerminalDeliversInterrupt(t, false)
	setSupervisionTimings(t, 100*time.Millisecond, 200*time.Millisecond)
	child := startAwaitingChild(t, runCommandChild, syscall.SIGTERM)

	start := time.Now()
	signals.RunExitCleanups()
	assert.Less(t, time.Since(start), 5*time.Second, "the cleanup must give up after the grace period")
	assert.Equal(t, awaitSignalReady, child.reportContent(), "the child ignored SIGTERM and is still running")

	// End the child with a forwarded SIGINT.
	require.NoError(t, syscall.Kill(os.Getpid(), syscall.SIGINT))
	requireSignalExit(t, child, child.wait(t), syscall.SIGINT)
}

// TestTerminalDeliversInterrupt exercises the real foreground-process-group check. Its
// result depends on how the tests are run (terminal or not), so only stability is checked.
func TestTerminalDeliversInterrupt(t *testing.T) {
	assert.Equal(t, terminalDeliversInterrupt(), terminalDeliversInterrupt())
}
