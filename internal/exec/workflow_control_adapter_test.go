package exec

import (
	"errors"
	"testing"

	stepPkg "github.com/cloudposse/atmos/pkg/runner/step"
	"github.com/cloudposse/atmos/pkg/scheduler"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/workflow"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorkflowControlTemplateDataInitializesState(t *testing.T) {
	ResetStepExecutorState()
	t.Cleanup(ResetStepExecutorState)
	require.Nil(t, stepExecutorState)

	data := workflowControlTemplateData("child", map[string]string{"stack": "dev"})
	require.NotNil(t, stepExecutorState)
	assert.Contains(t, data, "steps")
	assert.Contains(t, data, "env")

	stepExecutorState.Variables().Set("previous", stepPkg.NewStepResult("ok"))
	data = workflowControlTemplateData("child", nil)
	steps := data["steps"].(map[string]any)
	previous := steps["previous"].(map[string]any)
	assert.Equal(t, "ok", previous["value"])
}

func TestStoreWorkflowControlResultStoresMetadataAndErrors(t *testing.T) {
	ResetStepExecutorState()
	t.Cleanup(ResetStepExecutorState)
	errBoom := errors.New("boom")

	storeWorkflowControlResult(&scheduler.Result{
		NodeID: "child",
		Status: scheduler.StatusFailed,
		Value: &workflow.ControlResult{
			Stdout:   " output \n",
			Stderr:   "warning",
			Err:      errBoom,
			Canceled: true,
		},
	})

	require.NotNil(t, stepExecutorState)
	stored := stepExecutorState.Variables().Steps["child"]
	require.NotNil(t, stored)
	assert.Equal(t, "output", stored.Value)
	assert.Equal(t, " output \n", stored.Metadata["stdout"])
	assert.Equal(t, "warning", stored.Metadata["stderr"])
	assert.Equal(t, string(scheduler.StatusFailed), stored.Metadata["status"])
	assert.Equal(t, true, stored.Metadata["canceled"])
	assert.Equal(t, "boom", stored.Error)
	assert.False(t, stored.Skipped)
}

func TestStoreWorkflowControlResultMarksSkippedFallback(t *testing.T) {
	ResetStepExecutorState()
	t.Cleanup(ResetStepExecutorState)

	storeWorkflowControlResult(&scheduler.Result{
		NodeID: "skipped",
		Status: scheduler.StatusSkipped,
		Value:  "not a control result",
	})

	stored := stepExecutorState.Variables().Steps["skipped"]
	require.NotNil(t, stored)
	assert.Empty(t, stored.Value)
	assert.True(t, stored.Skipped)
}

func TestWorkflowControlStarlarkInputs(t *testing.T) {
	ResetStepExecutorState()
	t.Cleanup(ResetStepExecutorState)
	control := &workflowControlContext{}
	vars := workflowControlVariables(control)
	vars.SetFlag("stack", "dev")
	parent := &schema.WorkflowStep{Name: "group", Type: schema.TaskTypeParallel, Output: "none", Steps: []schema.WorkflowStep{{
		Name: "inspect", Type: schema.TaskTypeScript, Interpreter: "starlark",
		Script: `output = {"stack": ctx.flags["stack"], "arguments": ctx.arguments}`,
	}}}
	require.NoError(t, executeWorkflowControlStep(t.Context(), control, parent))
	value, ok := vars.GetValue("inspect")
	require.True(t, ok)
	assert.JSONEq(t, `{"stack":"dev","arguments":{}}`, value)
}

func TestWorkflowControlRenderUsesTheWorkflowRenderer(t *testing.T) {
	ResetStepExecutorState()
	t.Cleanup(ResetStepExecutorState)
	stepExecutorState = stepPkg.NewStepExecutor()
	vars := stepExecutorState.Variables()
	vars.SetTemplateRenderer(func(name, input string, data any) (string, error) {
		return ProcessTmpl(&schema.AtmosConfiguration{}, name, input, data, false)
	})
	vars.SetTemplatePasses(3)

	got, err := workflowControlRender("child", `{{ "x" | upper }}-{{ .matrix.word }}`, map[string]any{"matrix": map[string]string{"word": "y"}})

	require.NoError(t, err)
	assert.Equal(t, "X-y", got, "a Sprig function must work in a parallel child, as in a sequential step")
}

func TestWorkflowControlStep_ChildrenGetConfiguredCIReporter(t *testing.T) {
	for _, stepType := range []string{schema.TaskTypeParallel, schema.TaskTypeMatrix} {
		t.Run(stepType, func(t *testing.T) {
			ResetStepExecutorState()
			t.Cleanup(ResetStepExecutorState)
			h := newCIHostHarness(t)
			showSummary := false
			parent := &schema.WorkflowStep{
				Name: "fanout", Type: stepType, Output: "none",
				ParallelOutput: &schema.ParallelOutputConfig{ShowSummary: &showSummary},
				Steps:          []schema.WorkflowStep{h.child("child")},
			}
			if stepType == schema.TaskTypeMatrix {
				parent.Matrix = map[string][]string{"env": {"dev"}}
			}
			control := &workflowControlContext{atmosConfig: h.config, workflowDefinition: &schema.WorkflowDefinition{}}

			require.NoError(t, executeWorkflowControlStep(t.Context(), control, parent))

			h.assertConfigured(t, "child")
		})
	}
}
