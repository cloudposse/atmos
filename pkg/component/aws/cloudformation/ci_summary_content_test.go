package cloudformation

import (
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	cfntypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/hooks"
	"github.com/cloudposse/atmos/pkg/schema"
)

// captureCIHook runs runCIHook with a stubbed dispatcher and returns what it was given.
func captureCIHook(t *testing.T, p ciHookParams) *hooks.RunCIHooksOptions {
	t.Helper()
	original := runCIHooks
	t.Cleanup(func() { runCIHooks = original })
	var captured *hooks.RunCIHooksOptions
	runCIHooks = func(opts *hooks.RunCIHooksOptions) error {
		captured = opts
		return nil
	}
	runCIHook(p)
	require.NotNil(t, captured)
	return captured
}

// Apply must hand the stack Outputs (the masked, presented map) to the CI summary, so the
// "CloudFormation output" section renders instead of staying empty. A NoEcho value arrives
// already redacted because the summary carries the presented outputs, never raw API values.
func TestRunCIHook_ApplyPopulatesOutputs(t *testing.T) {
	captured := captureCIHook(t, ciHookParams{
		event: hooks.AfterAwsCloudFormationApply,
		info:  &schema.ConfigAndStacksInfo{ComponentFromArg: "vpc", Stack: "dev", SubCommand: "deploy"},
		summary: map[string]any{
			"stack_name": "dev-vpc",
			"no_op":      false,
			"outputs":    map[string]any{"VpcId": "vpc-123", "Password": "<MASKED>"},
		},
	})

	assert.Equal(t, "Password = <MASKED>\nVpcId = vpc-123", captured.Output)
	result, ok := captured.Aggregate.(*schema.CloudFormationCIResult)
	require.True(t, ok)
	assert.Equal(t, map[string]string{"VpcId": "vpc-123", "Password": "<MASKED>"}, result.Outputs)
	assert.Equal(t, "deploy", result.Command)
	assert.True(t, result.HasChanges)
	assert.False(t, result.NoOp)
}

// Diff must list the changeset's resource changes (Action, type, logical ID, replacement) and count
// only resource changes. A no-op changeset reports NoOp and no text.
func TestRunCIHook_DiffListsResourceChanges(t *testing.T) {
	changes := []cfntypes.Change{
		{ResourceChange: &cfntypes.ResourceChange{
			Action: cfntypes.ChangeActionModify, ResourceType: aws.String("AWS::SSM::Parameter"),
			LogicalResourceId: aws.String("Param"), Replacement: cfntypes.ReplacementTrue,
		}},
		{ResourceChange: &cfntypes.ResourceChange{
			Action: cfntypes.ChangeActionAdd, ResourceType: aws.String("AWS::SNS::Topic"), LogicalResourceId: aws.String("Topic"),
		}},
		{}, // A non-resource change carries no ResourceChange and must not be counted.
	}
	tests := []struct {
		name        string
		summary     map[string]any
		wantOutput  string
		wantChanges int
		wantNoOp    bool
	}{
		{
			name:        "changes",
			summary:     map[string]any{"changes": changes, "no_op": false},
			wantOutput:  "Modify   AWS::SSM::Parameter          Param (replacement: True)\nAdd      AWS::SNS::Topic              Topic",
			wantChanges: 2,
		},
		{name: "no-op", summary: map[string]any{"changes": []cfntypes.Change{}, "no_op": true}, wantNoOp: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			captured := captureCIHook(t, ciHookParams{
				event:   hooks.AfterAwsCloudFormationDiff,
				info:    &schema.ConfigAndStacksInfo{SubCommand: "plan"},
				summary: tt.summary,
			})
			assert.Equal(t, tt.wantOutput, captured.Output)
			result := captured.Aggregate.(*schema.CloudFormationCIResult)
			assert.Equal(t, tt.wantChanges, result.ResourceChanges)
			assert.Equal(t, tt.wantNoOp, result.NoOp)
			assert.Equal(t, !tt.wantNoOp, result.HasChanges)
			assert.Equal(t, "plan", result.Command, "the invoked verb must reach the summary")
		})
	}
}

// Drift detect with --fail-on-drift must record the flag so the reproduce command can include it;
// without it the flag stays false (negative path).
func TestRunCIHook_RecordsFailOnDrift(t *testing.T) {
	for _, set := range []bool{true, false} {
		captured := captureCIHook(t, ciHookParams{
			event:      hooks.AfterAwsCloudFormationDriftDetect,
			flags:      map[string]any{"fail-on-drift": set},
			info:       &schema.ConfigAndStacksInfo{SubCommand: "drift-detect"},
			summary:    map[string]any{"drift_status": "DRIFTED", "drifted_resource_count": int32(1)},
			commandErr: errors.New("drift"),
		})
		assert.Equal(t, set, captured.Aggregate.(*schema.CloudFormationCIResult).FailOnDrift)
	}
}

// Delete's observed final status must reach the CI result; operations without one leave it empty.
func TestPopulateCloudFormationCIResultFromSummary_StackStatus(t *testing.T) {
	result := &schema.CloudFormationCIResult{}
	populateCloudFormationCIResultFromSummary(result, map[string]any{"final_status": "DELETE_COMPLETE"})
	assert.Equal(t, "DELETE_COMPLETE", result.StackStatus)

	empty := &schema.CloudFormationCIResult{}
	populateCloudFormationCIResultFromSummary(empty, map[string]any{})
	assert.Empty(t, empty.StackStatus)
	assert.False(t, empty.HasChanges, "no no_op key means no changeset ran, so there are no changes to report")
}

// Drift describe must put the drifted resources, with property differences, in the CI output; an
// in-sync stack yields no text.
func TestCIOutputText_Drifts(t *testing.T) {
	drifts := []cfntypes.StackResourceDrift{
		{StackResourceDriftStatus: cfntypes.StackResourceDriftStatusInSync, LogicalResourceId: aws.String("Ok")},
		{
			StackResourceDriftStatus: cfntypes.StackResourceDriftStatusModified,
			ResourceType:             aws.String("AWS::SSM::Parameter"), LogicalResourceId: aws.String("Param"),
			PropertyDifferences: []cfntypes.PropertyDifference{{
				PropertyPath: aws.String("/Value"), ExpectedValue: aws.String("v2"), ActualValue: aws.String("v1"),
				DifferenceType: cfntypes.DifferenceTypeNotEqual,
			}},
		},
	}
	text := ciOutputText(map[string]any{"drifts": drifts})
	assert.Contains(t, text, "  MODIFIED   AWS::SSM::Parameter          Param")
	assert.Contains(t, text, "/Value: expected v2, actual v1 (NOT_EQUAL)")
	assert.NotContains(t, text, "Ok")

	assert.Empty(t, ciOutputText(map[string]any{"drifts": drifts[:1]}))
	assert.Empty(t, ciOutputText(nil))
}

// drift describe must show each MODIFIED resource's property differences (path, expected, actual,
// type), not only its status.
func TestDriftLines_PropertyDifferences(t *testing.T) {
	lines := driftLines([]cfntypes.StackResourceDrift{{
		StackResourceDriftStatus: cfntypes.StackResourceDriftStatusModified,
		ResourceType:             aws.String("AWS::S3::Bucket"), LogicalResourceId: aws.String("Bucket"),
		PropertyDifferences: []cfntypes.PropertyDifference{
			{PropertyPath: aws.String("/A"), ExpectedValue: aws.String("1"), ActualValue: aws.String("2"), DifferenceType: cfntypes.DifferenceTypeNotEqual},
			{PropertyPath: aws.String("/B"), ExpectedValue: aws.String("x"), DifferenceType: cfntypes.DifferenceTypeRemove},
		},
	}})
	require.Len(t, lines, 3)
	assert.Contains(t, lines[0], "MODIFIED")
	assert.Contains(t, lines[0], "Bucket")
	assert.Equal(t, "    /A: expected 1, actual 2 (NOT_EQUAL)", lines[1])
	assert.Equal(t, "    /B: expected x, actual  (REMOVE)", lines[2])
}

// A top-level alias verb reaches the CI result as the verb the user ran: `plan`
// dispatches with SubCommand "diff", and without the recorded invoked verb the
// summary would be titled "Diff". Without the flag, the dispatch identifier stays.
func TestRunCIHook_RecordsInvokedAliasVerb(t *testing.T) {
	tests := []struct {
		name  string
		flags map[string]any
		want  string
	}{
		{name: "plan alias", flags: map[string]any{invokedVerbFlag: "plan"}, want: "plan"},
		{name: "deploy alias", flags: map[string]any{invokedVerbFlag: "deploy"}, want: "deploy"},
		{name: "no alias keeps dispatch identifier", flags: map[string]any{}, want: "diff"},
		{name: "empty alias keeps dispatch identifier", flags: map[string]any{invokedVerbFlag: ""}, want: "diff"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			captured := captureCIHook(t, ciHookParams{
				event:   hooks.AfterAwsCloudFormationDiff,
				flags:   tt.flags,
				info:    &schema.ConfigAndStacksInfo{ComponentFromArg: "vpc", Stack: "dev", SubCommand: "diff"},
				summary: map[string]any{"stack_name": "dev-vpc"},
			})
			result, ok := captured.Aggregate.(*schema.CloudFormationCIResult)
			require.True(t, ok)
			assert.Equal(t, tt.want, result.Command)
		})
	}
}
