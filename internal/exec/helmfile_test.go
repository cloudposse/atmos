package exec

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
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
// helmfile component ("myapp") that has no chart releases — no AWS/EKS auth
// is configured, so ExecuteHelmfile reaches the real `helmfile <cmd>`
// execution deterministically and offline, failing only because the
// component has no matching releases. This is what lets the test reach
// info.NodeHooks.Before/After without a real cluster.
func newHelmfileNodeHooksFixture(t *testing.T) schema.ConfigAndStacksInfo {
	t.Helper()
	tests.RequireHelmfile(t)

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
		ComponentFromArg: "myapp",
		Stack:            "dev",
		SubCommand:       "diff",
		ComponentType:    "helmfile",
	}
}

// TestExecuteHelmfileNodeHooks_InvokedAroundExecution is a regression test
// for the NodeHooks wiring added to ExecuteHelmfile: both Before and After
// must fire around the real helmfile execution (which fails here because the
// fixture component has no chart releases — that failure is incidental, not
// the point of the test).
func TestExecuteHelmfileNodeHooks_InvokedAroundExecution(t *testing.T) {
	info := newHelmfileNodeHooksFixture(t)
	nodeHooks := &testHelmfileNodeHooks{}
	info.NodeHooks = nodeHooks

	err := ExecuteHelmfile(info)

	require.Error(t, err, "the fixture component has no releases, so helmfile itself fails")
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
	info.DryRun = true // Skips the real helmfile execution, so execErr is nil.
	sentinelErr := errors.New("after hook failed")
	nodeHooks := &testHelmfileNodeHooks{afterErr: sentinelErr}
	info.NodeHooks = nodeHooks

	err := ExecuteHelmfile(info)

	require.Error(t, err)
	assert.ErrorIs(t, err, sentinelErr)
	assert.NoError(t, nodeHooks.afterExecErr, "dry-run means the underlying exec reported no error")
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
