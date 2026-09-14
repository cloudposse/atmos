package exec

// terraform_streaming_ui_test.go covers the routing/dispatch logic in
// terraform_streaming_ui.go:
//   - executeStreamingOrShell: retry-gated shell fallback, the "streaming not
//     requested" silent fallback, and the "streaming requested but unsupported"
//     warn-then-fallback path.
//   - selectStreamingExecutor/dispatchStreamingExecutor: the subCommand ->
//     tfui.Execute* routing table.
//
// The actual "streaming succeeded" branch inside executeStreamingOrShell (where
// tfui.ShouldUseStreamingUI returns true and dispatchStreamingExecutor is invoked
// against a live TUI session) cannot be exercised here: ShouldUseStreamingUI's own
// TTY/CI gating means it only ever returns true against a real interactive terminal,
// and pkg/terraform/ui's Execute* functions have no DI seam to substitute a fake
// implementation. That branch is intentionally left uncovered by this package's
// tests (see the coverage report for this file).
//
// The switch cases inside dispatchStreamingExecutor ARE covered two ways:
//   - TestSelectStreamingExecutor_RoutesBySubcommand asserts the *identity* of
//     the resolved tfui.Execute* variant directly (via selectStreamingExecutor),
//     which is the only way to tell apart "routed to ExecuteInit and it
//     refused" from "fell through to the plain Execute and it refused" - both
//     return the identical error below.
//   - TestDispatchStreamingExecutor_RoutesSafely pins CI=true so
//     telemetry.IsCI() deterministically forces every tfui.Execute* variant's
//     own precondition check (stdout/stdin TTY, CI) to return
//     errUtils.ErrStreamingNotSupported before doing any real work, regardless
//     of whether the runner's stdout happens to be a real TTY. That's a
//     genuine safety property worth asserting: no matter which subcommand is
//     dispatched, the streaming path never attempts to spawn a real terraform
//     process outside a supported interactive environment.

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"reflect"
	"runtime"
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
			name:       "providers-lock shares the init phase's ExecuteInit dispatch",
			subCommand: subcommandProvidersLock,
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

// funcName returns the fully-qualified name of the function fn points to, e.g.
// "github.com/cloudposse/atmos/pkg/terraform/ui.ExecuteInit". Used below to
// assert selectStreamingExecutor picked a *specific* tfui.Execute* variant,
// since every variant returns the identical errUtils.ErrStreamingNotSupported
// outside a real TTY and so can't be told apart by their error alone.
func funcName(fn streamingExecutorFunc) string {
	return runtime.FuncForPC(reflect.ValueOf(fn).Pointer()).Name()
}

// TestSelectStreamingExecutor_RoutesBySubcommand asserts the *identity* of the
// tfui.Execute* variant selectStreamingExecutor resolves for each subCommand,
// rather than only the error it eventually returns. Without this, a
// providers-lock/workspace case that silently regressed to falling through to
// the plain tfui.Execute path (the "unrecognized subcommand" fallback) would
// still pass a test that only checks for errUtils.ErrStreamingNotSupported,
// since that fallback returns the exact same sentinel error outside a real TTY.
// ──────────────────────────────────────────────────────────────────────────────
// combineCaptureWriters / streamingCaptureWriters
// ──────────────────────────────────────────────────────────────────────────────

// TestCombineCaptureWriters covers all three branches: neither set, only one set
// (either side), and both set (must produce an io.MultiWriter teeing into both,
// verified by actually writing through it and checking both buffers received the
// bytes -- not just that a non-nil writer came back).
func TestCombineCaptureWriters(t *testing.T) {
	t.Run("both nil returns nil", func(t *testing.T) {
		got := combineCaptureWriters(nil, nil)
		assert.Nil(t, got)
	})

	t.Run("only a set returns a", func(t *testing.T) {
		var a bytes.Buffer
		got := combineCaptureWriters(&a, nil)
		assert.Same(t, io.Writer(&a), got)
	})

	t.Run("only b set returns b", func(t *testing.T) {
		var b bytes.Buffer
		got := combineCaptureWriters(nil, &b)
		assert.Same(t, io.Writer(&b), got)
	})

	t.Run("both set tees writes into both", func(t *testing.T) {
		var a, b bytes.Buffer
		got := combineCaptureWriters(&a, &b)
		require.NotNil(t, got)

		n, err := got.Write([]byte("hello"))
		require.NoError(t, err)
		assert.Equal(t, 5, n)
		assert.Equal(t, "hello", a.String(), "writer a must receive the tee'd bytes")
		assert.Equal(t, "hello", b.String(), "writer b must receive the tee'd bytes")
	})
}

// TestStreamingCaptureWriters verifies streamingCaptureWriters correctly assembles the
// shellCommandConfig from shellOpts and combines the ordinary stdout/stderr capture with
// the scoped exec-metadata capture, so neither consumer's capture goes dark when a phase
// runs through the streaming TUI instead of the plain shell path.
func TestStreamingCaptureWriters(t *testing.T) {
	t.Run("no capture options returns nil, nil", func(t *testing.T) {
		stdout, stderr := streamingCaptureWriters(nil)
		assert.Nil(t, stdout)
		assert.Nil(t, stderr)
	})

	t.Run("only ordinary capture set", func(t *testing.T) {
		var stdoutBuf, stderrBuf bytes.Buffer
		opts := []ShellCommandOption{
			WithStdoutCapture(&stdoutBuf),
			WithStderrCapture(&stderrBuf),
		}

		stdout, stderr := streamingCaptureWriters(opts)
		require.NotNil(t, stdout)
		require.NotNil(t, stderr)

		_, err := stdout.Write([]byte("out"))
		require.NoError(t, err)
		_, err = stderr.Write([]byte("err"))
		require.NoError(t, err)
		assert.Equal(t, "out", stdoutBuf.String())
		assert.Equal(t, "err", stderrBuf.String())
	})

	t.Run("ordinary and exec-metadata capture both set combine via MultiWriter", func(t *testing.T) {
		var stdoutBuf, stderrBuf, execStdoutBuf, execStderrBuf bytes.Buffer
		opts := []ShellCommandOption{
			WithStdoutCapture(&stdoutBuf),
			WithStderrCapture(&stderrBuf),
			withExecMetadataOutputCapture(&execStdoutBuf, &execStderrBuf),
		}

		stdout, stderr := streamingCaptureWriters(opts)
		require.NotNil(t, stdout)
		require.NotNil(t, stderr)

		_, err := stdout.Write([]byte("out"))
		require.NoError(t, err)
		_, err = stderr.Write([]byte("err"))
		require.NoError(t, err)

		assert.Equal(t, "out", stdoutBuf.String(), "ordinary stdout capture must still receive output")
		assert.Equal(t, "out", execStdoutBuf.String(), "exec-metadata stdout capture must also receive the same output")
		assert.Equal(t, "err", stderrBuf.String())
		assert.Equal(t, "err", execStderrBuf.String())
	})
}

func TestSelectStreamingExecutor_RoutesBySubcommand(t *testing.T) {
	tests := []struct {
		name       string
		subCommand string
		dryRun     bool
		want       streamingExecutorFunc
	}{
		{name: "dry run always uses the plain Execute path", subCommand: subcommandApply, dryRun: true, want: tfui.Execute},
		{name: "apply routes to ExecuteApply", subCommand: subcommandApply, dryRun: false, want: tfui.ExecuteApply},
		{name: "destroy routes to ExecuteDestroy", subCommand: "destroy", dryRun: false, want: tfui.ExecuteDestroy},
		{name: "plan routes to ExecutePlan", subCommand: "plan", dryRun: false, want: tfui.ExecutePlan},
		{name: "init routes to ExecuteInit", subCommand: subcommandInit, dryRun: false, want: tfui.ExecuteInit},
		{name: "workspace routes to ExecuteInit, not the plain Execute fallback", subCommand: subcommandWorkspace, dryRun: false, want: tfui.ExecuteInit},
		{name: "providers-lock routes to ExecuteInit, not the plain Execute fallback", subCommand: subcommandProvidersLock, dryRun: false, want: tfui.ExecuteInit},
		{name: "unrecognized subcommand falls through to the plain Execute path", subCommand: "unknown-subcommand", dryRun: false, want: tfui.Execute},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := selectStreamingExecutor(tt.subCommand, tt.dryRun)
			assert.Equal(t, funcName(tt.want), funcName(got))
		})
	}
}
