package asciicast

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	iolib "github.com/cloudposse/atmos/pkg/io"
)

func TestNormalizeSessionOptionsDefaultsAndClampsDurations(t *testing.T) {
	opts := &SessionOptions{
		Width:       -1,
		Height:      0,
		WriteRate:   -time.Second,
		KeyInterval: -time.Millisecond,
	}
	normalizeSessionOptions(opts)

	if opts.Width != DefaultWidth {
		t.Fatalf("width = %d, want %d", opts.Width, DefaultWidth)
	}
	if opts.Height != DefaultHeight {
		t.Fatalf("height = %d, want %d", opts.Height, DefaultHeight)
	}
	if opts.WriteRate != 0 {
		t.Fatalf("write rate = %s, want 0", opts.WriteRate)
	}
	if opts.KeyInterval != 0 {
		t.Fatalf("key interval = %s, want 0", opts.KeyInterval)
	}
}

func TestSessionShellPrefersConfiguredThenEnvironment(t *testing.T) {
	if got := sessionShell("/tmp/custom-shell"); got != "/tmp/custom-shell" {
		t.Fatalf("configured shell = %q", got)
	}

	t.Setenv("SHELL", "/bin/zsh")
	if got := sessionShell(""); got != "/bin/zsh" {
		t.Fatalf("environment shell = %q", got)
	}
}

func TestSessionShellFallsBackByPlatform(t *testing.T) {
	t.Setenv("SHELL", "")

	want := "/bin/sh"
	if runtime.GOOS == "windows" {
		want = "cmd.exe"
	}
	if got := sessionShell(""); got != want {
		t.Fatalf("fallback shell = %q, want %q", got, want)
	}
}

func TestSafePTYSizeBounds(t *testing.T) {
	if got := safePTYSize(-10); got != 1 {
		t.Fatalf("negative size = %d, want 1", got)
	}
	if got := safePTYSize(0); got != 1 {
		t.Fatalf("zero size = %d, want 1", got)
	}
	if got := safePTYSize(120); got != 120 {
		t.Fatalf("normal size = %d, want 120", got)
	}
	if got := safePTYSize(int(^uint16(0)) + 100); got != ^uint16(0) {
		t.Fatalf("large size = %d, want uint16 max", got)
	}
}

func TestKeySequenceAliasesAndLiteralCharacters(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{input: "enter", want: "\r"},
		{input: " Return ", want: "\r"},
		{input: "tab", want: "\t"},
		{input: "esc", want: "\x1b"},
		{input: "escape", want: "\x1b"},
		{input: "backspace", want: "\x7f"},
		{input: "space", want: " "},
		{input: "up", want: "\x1b[A"},
		{input: "down", want: "\x1b[B"},
		{input: "right", want: "\x1b[C"},
		{input: "left", want: "\x1b[D"},
		{input: "x", want: "x"},
	}

	for _, tt := range tests {
		got, err := keySequence(tt.input)
		if err != nil {
			t.Fatalf("keySequence(%q) error: %v", tt.input, err)
		}
		if got != tt.want {
			t.Fatalf("keySequence(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestKeySequenceRejectsUnsupportedKeys(t *testing.T) {
	_, err := keySequence("page-up")
	if !errors.Is(err, ErrUnsupportedCastKey) {
		t.Fatalf("expected ErrUnsupportedCastKey, got %v", err)
	}
}

func TestKeyIntervalUsesFallbackAndParsesOverride(t *testing.T) {
	got, err := keyInterval("", 25*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if got != 25*time.Millisecond {
		t.Fatalf("fallback interval = %s", got)
	}

	got, err = keyInterval("5ms", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if got != 5*time.Millisecond {
		t.Fatalf("parsed interval = %s", got)
	}
}

func TestKeyIntervalRejectsInvalidDuration(t *testing.T) {
	_, err := keyInterval("soon", 0)
	if err == nil || !strings.Contains(err.Error(), "invalid key interval") {
		t.Fatalf("expected invalid key interval error, got %v", err)
	}
}

func TestRunWriteActionWritesRunesAndRejectsInvalidRate(t *testing.T) {
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reader.Close() }()

	err = runWriteAction(writer, &SessionAction{Text: "hi", Rate: "0"}, time.Second)
	if err != nil {
		t.Fatalf("runWriteAction error: %v", err)
	}
	_ = writer.Close()

	content, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "hi" {
		t.Fatalf("written content = %q", content)
	}

	_, writer, err = os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = writer.Close() }()
	err = runWriteAction(writer, &SessionAction{Text: "x", Rate: "fast"}, 0)
	if err == nil || !strings.Contains(err.Error(), "invalid write rate") {
		t.Fatalf("expected invalid write rate error, got %v", err)
	}
}

func TestRunWriteActionSleepsBetweenRunesWhenRatePositive(t *testing.T) {
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reader.Close() }()
	defer func() { _ = writer.Close() }()

	// A tiny positive rate exercises the `if rate > 0 { time.Sleep(rate) }`
	// branch without meaningfully slowing down the test.
	start := time.Now()
	err = runWriteAction(writer, &SessionAction{Text: "ab", Rate: "1ms"}, 0)
	if err != nil {
		t.Fatalf("runWriteAction error: %v", err)
	}
	if elapsed := time.Since(start); elapsed <= 0 {
		t.Fatal("expected non-zero elapsed time when rate is positive")
	}
}

func TestRunWriteActionPropagatesWriteError(t *testing.T) {
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	_ = reader.Close() // closing the read end makes writes to the pipe fail.

	err = runWriteAction(writer, &SessionAction{Text: "boom", Rate: "0"}, 0)
	if err == nil {
		t.Fatal("expected write error when the pipe's reader is closed")
	}
	_ = writer.Close()
}

func TestRunKeyActionWritesRepeatedSequences(t *testing.T) {
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reader.Close() }()

	err = runKeyAction(writer, &SessionAction{Key: "enter", Repeat: 2, Interval: "0"}, time.Second)
	if err != nil {
		t.Fatalf("runKeyAction error: %v", err)
	}
	_ = writer.Close()

	content, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "\r\r" {
		t.Fatalf("written keys = %q", content)
	}
}

func TestRunKeyActionSleepsBetweenRepeatsWhenIntervalPositive(t *testing.T) {
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reader.Close() }()
	defer func() { _ = writer.Close() }()

	start := time.Now()
	err = runKeyAction(writer, &SessionAction{Key: "x", Repeat: 2, Interval: "1ms"}, 0)
	if err != nil {
		t.Fatalf("runKeyAction error: %v", err)
	}
	if elapsed := time.Since(start); elapsed <= 0 {
		t.Fatal("expected non-zero elapsed time when interval is positive")
	}
}

func TestRunKeyActionRejectsUnknownKeySequence(t *testing.T) {
	err := runKeyAction(io.Discard, &SessionAction{Key: "page-up"}, 0)
	if !errors.Is(err, ErrUnsupportedCastKey) {
		t.Fatalf("expected unsupported key error, got %v", err)
	}
}

func TestRunKeyActionRejectsInvalidInterval(t *testing.T) {
	err := runKeyAction(io.Discard, &SessionAction{Key: "enter", Interval: "soon"}, 0)
	if err == nil || !strings.Contains(err.Error(), "invalid key interval") {
		t.Fatalf("expected invalid key interval error, got %v", err)
	}
}

func TestRunKeyActionPropagatesWriteError(t *testing.T) {
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	_ = reader.Close() // closing the read end makes writes to the pipe fail.

	err = runKeyAction(writer, &SessionAction{Key: "enter", Repeat: 1}, 0)
	if err == nil {
		t.Fatal("expected write error when the pipe's reader is closed")
	}
	_ = writer.Close()
}

func TestRunPauseActionCompletesAndCancels(t *testing.T) {
	if err := runPauseAction(context.Background(), &SessionAction{Duration: "1ns"}); err != nil {
		t.Fatalf("pause error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := runPauseAction(ctx, &SessionAction{Duration: "1h"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled, got %v", err)
	}

	err = runPauseAction(context.Background(), &SessionAction{Duration: "later"})
	if err == nil || !strings.Contains(err.Error(), "invalid pause duration") {
		t.Fatalf("expected invalid pause duration, got %v", err)
	}
}

func TestWaitForOutputMatchesTextRegexTimeoutAndCancel(t *testing.T) {
	state := &sessionState{changed: make(chan struct{}, 1), done: make(chan error, 1)}
	state.output.WriteString("deployment complete")
	if err := waitForOutput(context.Background(), state, &SessionAction{Text: "complete", Timeout: "1ms"}); err != nil {
		t.Fatalf("wait text error: %v", err)
	}
	if err := waitForOutput(context.Background(), state, &SessionAction{Regex: "deploy.*complete", Timeout: "1ms"}); err != nil {
		t.Fatalf("wait regex error: %v", err)
	}

	err := waitForOutput(context.Background(), state, &SessionAction{Regex: "[", Timeout: "1ms"})
	if err == nil {
		t.Fatal("expected invalid regex error")
	}
	err = waitForOutput(context.Background(), state, &SessionAction{Text: "missing", Timeout: "not-a-duration"})
	if err == nil || !strings.Contains(err.Error(), "invalid wait timeout") {
		t.Fatalf("expected invalid timeout error, got %v", err)
	}
	err = waitForOutput(context.Background(), state, &SessionAction{Text: "missing", Timeout: "1ns"})
	if !errors.Is(err, ErrWaitTimeout) {
		t.Fatalf("expected wait timeout, got %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err = waitForOutput(ctx, state, &SessionAction{Text: "missing", Timeout: "1h"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled, got %v", err)
	}
}

func TestOutputMatchesReadsBufferedText(t *testing.T) {
	state := &sessionState{changed: make(chan struct{}, 1), done: make(chan error, 1)}
	state.output.WriteString("ready: 42")
	if !outputMatches(state, "ready", nil) {
		t.Fatal("expected text match")
	}
	if !outputMatches(state, "", regexp.MustCompile(`ready: \d+`)) {
		t.Fatal("expected regex match")
	}
	if outputMatches(state, "missing", nil) {
		t.Fatal("unexpected match")
	}
}

func TestRunActionDispatchesAndRejectsUnknownType(t *testing.T) {
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reader.Close() }()

	state := &sessionState{changed: make(chan struct{}, 1), done: make(chan error, 1)}
	err = runAction(context.Background(), writer, state, &SessionAction{Type: "write", Text: "ok", Rate: "0"}, &SessionOptions{})
	if err != nil {
		t.Fatalf("runAction write error: %v", err)
	}
	_ = writer.Close()
	content, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "ok" {
		t.Fatalf("written content = %q", content)
	}

	err = runAction(context.Background(), os.Stdout, state, &SessionAction{Type: "bogus"}, &SessionOptions{})
	if !errors.Is(err, ErrUnknownSessionAction) {
		t.Fatalf("expected unknown action error, got %v", err)
	}
}

func TestRunActionDispatchesPause(t *testing.T) {
	state := &sessionState{changed: make(chan struct{}, 1), done: make(chan error, 1)}
	err := runAction(context.Background(), os.Stdout, state, &SessionAction{Type: "pause", Duration: "1ns"}, &SessionOptions{})
	if err != nil {
		t.Fatalf("runAction pause error: %v", err)
	}
}

func TestNewSessionStateCapturesOutputAndEOF(t *testing.T) {
	if err := iolib.Initialize(); err != nil {
		t.Fatal(err)
	}
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reader.Close() }()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	state := newSessionState(ctx, reader, nil, reader.Close)

	if _, err := writer.WriteString("hello session"); err != nil {
		t.Fatal(err)
	}
	_ = writer.Close()

	select {
	case err := <-state.done:
		if err != nil {
			t.Fatalf("session read error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for session reader")
	}
	if !outputMatches(state, "hello session", nil) {
		t.Fatal("session output was not captured")
	}
}

func TestSessionStateDiscardOutputKeepsTeardownNoiseOutOfCapture(t *testing.T) {
	if err := iolib.Initialize(); err != nil {
		t.Fatal(err)
	}
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reader.Close() }()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	state := newSessionState(ctx, reader, nil, reader.Close)

	if _, err := writer.WriteString("command output"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-state.changed:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for command output")
	}

	state.discardOutput()
	if _, err := writer.WriteString("^D\b\b$ exit"); err != nil {
		t.Fatal(err)
	}
	_ = writer.Close()

	select {
	case err := <-state.done:
		if err != nil {
			t.Fatalf("session read error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for session reader")
	}

	if !outputMatches(state, "command output", nil) {
		t.Fatal("command output was not captured")
	}
	if outputMatches(state, "$ exit", nil) {
		t.Fatal("teardown output was captured")
	}
}

func TestFinishReadPropagatesUnexpectedError(t *testing.T) {
	state := &sessionState{changed: make(chan struct{}, 1), done: make(chan error, 1)}
	unexpected := errors.New("boom")
	state.finishRead(unexpected)
	select {
	case err := <-state.done:
		if !errors.Is(err, unexpected) {
			t.Fatalf("done error = %v, want %v", err, unexpected)
		}
	default:
		t.Fatal("expected error sent to done channel")
	}
}

func TestFinishReadTreatsExpectedErrorsAsNil(t *testing.T) {
	state := &sessionState{changed: make(chan struct{}, 1), done: make(chan error, 1)}
	state.finishRead(io.EOF)
	select {
	case err := <-state.done:
		if err != nil {
			t.Fatalf("done error = %v, want nil for expected read error", err)
		}
	default:
		t.Fatal("expected nil sent to done channel")
	}
}

func TestDiscardOutputIsNoopOnNilState(t *testing.T) {
	var state *sessionState
	state.discardOutput()
}

func TestStopIsNoopOnNilStateAndNilCancel(t *testing.T) {
	var state *sessionState
	state.stop()

	state = &sessionState{}
	state.stop()
}

// fakeSessionProcess implements just enough of sessionProcess's behavior
// (via free functions assigned into the struct) to drive finishSession
// through each of its branches without a real PTY/shell.
func newFakeSessionProcess(t *testing.T, waitErr error) *sessionProcess {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reader.Close() })
	return &sessionProcess{
		input: writer,
		kill:  func() {},
		wait:  func() error { return waitErr },
	}
}

func TestFinishSessionReturnsWaitErrWhenDoneErrIsNil(t *testing.T) {
	proc := newFakeSessionProcess(t, nil)
	defer func() { _ = proc.input.(io.Closer).Close() }()

	done := make(chan error, 1)
	done <- nil
	err := finishSession(context.Background(), proc, done)
	if err != nil {
		t.Fatalf("expected nil error when done and wait both succeed, got %v", err)
	}
}

func TestFinishSessionJoinsDoneAndWaitErrors(t *testing.T) {
	waitErr := errors.New("wait failed")
	proc := newFakeSessionProcess(t, waitErr)
	defer func() { _ = proc.input.(io.Closer).Close() }()

	doneErr := errors.New("read failed")
	done := make(chan error, 1)
	done <- doneErr
	err := finishSession(context.Background(), proc, done)
	if !errors.Is(err, doneErr) || !errors.Is(err, waitErr) {
		t.Fatalf("expected joined errors, got %v", err)
	}
}

func TestFinishSessionReturnsDoneErrWhenWaitSucceeds(t *testing.T) {
	proc := newFakeSessionProcess(t, nil)
	defer func() { _ = proc.input.(io.Closer).Close() }()

	doneErr := errors.New("read failed")
	done := make(chan error, 1)
	done <- doneErr
	err := finishSession(context.Background(), proc, done)
	if !errors.Is(err, doneErr) {
		t.Fatalf("expected done error, got %v", err)
	}
}

func TestFinishSessionKillsAndWaitsOnContextDone(t *testing.T) {
	killed := false
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reader.Close() }()
	proc := &sessionProcess{
		input: writer,
		kill:  func() { killed = true },
		wait:  func() error { return nil },
	}
	defer func() { _ = writer.Close() }()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	done := make(chan error) // never sent to, forcing the ctx.Done() branch.
	err = finishSession(ctx, proc, done)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled, got %v", err)
	}
	if !killed {
		t.Fatal("expected proc.kill to be called on context cancellation")
	}
}

func TestFinishSessionClosesInputWhenCloseInputOnFinishSet(t *testing.T) {
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reader.Close() }()
	closed := false
	proc := &sessionProcess{
		input:              &closeTrackingWriter{Writer: writer, onClose: func() { closed = true }},
		closeInputOnFinish: true,
		kill:               func() {},
		wait:               func() error { return nil },
	}
	done := make(chan error, 1)
	done <- nil
	if err := finishSession(context.Background(), proc, done); err != nil {
		t.Fatal(err)
	}
	if !closed {
		t.Fatal("expected input to be closed when closeInputOnFinish is set")
	}
	_ = writer.Close()
}

func TestFinishSessionLeavesInputOpenWhenCloseInputOnFinishUnset(t *testing.T) {
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reader.Close() }()
	closed := false
	proc := &sessionProcess{
		input:              &closeTrackingWriter{Writer: writer, onClose: func() { closed = true }},
		closeInputOnFinish: false,
		kill:               func() {},
		wait:               func() error { return nil },
	}
	done := make(chan error, 1)
	done <- nil
	if err := finishSession(context.Background(), proc, done); err != nil {
		t.Fatal(err)
	}
	if closed {
		t.Fatal("expected input to stay open when closeInputOnFinish is unset")
	}
	_ = writer.Close()
}

// closeTrackingWriter wraps an io.Writer with a Close method so tests can
// observe whether finishSession closed the session input.
type closeTrackingWriter struct {
	io.Writer
	onClose func()
}

func (w *closeTrackingWriter) Close() error {
	w.onClose()
	return nil
}

func TestFinishSessionKillsAndWaitsOnHardcodedTimeout(t *testing.T) {
	// Neither done nor ctx ever fire, so finishSession must fall through to
	// its final hardcoded time.After(2*time.Second) teardown branch.
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reader.Close() }()
	defer func() { _ = writer.Close() }()

	killed := false
	waited := false
	proc := &sessionProcess{
		input: writer,
		kill:  func() { killed = true },
		wait: func() error {
			waited = true
			return nil
		},
	}

	done := make(chan error) // never sent to.
	start := time.Now()
	err = finishSession(context.Background(), proc, done)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("expected nil error from the hardcoded-timeout branch, got %v", err)
	}
	if elapsed < 2*time.Second {
		t.Fatalf("finishSession returned after %s, want at least 2s", elapsed)
	}
	if !killed {
		t.Fatal("expected proc.kill to be called on hardcoded teardown timeout")
	}
	if !waited {
		t.Fatal("expected proc.wait to be called on hardcoded teardown timeout")
	}
}

func TestWaitForSessionQuietGuardsEachOrConditionIndependently(t *testing.T) {
	// Each of state==nil, quiet<=0, and maxWait<=0 must short-circuit on its
	// own; calling with a background context proves it returns immediately
	// rather than blocking on the (never-created) timers.
	waitForSessionQuiet(context.Background(), nil, time.Second, time.Second)

	state := &sessionState{changed: make(chan struct{}, 1), done: make(chan error, 1)}
	waitForSessionQuiet(context.Background(), state, 0, time.Second)
	waitForSessionQuiet(context.Background(), state, time.Second, 0)
}

func TestWaitForSessionQuietReturnsOnQuietTimerFire(t *testing.T) {
	state := &sessionState{changed: make(chan struct{}, 1), done: make(chan error, 1)}
	start := time.Now()
	waitForSessionQuiet(context.Background(), state, time.Millisecond, time.Hour)
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("expected quiet timer to fire quickly, took %s", elapsed)
	}
}

func TestWaitForSessionQuietReturnsOnMaxTimerFire(t *testing.T) {
	state := &sessionState{changed: make(chan struct{}, 1), done: make(chan error, 1)}
	start := time.Now()
	waitForSessionQuiet(context.Background(), state, time.Hour, time.Millisecond)
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("expected max timer to fire quickly, took %s", elapsed)
	}
}

func TestWaitForSessionQuietReturnsOnContextDone(t *testing.T) {
	state := &sessionState{changed: make(chan struct{}, 1), done: make(chan error, 1)}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	waitForSessionQuiet(ctx, state, time.Hour, time.Hour)
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("expected context cancellation to return quickly, took %s", elapsed)
	}
}

func TestWaitForSessionQuietResetsTimerOnChange(t *testing.T) {
	state := &sessionState{changed: make(chan struct{}, 1), done: make(chan error, 1)}
	state.changed <- struct{}{}
	start := time.Now()
	// The buffered "changed" signal is consumed once, resetting the quiet
	// timer via resetTimer before the quiet period elapses again.
	waitForSessionQuiet(context.Background(), state, 5*time.Millisecond, time.Second)
	if elapsed := time.Since(start); elapsed < 5*time.Millisecond {
		t.Fatalf("expected quiet timer reset to delay return, elapsed %s", elapsed)
	}
}

func TestResetTimerDrainsAlreadyFiredTimer(t *testing.T) {
	timer := time.NewTimer(time.Nanosecond)
	<-timer.C // consume the fired value so Stop() reports false (Go 1.23+ semantics).
	// timer.Stop() now returns false because the timer already expired and
	// its value was already received, so resetTimer must attempt to drain
	// the (now-empty) channel via the `select { case <-timer.C: default: }`
	// pattern before calling Reset — exercising the `default:` arm.
	resetTimer(timer, time.Millisecond)
	select {
	case <-timer.C:
		t.Fatal("timer channel should have been empty after drain attempt")
	default:
	}
	timer.Stop()
}

func TestResetTimerStopsRunningTimer(t *testing.T) {
	timer := time.NewTimer(time.Hour)
	// timer.Stop() returns true here since the timer hasn't fired, so the
	// drain branch is skipped entirely.
	resetTimer(timer, time.Millisecond)
	select {
	case <-timer.C:
		t.Fatal("timer fired unexpectedly")
	default:
	}
	timer.Stop()
}

func TestAnswerTerminalQueries(t *testing.T) {
	var input bytes.Buffer
	answerTerminalQueries([]byte("\x1b]11;?\x1b\\\x1b[6n\x1b]10;?\x1b\\\x1b[6n"), &input)

	got := input.String()
	for _, want := range []string{
		"\x1b]11;rgb:0000/0000/0000\x1b\\",
		"\x1b]10;rgb:ffff/ffff/ffff\x1b\\",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("terminal query response missing %q in %q", want, got)
		}
	}
	if count := strings.Count(got, "\x1b[1;1R"); count != 2 {
		t.Fatalf("cursor position replies = %d, want 2 in %q", count, got)
	}
}

func TestRunSessionExecutesScriptedShellActions(t *testing.T) {
	if err := iolib.Initialize(); err != nil {
		t.Fatal(err)
	}
	shell, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(asciicastSessionHelperEnv, "1")

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	err = RunSession(ctx, &SessionOptions{
		Shell:  shell,
		Width:  80,
		Height: 24,
		Actions: []SessionAction{
			{Type: "write", Text: "printf ready", Rate: "0"},
			{Type: "key", Key: "enter"},
			{Type: "wait", Text: "ready", Timeout: "2s"},
		},
	})
	if err != nil {
		t.Fatalf("RunSession error: %v", err)
	}
}

func TestRunSessionAppliesDirectoryAndEnvironment(t *testing.T) {
	if err := iolib.Initialize(); err != nil {
		t.Fatal(err)
	}
	shell, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	expectedDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	cwdPattern := "cwd=(" + regexp.QuoteMeta(dir) + "|" + regexp.QuoteMeta(expectedDir) + ")"
	t.Setenv(asciicastSessionHelperEnv, "1")

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	err = RunSession(ctx, &SessionOptions{
		Shell: shell,
		Dir:   dir,
		Env: map[string]string{
			"ATMOS_CAST_SESSION_MARKER": "from-session",
		},
		Actions: []SessionAction{
			// The freshly spawned PTY shell answers an initial burst of
			// terminal capability queries asynchronously right after start;
			// a short pause lets that settle before the scripted "write"
			// races it (see TestAnswerTerminalQueries for the query/response
			// exchange this waits out).
			{Type: "pause", Duration: "300ms"},
			{Type: "write", Text: "print context", Rate: "0"},
			{Type: "key", Key: "enter"},
			{Type: "wait", Regex: cwdPattern, Timeout: "2s"},
			{Type: "wait", Text: "marker=from-session", Timeout: "2s"},
		},
	})
	if err != nil {
		t.Fatalf("RunSession error: %v", err)
	}
}

func TestRunSessionDefaultsNilOptions(t *testing.T) {
	if err := iolib.Initialize(); err != nil {
		t.Fatal(err)
	}
	shell, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	// RunSession(ctx, nil) must default opts to &SessionOptions{} before use;
	// point SHELL at the test binary so sessionShell("") still resolves to a
	// deterministic, cross-platform "shell" instead of a real one.
	t.Setenv("SHELL", shell)
	t.Setenv(asciicastSessionHelperEnv, "1")

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := RunSession(ctx, nil); err != nil {
		t.Fatalf("RunSession with nil opts: %v", err)
	}
}

func TestRunSessionWrapsStartupError(t *testing.T) {
	// A nonexistent shell binary makes startSessionShell fail, exercising
	// RunSession's `fmt.Errorf("start cast session shell: %w", err)` branch.
	err := RunSession(context.Background(), &SessionOptions{
		Shell: filepath.Join(t.TempDir(), "does-not-exist-shell"),
	})
	if err == nil || !strings.Contains(err.Error(), "start cast session shell") {
		t.Fatalf("expected wrapped startup error, got %v", err)
	}
}

func TestRunSessionReturnsActionErrors(t *testing.T) {
	if err := iolib.Initialize(); err != nil {
		t.Fatal(err)
	}
	shell, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(asciicastSessionHelperEnv, "1")

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	err = RunSession(ctx, &SessionOptions{
		Shell:   shell,
		Actions: []SessionAction{{Type: "bogus"}},
	})
	if !errors.Is(err, ErrUnknownSessionAction) {
		t.Fatalf("expected unknown action error, got %v", err)
	}
}

func TestSessionProcessWaitIsIdempotent(t *testing.T) {
	want := errors.New("process exited")
	var calls atomic.Int32
	wait := newSessionProcessWait(func() error {
		calls.Add(1)
		time.Sleep(10 * time.Millisecond)
		return want
	})

	var wg sync.WaitGroup
	errs := make(chan error, 3)
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- wait()
		}()
	}
	wg.Wait()
	close(errs)

	if calls.Load() != 1 {
		t.Fatalf("wait function calls = %d, want 1", calls.Load())
	}
	for err := range errs {
		if !errors.Is(err, want) {
			t.Fatalf("wait error = %v, want %v", err, want)
		}
	}
}

func TestWaitForSessionProcessTimesOut(t *testing.T) {
	err := waitForSessionProcess(&sessionProcess{
		wait: newSessionProcessWait(func() error {
			time.Sleep(time.Hour)
			return nil
		}),
	}, time.Nanosecond)
	if !errors.Is(err, errSessionProcessWaitTimeout) {
		t.Fatalf("expected session process wait timeout, got %v", err)
	}
}
