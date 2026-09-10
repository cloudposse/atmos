package cloudformation

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/hooks"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestCloudformationCIModeEnabled(t *testing.T) {
	t.Setenv("ATMOS_CI", "")
	t.Setenv("CI", "")

	assert.True(t, cloudformationCIModeEnabled(map[string]any{"ci": true}))
	assert.False(t, cloudformationCIModeEnabled(map[string]any{"ci": false}))
	assert.False(t, cloudformationCIModeEnabled(map[string]any{}))
	assert.False(t, cloudformationCIModeEnabled(nil))

	t.Setenv("ATMOS_CI", "1")
	assert.True(t, cloudformationCIModeEnabled(map[string]any{}))

	t.Setenv("ATMOS_CI", "false")
	t.Setenv("CI", "yes")
	assert.True(t, cloudformationCIModeEnabled(map[string]any{}))

	t.Setenv("CI", "0")
	assert.False(t, cloudformationCIModeEnabled(map[string]any{}))
}

func TestRunCIHook_ApplySuccess(t *testing.T) {
	original := runCIHooks
	t.Cleanup(func() { runCIHooks = original })

	var captured *hooks.RunCIHooksOptions
	runCIHooks = func(opts *hooks.RunCIHooksOptions) error {
		captured = opts
		return nil
	}

	info := &schema.ConfigAndStacksInfo{
		ComponentFromArg: "vpc",
		Stack:            "dev",
		SubCommand:       "apply",
	}
	summary := map[string]any{
		"stack_name":     "dev-vpc",
		"changeset_name": "atmos-20260101",
		"changes":        []int{1, 2, 3},
	}

	runCIHook(ciHookParams{
		event:       hooks.AfterAwsCloudFormationApply,
		flags:       map[string]any{"ci": true},
		atmosConfig: &schema.AtmosConfiguration{},
		info:        info,
		summary:     summary,
	})

	require.NotNil(t, captured)
	assert.Equal(t, hooks.AfterAwsCloudFormationApply, captured.Event)
	assert.True(t, captured.ForceCIMode)
	assert.NoError(t, captured.CommandError)
	assert.Equal(t, 0, captured.ExitCode)

	result, ok := captured.Aggregate.(*schema.CloudFormationCIResult)
	require.True(t, ok)
	assert.Equal(t, "vpc", result.Component)
	assert.Equal(t, "dev", result.Stack)
	assert.Equal(t, "apply", result.Command)
	assert.Equal(t, "dev-vpc", result.StackName)
	assert.Equal(t, "atmos-20260101", result.ChangeSetName)
	assert.Equal(t, 3, result.ResourceChanges)
	assert.Empty(t, result.Error)
	assert.Equal(t, 0, result.ExitCode)
}

func TestRunCIHook_ApplyFailure(t *testing.T) {
	original := runCIHooks
	t.Cleanup(func() { runCIHooks = original })

	var captured *hooks.RunCIHooksOptions
	runCIHooks = func(opts *hooks.RunCIHooksOptions) error {
		captured = opts
		return nil
	}

	commandErr := errors.New("changeset execution failed")
	info := &schema.ConfigAndStacksInfo{
		ComponentFromArg: "vpc",
		Stack:            "dev",
		SubCommand:       "apply",
	}
	summary := map[string]any{"stack_name": "dev-vpc"}

	runCIHook(ciHookParams{
		event:       hooks.AfterAwsCloudFormationApply,
		atmosConfig: &schema.AtmosConfiguration{},
		info:        info,
		summary:     summary,
		commandErr:  commandErr,
	})

	require.NotNil(t, captured)
	assert.Equal(t, hooks.AfterAwsCloudFormationApply, captured.Event)
	assert.Equal(t, commandErr, captured.CommandError)
	assert.Equal(t, errUtils.GetExitCode(commandErr), captured.ExitCode)
	assert.NotZero(t, captured.ExitCode)

	result, ok := captured.Aggregate.(*schema.CloudFormationCIResult)
	require.True(t, ok)
	assert.Equal(t, commandErr.Error(), result.Error)
	assert.Equal(t, errUtils.GetExitCode(commandErr), result.ExitCode)
	assert.Equal(t, "dev-vpc", result.StackName)
}

func TestRunCIHook_Diff(t *testing.T) {
	original := runCIHooks
	t.Cleanup(func() { runCIHooks = original })

	var captured *hooks.RunCIHooksOptions
	runCIHooks = func(opts *hooks.RunCIHooksOptions) error {
		captured = opts
		return nil
	}

	info := &schema.ConfigAndStacksInfo{ComponentFromArg: "vpc", Stack: "dev", SubCommand: "diff"}
	summary := map[string]any{"stack_name": "dev-vpc", "changes": []int{1, 2}}

	runCIHook(ciHookParams{
		event:       hooks.AfterAwsCloudFormationDiff,
		atmosConfig: &schema.AtmosConfiguration{},
		info:        info,
		summary:     summary,
	})

	require.NotNil(t, captured)
	result, ok := captured.Aggregate.(*schema.CloudFormationCIResult)
	require.True(t, ok)
	assert.Equal(t, 2, result.ResourceChanges)
	assert.Equal(t, "dev-vpc", result.StackName)
}

func TestRunCIHook_DriftDetect(t *testing.T) {
	original := runCIHooks
	t.Cleanup(func() { runCIHooks = original })

	var captured *hooks.RunCIHooksOptions
	runCIHooks = func(opts *hooks.RunCIHooksOptions) error {
		captured = opts
		return nil
	}

	info := &schema.ConfigAndStacksInfo{ComponentFromArg: "vpc", Stack: "dev", SubCommand: "drift-detect"}
	summary := map[string]any{
		"stack_name":             "dev-vpc",
		"drift_status":           "DRIFTED",
		"drifted_resource_count": int32(4),
	}

	runCIHook(ciHookParams{
		event:       hooks.AfterAwsCloudFormationDriftDetect,
		atmosConfig: &schema.AtmosConfiguration{},
		info:        info,
		summary:     summary,
	})

	require.NotNil(t, captured)
	result, ok := captured.Aggregate.(*schema.CloudFormationCIResult)
	require.True(t, ok)
	assert.Equal(t, "DRIFTED", result.DriftStatus)
	assert.Equal(t, 4, result.DriftedCount)
	assert.Equal(t, "dev-vpc", result.StackName)
}

func TestRunCIHook_EmptyEventNoOp(t *testing.T) {
	original := runCIHooks
	t.Cleanup(func() { runCIHooks = original })

	called := false
	runCIHooks = func(*hooks.RunCIHooksOptions) error {
		called = true
		return nil
	}

	runCIHook(ciHookParams{atmosConfig: &schema.AtmosConfiguration{}, info: &schema.ConfigAndStacksInfo{}})
	assert.False(t, called)
}

func TestRunCIHook_SwallowsHookError(t *testing.T) {
	original := runCIHooks
	t.Cleanup(func() { runCIHooks = original })

	called := false
	runCIHooks = func(*hooks.RunCIHooksOptions) error {
		called = true
		return errors.New("hook failed")
	}

	assert.NotPanics(t, func() {
		runCIHook(ciHookParams{
			event:       hooks.AfterAwsCloudFormationDelete,
			atmosConfig: &schema.AtmosConfiguration{},
			info:        &schema.ConfigAndStacksInfo{},
		})
	})
	assert.True(t, called)
}

func TestPopulateCloudFormationCIResultFromSummary_NilSummary(t *testing.T) {
	result := &schema.CloudFormationCIResult{}
	populateCloudFormationCIResultFromSummary(result, nil)
	assert.Equal(t, &schema.CloudFormationCIResult{}, result)
}

func TestPopulateCloudFormationCIResultFromSummary_DriftDescribeFallsBackToDriftsLen(t *testing.T) {
	result := &schema.CloudFormationCIResult{}
	populateCloudFormationCIResultFromSummary(result, map[string]any{
		"stack_name": "dev-vpc",
		"drifts":     []string{"a", "b", "c"},
	})
	assert.Equal(t, "dev-vpc", result.StackName)
	assert.Equal(t, 3, result.DriftedCount)
}

func TestSummaryLen(t *testing.T) {
	tests := []struct {
		name  string
		value any
		want  int
	}{
		{"nil value", nil, 0},
		{"non-slice value", "not-a-slice", 0},
		{"non-slice int", 42, 0},
		{"empty slice", []int{}, 0},
		{"populated slice", []int{1, 2, 3}, 3},
		{"populated string slice", []string{"a", "b"}, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, summaryLen(tt.value))
		})
	}
}
