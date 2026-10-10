package process

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/signals"
)

// Helper-mode contract. The test binary re-executes itself (see TestMain in
// script_test.go) as a cross-platform stand-in for "sleep" and for a process
// that spawns a grandchild, so no Unix-only binaries are needed.
const (
	helperModeEnv   = "_ATMOS_TEST_PROCESS_MODE"
	helperPIDDirEnv = "_ATMOS_TEST_PID_DIR"

	// Mode parent spawns a grandchild (modeSleep) and then sleeps.
	modeParent = "parent"
	// Mode slowParent delays readiness beyond the former 750ms deadline.
	modeSlowParent = "slow-parent"
	// Mode stubbornParent spawns a grandchild that ignores graceful termination.
	modeStubbornParent = "stubborn-parent"
	modeStubbornChild  = "stubborn-child"
	// Mode sleep records its pid and sleeps.
	modeSleep = "sleep"
	// Mode input waits for stdin to close before exiting successfully.
	modeInput = "input"
	// Mode holder spawns a grandchild (modeSleep) that inherits stdout, so the pipe stays open after
	// the child is killed, and then sleeps.
	modeHolder = "holder"
	// Mode detach spawns a grandchild that inherits stdout and keeps running,
	// then exits 0 immediately.
	modeDetach = "detach"

	helperSleep = 30 * time.Second
	pidWait     = 10 * time.Second
	goneWait    = 5 * time.Second
)

// runProcessHelper implements the helper modes. It never returns normally
// except for modeDetach.
func runProcessHelper(mode string) {
	dir := os.Getenv(helperPIDDirEnv)
	switch mode {
	case modeParent, modeSlowParent:
		if mode == modeSlowParent {
			time.Sleep(time.Second)
		}
		writePIDFile(dir, "parent")
		spawnGrandchild(modeSleep, false)
		time.Sleep(helperSleep)
	case modeHolder:
		writePIDFile(dir, "parent")
		spawnGrandchild(modeSleep, true)
		time.Sleep(helperSleep)
	case modeStubbornParent:
		writePIDFile(dir, "parent")
		spawnGrandchild(modeStubbornChild, false)
		time.Sleep(helperSleep)
	case modeStubbornChild:
		// Ignoring every incoming signal is portable; SIGKILL remains effective.
		signal.Ignore()
		writePIDFile(dir, "child")
		time.Sleep(helperSleep)
	case modeSleep:
		writePIDFile(dir, "child")
		time.Sleep(helperSleep)
	case modeInput:
		writePIDFile(dir, "child")
		_, _ = io.Copy(io.Discard, os.Stdin)
	case modeDetach:
		spawnGrandchild(modeSleep, true)
	}
}

func writePIDFile(dir, name string) {
	_ = os.WriteFile(filepath.Join(dir, name+".pid"), []byte(strconv.Itoa(os.Getpid())), 0o600)
}

func spawnGrandchild(mode string, inheritStdout bool) {
	exe, err := os.Executable()
	if err != nil {
		os.Exit(2)
	}
	gc := exec.Command(exe)
	gc.Env = append(os.Environ(), helperModeEnv+"="+mode)
	if inheritStdout {
		gc.Stdout = os.Stdout
	}
	if err := gc.Start(); err != nil {
		os.Exit(2)
	}
}

// helperSpec builds a TaskSpec that re-executes the test binary in mode.
func helperSpec(t *testing.T, mode, pidDir string, stdout io.Writer) TaskSpec {
	t.Helper()
	exe, err := os.Executable()
	require.NoError(t, err)
	return TaskSpec{
		Command: exe,
		Dir:     pidDir,
		Env:     append(os.Environ(), helperModeEnv+"="+mode, helperPIDDirEnv+"="+pidDir),
		Streams: Streams{Stdout: stdout},
	}
}

// readPID returns the pid recorded by a helper, or 0 when not yet written.
func readPID(dir, name string) int {
	data, err := os.ReadFile(filepath.Join(dir, name+".pid"))
	if err != nil {
		return 0
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0
	}
	return pid
}

func waitForPID(t *testing.T, dir, name string) int {
	t.Helper()
	var pid int
	require.Eventually(t, func() bool {
		pid = readPID(dir, name)
		return pid > 0
	}, pidWait, 20*time.Millisecond, "helper %q never reported its pid", name)
	return pid
}

// killAtCleanup makes sure a helper process cannot outlive the test run.
func killAtCleanup(t *testing.T, dir string, names ...string) {
	t.Helper()
	t.Cleanup(func() {
		for _, name := range names {
			if pid := readPID(dir, name); pid > 0 {
				if p, err := os.FindProcess(pid); err == nil {
					_ = p.Kill()
				}
			}
		}
	})
}

type runOutcome struct {
	result Result
}

// runAsync starts spec in a goroutine and returns a channel with its result.
func runAsync(ctx context.Context, spec *TaskSpec) <-chan runOutcome {
	ch := make(chan runOutcome, 1)
	go func() {
		ch <- runOutcome{result: NewDefaultRunner().Run(ctx, *spec)}
	}()
	return ch
}

func awaitRun(t *testing.T, ch <-chan runOutcome) Result {
	t.Helper()
	select {
	case out := <-ch:
		return out.result
	case <-time.After(goneWait + 5*time.Second):
		t.Fatal("Run did not return in time")
		return Result{}
	}
}

func requireGone(t *testing.T, pid int, what string) {
	t.Helper()
	require.Eventually(t, func() bool { return processGone(pid) }, goneWait, 25*time.Millisecond,
		"%s (pid %d) is still running", what, pid)
}

func skipOnWindows(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("process-group termination is Unix-only; Windows only terminates the direct child")
	}
}

func TestRun_ContextCancelKillsGrandchildren(t *testing.T) {
	skipOnWindows(t)
	dir := t.TempDir()
	killAtCleanup(t, dir, "parent", "child")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := runAsync(ctx, ptr(helperSpec(t, modeParent, dir, nil)))

	parent := waitForPID(t, dir, "parent")
	child := waitForPID(t, dir, "child")

	cancel()
	res := awaitRun(t, done)

	assert.True(t, res.Canceled)
	requireGone(t, parent, "child process")
	requireGone(t, child, "grandchild process")
	assert.Empty(t, liveChildren(), "finished children must be unregistered")
}

// The leader exits immediately on SIGTERM, but the grandchild has no inherited
// output pipe and ignores SIGTERM. Waiting only for the leader loses the group.
func TestRun_ContextCancelKillsGrandchildAfterLeaderExits(t *testing.T) {
	skipOnWindows(t)
	dir := t.TempDir()
	killAtCleanup(t, dir, "parent", "child")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := runAsync(ctx, ptr(helperSpec(t, modeStubbornParent, dir, nil)))
	parent := waitForPID(t, dir, "parent")
	child := waitForPID(t, dir, "child")

	cancel()
	res := awaitRun(t, done)

	assert.True(t, res.Canceled)
	requireGone(t, parent, "child process")
	requireGone(t, child, "SIGTERM-ignoring grandchild process")
	assert.Empty(t, liveChildren(), "cleanup must finish before Run returns")
}

func TestRun_ContextDeadlineKillsGrandchildren(t *testing.T) {
	skipOnWindows(t)
	dir := t.TempDir()
	killAtCleanup(t, dir, "parent", "child")

	// Race-instrumented subprocess startup needs time on a busy CI runner.
	// Both helpers must be ready before the real context deadline expires.
	ctx, cancel := context.WithTimeout(context.Background(), 2*pidWait)
	defer cancel()
	done := runAsync(ctx, ptr(helperSpec(t, modeSlowParent, dir, nil)))
	parent := waitForPID(t, dir, "parent")
	child := waitForPID(t, dir, "child")
	require.NoError(t, ctx.Err(), "both helpers must start before the deadline")

	<-ctx.Done()
	res := awaitRun(t, done)

	assert.ErrorIs(t, ctx.Err(), context.DeadlineExceeded)
	assert.ErrorIs(t, res.Err, context.DeadlineExceeded)
	assert.True(t, res.Canceled)
	requireGone(t, parent, "child process")
	requireGone(t, child, "grandchild process")
	assert.Empty(t, liveChildren(), "deadline cleanup must unregister the child")
}

// Direct-child cleanup is guaranteed on every platform, including Windows.
// A completed Run proves the child was reaped without a platform-specific liveness probe.
func TestExitCleanupKillsRegisteredDirectChild(t *testing.T) {
	dir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	done := runAsync(ctx, ptr(helperSpec(t, modeSleep, dir, nil)))
	finished := false
	t.Cleanup(func() {
		cancel()
		if !finished {
			awaitRun(t, done)
		}
	})

	pid := waitForPID(t, dir, "child")
	require.Eventually(t, func() bool {
		registered := liveChildren()
		return len(registered) == 1 && registered[0].pid == pid
	}, pidWait, 10*time.Millisecond, "running helper must be registered for exit cleanup")

	// Keep the context active so cancellation cannot make a broken exit cleanup pass.
	signals.RunExitCleanups()
	res := awaitRun(t, done)
	finished = true

	require.NoError(t, ctx.Err())
	assert.True(t, res.Started)
	assert.False(t, res.Canceled)
	var exitErr *exec.ExitError
	require.ErrorAs(t, res.Err, &exitErr, "exit cleanup must terminate the running child")
	assert.NotZero(t, res.ExitCode)
	assert.True(t, exitErr.Exited() || res.Signaled)
	assert.Empty(t, liveChildren(), "the reaped child must be unregistered")
}

func TestExitCleanupKillsRegisteredProcessTrees(t *testing.T) {
	skipOnWindows(t)
	for _, mode := range []string{modeParent, modeStubbornParent} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			killAtCleanup(t, dir, "parent", "child")

			done := runAsync(context.Background(), ptr(helperSpec(t, mode, dir, nil)))
			parent := waitForPID(t, dir, "parent")
			child := waitForPID(t, dir, "child")
			require.Eventually(t, func() bool { return len(liveChildren()) == 1 }, pidWait, 10*time.Millisecond)

			// This is the same entry point main's signal handler uses. The stubborn
			// grandchild must still be killed after its leader exits on SIGTERM.
			signals.RunExitCleanups()

			requireGone(t, parent, "child process")
			requireGone(t, child, "grandchild process")
			res := awaitRun(t, done)
			assert.False(t, res.Success())
			assert.Empty(t, liveChildren())
		})
	}
}

func TestExitCleanupStaleRegistrationLeavesReplacementRunning(t *testing.T) {
	dir := t.TempDir()
	input, release, err := os.Pipe()
	require.NoError(t, err)
	defer input.Close()
	defer release.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	spec := helperSpec(t, modeInput, dir, nil)
	spec.Streams.Stdin = input
	done := runAsync(ctx, &spec)
	pid := waitForPID(t, dir, "child")
	require.Eventually(t, func() bool { return len(liveChildren()) == 1 }, pidWait, 10*time.Millisecond)

	// Simulate a completed invocation's stale registration after its numeric
	// PID is reused. Removing or canceling it must leave the real child alone.
	completedCtx, completedCancel := context.WithCancel(context.Background())
	defer completedCancel()
	stale := registerChild(pid, runtime.GOOS != "windows", completedCancel)
	unregisterChild(stale)
	require.Len(t, liveChildren(), 1, "stale removal must preserve the replacement registration")
	cancelChildren([]*childProc{stale}, childExitCleanupWait)
	require.ErrorIs(t, completedCtx.Err(), context.Canceled)

	// Closing stdin lets the replacement prove it survived cleanup by exiting 0.
	require.NoError(t, release.Close())
	res := awaitRun(t, done)
	require.True(t, res.Success(), "stale cleanup terminated the replacement: %v", res.Err)
	assert.Empty(t, liveChildren(), "the replacement must keep its own registration")
}

func TestExitCleanupCancelsAllChildrenBeforeBoundedWait(t *testing.T) {
	firstCtx, firstCancel := context.WithCancel(context.Background())
	secondCtx, secondCancel := context.WithCancel(context.Background())
	first := registerChild(1, false, firstCancel)
	second := registerChild(2, false, secondCancel)
	t.Cleanup(func() {
		firstCancel()
		secondCancel()
		unregisterChild(first)
		unregisterChild(second)
	})

	// Neither invocation reports completion. Shutdown must still cancel both
	// and return within its shared budget instead of waiting indefinitely.
	done := make(chan struct{})
	go func() {
		cancelChildren([]*childProc{first, second}, time.Millisecond)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(goneWait):
		t.Fatal("exit cleanup exceeded its bounded wait")
	}
	assert.ErrorIs(t, firstCtx.Err(), context.Canceled)
	assert.ErrorIs(t, secondCtx.Err(), context.Canceled)
}

func TestRun_TerminalStdinChildIsNotMovedToNewProcessGroup(t *testing.T) {
	skipOnWindows(t)
	orig := stdinIsTerminal
	stdinIsTerminal = func(io.Reader) bool { return true }
	t.Cleanup(func() { stdinIsTerminal = orig })

	dir := t.TempDir()
	killAtCleanup(t, dir, "child")
	done := runAsync(context.Background(), ptr(helperSpec(t, modeSleep, dir, nil)))
	pid := waitForPID(t, dir, "child")

	assert.Equal(t, currentProcessGroup(), processGroupOf(t, pid),
		"a child that inherits a terminal must stay in the foreground process group")

	// It is still tracked, so the exit cleanup signals the direct child.
	require.Len(t, liveChildren(), 1)
	assert.False(t, liveChildren()[0].grouped)
	signals.RunExitCleanups()
	requireGone(t, pid, "terminal child")
	awaitRun(t, done)
}

func TestRun_NonTerminalStdinChildGetsOwnProcessGroup(t *testing.T) {
	skipOnWindows(t)
	dir := t.TempDir()
	killAtCleanup(t, dir, "child")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := runAsync(ctx, ptr(helperSpec(t, modeSleep, dir, nil)))
	pid := waitForPID(t, dir, "child")

	assert.NotEqual(t, currentProcessGroup(), processGroupOf(t, pid))
	assert.Equal(t, pid, processGroupOf(t, pid), "child must lead its own process group")
	cancel()
	awaitRun(t, done)
}

func TestNewChildGroup_Configuration(t *testing.T) {
	tests := []struct {
		name         string
		stdin        io.Reader
		terminal     bool
		wantGrouped  bool
		wantWaitDely time.Duration
	}{
		{name: "nil stdin", stdin: nil, wantGrouped: runtime.GOOS != "windows"},
		{name: "reader stdin", stdin: strings.NewReader("x"), wantGrouped: runtime.GOOS != "windows"},
		{name: "terminal stdin", stdin: os.Stdin, terminal: true, wantGrouped: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			orig := stdinIsTerminal
			stdinIsTerminal = func(io.Reader) bool { return tc.terminal }
			t.Cleanup(func() { stdinIsTerminal = orig })

			cmd := exec.Command(os.Args[0])
			cmd.Stdin = tc.stdin
			g := newChildGroup(cmd)

			assert.Equal(t, tc.wantGrouped, g.grouped)
			assertProcessGroupAttr(t, cmd, tc.wantGrouped)
			if tc.wantGrouped {
				assert.Equal(t, childShutdownGrace, cmd.WaitDelay)
			} else {
				assert.Zero(t, cmd.WaitDelay)
			}
		})
	}
}

func TestStdinIsTerminal(t *testing.T) {
	assert.False(t, stdinIsTerminal(nil))
	assert.False(t, stdinIsTerminal(strings.NewReader("x")))

	f, err := os.Open(os.Args[0])
	require.NoError(t, err)
	defer f.Close()
	assert.False(t, stdinIsTerminal(f), "a regular file is not a terminal")
}

func TestRun_BackgroundedOutputHolderDoesNotHangOrFail(t *testing.T) {
	skipOnWindows(t)
	dir := t.TempDir()
	killAtCleanup(t, dir, "child")

	var out bytes.Buffer
	start := time.Now()
	res := awaitRun(t, runAsync(context.Background(), ptr(helperSpec(t, modeDetach, dir, &out))))

	assert.True(t, res.Success(), "exit 0 must stay a success even if a grandchild holds stdout: %v", res.Err)
	assert.Less(t, time.Since(start), childShutdownGrace+3*time.Second)
	// The deliberately detached grandchild is left alone on a normal exit.
	child := waitForPID(t, dir, "child")
	assert.False(t, processGone(child))
}

func TestTolerateWaitDelay(t *testing.T) {
	waitDelay := exec.ErrWaitDelay
	other := errors.New("boom")

	t.Run("ungrouped keeps error", func(t *testing.T) {
		g := &childGroup{cmd: &exec.Cmd{}}
		assert.ErrorIs(t, g.tolerateWaitDelay(context.Background(), waitDelay), exec.ErrWaitDelay)
	})
	t.Run("nil stays nil", func(t *testing.T) {
		g := &childGroup{cmd: &exec.Cmd{}, grouped: true}
		assert.NoError(t, g.tolerateWaitDelay(context.Background(), nil))
	})
	t.Run("unrelated error kept", func(t *testing.T) {
		g := &childGroup{cmd: &exec.Cmd{}, grouped: true}
		assert.ErrorIs(t, g.tolerateWaitDelay(context.Background(), other), other)
	})
	t.Run("missing process state keeps error", func(t *testing.T) {
		g := &childGroup{cmd: &exec.Cmd{}, grouped: true}
		assert.ErrorIs(t, g.tolerateWaitDelay(context.Background(), waitDelay), exec.ErrWaitDelay)
	})
	t.Run("canceled context keeps error", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		g := &childGroup{cmd: &exec.Cmd{}, grouped: true}
		assert.ErrorIs(t, g.tolerateWaitDelay(ctx, waitDelay), exec.ErrWaitDelay)
	})
}

func TestKillAllChildren_NoChildrenIsNoop(t *testing.T) {
	require.Empty(t, liveChildren())
	start := time.Now()
	killAllChildren()
	assert.Less(t, time.Since(start), time.Second)
}

func ptr[T any](v T) *T { return &v }
