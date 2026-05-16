package client

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	mcpclient "github.com/cloudposse/atmos/pkg/mcp/client"
)

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
// `ui.Errorf("Error: %v", result.Error)` line surfaces the underlying error
// so users can troubleshoot without having to re-run with --debug.
//
// We can't easily capture stderr in a unit test (ui.Errorf goes through
// the formatter pipeline), but we can pin the structural contract: with
// a non-nil Error, printTestResult must not panic and must walk through
// the error branch.
func TestPrintTestResult_SurfacesUnderlyingError(t *testing.T) {
	result := &mcpclient.TestResult{
		ServerStarted: false,
		Error:         errors.New("connection refused on /tmp/mcp.sock"),
	}
	assert.NotPanics(t, func() {
		printTestResult(result)
	}, "printTestResult must handle the non-nil Error branch without panicking")
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
