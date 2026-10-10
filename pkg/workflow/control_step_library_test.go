package workflow

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestControlScriptStepLibrary(t *testing.T) {
	executor := &ControlCommandExecutor{}
	result, err := executor.Execute(t.Context(), &ControlChild{Step: schema.WorkflowStep{
		Name: "script", Type: "script", Interpreter: "starlark", Script: `output = steps.join(options=["api","worker"],separator=",").value`,
	}}, ControlChildOutput{})
	require.NoError(t, err)
	require.NotNil(t, result.Value)
	assert.Equal(t, "api,worker", *result.Value)
}

func TestControlScriptRejectsPrompts(t *testing.T) {
	_, err := (&ControlCommandExecutor{}).Execute(t.Context(), &ControlChild{Step: schema.WorkflowStep{
		Name: "script", Type: "script", Interpreter: "starlark", Script: `steps.input(prompt="Unsafe",default="no")`,
	}}, ControlChildOutput{})
	require.ErrorContains(t, err, "exclusive terminal")
}
