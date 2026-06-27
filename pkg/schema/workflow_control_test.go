package schema

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestWorkflowStep_UnmarshalYAML_StructuredParallelOutput(t *testing.T) {
	input := `
type: parallel
output:
  mode: grouped
  order: definition
  show_summary: false
  prefix: "{{ .step.name }}"
steps:
  - name: test
    type: shell
    command: make test
`
	var step WorkflowStep
	require.NoError(t, yaml.Unmarshal([]byte(input), &step))

	require.NotNil(t, step.ParallelOutput)
	assert.Equal(t, "grouped", step.Output)
	assert.Equal(t, "grouped", step.ParallelOutput.Mode)
	assert.Equal(t, "definition", step.ParallelOutput.Order)
	require.NotNil(t, step.ParallelOutput.ShowSummary)
	assert.False(t, *step.ParallelOutput.ShowSummary)
	assert.Equal(t, "{{ .step.name }}", step.ParallelOutput.Prefix)
}

func TestValidateWorkflowSteps_ControlSteps(t *testing.T) {
	tests := []struct {
		name    string
		steps   []WorkflowStep
		wantErr string
	}{
		{
			name: "valid parallel needs",
			steps: []WorkflowStep{{
				Name: "checks",
				Type: TaskTypeParallel,
				Steps: []WorkflowStep{
					{Name: "lint", Type: TaskTypeShell, Command: "make lint"},
					{Name: "test", Type: TaskTypeShell, Command: "make test", Needs: []string{"lint"}},
				},
			}},
		},
		{
			name: "missing need",
			steps: []WorkflowStep{{
				Name:  "checks",
				Type:  TaskTypeParallel,
				Steps: []WorkflowStep{{Name: "test", Type: TaskTypeShell, Command: "make test", Needs: []string{"lint"}}},
			}},
			wantErr: "needs unknown step",
		},
		{
			name: "cycle",
			steps: []WorkflowStep{{
				Name: "checks",
				Type: TaskTypeParallel,
				Steps: []WorkflowStep{
					{Name: "a", Type: TaskTypeShell, Command: "a", Needs: []string{"b"}},
					{Name: "b", Type: TaskTypeShell, Command: "b", Needs: []string{"a"}},
				},
			}},
			wantErr: "cyclic needs dependency",
		},
		{
			name: "unsupported child type disallowed",
			steps: []WorkflowStep{{
				Name:  "checks",
				Type:  TaskTypeParallel,
				Steps: []WorkflowStep{{Name: "prompt", Type: "input", Prompt: "Continue?"}},
			}},
			wantErr: "cannot run inside concurrent step",
		},
		{
			name: "interactive child disallowed",
			steps: []WorkflowStep{{
				Name: "checks",
				Type: TaskTypeParallel,
				Steps: []WorkflowStep{{
					Name:        "prompt",
					Type:        TaskTypeShell,
					Command:     "read answer",
					Interactive: true,
				}},
			}},
			wantErr: "cannot set tty or interactive",
		},
		{
			name: "top-level duplicate names allowed",
			steps: []WorkflowStep{
				{Name: "deploy", Type: TaskTypeShell, Command: "echo first"},
				{Name: "deploy", Type: TaskTypeShell, Command: "echo second"},
			},
		},
		{
			name: "top-level needs disallowed",
			steps: []WorkflowStep{
				{Name: "build", Type: TaskTypeShell, Command: "make build"},
				{Name: "test", Type: TaskTypeShell, Command: "make test", Needs: []string{"build"}},
			},
			wantErr: "sets needs outside a concurrent control step",
		},
		{
			name: "duplicate child names disallowed",
			steps: []WorkflowStep{{
				Name: "checks",
				Type: TaskTypeParallel,
				Steps: []WorkflowStep{
					{Name: "lint", Type: TaskTypeShell, Command: "make lint"},
					{Name: "lint", Type: TaskTypeShell, Command: "make lint-again"},
				},
			}},
			wantErr: "duplicate step name",
		},
		{
			name: "invalid output mode",
			steps: []WorkflowStep{{
				Name:   "checks",
				Type:   TaskTypeParallel,
				Output: "raw",
				Steps:  []WorkflowStep{{Name: "test", Type: TaskTypeShell, Command: "make test"}},
			}},
			wantErr: "unsupported output mode",
		},
		{
			name: "invalid fail mode",
			steps: []WorkflowStep{{
				Name:  "checks",
				Type:  TaskTypeParallel,
				Fail:  &ParallelFailConfig{Mode: "sometimes"},
				Steps: []WorkflowStep{{Name: "test", Type: TaskTypeShell, Command: "make test"}},
			}},
			wantErr: "unsupported fail.mode",
		},
		{
			name: "matrix needs axes",
			steps: []WorkflowStep{{
				Name:  "plans",
				Type:  TaskTypeMatrix,
				Steps: []WorkflowStep{{Name: "plan", Type: TaskTypeShell, Command: "plan"}},
			}},
			wantErr: "requires at least one matrix axis",
		},
		{
			name: "unnamed control step needs children",
			steps: []WorkflowStep{{
				Type: TaskTypeParallel,
			}},
			wantErr: "step at index 0 requires at least one nested step",
		},
		{
			name: "negative max concurrency",
			steps: []WorkflowStep{{
				Name:           "checks",
				Type:           TaskTypeParallel,
				MaxConcurrency: -1,
				Steps:          []WorkflowStep{{Name: "test", Type: TaskTypeShell, Command: "make test"}},
			}},
			wantErr: "sets negative max_concurrency",
		},
		{
			name: "negative fail max failures",
			steps: []WorkflowStep{{
				Name:  "checks",
				Type:  TaskTypeParallel,
				Fail:  &ParallelFailConfig{MaxFailures: -1},
				Steps: []WorkflowStep{{Name: "test", Type: TaskTypeShell, Command: "make test"}},
			}},
			wantErr: "sets negative fail.max_failures",
		},
		{
			name: "matrix empty axis name",
			steps: []WorkflowStep{{
				Name:   "plans",
				Type:   TaskTypeMatrix,
				Matrix: map[string][]string{"": {"dev"}},
				Steps:  []WorkflowStep{{Name: "plan", Type: TaskTypeShell, Command: "plan"}},
			}},
			wantErr: "contains an empty matrix axis name",
		},
		{
			name: "matrix empty axis values",
			steps: []WorkflowStep{{
				Name:   "plans",
				Type:   TaskTypeMatrix,
				Matrix: map[string][]string{"stack": {}},
				Steps:  []WorkflowStep{{Name: "plan", Type: TaskTypeShell, Command: "plan"}},
			}},
			wantErr: "must contain at least one value",
		},
		{
			name: "invalid output order",
			steps: []WorkflowStep{{
				Name:           "checks",
				Type:           TaskTypeParallel,
				ParallelOutput: &ParallelOutputConfig{Mode: "grouped", Order: "random"},
				Steps:          []WorkflowStep{{Name: "test", Type: TaskTypeShell, Command: "make test"}},
			}},
			wantErr: "sets unsupported output.order",
		},
		{
			name: "default child type allowed",
			steps: []WorkflowStep{{
				Name:  "checks",
				Type:  TaskTypeParallel,
				Steps: []WorkflowStep{{Name: "plan", Command: "terraform plan"}},
			}},
		},
		{
			name: "child output mode disallowed",
			steps: []WorkflowStep{{
				Name:  "checks",
				Type:  TaskTypeParallel,
				Steps: []WorkflowStep{{Name: "test", Type: TaskTypeShell, Command: "make test", Output: "raw"}},
			}},
			wantErr: "cannot set child output mode",
		},
		{
			name: "child nested steps disallowed",
			steps: []WorkflowStep{{
				Name: "checks",
				Type: TaskTypeParallel,
				Steps: []WorkflowStep{{
					Name:  "nested",
					Type:  TaskTypeShell,
					Steps: []WorkflowStep{{Name: "inner", Type: TaskTypeShell, Command: "echo inner"}},
				}},
			}},
			wantErr: "cannot declare nested steps",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateWorkflowSteps(tt.steps)
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestWorkflowStep_UnmarshalYAML_InvalidStructuredOutput(t *testing.T) {
	input := `
type: parallel
output:
  - grouped
steps:
  - name: test
    type: shell
    command: make test
`
	var step WorkflowStep
	require.Error(t, yaml.Unmarshal([]byte(input), &step))
}

// TestWorkflowStep_UnmarshalYAML_ResetsReusedReceiver verifies that decoding into a
// reused WorkflowStep clears fields omitted from the second document (Decode merges
// into the destination, so without a reset the first step's fields would leak).
func TestWorkflowStep_UnmarshalYAML_ResetsReusedReceiver(t *testing.T) {
	var step WorkflowStep
	require.NoError(t, yaml.Unmarshal([]byte("name: first\ncommand: echo hi\ntype: shell\n"), &step))
	assert.Equal(t, "echo hi", step.Command)
	assert.Equal(t, "shell", step.Type)

	// Second decode into the SAME value omits command/type.
	require.NoError(t, yaml.Unmarshal([]byte("name: second\n"), &step))
	assert.Equal(t, "second", step.Name)
	assert.Empty(t, step.Command, "omitted command must be cleared, not retained from the first decode")
	assert.Empty(t, step.Type, "omitted type must be cleared, not retained from the first decode")
}

// TestWorkflowStep_UnmarshalYAML_ForRejectsNonStringScalar verifies `for:` accepts
// string scalars / sequences but rejects coerced scalars like booleans and numbers.
func TestWorkflowStep_UnmarshalYAML_ForRejectsNonStringScalar(t *testing.T) {
	// Valid: unquoted identifier resolves to !!str.
	var ok WorkflowStep
	require.NoError(t, yaml.Unmarshal([]byte("type: cancel\nfor: cache\n"), &ok))
	assert.Equal(t, []string{"cache"}, ok.For)

	// Valid: sequence of strings.
	var okList WorkflowStep
	require.NoError(t, yaml.Unmarshal([]byte("type: cancel\nfor: [redis, web]\n"), &okList))
	assert.Equal(t, []string{"redis", "web"}, okList.For)

	for _, bad := range []string{"type: cancel\nfor: true\n", "type: cancel\nfor: 1\n"} {
		var step WorkflowStep
		err := yaml.Unmarshal([]byte(bad), &step)
		require.Error(t, err, "input %q must be rejected", bad)
		assert.ErrorIs(t, err, ErrWorkflowControlStepInvalid)
	}
}
