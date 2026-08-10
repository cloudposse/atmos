package exec

import (
	"errors"
	"testing"

	stepPkg "github.com/cloudposse/atmos/pkg/runner/step"
	"github.com/cloudposse/atmos/pkg/scheduler"
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
