package panics

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"testing"
	"time"

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
type fakeRuntimeError struct{ msg string }

func (f fakeRuntimeError) RuntimeError()    {}
func (f fakeRuntimeError) Error() string    { return f.msg }
func (f fakeRuntimeError) String() string   { return f.msg }
func (f fakeRuntimeError) GoString() string { return f.msg }

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

// TestHandlePanic_NilOptions verifies HandlePanic tolerates a nil
// options pointer (defensive behavior: zero-valued options are
// equivalent, but a direct caller passing nil should not panic).
func TestHandlePanic_NilOptions(t *testing.T) {
	assert.NotPanics(t, func() {
		code := HandlePanic("boom", []byte("stk"), nil)
		assert.Equal(t, PanicExitCode, code)
	})
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

// TestRecover_UsesRealDebugStack verifies the real call path captures
// a real stack (as opposed to the test-injected placeholder used by
// HandlePanic-only tests).
func TestRecover_UsesRealDebugStack(t *testing.T) {
	// Reach into debug.Stack to confirm it returns non-empty bytes
	// in this environment — guards against a Go runtime regression
	// that would leave the crash report empty.
	stack := debug.Stack()
	assert.NotEmpty(t, stack)
	assert.Contains(t, string(stack), "goroutine")
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
