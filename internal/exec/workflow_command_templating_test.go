package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	stepPkg "github.com/cloudposse/atmos/pkg/runner/step"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestWorkflowCommandSupportsTemplating(t *testing.T) {
	tests := []struct {
		commandType string
		want        bool
	}{
		{"shell", true},
		{"atmos", true},
		{schema.TaskTypeExec, true},
		{"choose", false},
		{"parallel", false},
		{"container", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.commandType, func(t *testing.T) {
			assert.Equal(t, tt.want, workflowCommandSupportsTemplating(tt.commandType))
		})
	}
}

func TestResolveWorkflowStepCommand(t *testing.T) {
	ResetStepExecutorState()
	t.Cleanup(ResetStepExecutorState)

	t.Run("no executor state returns the command unchanged", func(t *testing.T) {
		ResetStepExecutorState()
		got, err := resolveWorkflowStepCommand("echo hi", nil)
		require.NoError(t, err)
		assert.Equal(t, "echo hi", got)
	})

	t.Run("resolves prior step results and step env overlay", func(t *testing.T) {
		initStepExecutorWithStages(&schema.WorkflowDefinition{})
		stepExecutorState.Variables().Set("account", stepPkg.NewStepResult("prod"))

		got, err := resolveWorkflowStepCommand(
			`deploy {{ .steps.account.value }} region={{ .env.ATMOS_TEST_OVERLAY_REGION }}`,
			[]string{"ATMOS_TEST_OVERLAY_REGION=euc1"},
		)
		require.NoError(t, err)
		assert.Equal(t, "deploy prod region=euc1", got)
	})

	t.Run("step env overlay does not leak into the shared executor env", func(t *testing.T) {
		initStepExecutorWithStages(&schema.WorkflowDefinition{})

		_, err := resolveWorkflowStepCommand(`x={{ .env.ATMOS_TEST_LEAK_KEY }}`, []string{"ATMOS_TEST_LEAK_KEY=leaked"})
		require.NoError(t, err)

		_, present := stepExecutorState.Variables().Env["ATMOS_TEST_LEAK_KEY"]
		assert.False(t, present, "per-step env overlay leaked into the shared executor env")
	})

	t.Run("plain command without templates is unchanged", func(t *testing.T) {
		initStepExecutorWithStages(&schema.WorkflowDefinition{})
		got, err := resolveWorkflowStepCommand("terraform plan", nil)
		require.NoError(t, err)
		assert.Equal(t, "terraform plan", got)
	})

	t.Run("engine parity: Sprig/Gomplate functions resolve like custom commands", func(t *testing.T) {
		initStepExecutorWithStages(&schema.WorkflowDefinition{})
		// Mirror the renderer alignment done in ExecuteWorkflow.
		stepExecutorState.Variables().SetTemplateRenderer(func(name, input string, data any) (string, error) {
			return ProcessTmpl(&schema.AtmosConfiguration{}, name, input, data, false)
		})
		stepExecutorState.Variables().Set("account", stepPkg.NewStepResult("prod"))

		got, err := resolveWorkflowStepCommand(`deploy {{ .steps.account.value | upper }}`, nil)
		require.NoError(t, err)
		assert.Equal(t, "deploy PROD", got)
	})

	t.Run("invalid template in command returns an error", func(t *testing.T) {
		initStepExecutorWithStages(&schema.WorkflowDefinition{})
		_, err := resolveWorkflowStepCommand(`{{ .steps.invalid.value`, nil)
		require.Error(t, err)
	})
}

func TestResolveWorkflowStepEnvs(t *testing.T) {
	ResetStepExecutorState()
	t.Cleanup(ResetStepExecutorState)

	t.Run("resolves .steps in workflow and step env values", func(t *testing.T) {
		initStepExecutorWithStages(&schema.WorkflowDefinition{})
		stepExecutorState.Variables().Set("select", stepPkg.NewStepResult("vpc"))

		workflowEnv := map[string]string{"WF": "{{ .steps.select.value }}-wf"}
		stepEnv := map[string]string{"COMPONENT": "{{ .steps.select.value }}"}

		resolvedWorkflow, resolvedStep, err := resolveWorkflowStepEnvs(workflowEnv, stepEnv, nil)
		require.NoError(t, err)
		assert.Equal(t, "vpc-wf", resolvedWorkflow["WF"])
		assert.Equal(t, "vpc", resolvedStep["COMPONENT"])

		// Inputs are not mutated.
		assert.Equal(t, "{{ .steps.select.value }}", stepEnv["COMPONENT"])
	})

	t.Run("empty maps are returned unchanged", func(t *testing.T) {
		initStepExecutorWithStages(&schema.WorkflowDefinition{})
		wf, step, err := resolveWorkflowStepEnvs(nil, nil, nil)
		require.NoError(t, err)
		assert.Empty(t, wf)
		assert.Empty(t, step)
	})

	t.Run("base env is available as {{ .env.* }} in env values", func(t *testing.T) {
		initStepExecutorWithStages(&schema.WorkflowDefinition{})
		stepEnv := map[string]string{"DERIVED": "{{ .env.ATMOS_TEST_BASE }}-x"}

		_, resolvedStep, err := resolveWorkflowStepEnvs(nil, stepEnv, []string{"ATMOS_TEST_BASE=base"})
		require.NoError(t, err)
		assert.Equal(t, "base-x", resolvedStep["DERIVED"])
	})
}
