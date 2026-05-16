package client

import (
	"bytes"
	"errors"
	stdio "io"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	iolib "github.com/cloudposse/atmos/pkg/io"
	mcpclient "github.com/cloudposse/atmos/pkg/mcp/client"
	"github.com/cloudposse/atmos/pkg/ui"
)

// uiCaptureStreams implements iolib.Streams with in-memory buffers so tests
// can assert on the bytes ui.Success/Error/Errorf produces.
type uiCaptureStreams struct {
	stdin  stdio.Reader
	stdout stdio.Writer
	stderr stdio.Writer
}

func (s *uiCaptureStreams) Input() stdio.Reader     { return s.stdin }
func (s *uiCaptureStreams) Output() stdio.Writer    { return s.stdout }
func (s *uiCaptureStreams) Error() stdio.Writer     { return s.stderr }
func (s *uiCaptureStreams) RawOutput() stdio.Writer { return s.stdout }
func (s *uiCaptureStreams) RawError() stdio.Writer  { return s.stderr }

// setupCapturedUI wires ui.* into in-memory buffers so the test can read
// exactly what ui.Errorf / ui.Success printed. Returns the stderr buffer
// the caller asserts against. Cleanup restores the global formatter so
// other tests in this package don't see a leaked one.
func setupCapturedUI(t *testing.T) *bytes.Buffer {
	t.Helper()
	stderr := &bytes.Buffer{}
	streams := &uiCaptureStreams{
		stdin:  &bytes.Buffer{},
		stdout: &bytes.Buffer{},
		stderr: stderr,
	}
	ioCtx, err := iolib.NewContext(iolib.WithStreams(streams))
	require.NoError(t, err)
	ui.InitFormatter(ioCtx)
	t.Cleanup(func() {
		ui.Reset()
	})
	return stderr
}

func TestPrintTestResult_AllSuccess(t *testing.T) {
	result := &mcpclient.TestResult{
		ServerStarted: true,
		Initialized:   true,
		ToolCount:     5,
		PingOK:        true,
	}
	// Should not panic — printTestResult only calls ui.Success/Warning/Error.
	assert.NotPanics(t, func() {
		printTestResult(result)
	})
}

func TestPrintTestResult_FailedStart(t *testing.T) {
	result := &mcpclient.TestResult{
		ServerStarted: false,
	}
	assert.NotPanics(t, func() {
		printTestResult(result)
	})
}

func TestPrintTestResult_StartedButNoTools(t *testing.T) {
	result := &mcpclient.TestResult{
		ServerStarted: true,
		Initialized:   true,
		ToolCount:     0,
		PingOK:        false,
	}
	assert.NotPanics(t, func() {
		printTestResult(result)
	})
}

// TestPrintTestResult_SurfacesUnderlyingError covers the `result.Error != nil`
// branch added in response to CodeRabbit feedback. Before this branch was
// added, `printTestResult` told the user WHICH stage failed but never WHY —
// users saw `✗ Server failed to start` with no actionable context. The new
// `ui.Errorf("Error: %v", result.Error)` line surfaces the underlying error.
//
// This is a behavioral test, not a "doesn't panic" check: it wires ui.*
// to an in-memory buffer and asserts the exact failure message reaches
// stderr. That is the only assertion that proves the new branch actually
// improves user-facing diagnostics — a NotPanics-only test would still
// pass if the branch was deleted.
func TestPrintTestResult_SurfacesUnderlyingError(t *testing.T) {
	stderr := setupCapturedUI(t)

	const failureMessage = "connection refused on /tmp/mcp.sock"
	result := &mcpclient.TestResult{
		ServerStarted: false,
		Error:         errors.New(failureMessage),
	}
	printTestResult(result)

	captured := stderr.String()
	assert.Contains(t, captured, failureMessage,
		"printTestResult MUST surface result.Error to stderr (the regression CodeRabbit flagged was the message being swallowed); got stderr:\n%s",
		captured)
	// The "Error:" prefix is the contract the call site sets ("Error: %v").
	// Pinning it confirms the new ui.Errorf line is responsible — not some
	// other accidental path that happens to mention the string.
	assert.Contains(t, captured, "Error:",
		"the failure surface must use the explicit `Error:` prefix so users can distinguish it from the ✗ status markers; got:\n%s",
		captured)
}

// TestPrintTestResult_OmitsErrorPrefixWhenSuccessful is the negative-path
// guard: when result.Error is nil, the "Error:" line must not appear in
// stderr. Without this test, a future "always print Error: <nil>" regression
// would pass the previous test.
func TestPrintTestResult_OmitsErrorPrefixWhenSuccessful(t *testing.T) {
	stderr := setupCapturedUI(t)

	printTestResult(&mcpclient.TestResult{
		ServerStarted: true,
		Initialized:   true,
		ToolCount:     5,
		PingOK:        true,
	})

	captured := stderr.String()
	assert.NotContains(t, captured, "Error:",
		"printTestResult MUST NOT print an Error: line when result.Error is nil; got stderr:\n%s",
		captured)
}

// TestTestCmd_Registration is the basic shape guard.
func TestTestCmd_Registration(t *testing.T) {
	assert.Equal(t, "test <name>", testCmd.Use)
	assert.NotEmpty(t, testCmd.Short)
	assert.NotEmpty(t, testCmd.Long)
	assert.NotNil(t, testCmd.RunE)
}

// TestExecuteMCPTest_ReturnsNilEvenOnFailure is the regression guard for
// issue #9 in docs/fixes/2026-05-15-mcp-review-fixes.md.
//
// Pre-fix, executeMCPTest returned result.Error to cobra after
// printTestResult had already called ui.Error on it — so main.go's
// errUtils.Format pipeline printed the same message a second time.
// Post-fix, the command always returns nil; the ✓/✗ markers from
// printTestResult are the single source of stderr output.
//
// We exercise this contract via the structural assertion that the
// RunE function reaches a "return nil" point even when the underlying
// Test result carries an Error. The cleanest way to do that without
// driving the full cobra+config pipeline is to build a fake MCP
// server config that points at a known-bad command (so manager.Test
// returns an error), invoke executeMCPTest with the test cobra
// command, and assert the returned error is nil.
//
// We use t.Chdir to point InitCliConfig at a temp dir with a
// minimal atmos.yaml — this keeps the test hermetic and CWD-
// independent (per the CLAUDE.md fixture guidance).
func TestExecuteMCPTest_ReturnsNilEvenOnFailure(t *testing.T) {
	tempDir := t.TempDir()
	atmosYAML := `
mcp:
  servers:
    broken:
      command: "nonexistent-binary-that-does-not-exist-xyz"
      description: "intentionally invalid command for testing the failure path"
`
	require.NoError(t,
		os.WriteFile(filepath.Join(tempDir, "atmos.yaml"), []byte(atmosYAML), 0o644))
	t.Chdir(tempDir)

	// Build a cobra command that mirrors the production registration so
	// executeMCPTest can pull its name from positional args.
	cmd := newTestCobraCmdForMCPTest(t)
	err := executeMCPTest(cmd, []string{"broken"})

	// The headline assertion: even though `broken` cannot start, RunE
	// must return nil so main.go's formatter doesn't re-print the error.
	assert.NoError(t, err,
		"executeMCPTest MUST return nil even when the test fails — "+
			"otherwise main.go's errUtils.Format prints a second copy of "+
			"the message that printTestResult already showed (issue #9)")
}

// newTestCobraCmdForMCPTest builds a cobra command with the minimal
// surface executeMCPTest needs (no flags — it doesn't parse any —
// just the args validator and a context). Keeps the test isolated
// from the production init() side effects.
func newTestCobraCmdForMCPTest(t *testing.T) *cobra.Command {
	t.Helper()
	return &cobra.Command{
		Use:  "test <name>",
		Args: cobra.ExactArgs(1),
	}
}
