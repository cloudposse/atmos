package asciicast

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
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
	state := newSessionState(ctx, reader, reader.Close)

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
			{Type: "write", Text: "print context", Rate: "0"},
			{Type: "key", Key: "enter"},
			{Type: "wait", Text: "cwd=" + expectedDir, Timeout: "2s"},
			{Type: "wait", Text: "marker=from-session", Timeout: "2s"},
		},
	})
	if err != nil {
		t.Fatalf("RunSession error: %v", err)
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
