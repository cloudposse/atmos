package asciicast

import (
	"context"
	"errors"
	"io"
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	iolib "github.com/cloudposse/atmos/pkg/io"
)

// appendSessionOutput simulates the shell producing output, the way the
// session reader goroutine appends it to the wait-matching buffer.
func appendSessionOutput(state *sessionState, text string) {
	state.mu.Lock()
	state.output.WriteString(text)
	state.mu.Unlock()
	select {
	case state.changed <- struct{}{}:
	default:
	}
}

func newWaitTestState() *sessionState {
	return &sessionState{changed: make(chan struct{}, 1), done: make(chan error, 1)}
}

func TestWaitIgnoresOutputFromBeforeTheLastInputAction(t *testing.T) {
	state := newWaitTestState()
	appendSessionOutput(state, "build finished\r\n> ")

	// Nothing has been typed yet, so the startup output is still visible.
	require.NoError(t, waitForOutput(context.Background(), state, &SessionAction{Text: "build finished", Timeout: "1s"}))

	require.NoError(t, runAction(context.Background(), &syncWriter{w: io.Discard}, state, &SessionAction{Type: "write", Text: "echo hi"}, &SessionOptions{}))
	appendSessionOutput(state, "echo hi")

	// Stale output from before the write must not satisfy a later wait.
	err := waitForOutput(context.Background(), state, &SessionAction{Text: "build finished", Timeout: "30ms"})
	require.ErrorIs(t, err, ErrWaitTimeout)
	err = waitForOutput(context.Background(), state, &SessionAction{Regex: `build\s+finished`, Timeout: "30ms"})
	require.ErrorIs(t, err, ErrWaitTimeout)
	// The timeout error names what was awaited and for how long.
	assert.Contains(t, err.Error(), `/build\s+finished/ not seen within 30ms`)
}

func TestWaitIgnoresTypedCommandEchoAfterEnter(t *testing.T) {
	state := newWaitTestState()
	input := &syncWriter{w: io.Discard}
	opts := &SessionOptions{}

	require.NoError(t, runAction(context.Background(), input, state, &SessionAction{Type: "write", Text: "sleep 3; echo late"}, opts))
	// The tty echoes the typed text.
	appendSessionOutput(state, "sleep 3; echo late")
	require.NoError(t, runAction(context.Background(), input, state, &SessionAction{Type: "key", Key: "enter"}, opts))
	appendSessionOutput(state, "\r\n")

	// "late" exists only in the echoed command; the wait must keep waiting.
	err := waitForOutput(context.Background(), state, &SessionAction{Regex: "late", Timeout: "30ms"})
	require.ErrorIs(t, err, ErrWaitTimeout)

	// The command's real output arrives later and satisfies the wait.
	go func() {
		time.Sleep(20 * time.Millisecond)
		appendSessionOutput(state, "late\r\n> ")
	}()
	require.NoError(t, waitForOutput(context.Background(), state, &SessionAction{Regex: "late", Timeout: "2s"}))
}

func TestWaitAfterWriteWithoutEnterSeesTheTypedEcho(t *testing.T) {
	state := newWaitTestState()
	appendSessionOutput(state, "> ")
	require.NoError(t, runAction(context.Background(), &syncWriter{w: io.Discard}, state, &SessionAction{Type: "write", Text: "partial"}, &SessionOptions{}))
	appendSessionOutput(state, "partial")

	// Only a following Enter moves the mark past the echo.
	require.NoError(t, waitForOutput(context.Background(), state, &SessionAction{Text: "partial", Timeout: "1s"}))
}

func TestWaitRegexAnchorsToEndOfBufferAfterInput(t *testing.T) {
	state := newWaitTestState()
	appendSessionOutput(state, "old prompt> ")
	state.markInput()
	appendSessionOutput(state, "result\r\nnew prompt> ")

	require.NoError(t, waitForOutput(context.Background(), state, &SessionAction{Regex: `prompt> $`, Timeout: "1s"}))
	err := waitForOutput(context.Background(), state, &SessionAction{Regex: `^old prompt`, Timeout: "30ms"})
	require.ErrorIs(t, err, ErrWaitTimeout)
}

func TestKeyActionMarksInputAfterTheTypedEchoSettles(t *testing.T) {
	state := newWaitTestState()
	require.NoError(t, runAction(context.Background(), &syncWriter{w: io.Discard}, state, &SessionAction{Type: "write", Text: "echo x"}, &SessionOptions{}))

	// The echo lands just after the key action starts; the settle period must
	// absorb it so it counts as "before" the Enter.
	go func() {
		time.Sleep(5 * time.Millisecond)
		appendSessionOutput(state, "echo x")
	}()
	require.NoError(t, runAction(context.Background(), &syncWriter{w: io.Discard}, state, &SessionAction{Type: "key", Key: "enter"}, &SessionOptions{}))

	assert.False(t, outputMatches(state, "echo x", nil))
	assert.False(t, outputMatches(state, "", regexp.MustCompile(`echo`)))
}

func TestMarkInputIsNilSafe(t *testing.T) {
	var state *sessionState
	assert.NotPanics(t, state.markInput)
}

func TestOutputSinceInputClampsAnOffsetPastTheBuffer(t *testing.T) {
	state := newWaitTestState()
	appendSessionOutput(state, "abc")
	state.mu.Lock()
	state.inputOffset = 99
	state.mu.Unlock()
	assert.Equal(t, "abc", state.outputSinceInput())
}

func TestRunSessionWaitIgnoresEarlierOutputAndTypedEcho(t *testing.T) {
	require.NoError(t, iolib.Initialize())
	shell := sessionHelperShell(t)

	run := func(actions ...SessionAction) error {
		ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
		defer cancel()
		// Synchronize with process readiness: Windows pipes do not echo input.
		actions = append([]SessionAction{{Type: "wait", Text: sessionHelperBanner, Timeout: "10s"}}, actions...)
		return RunSession(ctx, &SessionOptions{Shell: shell, Actions: actions})
	}

	// "ready" is printed by the first command only; a later wait for it must
	// time out instead of matching the earlier output.
	err := run(
		SessionAction{Type: "write", Text: "printf ready", Rate: "0"},
		SessionAction{Type: "key", Key: "enter"},
		SessionAction{Type: "wait", Text: "ready", Timeout: "10s"},
		SessionAction{Type: "write", Text: "print context", Rate: "0"},
		SessionAction{Type: "key", Key: "enter"},
		SessionAction{Type: "wait", Text: "ready", Timeout: "400ms"},
	)
	require.ErrorIs(t, err, ErrWaitTimeout)
	assert.Contains(t, err.Error(), `"ready" not seen within 400ms`)

	// The typed command text alone must not satisfy the wait after Enter.
	err = run(
		SessionAction{Type: "write", Text: "print context", Rate: "0"},
		SessionAction{Type: "key", Key: "enter"},
		SessionAction{Type: "wait", Text: "print context", Timeout: "400ms"},
	)
	require.ErrorIs(t, err, ErrWaitTimeout)
	assert.Contains(t, err.Error(), `"print context" not seen within 400ms`)

	// Real output produced after Enter still satisfies the wait.
	require.NoError(t, run(
		SessionAction{Type: "write", Text: "print context", Rate: "0"},
		SessionAction{Type: "key", Key: "enter"},
		SessionAction{Type: "wait", Regex: "marker=", Timeout: "10s"},
	))
}

// TestReapKilledSessionProcessDropsExpectedKillError verifies the shell's own
// "signal: killed" exit, caused by this package killing it after a failed
// action, is not joined onto that action's error, while a process that never
// exits is still reported.
func TestReapKilledSessionProcessDropsExpectedKillError(t *testing.T) {
	killed := &sessionProcess{wait: func() error { return errors.New("signal: killed") }}
	assert.NoError(t, reapKilledSessionProcess(killed))

	assert.NoError(t, reapKilledSessionProcess(nil))
}
