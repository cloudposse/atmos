package workflow

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/telemetry"
)

func TestBuildConditionContext(t *testing.T) {
	tests := []struct {
		name             string
		workflow         string
		workflowDef      *schema.WorkflowDefinition
		step             *schema.WorkflowStep
		commandLineStack string
		baseEnv          map[string]string
		wantStack        string
		wantStep         string
		wantEnv          map[string]string
	}{
		{
			name:     "all defaults",
			workflow: "",
		},
		{
			name:        "workflow definition supplies stack and env when no baseEnv",
			workflow:    "deploy",
			workflowDef: &schema.WorkflowDefinition{Stack: "wf-stack", Env: map[string]string{"A": "1"}},
			wantStack:   "wf-stack",
			wantEnv:     map[string]string{"A": "1"},
		},
		{
			name:        "baseEnv takes precedence over workflow definition env",
			workflowDef: &schema.WorkflowDefinition{Stack: "wf-stack", Env: map[string]string{"A": "1"}},
			baseEnv:     map[string]string{"B": "2"},
			wantStack:   "wf-stack",
			wantEnv:     map[string]string{"B": "2"},
		},
		{
			name:        "step stack overrides workflow definition stack",
			workflowDef: &schema.WorkflowDefinition{Stack: "wf-stack"},
			step:        &schema.WorkflowStep{Name: "deploy-step", Stack: "step-stack"},
			wantStack:   "step-stack",
			wantStep:    "deploy-step",
		},
		{
			name:      "step name is reported with no stack override",
			step:      &schema.WorkflowStep{Name: "no-stack-step"},
			wantStack: "",
			wantStep:  "no-stack-step",
		},
		{
			name:             "command line stack overrides workflow and step stack",
			workflowDef:      &schema.WorkflowDefinition{Stack: "wf-stack"},
			step:             &schema.WorkflowStep{Stack: "step-stack"},
			commandLineStack: "cli-stack",
			wantStack:        "cli-stack",
		},
		{
			name:    "step env merges over base env",
			baseEnv: map[string]string{"A": "1", "B": "2"},
			step:    &schema.WorkflowStep{Env: map[string]string{"B": "9", "C": "3"}},
			wantEnv: map[string]string{"A": "1", "B": "9", "C": "3"},
		},
		{
			name:    "empty step env leaves base env untouched",
			baseEnv: map[string]string{"A": "1"},
			step:    &schema.WorkflowStep{},
			wantEnv: map[string]string{"A": "1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := BuildConditionContext(tt.workflow, tt.workflowDef, tt.step, tt.commandLineStack, tt.baseEnv)

			assert.Equal(t, telemetry.IsCI(), ctx.CI)
			assert.Equal(t, schema.ConditionPredicateSuccess, ctx.Status)
			assert.Equal(t, tt.workflow, ctx.Workflow)
			assert.Equal(t, tt.wantStack, ctx.Stack)
			assert.Equal(t, tt.wantStep, ctx.Step)
			assert.Equal(t, tt.wantEnv, ctx.Env)
		})
	}
}

// TestBuildConditionContext_EnvIsolation confirms merging step env over base env
// never mutates either input map, in both directions: mutating the returned
// context's env must not affect the original inputs, and mutating the inputs
// after the call must not affect the already-returned context.
func TestBuildConditionContext_EnvIsolation(t *testing.T) {
	baseEnv := map[string]string{"A": "1"}
	stepEnv := map[string]string{"B": "2"}
	step := &schema.WorkflowStep{Env: stepEnv}

	ctx := BuildConditionContext("wf", nil, step, "", baseEnv)
	require.Equal(t, map[string]string{"A": "1", "B": "2"}, ctx.Env)

	// result -> src isolation: mutating the merged result must not leak into inputs.
	ctx.Env["A"] = "mutated"
	ctx.Env["C"] = "new"
	assert.Equal(t, map[string]string{"A": "1"}, baseEnv, "mutating result must not affect baseEnv")
	assert.Equal(t, map[string]string{"B": "2"}, stepEnv, "mutating result must not affect step.Env")

	// src -> result isolation: mutating inputs after the call must not affect the
	// already-returned context. baseEnv/stepEnv are still {"A":"1"}/{"B":"2"} here
	// since the previous assertions proved the first call didn't mutate them.
	ctx2 := BuildConditionContext("wf", nil, step, "", baseEnv)
	baseEnv["A"] = "changed"
	stepEnv["B"] = "changed"
	assert.Equal(t, "1", ctx2.Env["A"], "mutating baseEnv after the call must not affect the returned context")
	assert.Equal(t, "2", ctx2.Env["B"], "mutating step.Env after the call must not affect the returned context")
}
