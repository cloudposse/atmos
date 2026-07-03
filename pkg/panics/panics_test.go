package panics

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	cockroachErrors "github.com/cockroachdb/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testOptions returns an Options preconfigured to capture output in a
// buffer and write crash reports to a temp dir, so tests can assert
// content without touching global state or pkg/ui.
func testOptions(t *testing.T, debugInline bool) (*bytes.Buffer, *Options) {
	t.Helper()
	buf := &bytes.Buffer{}
	return buf, &Options{
		crashDir:        t.TempDir(),
		args:            []string{"atmos", "test-cmd"},
		now:             func() time.Time { return time.Date(2026, 4, 17, 22, 43, 10, 0, time.UTC) },
		exitCode:        PanicExitCode,
		showStackInline: debugInline,
		useUI:           false,
		stderr:          buf,
	}
}

// fakeRuntimeError is a minimal runtime.Error to exercise the
// summarize() case without actually dereferencing a nil pointer.
// Only RuntimeError() and Error() are required to satisfy
// runtime.Error; additional Stringer/GoStringer methods would be
// dead weight.
type fakeRuntimeError struct{ msg string }

func (f fakeRuntimeError) RuntimeError() {}
func (f fakeRuntimeError) Error() string { return f.msg }

func TestHandlePanic_StringValue(t *testing.T) {
	buf, opts := testOptions(t, false)

	code := HandlePanic("boom", []byte("stack-placeholder"), opts)

	assert.Equal(t, PanicExitCode, code)
	out := buf.String()
	assert.Contains(t, out, "Atmos crashed unexpectedly")
	assert.Contains(t, out, "**Summary:** boom")
	assert.Contains(t, out, "github.com/cloudposse/atmos/issues")
	// Stack must NOT be inline in non-debug mode.
	assert.NotContains(t, out, "stack-placeholder")
	// And the debug hint should appear.
	assert.Contains(t, out, "ATMOS_LOGS_LEVEL=Debug")
}

func TestHandlePanic_ErrorValue(t *testing.T) {
	buf, opts := testOptions(t, false)

	code := HandlePanic(errors.New("wrapped failure"), []byte("stk"), opts)

	assert.Equal(t, PanicExitCode, code)
	assert.Contains(t, buf.String(), "**Summary:** wrapped failure")
}

func TestHandlePanic_RuntimeError(t *testing.T) {
	buf, opts := testOptions(t, false)

	err := fakeRuntimeError{msg: "runtime error: invalid memory address or nil pointer dereference"}
	code := HandlePanic(err, []byte("stk"), opts)

	assert.Equal(t, PanicExitCode, code)
	assert.Contains(t, buf.String(), "runtime error: invalid memory address or nil pointer dereference")
}

func TestHandlePanic_DebugModeIncludesStackInline(t *testing.T) {
	buf, opts := testOptions(t, true)
	stack := []byte("goroutine 1 [running]:\nmain.doStuff(...)\n")

	HandlePanic("boom", stack, opts)

	out := buf.String()
	assert.Contains(t, out, "Stack trace:")
	assert.Contains(t, out, "main.doStuff")
	// Debug hint should NOT appear when stack is already inline.
	assert.NotContains(t, out, "ATMOS_LOGS_LEVEL=Debug")
}

func TestHandlePanic_NonDebugModeHidesStackButWritesReport(t *testing.T) {
	buf, opts := testOptions(t, false)
	stack := []byte("goroutine 1 [running]:\nmain.doStuff(...)\n")

	HandlePanic("boom", stack, opts)

	// Stack is not inline in user-visible output.
	assert.NotContains(t, buf.String(), "main.doStuff")

	// But it IS in the crash report file.
	entries, err := os.ReadDir(opts.crashDir)
	require.NoError(t, err)
	require.Len(t, entries, 1, "exactly one crash report file should exist")
	assert.True(t, strings.HasPrefix(entries[0].Name(), "atmos-crash-"), "crash file name prefix")

	body, err := os.ReadFile(filepath.Join(opts.crashDir, entries[0].Name()))
	require.NoError(t, err)
	bodyStr := string(body)
	assert.Contains(t, bodyStr, "main.doStuff", "stack should be in report")
	assert.Contains(t, bodyStr, "Panic: boom")
	assert.Contains(t, bodyStr, "atmos test-cmd", "argv should be in report")
	assert.Contains(t, bodyStr, "Built with:", "Go build version label")
	assert.Contains(t, bodyStr, runtime.GOOS)
	assert.Contains(t, bodyStr, runtime.Version())
}

func TestHandlePanic_CrashFileUnwritable(t *testing.T) {
	buf, opts := testOptions(t, false)
	// Point at a path that doesn't exist and can't be created (file,
	// not directory, at the parent).
	parent := t.TempDir()
	blocker := filepath.Join(parent, "blocker")
	require.NoError(t, os.WriteFile(blocker, []byte("x"), 0o600))
	opts.crashDir = filepath.Join(blocker, "under-a-file")

	// Must not re-panic.
	assert.NotPanics(t, func() {
		code := HandlePanic("boom", []byte("stk"), opts)
		assert.Equal(t, PanicExitCode, code)
	})

	// Friendly message still printed to the fallback writer.
	out := buf.String()
	assert.Contains(t, out, "Atmos crashed unexpectedly")
	assert.Contains(t, out, "**Summary:** boom")
}

func TestHandlePanic_AppliesDefaults(t *testing.T) {
	// Empty Options except for the fallback writer and a custom
	// crashDir — verify applyDefaults fills in args / now / exitCode.
	buf := &bytes.Buffer{}
	opts := &Options{
		crashDir: t.TempDir(),
		useUI:    false,
		stderr:   buf,
	}

	code := HandlePanic("boom", []byte("stk"), opts)
	assert.Equal(t, PanicExitCode, code, "exitCode default should be applied")
	assert.Contains(t, buf.String(), "Atmos crashed unexpectedly")
}

// TestHandlePanic_UIMode_NonDebug exercises the useUI=true happy
// path — the ui.Error + ui.MarkdownMessage calls in HandlePanic.
// Note: pkg/ui is not initialized inside this test binary, so ui.*
// are no-ops, but the branches themselves execute and close the
// coverage gap for the UI path.
func TestHandlePanic_UIMode_NonDebug(t *testing.T) {
	_, opts := testOptions(t, false)
	opts.useUI = true
	opts.stderr = nil // ensure nothing falls back to a buffer by accident.

	assert.NotPanics(t, func() {
		code := HandlePanic("boom", []byte("stk"), opts)
		assert.Equal(t, PanicExitCode, code)
	})

	// Crash report is still written regardless of UI mode.
	entries, err := os.ReadDir(opts.crashDir)
	require.NoError(t, err)
	require.Len(t, entries, 1, "crash report should still land in opts.crashDir under useUI=true")
}

// TestHandlePanic_UIMode_DebugStack pairs with the non-debug variant
// to cover `if opts.useUI { ui.Write(...) }` inside the
// showStackInline block — the final uncovered branch in HandlePanic.
func TestHandlePanic_UIMode_DebugStack(t *testing.T) {
	_, opts := testOptions(t, true)
	opts.useUI = true
	opts.stderr = nil

	assert.NotPanics(t, func() {
		code := HandlePanic("boom", []byte("goroutine 1 [running]:\nmain.x()\n"), opts)
		assert.Equal(t, PanicExitCode, code)
	})
}

// TestHandlePanic_NilStderrFallback exercises fallbackWrite's nil-w
// guard via HandlePanic (fallbackWrite is unexported). With useUI=false
// and stderr=nil, the helper must return silently instead of panicking
// — the whole point of the guard is that we've already panicked once.
func TestHandlePanic_NilStderrFallback(t *testing.T) {
	opts := &Options{
		crashDir: t.TempDir(),
		useUI:    false,
		stderr:   nil,
	}

	assert.NotPanics(t, func() {
		code := HandlePanic("boom", []byte("stk"), opts)
		assert.Equal(t, PanicExitCode, code)
	})
}

// TestHandlePanic_NilOptions verifies HandlePanic tolerates a nil
// options pointer (defensive behavior: zero-valued options are
// equivalent, but a direct caller passing nil should not panic).
//
// Redirect the OS temp dir so the crash-report file that
// applyDefaults() would drop into the real os.TempDir() lands in a
// test-managed directory that t.TempDir() cleans up. TMPDIR covers
// Unix-likes; TMP / TEMP cover Windows. If we don't do this, the
// test leaks a real atmos-crash-*.txt on every run.
func TestHandlePanic_NilOptions(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	t.Setenv("TMP", tmp)
	t.Setenv("TEMP", tmp)

	assert.NotPanics(t, func() {
		code := HandlePanic("boom", []byte("stk"), nil)
		assert.Equal(t, PanicExitCode, code)
	})
}

// TestBuildSentryError_AttachesPanicStack pins the Sentry-wrapping
// contract: the panic-origin stack must end up as a safe detail on
// the error so BuildSentryReport (errors/sentry.go) surfaces the
// true crash frames. Guards against a regression that swaps back to
// cockroachErrors.WithStack — which would snapshot HandlePanic's own
// frames instead of the panic origin.
func TestBuildSentryError_AttachesPanicStack(t *testing.T) {
	stack := "goroutine 1 [running]:\nruntime.gopanic(...)\nmain.doStuff(...)\n\t/app/main.go:42"
	err := buildSentryError("nil deref", stack)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "panic: nil deref",
		"top-level error message must carry the summary")

	// Flatten every safe-detail payload in the wrap chain and assert
	// the panic-origin stack is present. This is exactly what
	// errbase.GetAllSafeDetails feeds into BuildSentryReport.
	var all strings.Builder
	for _, p := range cockroachErrors.GetAllSafeDetails(err) {
		for _, d := range p.SafeDetails {
			all.WriteString(d)
			all.WriteByte('\n')
		}
	}
	details := all.String()
	assert.Contains(t, details, "main.doStuff(...)",
		"panic-origin stack must be attached as a safe detail so Sentry sees real frames")
	assert.Contains(t, details, "/app/main.go:42",
		"file/line of the panic must be preserved")
}

func TestSummarize_FallsBackToGenericFormat(t *testing.T) {
	type customPanic struct{ N int }
	s := summarize(customPanic{N: 42})
	assert.Equal(t, "{42}", s)
}

// TestRecover_NoPanic verifies that Recover returns silently when no
// panic is pending, leaving the exit-code pointer untouched.
func TestRecover_NoPanic(t *testing.T) {
	code := -7
	func() {
		defer Recover(&code)
		// No panic.
	}()
	assert.Equal(t, -7, code, "exit code must not be modified when no panic occurred")
}

// TestRecover_CapturesPanic verifies Recover catches a panic on the
// same goroutine, populates the exit-code pointer, and does not let
// the panic escape.
func TestRecover_CapturesPanic(t *testing.T) {
	// Redirect pkg/ui to a buffer would require InitFormatter; here
	// we just want to verify Recover's behavior (exit code, no
	// re-panic). useUI=true in defaultOptions means ui.Error/.MarkdownMessage
	// are called — they are no-ops when pkg/ui is not initialized in
	// tests, which is fine for this assertion.

	code := 0
	assert.NotPanics(t, func() {
		func() {
			defer Recover(&code)
			panic("goroutine crash")
		}()
	})
	assert.Equal(t, PanicExitCode, code)
}

// TestRecover_NilExitCodePointer verifies Recover tolerates a nil
// pointer (useful when a caller only wants the side effects).
func TestRecover_NilExitCodePointer(t *testing.T) {
	assert.NotPanics(t, func() {
		func() {
			defer Recover(nil)
			panic("no one is listening")
		}()
	})
}

// TestRecover_CapturesRealStackIntoReport exercises the real call
// path — Recover → debug.Stack() → HandlePanic → writeCrashReport —
// and asserts the captured stack ends up in the crash-report file.
// This is the behavior users rely on for bug reports; previously this
// test just re-asserted that debug.Stack() works, which is a stdlib
// invariant, not an Atmos-behavior check.
func TestRecover_CapturesRealStackIntoReport(t *testing.T) {
	// Recover uses defaultOptions(), which drops the crash report
	// under os.TempDir(); redirect that so the file lands in a
	// test-managed directory and we can inspect it. TMPDIR covers
	// Unix-likes; TMP/TEMP cover Windows.
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	t.Setenv("TMP", tmp)
	t.Setenv("TEMP", tmp)

	code := 0
	assert.NotPanics(t, func() {
		func() {
			defer Recover(&code)
			panic("stack-capture check")
		}()
	})
	assert.Equal(t, PanicExitCode, code)

	// The real debug.Stack() captured by Recover must show up in
	// the crash report — that's the behavior we actually care about.
	entries, err := os.ReadDir(tmp)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	body, err := os.ReadFile(filepath.Join(tmp, entries[0].Name()))
	require.NoError(t, err)
	assert.Contains(t, string(body), "goroutine", "real runtime stack must appear in the report")
	assert.Contains(t, string(body), "Panic: stack-capture check", "panic value must be recorded in the report")
}

// TestStackInlineFromEnv covers the env-gated stack visibility.
// Atmos canonical log levels are Title-cased ("Debug", "Trace") — we
// match case-insensitively for user-friendly behavior at the shell.
func TestStackInlineFromEnv(t *testing.T) {
	tests := []struct {
		name     string
		value    string
		expected bool
	}{
		{"unset", "", false},
		{"Debug (canonical)", "Debug", true},
		{"Trace (canonical)", "Trace", true},
		{"debug (lowercase)", "debug", true},
		{"trace (lowercase)", "trace", true},
		{"DEBUG (uppercase)", "DEBUG", true},
		{"Info", "Info", false},
		{"Warning", "Warning", false},
		{"Error", "Error", false},
		{"whitespace-padded debug", "  debug  ", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(LogLevelEnvVar, tc.value)
			assert.Equal(t, tc.expected, stackInlineFromEnv())
		})
	}
}
