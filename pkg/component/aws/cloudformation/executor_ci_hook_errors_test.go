package cloudformation

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/component"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/hooks"
	"github.com/cloudposse/atmos/pkg/schema"
)

func captureHookCI(t *testing.T) *[]*hooks.RunCIHooksOptions {
	t.Helper()
	original := runCIHooks
	t.Cleanup(func() { runCIHooks = original })
	var captured []*hooks.RunCIHooksOptions
	runCIHooks = func(options *hooks.RunCIHooksOptions) error {
		captured = append(captured, options)
		return nil
	}
	return &captured
}

func TestRunWithHooksCISetupFailure(t *testing.T) {
	sentinel := errors.New("hook setup failed")
	installExecutorSeamStubs(t, executorSeamStubs{
		getHooks: func(*schema.AtmosConfiguration, *schema.ConfigAndStacksInfo) (*hooks.Hooks, error) {
			return nil, sentinel
		},
	})
	captured := captureHookCI(t)
	ctx := &component.ExecutionContext{Flags: map[string]any{"ci": true}}
	err := runWithHooks(ctx, &schema.AtmosConfiguration{}, &schema.ConfigAndStacksInfo{}, OperationDelete, nil)
	assert.Same(t, sentinel, err)
	require.Len(t, *captured, 1)
	assert.Equal(t, hooks.AfterAwsCloudFormationDelete, (*captured)[0].Event)
	assert.Same(t, sentinel, (*captured)[0].CommandError)
	assert.True(t, (*captured)[0].ForceCIMode)
	result := (*captured)[0].Aggregate.(*schema.CloudFormationCIResult)
	assert.Equal(t, sentinel.Error(), result.Error)
	assert.Empty(t, result.StackName, "operation must not run after setup fails")
	assert.NotZero(t, result.ExitCode)
}

func TestRunWithHooksCIBeforeFailure(t *testing.T) {
	config, info, marker := hookFailureFixture(t, true)
	captured := captureHookCI(t)
	ctx := &component.ExecutionContext{Flags: map[string]any{"ci": true}}
	// A nil spec panics if the operation is incorrectly dispatched after the failed hook.
	err := runWithHooks(ctx, config, info, OperationDelete, nil)
	require.Error(t, err)
	require.Len(t, *captured, 1)
	assert.Same(t, err, (*captured)[0].CommandError)
	assert.Equal(t, hooks.AfterAwsCloudFormationDelete, (*captured)[0].Event)
	result := (*captured)[0].Aggregate.(*schema.CloudFormationCIResult)
	assert.Equal(t, err.Error(), result.Error)
	assert.NotZero(t, result.ExitCode)
	_, markerErr := os.Stat(marker)
	assert.True(t, os.IsNotExist(markerErr), "user after hook must not run when before hook fails")
}

func TestRunWithHooksCIAfterFailure(t *testing.T) {
	for _, operationFails := range []bool{false, true} {
		t.Run(fmt.Sprintf("operation-fails=%t", operationFails), func(t *testing.T) {
			config, info, _ := hookFailureFixture(t, false)
			// Dry-run returns a successful operation without contacting AWS.
			info.DryRun = !operationFails
			stubConfirmOperation(t, false, nil)
			captured := captureHookCI(t)
			ctx := &component.ExecutionContext{Flags: map[string]any{"ci": true}}
			err := runWithHooks(ctx, config, info, OperationDelete, &stackSpec{StackName: "vpc"})
			require.Error(t, err)
			require.Len(t, *captured, 1)
			result := (*captured)[0].Aggregate.(*schema.CloudFormationCIResult)
			assert.Contains(t, result.Error, "missing-hook-binary")
			assert.Equal(t, "vpc", result.StackName)
			assert.NotZero(t, result.ExitCode)
			if operationFails {
				assert.ErrorIs(t, err, errUtils.ErrUserAborted)
				assert.NotContains(t, err.Error(), "missing-hook-binary", "operation error keeps command return precedence")
				assert.Contains(t, result.Error, errUtils.ErrUserAborted.Error())
			} else {
				assert.Equal(t, err.Error(), result.Error)
			}
		})
	}
}

func TestRunWithHooksCISuccess(t *testing.T) {
	installExecutorSeamStubs(t, executorSeamStubs{getHooks: noopGetHooks})
	captured := captureHookCI(t)
	ctx := &component.ExecutionContext{Flags: map[string]any{"ci": true}}
	info := &schema.ConfigAndStacksInfo{DryRun: true}
	require.NoError(t, runWithHooks(ctx, &schema.AtmosConfiguration{}, info, OperationDelete, &stackSpec{StackName: "vpc"}))
	require.Len(t, *captured, 1)
	result := (*captured)[0].Aggregate.(*schema.CloudFormationCIResult)
	assert.Empty(t, result.Error)
	assert.Zero(t, result.ExitCode)
}

// hookFailureFixture uses real hook discovery and preflight. Missing binaries fail locally;
// the test binary writes a marker if an after hook runs unexpectedly.
func hookFailureFixture(t *testing.T, beforeFails bool) (*schema.AtmosConfiguration, *schema.ConfigAndStacksInfo, string) {
	t.Helper()
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "stacks"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "components", "cloudformation", "vpc"), 0o755))
	configYAML := `base_path: "./"
components:
  aws/cloudformation:
    base_path: "components/cloudformation"
stacks:
  base_path: "stacks"
  included_paths: ["**/*"]
  name_pattern: "{stage}"
schemas: {}
`
	require.NoError(t, os.WriteFile(filepath.Join(root, "atmos.yaml"), []byte(configYAML), 0o600))
	executable, err := os.Executable()
	require.NoError(t, err)
	marker := filepath.Join(root, "after-hook-marker")
	beforeCommand, afterCommand := executable, "missing-hook-binary"
	if beforeFails {
		beforeCommand, afterCommand = afterCommand, beforeCommand
	}
	stackYAML := fmt.Sprintf(`vars:
  stage: test
components:
  aws/cloudformation:
    vpc:
      hooks:
        before:
          kind: command
          events: [before.aws/cloudformation.delete]
          command: %q
          args: ["-test.run", "^$"]
          on_failure: fail
        after:
          kind: command
          events: [after.aws/cloudformation.delete]
          when: always
          command: %q
          args: ["-test.run", "^$"]
          on_failure: fail
          env:
            _ATMOS_TEST_WRITE_MARKER: %q
`, beforeCommand, afterCommand, marker)
	require.NoError(t, os.WriteFile(filepath.Join(root, "stacks", "test.yaml"), []byte(stackYAML), 0o600))
	t.Chdir(root)
	info := &schema.ConfigAndStacksInfo{ComponentFromArg: "vpc", Stack: "test"}
	config, err := cfg.InitCliConfig(*info, true)
	require.NoError(t, err)
	return &config, info, marker
}
