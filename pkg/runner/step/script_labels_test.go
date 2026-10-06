package step

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ansi"
	"github.com/cloudposse/atmos/pkg/schema"
)

// runLabelScriptStep executes an embedded Starlark script step and returns the terminal streams.
func runLabelScriptStep(t *testing.T, step *schema.WorkflowStep, workflow *schema.WorkflowDefinition) (string, string, *StepResult) {
	t.Helper()
	// The shared I/O setup changes process globals, so callers must not run in parallel.
	stdout, stderr, cleanup := setupOutputModeCapture(t)
	defer cleanup()

	step.Interpreter = "starlark"
	step.Script = "print(\"hello-from-script\")\n"
	handler := &ScriptHandler{}
	var (
		result *StepResult
		err    error
	)
	if workflow != nil {
		result, err = handler.ExecuteWithWorkflow(context.Background(), step, NewVariables(), workflow)
	} else {
		result, err = handler.Execute(context.Background(), step, NewVariables())
	}
	require.NoError(t, err)
	cleanup()
	return ansi.Strip(stdout.String()), ansi.Strip(stderr.String()), result
}

func TestScriptStepLabels(t *testing.T) {
	t.Setenv("ATMOS_PAGER", "")
	t.Setenv("NO_PAGER", "")

	tests := []struct {
		name       string
		step       schema.WorkflowStep
		workflow   *schema.WorkflowDefinition
		wantLabels bool
	}{
		{name: "defaults to no labels", step: schema.WorkflowStep{Name: "plain"}},
		{
			name:       "show labels true restores labels",
			step:       schema.WorkflowStep{Name: "labeled", Show: &schema.ShowConfig{Labels: BoolPtr(true)}},
			wantLabels: true,
		},
		{
			name: "explicit log output has no labels unless requested",
			step: schema.WorkflowStep{Name: "logged", Output: string(OutputModeLog)},
		},
		{
			name:       "explicit log output with show labels true",
			step:       schema.WorkflowStep{Name: "logged-labels", Output: string(OutputModeLog), Show: &schema.ShowConfig{Labels: BoolPtr(true)}},
			wantLabels: true,
		},
		{
			name:     "workflow log output has no labels unless requested",
			step:     schema.WorkflowStep{Name: "wf-logged"},
			workflow: &schema.WorkflowDefinition{Output: string(OutputModeLog)},
		},
		{
			name:       "workflow show labels true restores labels",
			step:       schema.WorkflowStep{Name: "wf-labeled"},
			workflow:   &schema.WorkflowDefinition{Show: &schema.ShowConfig{Labels: BoolPtr(true)}},
			wantLabels: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			step := tt.step
			stdout, stderr, result := runLabelScriptStep(t, &step, tt.workflow)

			assert.Contains(t, stdout, "hello-from-script")
			assert.Equal(t, "hello-from-script\n", result.Value)
			if tt.wantLabels {
				assert.Contains(t, stderr, "["+step.Name+"]")
				assert.Contains(t, stderr, step.Name+" completed")
				return
			}
			assert.NotContains(t, stderr, "["+step.Name+"]")
			assert.NotContains(t, stderr, "completed")
		})
	}
}

func TestScriptStepOutputNoneStaysSilent(t *testing.T) {
	t.Setenv("ATMOS_PAGER", "")
	t.Setenv("NO_PAGER", "")

	step := schema.WorkflowStep{Name: "quiet", Output: string(OutputModeNone)}
	stdout, stderr, result := runLabelScriptStep(t, &step, nil)

	assert.Empty(t, stdout)
	assert.Empty(t, stderr)
	assert.Equal(t, "hello-from-script\n", result.Value)
}
