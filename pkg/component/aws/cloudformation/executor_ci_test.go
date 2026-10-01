package cloudformation

import (
	"errors"
	"testing"

	cfntypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/hooks"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestCloudformationCIModeEnabled covers explicit operation flags and truthy CI environment values,
// including conflicting environment settings.
func TestCloudformationCIModeEnabled(t *testing.T) {
	tests := []struct {
		name     string
		atmosCI  string
		ci       string
		settings map[string]any
		expected bool
	}{
		{name: "settings ci true", atmosCI: "", ci: "", settings: map[string]any{"ci": true}, expected: true},
		{name: "settings ci false", atmosCI: "", ci: "", settings: map[string]any{"ci": false}, expected: false},
		{name: "empty settings, no env", atmosCI: "", ci: "", settings: map[string]any{}, expected: false},
		{name: "nil settings, no env", atmosCI: "", ci: "", settings: nil, expected: false},
		{name: "ATMOS_CI=1 overrides empty settings", atmosCI: "1", ci: "", settings: map[string]any{}, expected: true},
		{name: "CI=yes overrides ATMOS_CI=false", atmosCI: "false", ci: "yes", settings: map[string]any{}, expected: true},
		{name: "CI=0, no ATMOS_CI", atmosCI: "", ci: "0", settings: map[string]any{}, expected: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("ATMOS_CI", tt.atmosCI)
			t.Setenv("CI", tt.ci)

			assert.Equal(t, tt.expected, cloudformationCIModeEnabled(tt.settings))
		})
	}
}

// TestRunCIHook_ApplySuccess verifies that successful apply metadata and the explicit CI override
// reach the shared hook dispatcher.
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

// TestRunCIHook_ApplyFailure retains the command error, nonzero exit code, and available stack
// identity in the CI result.
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

// TestRunCIHook_Diff transfers the preview resource-change count into the aggregate consumed by CI
// templates.
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

// TestRunCIHook_DriftDetect transfers the measured stack drift status and resource count into the CI
// aggregate.
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

// TestRunCIHook_EmptyEventNoOp prevents operations without an associated event from invoking the CI
// dispatcher.
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

// TestRunCIHook_SwallowsHookError checks that a failing CI integration does not panic during operation
// completion.
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

// TestPopulateCloudFormationCIResultFromSummary_NilSummary keeps an absent operation summary safe for
// CI result construction.
func TestPopulateCloudFormationCIResultFromSummary_NilSummary(t *testing.T) {
	result := &schema.CloudFormationCIResult{}
	populateCloudFormationCIResultFromSummary(result, nil)
	assert.Equal(t, &schema.CloudFormationCIResult{}, result)
}

// TestPopulateCloudFormationCIResultFromSummary_DriftCounts preserves explicit zero and nonzero
// measurements instead of counting all returned resource records.
func TestPopulateCloudFormationCIResultFromSummary_DriftCounts(t *testing.T) {
	for _, count := range []int32{0, 2} {
		result := &schema.CloudFormationCIResult{}
		populateCloudFormationCIResultFromSummary(result, map[string]any{
			"stack_name": "dev-vpc", "drifted_resource_count": count,
			"drifts": []cfntypes.StackResourceDrift{{StackResourceDriftStatus: cfntypes.StackResourceDriftStatusInSync}},
		})
		assert.Equal(t, int(count), result.DriftedCount)
	}
}

// TestSummaryLen accepts slices of different element types and treats absent or nonslice summary
// values as empty.
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
