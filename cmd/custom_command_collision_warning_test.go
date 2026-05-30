package cmd

import (
	"bytes"
	"errors"
	"io"
	"os"
	"runtime"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

// errBuiltinPreRun is a sentinel returned by a fake built-in PreRunE to verify the
// deferred-warning wrapper propagates the original handler's error.
var errBuiltinPreRun = errors.New("built-in prerun error")

// captureUIStderr runs fn while the UI layer writes to a captured pipe and returns the
// ANSI-stripped stderr produced during fn. The global formatter is re-initialized so
// ui.Warningf routes through the captured stream.
func captureUIStderr(t *testing.T, fn func()) string {
	t.Helper()

	oldStderr := os.Stderr
	r, w, err := os.Pipe()
	require.NoError(t, err, "creating pipe should not error")

	os.Stderr = w
	ui.ReinitFormatter()
	// Restore global stderr/formatter via defer so state is never leaked into later
	// tests, even when fn() aborts the goroutine via require.*/t.FailNow (Goexit).
	// The Close calls here are a safety net; on the normal path w is already closed
	// below (so io.Copy can observe EOF) and a second Close is a harmless no-op.
	defer func() {
		os.Stderr = oldStderr
		ui.ReinitFormatter()
		_ = w.Close()
		_ = r.Close()
	}()

	fn()

	// Close the writer so io.Copy observes EOF, then drain the captured output.
	require.NoError(t, w.Close(), "closing pipe writer should not error")

	var buf bytes.Buffer
	_, err = io.Copy(&buf, r)
	require.NoError(t, err, "reading from pipe should not error")

	return stripANSI(buf.String())
}

// TestCustomCommand_StepsConflictWarning_DeferredToInvocation verifies the regression
// from https://github.com/cloudposse/atmos/issues/2102: a custom command whose steps
// conflict with a built-in must NOT print the collision warning at registration time
// (processCustomCommands runs for nearly every invocation), but MUST print it when the
// conflicting command is actually run.
func TestCustomCommand_StepsConflictWarning_DeferredToInvocation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping on Windows: pipe/stderr capture interacts poorly with background goroutines")
	}

	testDir := "../tests/fixtures/scenarios/complete"
	t.Setenv("ATMOS_CLI_CONFIG_PATH", testDir)
	t.Setenv("ATMOS_BASE_PATH", testDir)

	_ = NewTestKit(t)

	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	require.NoError(t, err)

	// Built-in command tree: "builtin-leaf-ns" > "plan" (runnable leaf).
	builtinNS := &cobra.Command{
		Use:   "builtin-leaf-ns",
		Short: "A built-in namespace",
	}
	builtinPlan := &cobra.Command{
		Use:   "plan",
		Short: "Built-in plan subcommand",
		RunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}
	builtinNS.AddCommand(builtinPlan)
	RootCmd.AddCommand(builtinNS)

	// Custom command that collides with the built-in "plan" leaf and defines steps.
	atmosConfig.Commands = []schema.Command{
		{
			Name: "builtin-leaf-ns",
			Commands: []schema.Command{
				{
					Name:        "plan",
					Description: "Custom plan whose steps conflict with the built-in",
					Steps:       stepsFromStrings("echo custom-plan"),
				},
			},
		},
	}

	const warningMarker = "conflict with built-in"

	// Registration must NOT emit the warning.
	regOutput := captureUIStderr(t, func() {
		require.NoError(t, processCustomCommands(atmosConfig, atmosConfig.Commands, RootCmd))
	})
	assert.NotContains(t, regOutput, warningMarker,
		"collision warning must not be emitted at registration time (issue #2102)")

	// Locate the (preserved) built-in "plan" leaf after processing.
	var planCmd *cobra.Command
	for _, c := range builtinNS.Commands() {
		if c.Name() == "plan" {
			planCmd = c
		}
	}
	require.NotNil(t, planCmd, "built-in plan should still exist after merge")
	require.NotNil(t, planCmd.PreRunE, "a PreRunE should be installed to defer the warning")

	// Invoking the conflicting command MUST emit the warning exactly once.
	runOutput := captureUIStderr(t, func() {
		require.NoError(t, planCmd.PreRunE(planCmd, []string{}))
	})
	assert.Contains(t, runOutput, warningMarker,
		"collision warning must be emitted when the conflicting command runs")
	assert.Equal(t, 1, bytes.Count([]byte(runOutput), []byte(warningMarker)),
		"collision warning should be emitted exactly once per invocation")
}

// TestCustomCommand_StepsConflictWarning_PreservesExistingPreRunE verifies that when the
// conflicting built-in already defines its own PreRunE (as the real `terraform plan` does),
// the deferred-warning wrapper still invokes that original PreRunE and propagates its error.
func TestCustomCommand_StepsConflictWarning_PreservesExistingPreRunE(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping on Windows: pipe/stderr capture interacts poorly with background goroutines")
	}

	testDir := "../tests/fixtures/scenarios/complete"
	t.Setenv("ATMOS_CLI_CONFIG_PATH", testDir)
	t.Setenv("ATMOS_BASE_PATH", testDir)

	_ = NewTestKit(t)

	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	require.NoError(t, err)

	builtinPreRunRan := false

	builtinNS := &cobra.Command{Use: "builtin-prerun-ns", Short: "A built-in namespace"}
	builtinLeaf := &cobra.Command{
		Use:   "leaf",
		Short: "Built-in leaf with its own PreRunE",
		PreRunE: func(cmd *cobra.Command, args []string) error {
			builtinPreRunRan = true
			return errBuiltinPreRun
		},
		RunE: func(cmd *cobra.Command, args []string) error { return nil },
	}
	builtinNS.AddCommand(builtinLeaf)
	RootCmd.AddCommand(builtinNS)

	atmosConfig.Commands = []schema.Command{
		{
			Name: "builtin-prerun-ns",
			Commands: []schema.Command{
				{
					Name:        "leaf",
					Description: "Custom leaf whose steps conflict with the built-in",
					Steps:       stepsFromStrings("echo custom-leaf"),
				},
			},
		},
	}

	require.NoError(t, processCustomCommands(atmosConfig, atmosConfig.Commands, RootCmd))

	require.NotNil(t, builtinLeaf.PreRunE, "PreRunE should be installed")

	output := captureUIStderr(t, func() {
		// The wrapper must propagate the original PreRunE's error.
		err := builtinLeaf.PreRunE(builtinLeaf, []string{})
		require.ErrorIs(t, err, errBuiltinPreRun, "original PreRunE error must propagate through the wrapper")
	})

	assert.True(t, builtinPreRunRan, "the built-in's original PreRunE must still run")
	assert.Contains(t, output, "conflict with built-in", "warning must still be emitted")
}
