package exec

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/terminal"
	"github.com/cloudposse/atmos/tests"
)

func TestExecuteHelmfile_Version(t *testing.T) {
	tests.RequireHelmfile(t)

	testCases := []struct {
		name           string
		workDir        string
		expectedOutput string
	}{
		{
			name:           "helmfile version",
			workDir:        "../../tests/fixtures/scenarios/atmos-helmfile-version",
			expectedOutput: "helmfile",
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			// Set info for ExecuteHelmfile.
			info := schema.ConfigAndStacksInfo{
				SubCommand: "version",
			}

			testCaptureCommandOutput(t, tt.workDir, func() error {
				return ExecuteHelmfile(info)
			}, tt.expectedOutput)
		})
	}
}

func TestExecuteHelmfile_MissingStack(t *testing.T) {
	workDir := "../../tests/fixtures/scenarios/complete"
	t.Chdir(workDir)

	info := schema.ConfigAndStacksInfo{
		ComponentFromArg: "echo-server",
		Stack:            "",
		SubCommand:       "diff",
	}

	err := ExecuteHelmfile(info)
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrMissingStack)
}

func TestExecuteHelmfile_ComponentNotFound(t *testing.T) {
	workDir := "../../tests/fixtures/scenarios/complete"
	t.Chdir(workDir)

	info := schema.ConfigAndStacksInfo{
		ComponentFromArg: "non-existent-component",
		Stack:            "tenant1-ue2-dev",
		SubCommand:       "diff",
	}

	err := ExecuteHelmfile(info)
	assert.Error(t, err)
	// ExecuteHelmfile calls ProcessStacks which will fail to find the component.
	assert.Contains(t, err.Error(), "Could not find the component")
}

func TestExecuteHelmfile_DisabledComponent(t *testing.T) {
	workDir := "../../tests/fixtures/scenarios/complete"
	t.Chdir(workDir)

	info := schema.ConfigAndStacksInfo{
		ComponentFromArg:   "echo-server",
		Stack:              "tenant1-ue2-dev",
		SubCommand:         "diff",
		ComponentIsEnabled: false,
	}

	// When component is disabled during processing, ExecuteHelmfile should skip it.
	// This test verifies the disabled component path is handled correctly.
	err := ExecuteHelmfile(info)
	// Note: This will still try to process stacks because ComponentIsEnabled is set
	// after ProcessStacks, but it verifies the function handles the scenario.
	assert.Error(t, err) // Error expected due to missing helmfile component.
}

func TestExecuteHelmfile_DeploySubcommand(t *testing.T) {
	workDir := "../../tests/fixtures/scenarios/complete"
	t.Chdir(workDir)

	info := schema.ConfigAndStacksInfo{
		ComponentFromArg: "echo-server",
		Stack:            "tenant1-ue2-dev",
		SubCommand:       "deploy", // Should be converted to "sync".
	}

	err := ExecuteHelmfile(info)
	assert.Error(t, err) // Error expected but exercises deploy->sync conversion.
}

// TestHelmfileComponentEnvSectionConversion verifies that ComponentEnvSection is properly
// converted to ComponentEnvList in Helmfile execution. This ensures auth environment variables
// and stack config env sections are passed to Helmfile commands.
//
//nolint:dupl // Test logic is intentionally similar across terraform/helmfile/packer for consistency
func TestHelmfileComponentEnvSectionConversion(t *testing.T) {
	tests := []struct {
		name            string
		envSection      map[string]any
		expectedEnvList map[string]string
	}{
		{
			name: "converts AWS auth environment variables for Helmfile",
			envSection: map[string]any{
				"AWS_CONFIG_FILE":             "/path/to/config",
				"AWS_SHARED_CREDENTIALS_FILE": "/path/to/credentials",
				"AWS_PROFILE":                 "helmfile-profile",
				"AWS_REGION":                  "eu-west-1",
			},
			expectedEnvList: map[string]string{
				"AWS_CONFIG_FILE":             "/path/to/config",
				"AWS_SHARED_CREDENTIALS_FILE": "/path/to/credentials",
				"AWS_PROFILE":                 "helmfile-profile",
				"AWS_REGION":                  "eu-west-1",
			},
		},
		{
			name: "handles Kubernetes environment variables",
			envSection: map[string]any{
				"KUBECONFIG":     "/path/to/kubeconfig",
				"HELM_NAMESPACE": "my-namespace",
				"CUSTOM_VAR":     "helmfile-value",
			},
			expectedEnvList: map[string]string{
				"KUBECONFIG":     "/path/to/kubeconfig",
				"HELM_NAMESPACE": "my-namespace",
				"CUSTOM_VAR":     "helmfile-value",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a test ConfigAndStacksInfo with ComponentEnvSection populated.
			info := schema.ConfigAndStacksInfo{
				ComponentEnvSection: tt.envSection,
				ComponentEnvList:    []string{},
			}

			// Call the production conversion function.
			ConvertComponentEnvSectionToList(&info)

			// Verify all expected environment variables are in ComponentEnvList.
			envListMap := make(map[string]string)
			for _, envVar := range info.ComponentEnvList {
				parts := strings.SplitN(envVar, "=", 2)
				if len(parts) == 2 {
					envListMap[parts[0]] = parts[1]
				}
			}

			// Check that all expected vars are present with correct values.
			for key, expectedValue := range tt.expectedEnvList {
				actualValue, exists := envListMap[key]
				assert.True(t, exists, "Expected environment variable %s to be in ComponentEnvList", key)
				assert.Equal(t, expectedValue, actualValue,
					"Environment variable %s should have value %s, got %s", key, expectedValue, actualValue)
			}

			// Verify count matches.
			assert.Equal(t, len(tt.expectedEnvList), len(envListMap),
				"ComponentEnvList should contain exactly %d variables", len(tt.expectedEnvList))
		})
	}
}

// testHelmfileNodeHooks is a schema.ComponentNodeHooks test double for
// ExecuteHelmfile, mirroring testNodeHooks in
// pkg/scheduler/adapters/terraform_test.go. It records whether Before/After
// were invoked and can be configured to return a specific error.
type testHelmfileNodeHooks struct {
	beforeCalled bool
	afterCalled  bool
	beforeErr    error
	afterErr     error
	afterExecErr error // the execErr After was actually called with.
}

func (n *testHelmfileNodeHooks) Before(_ context.Context, _ *schema.ConfigAndStacksInfo) error {
	n.beforeCalled = true
	return n.beforeErr
}

func (n *testHelmfileNodeHooks) After(_ context.Context, _ *schema.ConfigAndStacksInfo, _ string, execErr error) error {
	n.afterCalled = true
	n.afterExecErr = execErr
	return n.afterErr
}

// newHelmfileNodeHooksFixture writes a minimal atmos project with one
// helmfile component ("myapp") and a portable subprocess with a controlled
// exit code. No AWS/EKS auth, Helm installation, or cluster is required to
// exercise info.NodeHooks.Before/After.
func newHelmfileNodeHooksFixture(t *testing.T) schema.ConfigAndStacksInfo {
	t.Helper()
	binary, err := os.Executable()
	require.NoError(t, err)
	t.Setenv("_ATMOS_TEST_EXIT_CODE", "1")

	tempDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(tempDir, "components", "helmfile", "myapp"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(tempDir, "stacks", "deploy"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "atmos.yaml"), []byte(`base_path: "./"
components:
  helmfile:
    base_path: "components/helmfile"
    use_eks: false
stacks:
  base_path: "stacks"
  included_paths:
    - "deploy/**/*"
  name_pattern: "{stage}"
logs:
  level: Info
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "stacks", "deploy", "dev.yaml"), []byte(`vars:
  stage: dev
components:
  helmfile:
    myapp:
      vars:
        foo: bar
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "components", "helmfile", "myapp", "helmfile.yaml"), []byte("releases: []\n"), 0o644))

	t.Chdir(tempDir)

	return schema.ConfigAndStacksInfo{
		ComponentCommand: binary,
		ComponentFromArg: "myapp",
		Stack:            "dev",
		SubCommand:       "diff",
		ComponentType:    "helmfile",
	}
}

// TestExecuteHelmfileNodeHooks_InvokedAroundExecution is a regression test
// for the NodeHooks wiring added to ExecuteHelmfile: both Before and After
// must fire around a portable subprocess that reports an execution failure.
func TestExecuteHelmfileNodeHooks_InvokedAroundExecution(t *testing.T) {
	info := newHelmfileNodeHooksFixture(t)
	nodeHooks := &testHelmfileNodeHooks{}
	info.NodeHooks = nodeHooks

	err := ExecuteHelmfile(info)

	require.Error(t, err, "the fixture subprocess reports an execution failure")
	assert.True(t, nodeHooks.beforeCalled)
	assert.True(t, nodeHooks.afterCalled)
	assert.Error(t, nodeHooks.afterExecErr, "After must receive the real execution error")
}

// TestExecuteHelmfileNodeHooks_BeforeErrorAbortsExecution asserts that a
// Before-hook failure aborts execution before helmfile ever runs, and that
// the returned error wraps ErrPerComponentHookFailed.
func TestExecuteHelmfileNodeHooks_BeforeErrorAbortsExecution(t *testing.T) {
	info := newHelmfileNodeHooksFixture(t)
	sentinelErr := errors.New("before hook failed")
	nodeHooks := &testHelmfileNodeHooks{beforeErr: sentinelErr}
	info.NodeHooks = nodeHooks

	err := ExecuteHelmfile(info)

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrPerComponentHookFailed)
	assert.ErrorIs(t, err, sentinelErr)
	assert.True(t, nodeHooks.beforeCalled)
	assert.False(t, nodeHooks.afterCalled, "After must never run when Before aborted execution")
}

// TestExecuteHelmfileNodeHooks_AfterErrorBecomesResultWhenExecSucceeded
// asserts that an After-hook failure becomes the returned error when the
// underlying helmfile execution itself reported no error.
func TestExecuteHelmfileNodeHooks_AfterErrorBecomesResultWhenExecSucceeded(t *testing.T) {
	info := newHelmfileNodeHooksFixture(t)
	t.Setenv("_ATMOS_TEST_EXIT_CODE", "0") // Successful execution still fires hooks.
	sentinelErr := errors.New("after hook failed")
	nodeHooks := &testHelmfileNodeHooks{afterErr: sentinelErr}
	info.NodeHooks = nodeHooks

	err := ExecuteHelmfile(info)

	require.Error(t, err)
	assert.ErrorIs(t, err, sentinelErr)
	assert.NoError(t, nodeHooks.afterExecErr, "the subprocess completed successfully")
}

// Dry-run must not execute the subprocess or either component hook.
func TestExecuteHelmfileNodeHooks_DryRunSkipsExecution(t *testing.T) {
	info := newHelmfileNodeHooksFixture(t)
	info.DryRun = true
	marker := filepath.Join(t.TempDir(), "subprocess-ran")
	t.Setenv("_ATMOS_TEST_COUNTER_FILE", marker)
	nodeHooks := &testHelmfileNodeHooks{beforeErr: errors.New("before must not run"), afterErr: errors.New("after must not run")}
	info.NodeHooks = nodeHooks

	require.NoError(t, ExecuteHelmfile(info))
	assert.False(t, nodeHooks.beforeCalled)
	assert.False(t, nodeHooks.afterCalled)
	require.NoFileExists(t, marker)
}

// TestExecuteHelmfileNodeHooks_AfterErrorDroppedWhenExecAlreadyFailed asserts
// that when the underlying execution already failed, an additional
// After-hook failure is dropped (not joined) — the original execution error
// wins. This is a real behavioral difference from
// pkg/scheduler/adapters/terraform.go's runAfterNodeHooks, which uses
// errors.Join instead.
func TestExecuteHelmfileNodeHooks_AfterErrorDroppedWhenExecAlreadyFailed(t *testing.T) {
	info := newHelmfileNodeHooksFixture(t)
	afterErr := errors.New("after hook failed")
	nodeHooks := &testHelmfileNodeHooks{afterErr: afterErr}
	info.NodeHooks = nodeHooks

	err := ExecuteHelmfile(info)

	require.Error(t, err)
	assert.NotErrorIs(t, err, afterErr, "the after-hook error must be dropped when the exec error already won")
	assert.Error(t, nodeHooks.afterExecErr, "After must have been called with the real (non-nil) exec error")
}

// TestExecuteHelmfileCommandWithRetry_MatchingError_Retries proves the retry wiring added
// to ExecuteHelmfile actually triggers through the real call chain
// (executeHelmfileCommandWithRetry -> ExecuteShellCommandWithRetry -> ExecuteShellCommand),
// not just that the shared helper works in isolation. Uses the test binary itself as the
// "helmfile" command (cross-platform, no real helmfile install needed) via the
// _ATMOS_TEST_EXIT_ONE/_ATMOS_TEST_STDERR TestMain gate.
func TestExecuteHelmfileCommandWithRetry_MatchingError_Retries(t *testing.T) {
	exePath, err := os.Executable()
	require.NoError(t, err)

	counterFile := filepath.Join(t.TempDir(), "counter")

	info := &schema.ConfigAndStacksInfo{
		Command:    exePath,
		SubCommand: "sync",
		ComponentRetrySection: &schema.RetryConfig{
			MaxAttempts: intPtr(3),
			Conditions:  []string{"/Bad Gateway/"},
		},
	}
	envVars := []string{
		"_ATMOS_TEST_COUNTER_FILE=" + counterFile,
		"_ATMOS_TEST_EXIT_ONE=1",
		"_ATMOS_TEST_STDERR=Error: 502 Bad Gateway returned",
	}

	atmosConfig := schema.AtmosConfiguration{}
	err = executeHelmfileCommandWithRetry(&atmosConfig, info, nil, retryExecParams{
		allArgsAndFlags: []string{"sync"},
		componentPath:   t.TempDir(),
		envVars:         envVars,
	})
	require.Error(t, err, "all 3 attempts fail in this fixture, so the final error must propagate")

	counterBytes, readErr := os.ReadFile(counterFile)
	require.NoError(t, readErr)
	assert.Len(t, counterBytes, 3, "all 3 configured attempts must execute before the final error propagates")
}

// TestExecuteHelmfileCommandWithRetry_NonMatchingError_FailsFast proves a real helmfile
// failure whose output does not match `conditions` is NOT retried through the real call
// chain -- the counter file lets us assert exactly one subprocess invocation happened.
func TestExecuteHelmfileCommandWithRetry_NonMatchingError_FailsFast(t *testing.T) {
	exePath, err := os.Executable()
	require.NoError(t, err)

	counterFile := filepath.Join(t.TempDir(), "counter")

	info := &schema.ConfigAndStacksInfo{
		Command:    exePath,
		SubCommand: "sync",
		ComponentRetrySection: &schema.RetryConfig{
			MaxAttempts: intPtr(3),
			Conditions:  []string{"/Bad Gateway/"},
		},
	}
	envVars := []string{
		"_ATMOS_TEST_COUNTER_FILE=" + counterFile,
		"_ATMOS_TEST_EXIT_ONE=1",
		"_ATMOS_TEST_STDERR=permission denied",
	}

	atmosConfig := schema.AtmosConfiguration{}
	err = executeHelmfileCommandWithRetry(&atmosConfig, info, nil, retryExecParams{
		allArgsAndFlags: []string{"sync"},
		componentPath:   t.TempDir(),
		envVars:         envVars,
	})
	require.Error(t, err)

	counterBytes, readErr := os.ReadFile(counterFile)
	require.NoError(t, readErr)
	assert.Len(t, counterBytes, 1, "non-matching error must fail fast on the first attempt")
}

// TestExecuteHelmfileCommandWithRetry_ComposesWithNodeHooksCapture proves that
// executeHelmfileCommandWithRetry's caller-supplied NodeHooks capture buffers (shellOpts)
// still receive output when retry is also configured -- guards the
// ExecuteShellCommandWithRetry MultiWriter composition fix at the actual helmfile call site,
// not just in the shared helper's own unit tests.
func TestExecuteHelmfileCommandWithRetry_ComposesWithNodeHooksCapture(t *testing.T) {
	exePath, err := os.Executable()
	require.NoError(t, err)

	info := &schema.ConfigAndStacksInfo{
		Command:    exePath,
		SubCommand: "sync",
		ComponentRetrySection: &schema.RetryConfig{
			MaxAttempts: intPtr(2),
			Conditions:  []string{"/Bad Gateway/"},
		},
	}
	envVars := []string{
		"_ATMOS_TEST_EXIT_ONE=1",
		"_ATMOS_TEST_STDERR=502 Bad Gateway",
	}

	var nodeHooksStderr bytes.Buffer
	shellOpts := []ShellCommandOption{WithStderrCapture(&nodeHooksStderr)}

	atmosConfig := schema.AtmosConfiguration{}
	err = executeHelmfileCommandWithRetry(&atmosConfig, info, nil, retryExecParams{
		allArgsAndFlags: []string{"sync"},
		componentPath:   t.TempDir(),
		envVars:         envVars,
	}, shellOpts...)
	require.Error(t, err)
	assert.Contains(t, nodeHooksStderr.String(), "502 Bad Gateway",
		"the NodeHooks-style caller capture buffer must still receive output when retry is also active")
}

// TestHelmDiffColorEnv covers the HELM_DIFF_COLOR override that works around
// helm-diff's own isatty check failing once helmfile pipes its output (issue #2561:
// `atmos helmfile diff` showed no color without an explicit `--color` flag).
func TestHelmDiffColorEnv(t *testing.T) {
	// unsetHelmDiffColor truly removes HELM_DIFF_COLOR from the process environment
	// (not just sets it to ""), restoring whatever the developer's shell had on
	// cleanup. This must be a real unset: since helmDiffColorEnv now distinguishes
	// "unset" from "explicitly set to empty" via os.LookupEnv, t.Setenv(key, "")
	// would be observed as explicitly set and no longer isolates these cases.
	unsetHelmDiffColor := func(t *testing.T) {
		t.Helper()
		original, hadValue := os.LookupEnv("HELM_DIFF_COLOR")
		require.NoError(t, os.Unsetenv("HELM_DIFF_COLOR"))
		t.Cleanup(func() {
			if hadValue {
				require.NoError(t, os.Setenv("HELM_DIFF_COLOR", original))
			}
		})
	}

	t.Run("color disabled: no override", func(t *testing.T) {
		unsetHelmDiffColor(t)
		assert.Empty(t, helmDiffColorEnv(nil, nil, false))
	})

	t.Run("color enabled, nothing set: forces HELM_DIFF_COLOR=true", func(t *testing.T) {
		unsetHelmDiffColor(t)
		assert.Equal(t, "HELM_DIFF_COLOR=true", helmDiffColorEnv(nil, nil, true))
	})

	t.Run("color enabled, already set in component env: left alone", func(t *testing.T) {
		unsetHelmDiffColor(t)
		envVars := []string{"HELM_DIFF_COLOR=false"}
		assert.Empty(t, helmDiffColorEnv(envVars, nil, true))
	})

	t.Run("color enabled, already set in parent process env: left alone", func(t *testing.T) {
		t.Setenv("HELM_DIFF_COLOR", "false")
		assert.Empty(t, helmDiffColorEnv(nil, nil, true))
	})

	t.Run("color enabled, explicitly set to empty string in parent env: respects the user's explicit opt-out", func(t *testing.T) {
		// Regression test for the bug where os.Getenv(key) != "" could not tell
		// "unset" apart from "explicitly set to empty", so an explicit opt-out
		// (HELM_DIFF_COLOR=) was silently overridden back to "true".
		t.Setenv("HELM_DIFF_COLOR", "")
		assert.Empty(t, helmDiffColorEnv(nil, nil, true))
	})

	t.Run("color enabled, explicitly set to empty string in component env: respects the user's explicit opt-out", func(t *testing.T) {
		unsetHelmDiffColor(t)
		envVars := []string{"HELM_DIFF_COLOR="}
		assert.Empty(t, helmDiffColorEnv(envVars, nil, true))
	})

	t.Run("color enabled, set false in atmos.yaml global env: respects the global setting", func(t *testing.T) {
		// Regression test (CodeRabbit finding on 4ea50ae4e): ExecuteShellCommand
		// merges process env < atmosConfig.Env (global `env:` section) < envVars
		// (env.MergeGlobalEnv, then ExecuteShellCommand's own append), with
		// exec.Cmd resolving duplicate "KEY=value" entries by last-one-wins. Since
		// this function's return value gets appended into that same envVars slice,
		// an override returned here would always outrank an explicit global
		// `env: HELM_DIFF_COLOR: false` in atmos.yaml, even though neither envVars
		// nor the parent process env set the key.
		unsetHelmDiffColor(t)
		globalEnv := map[string]string{"HELM_DIFF_COLOR": "false"}
		assert.Empty(t, helmDiffColorEnv(nil, globalEnv, true))
	})

	t.Run("color enabled, global env sets unrelated key: still overrides", func(t *testing.T) {
		unsetHelmDiffColor(t)
		globalEnv := map[string]string{"SOME_OTHER_VAR": "x"}
		assert.Equal(t, "HELM_DIFF_COLOR=true", helmDiffColorEnv(nil, globalEnv, true))
	})
}

// TestHelmDiffColorEnabled covers the bug (CodeRabbit finding on 4ea50ae4e)
// where using the general terminal.ColorProfile() -- cached from stderr's TTY
// status -- could say "color enabled" purely because stderr is an interactive
// terminal, even when stdout (where helmfile's labeled diff output, and thus
// helm-diff's ANSI codes, actually goes) is redirected to a file or pipe. The
// fix asks about terminal.Stdout specifically via ColorProfileFor.
//
// Only terminal.Stdout is stubbed on the mock (not terminal.Stderr or
// ColorProfile()): a regression that falls back to the general, stderr-biased
// profile would call one of those instead, failing the test on the
// unexpected call rather than silently returning a wrong value.
func TestHelmDiffColorEnabled(t *testing.T) {
	t.Run("stderr is a TTY but stdout is redirected: color disabled", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		term := terminal.NewMockTerminal(ctrl)
		term.EXPECT().ColorProfileFor(terminal.Stdout).Return(terminal.ColorNone)

		assert.False(t, helmDiffColorEnabled(term),
			"must check stdout's color profile, not stderr's -- stdout is where helmfile's output actually goes")
	})

	t.Run("stdout is a TTY: color enabled", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		term := terminal.NewMockTerminal(ctrl)
		term.EXPECT().ColorProfileFor(terminal.Stdout).Return(terminal.ColorTrue)

		assert.True(t, helmDiffColorEnabled(term))
	})
}
