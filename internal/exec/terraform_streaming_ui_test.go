package exec

// terraform_streaming_ui_test.go covers the routing/dispatch logic in
// terraform_streaming_ui.go:
//   - executeStreamingOrShell: retry-gated shell fallback, the "streaming not
//     requested" silent fallback, and the "streaming requested but unsupported"
//     warn-then-fallback path.
//   - dispatchStreamingExecutor: the subCommand -> tfui.Execute* routing table.
//
// The actual "streaming succeeded" branch inside executeStreamingOrShell (where
// tfui.ShouldUseStreamingUI returns true and dispatchStreamingExecutor is invoked
// against a live TUI session) cannot be exercised here: ShouldUseStreamingUI's own
// TTY/CI gating means it only ever returns true against a real interactive terminal,
// and pkg/terraform/ui's Execute* functions have no DI seam to substitute a fake
// implementation. That branch is intentionally left uncovered by this package's
// tests (see the coverage report for this file).
//
// The switch cases inside dispatchStreamingExecutor ARE covered directly: each test pins
// CI=true so telemetry.IsCI() deterministically forces every tfui.Execute* variant's own
// precondition check (stdout/stdin TTY, CI) to return errUtils.ErrStreamingNotSupported
// before doing any real work, regardless of whether the runner's stdout happens to be a
// real TTY. That's a genuine safety property worth asserting: no matter which subcommand
// is dispatched, the streaming path never attempts to spawn a real terraform process
// outside a supported interactive environment.

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
	tfui "github.com/cloudposse/atmos/pkg/terraform/ui"
)

// ──────────────────────────────────────────────────────────────────────────────
// executeStreamingOrShell
// ──────────────────────────────────────────────────────────────────────────────

// TestExecuteStreamingOrShell_RetryActiveAlwaysUsesShellPath verifies that a
// component with retry conditions configured always takes the shell path, even
// when the streaming UI is otherwise enabled - streaming never populates the
// output-capture buffer that executeShellCommandWithRetry matches conditions
// against, so retries would silently never fire if streaming ran instead.
func TestExecuteStreamingOrShell_RetryActiveAlwaysUsesShellPath(t *testing.T) {
	exePath, err := os.Executable()
	require.NoError(t, err, "os.Executable() must succeed")

	atmosConfig := schema.AtmosConfiguration{}
	info := &schema.ConfigAndStacksInfo{
		Command: exePath,
		// UIEnabled true would normally select streaming; the retry condition must
		// override that and force the shell path regardless.
		UIEnabled: true,
		ComponentRetrySection: &schema.RetryConfig{
			Conditions: []string{"some transient error"},
		},
	}
	req := &streamingExecRequest{
		componentPath: t.TempDir(),
		// -test.run=^$ makes the re-exec'd test binary exit 0 immediately without
		// running any tests, matching the pattern used elsewhere in this package
		// for a portable, cross-platform fake subprocess.
		args:      []string{"-test.run=^$"},
		gatePhase: "apply",
	}

	execErr := executeStreamingOrShell(&atmosConfig, info, req)
	require.NoError(t, execErr, "retry-gated path must run the shell command successfully")
}

// TestExecuteStreamingOrShell_FallsBackToShell table-drives the two ways the
// streaming UI can be skipped in favor of the shell path when there's no retry
// condition: not requested at all, and requested but unsupported for the given
// gate phase (e.g. refresh). Both must complete via the shell fallback.
func TestExecuteStreamingOrShell_FallsBackToShell(t *testing.T) {
	exePath, err := os.Executable()
	require.NoError(t, err, "os.Executable() must succeed")

	tests := []struct {
		name                string
		uiFlagExplicitlySet bool
		uiEnabled           bool
		gatePhase           string
		subCommand          string
	}{
		{
			name:                "streaming not requested at all",
			uiFlagExplicitlySet: false,
			uiEnabled:           false,
			gatePhase:           "plan",
			subCommand:          "plan",
		},
		{
			name:                "streaming requested but unsupported for this subcommand",
			uiFlagExplicitlySet: true,
			uiEnabled:           true,
			gatePhase:           "refresh",
			subCommand:          "refresh",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			atmosConfig := schema.AtmosConfiguration{}
			info := &schema.ConfigAndStacksInfo{
				Command:             exePath,
				UIFlagExplicitlySet: tt.uiFlagExplicitlySet,
				UIEnabled:           tt.uiEnabled,
			}
			req := &streamingExecRequest{
				componentPath: t.TempDir(),
				args:          []string{"-test.run=^$"},
				gatePhase:     tt.gatePhase,
				subCommand:    tt.subCommand,
			}

			execErr := executeStreamingOrShell(&atmosConfig, info, req)
			require.NoError(t, execErr, "must fall back to the shell command and run it successfully")
		})
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// dispatchStreamingExecutor
// ──────────────────────────────────────────────────────────────────────────────

// TestDispatchStreamingExecutor_RoutesSafely drives every subCommand branch of the
// switch. This test binary never has a real TTY attached, so each routed
// tfui.Execute* variant refuses via its own precondition check instead of touching
// a real terraform process - that refusal (errUtils.ErrStreamingNotSupported) is
// exactly what we assert, confirming the dispatch never lets non-interactive/CI
// environments reach real streaming work no matter which subcommand is requested.
func TestDispatchStreamingExecutor_RoutesSafely(t *testing.T) {
	tests := []struct {
		name       string
		subCommand string
		dryRun     bool
	}{
		{
			name:       "dry run skips the switch entirely and calls the plain Execute path",
			subCommand: subcommandApply,
			dryRun:     true,
		},
		{
			name:       "apply routes to ExecuteApply",
			subCommand: subcommandApply,
			dryRun:     false,
		},
		{
			name:       "destroy routes to ExecuteDestroy",
			subCommand: "destroy",
			dryRun:     false,
		},
		{
			name:       "plan routes to ExecutePlan",
			subCommand: "plan",
			dryRun:     false,
		},
		{
			name:       "init routes to ExecuteInit",
			subCommand: subcommandInit,
			dryRun:     false,
		},
		{
			name:       "workspace shares the init phase's ExecuteInit dispatch",
			subCommand: subcommandWorkspace,
			dryRun:     false,
		},
		{
			name:       "unrecognized subcommand falls through to the plain Execute path",
			subCommand: "unknown-subcommand",
			dryRun:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Pin the CI gate explicitly: checkStreamingUIPreconditions returns
			// ErrStreamingNotSupported when either stdout isn't a real TTY or CI is set.
			// Setting CI=true forces that regardless of the runner's TTY state, instead of
			// relying on go test's stdout incidentally not being a TTY.
			t.Setenv("CI", "true")

			execOpts := &tfui.ExecuteOptions{
				Command:    "terraform",
				WorkingDir: t.TempDir(),
			}

			err := dispatchStreamingExecutor(context.Background(), tt.subCommand, tt.dryRun, execOpts)
			require.Error(t, err, "must refuse to run outside a supported interactive environment")
			assert.True(t, errors.Is(err, errUtils.ErrStreamingNotSupported),
				"expected ErrStreamingNotSupported, got: %v", err)
		})
	}
}
