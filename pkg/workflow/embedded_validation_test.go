package workflow

import (
	"context"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile-time sentinels so a schema field rename fails the build here.
var (
	_ = schema.WorkflowStep{Type: schema.TaskTypeScript, Interpreter: "starlark", Container: &schema.WorkflowContainer{}}
	_ = schema.WorkflowDefinition{Container: &schema.WorkflowContainer{}}
)

func TestValidateEmbeddedInterpreters(t *testing.T) {
	enabled := &schema.WorkflowContainer{}
	off := false
	disabled := &schema.WorkflowContainer{Enabled: &off}

	starlarkStep := func(name string, container *schema.WorkflowContainer) schema.WorkflowStep {
		return schema.WorkflowStep{Name: name, Type: schema.TaskTypeScript, Interpreter: "starlark", Script: "print(1)", Container: container}
	}
	shellStep := schema.WorkflowStep{Name: "shell", Type: schema.TaskTypeShell, Command: "echo hi"}

	tests := []struct {
		name     string
		def      *schema.WorkflowDefinition
		wantStep string // Empty means no error expected.
	}{
		{name: "nil definition", def: nil},
		{
			name:     "workflow container with starlark step is rejected",
			def:      &schema.WorkflowDefinition{Container: enabled, Steps: []schema.WorkflowStep{shellStep, starlarkStep("sl", nil)}},
			wantStep: "sl",
		},
		{
			name:     "step container with starlark step is rejected",
			def:      &schema.WorkflowDefinition{Steps: []schema.WorkflowStep{starlarkStep("own-container", enabled)}},
			wantStep: "own-container",
		},
		{
			name: "nested parallel child is detected",
			def: &schema.WorkflowDefinition{Container: enabled, Steps: []schema.WorkflowStep{{
				Name: "group", Type: schema.TaskTypeParallel,
				Steps: []schema.WorkflowStep{shellStep, {Name: "inner", Type: schema.TaskTypeMatrix, Steps: []schema.WorkflowStep{starlarkStep("deep-child", nil)}}},
			}}},
			wantStep: "deep-child",
		},
		{
			name: "test child is detected",
			def: &schema.WorkflowDefinition{Container: enabled, Steps: []schema.WorkflowStep{{
				Name: "checks", Type: schema.TaskTypeTest, Steps: []schema.WorkflowStep{starlarkStep("check-leaf", nil)},
			}}},
			wantStep: "check-leaf",
		},
		{
			name: "container false opt-out passes",
			def:  &schema.WorkflowDefinition{Container: enabled, Steps: []schema.WorkflowStep{starlarkStep("sl", disabled)}},
		},
		{
			name: "opt-out passes in nested child",
			def: &schema.WorkflowDefinition{Container: enabled, Steps: []schema.WorkflowStep{{
				Name: "group", Type: schema.TaskTypeParallel, Steps: []schema.WorkflowStep{starlarkStep("sl", disabled)},
			}}},
		},
		{
			name: "no container anywhere passes",
			def:  &schema.WorkflowDefinition{Steps: []schema.WorkflowStep{starlarkStep("sl", nil)}},
		},
		{
			name: "workflow container disabled passes",
			def:  &schema.WorkflowDefinition{Container: disabled, Steps: []schema.WorkflowStep{starlarkStep("sl", nil)}},
		},
		{
			name: "templated interpreter is skipped",
			def: &schema.WorkflowDefinition{Container: enabled, Steps: []schema.WorkflowStep{
				{Name: "tpl", Type: schema.TaskTypeScript, Interpreter: "{{ .vars.interp }}", Script: "x"},
			}},
		},
		{
			name: "external interpreter is not embedded",
			def: &schema.WorkflowDefinition{Container: enabled, Steps: []schema.WorkflowStep{
				{Name: "py", Type: schema.TaskTypeScript, Interpreter: "python3", Script: "print(1)"},
			}},
		},
		{
			name: "interpreter on non-script step is ignored",
			def: &schema.WorkflowDefinition{Container: enabled, Steps: []schema.WorkflowStep{
				{Name: "sh", Type: schema.TaskTypeShell, Interpreter: "starlark", Command: "echo"},
			}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateEmbeddedInterpreters("wf", tt.def)
			if tt.wantStep == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.ErrorIs(t, err, errUtils.ErrScript)
			assert.Contains(t, err.Error(), tt.wantStep)
			hints := errors.GetAllHints(err)
			require.NotEmpty(t, hints)
			assert.Contains(t, hints[len(hints)-1], "Set `container: false` on step "+tt.wantStep)
		})
	}
}

func TestValidateEmbeddedInterpreterSteps_UnnamedStepAndOwner(t *testing.T) {
	enabled := &schema.WorkflowContainer{}
	steps := []schema.WorkflowStep{
		{Type: schema.TaskTypeShell, Command: "echo"},
		{Type: schema.TaskTypeScript, Interpreter: " starlark ", Script: "print(1)", Container: enabled},
	}

	err := ValidateEmbeddedInterpreterSteps("custom command `deploy`", nil, steps)

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrScript)
	assert.Contains(t, err.Error(), "#2")
	assert.Contains(t, errors.GetAllDetails(err)[0], "custom command `deploy`")
}

func TestRenderScriptInterpreterExposesEmbeddedInterpreterToContainerChecks(t *testing.T) {
	step := schema.WorkflowStep{
		Name: "star", Type: schema.TaskTypeScript, Interpreter: `{{ printf "starlark" }}`, Script: "x",
		Container: &schema.WorkflowContainer{Image: "example"},
	}
	// The raw template hides the embedded interpreter from the container check.
	require.NoError(t, rejectEmbeddedScriptInContainer(&step))

	render := func(value string) (string, error) { return "starlark", nil }
	require.NoError(t, RenderScriptInterpreter(&step, render))
	assert.Equal(t, "starlark", step.Interpreter)

	require.ErrorIs(t, rejectEmbeddedScriptInContainer(&step), errUtils.ErrScript)
	params := &ContainerStepParams{WorkflowDef: &schema.WorkflowDefinition{}, Step: &step}
	require.ErrorIs(t, RunStepContainerOverride(context.Background(), params), errUtils.ErrScript)
}

func TestRenderScriptInterpreterLeavesOtherStepsAlone(t *testing.T) {
	failing := func(string) (string, error) { return "", errors.New("must not render") }

	plain := schema.WorkflowStep{Type: schema.TaskTypeScript, Interpreter: "bash"}
	require.NoError(t, RenderScriptInterpreter(&plain, failing))
	assert.Equal(t, "bash", plain.Interpreter)

	shell := schema.WorkflowStep{Type: schema.TaskTypeShell, Interpreter: "{{ x }}"}
	require.NoError(t, RenderScriptInterpreter(&shell, failing))
	assert.Equal(t, "{{ x }}", shell.Interpreter)

	require.NoError(t, RenderScriptInterpreter(nil, failing))
}

func TestRenderScriptInterpreterReportsTemplateFailure(t *testing.T) {
	step := schema.WorkflowStep{Name: "star", Type: schema.TaskTypeScript, Interpreter: "{{ boom }}"}
	err := RenderScriptInterpreter(&step, func(string) (string, error) { return "", errors.New("boom") })
	require.ErrorIs(t, err, errUtils.ErrTemplateEvaluation)
}

func TestRejectEmbeddedScriptInContainerNonEmbedded(t *testing.T) {
	step := schema.WorkflowStep{Type: schema.TaskTypeScript, Interpreter: "bash"}
	require.NoError(t, rejectEmbeddedScriptInContainer(&step))
	require.NoError(t, rejectEmbeddedScriptInContainer(nil))
}
