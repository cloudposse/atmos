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
