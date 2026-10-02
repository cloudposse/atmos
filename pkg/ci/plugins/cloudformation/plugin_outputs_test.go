package cloudformation

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ci/internal/plugin"
	"github.com/cloudposse/atmos/pkg/ci/internal/provider"
	"github.com/cloudposse/atmos/pkg/ci/templates"
	"github.com/cloudposse/atmos/pkg/schema"
)

// recordingWriter captures both summaries and output variables.
type recordingWriter struct {
	summary string
	outputs map[string]string
	order   []string
}

func (r *recordingWriter) WriteOutput(key, value string) error {
	if r.outputs == nil {
		r.outputs = map[string]string{}
	}
	r.outputs[key] = value
	r.order = append(r.order, key)
	return nil
}

func (r *recordingWriter) WriteSummary(content string) error {
	r.summary = content
	return nil
}

var _ provider.OutputWriter = (*recordingWriter)(nil)

func runHook(t *testing.T, cfg *schema.AtmosConfiguration, command, output string, result *schema.CloudFormationCIResult, exitCode int) *recordingWriter {
	t.Helper()
	writer := &recordingWriter{}
	require.NoError(t, (&Plugin{}).onAfterOperation(&plugin.HookContext{
		Provider: fakeProvider{writer: writer}, TemplateLoader: templates.NewLoader(nil), Config: cfg, Command: command,
		Info: &schema.ConfigAndStacksInfo{ComponentFromArg: "app", Stack: "dev"}, Output: output, Aggregate: result, ExitCode: exitCode,
	}))
	return writer
}

// The apply summary must show the stack Outputs under "CloudFormation output" (the section never
// rendered before because nothing filled .Output) and name the invoked verb.
func TestSummary_ApplyShowsOutputsAndInvokedVerb(t *testing.T) {
	writer := runHook(t, nil, "apply", "Password = <MASKED>\nVpcId = vpc-123", &schema.CloudFormationCIResult{
		Command: "deploy", StackName: "dev-app", HasChanges: true,
	}, 0)

	assert.Contains(t, writer.summary, "CloudFormation Deploy Summary")
	assert.Contains(t, writer.summary, "atmos aws cloudformation deploy app -s dev")
	assert.Contains(t, writer.summary, "<summary>CloudFormation output</summary>")
	assert.Contains(t, writer.summary, "VpcId = vpc-123")
	assert.Contains(t, writer.summary, "Password = <MASKED>")
	assert.NotContains(t, writer.summary, "No changes")
}

// A plan is not a diff: the summary title and the reproduce command use the invoked verb. The
// alias only applies to the hook command's own alias; unrelated verbs fall back to the hook command.
func TestSummary_PlanIsTitledPlan(t *testing.T) {
	tests := []struct {
		name      string
		invoked   string
		wantTitle string
		wantCmd   string
	}{
		{name: "plan", invoked: "plan", wantTitle: "CloudFormation Plan Summary", wantCmd: "atmos aws cloudformation plan app -s dev"},
		{name: "diff", invoked: "diff", wantTitle: "CloudFormation Diff Summary", wantCmd: "atmos aws cloudformation diff app -s dev"},
		{name: "unset", invoked: "", wantTitle: "CloudFormation Diff Summary", wantCmd: "atmos aws cloudformation diff app -s dev"},
		{name: "unrelated verb", invoked: "delete", wantTitle: "CloudFormation Diff Summary", wantCmd: "atmos aws cloudformation diff app -s dev"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			writer := runHook(t, nil, "diff", "", &schema.CloudFormationCIResult{Command: tt.invoked, HasChanges: true, ResourceChanges: 1}, 0)
			assert.Contains(t, writer.summary, tt.wantTitle)
			assert.Contains(t, writer.summary, tt.wantCmd)
		})
	}
}

// A no-op diff/apply must say "No changes"; a failed run or one with changes must not.
func TestSummary_NoChanges(t *testing.T) {
	for _, command := range []string{"diff", "apply"} {
		t.Run(command+" no-op", func(t *testing.T) {
			writer := runHook(t, nil, command, "", &schema.CloudFormationCIResult{NoOp: true}, 0)
			assert.Contains(t, writer.summary, "No changes")
		})
		t.Run(command+" failed no-op flag", func(t *testing.T) {
			writer := runHook(t, nil, command, "", &schema.CloudFormationCIResult{NoOp: true, Error: "boom"}, 1)
			assert.NotContains(t, writer.summary, "No changes")
		})
	}
	writer := runHook(t, nil, "diff", "", &schema.CloudFormationCIResult{ResourceChanges: 2, HasChanges: true}, 0)
	assert.NotContains(t, writer.summary, "No changes")
	assert.Contains(t, writer.summary, "Resource changes: **2**")
}

// drift detect run with --fail-on-drift must reproduce with it; without it the flag is absent.
func TestSummary_DriftDetectReproduceIncludesFailOnDrift(t *testing.T) {
	with := runHook(t, nil, "drift-detect", "", &schema.CloudFormationCIResult{FailOnDrift: true, DriftStatus: "DRIFTED", DriftedCount: 1}, 1)
	assert.Contains(t, with.summary, "atmos aws cloudformation drift detect app -s dev --fail-on-drift\n")
	without := runHook(t, nil, "drift-detect", "", &schema.CloudFormationCIResult{DriftStatus: "IN_SYNC"}, 0)
	assert.Contains(t, without.summary, "atmos aws cloudformation drift detect app -s dev\n")
	assert.NotContains(t, without.summary, "--fail-on-drift")
}

// Output variables follow the terraform plugin's pattern: has_changes for diff/apply, drifted for
// the drift verbs, changeset_name, stack_status, and stack outputs as output_<key>.
func TestOutputs_PerCommand(t *testing.T) {
	tests := []struct {
		name    string
		command string
		result  *schema.CloudFormationCIResult
		code    int
		want    map[string]string
		absent  []string
	}{
		{
			name: "apply with changes", command: "apply", code: 0,
			result: &schema.CloudFormationCIResult{
				Command: "deploy", StackName: "dev-app", ChangeSetName: "cs-1", HasChanges: true,
				Outputs: map[string]string{"VpcId": "vpc-1", "Secret": "<MASKED>"},
			},
			want: map[string]string{
				"has_changes": "true", "changeset_name": "cs-1", "stack_name": "dev-app", "command": "deploy",
				"stack": "dev", "component": "app", "exit_code": "0", "output_VpcId": "vpc-1", "output_Secret": "<MASKED>",
			},
			absent: []string{"drifted", "stack_status"},
		},
		{
			name: "diff no-op", command: "diff", result: &schema.CloudFormationCIResult{NoOp: true}, code: 0,
			want: map[string]string{"has_changes": "false", "command": "diff"}, absent: []string{"changeset_name", "drifted"},
		},
		{
			name: "delete", command: "delete", result: &schema.CloudFormationCIResult{StackStatus: "DELETE_COMPLETE"}, code: 0,
			want: map[string]string{"stack_status": "DELETE_COMPLETE"}, absent: []string{"has_changes", "drifted"},
		},
		{
			name: "drift detect drifted", command: "drift-detect", code: 1,
			result: &schema.CloudFormationCIResult{DriftStatus: "DRIFTED", DriftedCount: 2},
			want:   map[string]string{"drifted": "true", "drift_status": "DRIFTED", "drifted_resource_count": "2", "exit_code": "1"},
			absent: []string{"has_changes"},
		},
		{
			name: "drift describe in sync", command: "drift-describe", result: &schema.CloudFormationCIResult{DriftStatus: "IN_SYNC"}, code: 0,
			want: map[string]string{"drifted": "false", "drifted_resource_count": "0"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			writer := runHook(t, nil, tt.command, "", tt.result, tt.code)
			for key, value := range tt.want {
				assert.Equal(t, value, writer.outputs[key], key)
			}
			for _, key := range tt.absent {
				assert.NotContains(t, writer.outputs, key)
			}
			assert.True(t, sortedStrings(writer.order), "variables must be written in a deterministic order: %v", writer.order)
		})
	}
}

func sortedStrings(values []string) bool {
	for i := 1; i < len(values); i++ {
		if strings.Compare(values[i-1], values[i]) > 0 {
			return false
		}
	}
	return true
}

// ci.output.enabled: false suppresses variables (but not the summary), and ci.output.variables
// whitelists the bookkeeping variables while stack outputs always pass.
func TestOutputs_ConfigGates(t *testing.T) {
	result := &schema.CloudFormationCIResult{HasChanges: true, ChangeSetName: "cs", Outputs: map[string]string{"VpcId": "vpc-1"}}

	disabled := false
	cfg := &schema.AtmosConfiguration{}
	cfg.CI.Output.Enabled = &disabled
	off := runHook(t, cfg, "apply", "", result, 0)
	assert.Empty(t, off.outputs)
	assert.NotEmpty(t, off.summary, "disabling outputs must not disable the summary")

	cfg = &schema.AtmosConfiguration{}
	cfg.CI.Output.Variables = []string{"has_changes"}
	filtered := runHook(t, cfg, "apply", "", result, 0)
	assert.Equal(t, map[string]string{"has_changes": "true", "output_VpcId": "vpc-1"}, filtered.outputs)
}

// A summary failure must still be returned, and output-variable writing must not hide it.
func TestOutputs_NoWriterIsNoOp(t *testing.T) {
	require.NoError(t, (&Plugin{}).onAfterOperation(&plugin.HookContext{
		Provider: fakeProvider{}, TemplateLoader: templates.NewLoader(nil), Command: "apply",
		Info: &schema.ConfigAndStacksInfo{ComponentFromArg: "app", Stack: "dev"},
	}))
}

// verbName and verbTitle unit coverage.
func TestVerbNameAndTitle(t *testing.T) {
	assert.Equal(t, "plan", verbName("diff", "plan"))
	assert.Equal(t, "deploy", verbName("apply", "deploy"))
	assert.Equal(t, "diff", verbName("diff", "deploy"))
	assert.Equal(t, "delete", verbName("delete", ""))
	assert.Equal(t, "Drift Detect", verbTitle("drift-detect"))
	assert.Equal(t, "Plan", verbTitle("plan"))
}
