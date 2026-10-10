package workflow

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	stepPkg "github.com/cloudposse/atmos/pkg/runner/step"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestControlEmbeddedStarlark(t *testing.T) {
	t.Parallel()
	executor := &ControlCommandExecutor{BaseEnv: []string{"AMBIENT=hidden"}}
	result, err := executor.Execute(context.Background(), &ControlChild{Step: schema.WorkflowStep{
		Name: "embedded", Type: schema.TaskTypeScript, Interpreter: "starlark",
		Script: `
def branch():
    return "ok"
print(json.encode(steps.parallel(functions=[branch, branch])))
print("AMBIENT" in env)
output = {"result": 42}
`,
	}}, ControlChildOutput{Mode: ControlOutputNone})
	require.NoError(t, err)
	assert.Equal(t, "[\"ok\",\"ok\"]\nFalse\n", result.Stdout)
	require.NotNil(t, result.Value)
	assert.JSONEq(t, `{"result":42}`, *result.Value)
}

func TestControlStarlarkStructuredOutputThroughRegistry(t *testing.T) {
	initControlTestIO(t)
	vars := stepPkg.NewVariables()
	handler, ok := stepPkg.Get(schema.TaskTypeParallel)
	require.True(t, ok)
	_, err := handler.Execute(context.Background(), &schema.WorkflowStep{
		Name: "fanout", Type: schema.TaskTypeParallel,
		Steps: []schema.WorkflowStep{{Name: "calculate", Type: schema.TaskTypeScript, Interpreter: "starlark", Script: `print("log")
output = steps.parallel(functions=[lambda: 1, lambda: 2])`}},
	}, vars)
	require.NoError(t, err)
	value, ok := vars.GetValue("calculate")
	require.True(t, ok)
	assert.JSONEq(t, `[1,2]`, value)
}

func TestControlStarlarkRejectsContainer(t *testing.T) {
	t.Parallel()
	child := &ControlChild{Step: schema.WorkflowStep{Type: schema.TaskTypeScript, Interpreter: "starlark", Script: `fail("not run")`}}
	executor := &ControlCommandExecutor{WorkflowDefinition: &schema.WorkflowDefinition{Container: &schema.WorkflowContainer{Image: "example"}}}
	_, err := executor.Execute(context.Background(), child, ControlChildOutput{})
	require.ErrorIs(t, err, errUtils.ErrStarlark)
	require.ErrorContains(t, err, "container: false")
	params := &ContainerStepParams{WorkflowDef: executor.WorkflowDefinition, Step: &child.Step}
	require.ErrorContains(t, RunStepContainerOverride(context.Background(), params), "container: false")
	child.Step.Container = &schema.WorkflowContainer{Enabled: new(false)}
	child.Step.Script = `print("host")`
	result, err := executor.Execute(context.Background(), child, ControlChildOutput{})
	require.NoError(t, err)
	assert.Equal(t, "host\n", result.Stdout)
}

func TestTestRunnerEmbeddedStarlark(t *testing.T) {
	t.Setenv("STARLARK_TEST_AMBIENT", "must-not-leak")
	result, output, err := runTestYAML(t, `type: test
name: checks
steps:
 - name: embedded
   type: script
   interpreter: starlark
   env: {INPUT: declared}
   script: |
     def branch():
         if "STARLARK_TEST_AMBIENT" in env:
             fail("ambient environment leaked")
         return env["INPUT"]
     output = steps.parallel(functions=[branch, branch])
`)
	require.NoError(t, err, output)
	assert.Equal(t, 1, result.Metadata["passed"])
}

func TestControlStarlarkRetainsParsedInputs(t *testing.T) {
	initControlTestIO(t)
	for _, kind := range []string{schema.TaskTypeParallel, schema.TaskTypeMatrix} {
		t.Run(kind, func(t *testing.T) {
			vars := stepPkg.NewVariables()
			vars.SetTemplateData(map[string]any{
				"Flags":     map[string]any{"enabled": true, "literal": "{{ .Env.NOT_DEFINED }}"},
				"Arguments": map[string]string{"service": "api"},
			})
			parent := &schema.WorkflowStep{
				Name: "group", Type: kind, Output: ControlOutputNone,
				Steps: []schema.WorkflowStep{{
					Name: "inspect", Type: schema.TaskTypeScript, Interpreter: "starlark", Script: `
if ctx.flags["enabled"] != True or ctx.arguments["service"] != "api":
    fail("parsed inputs were not inherited")
output = ctx.flags["literal"]`,
				}},
			}
			if kind == schema.TaskTypeMatrix {
				parent.Matrix = map[string][]string{"region": {"east", "west"}}
			}
			handler, ok := stepPkg.Get(kind)
			require.True(t, ok)
			_, err := handler.Execute(t.Context(), parent, vars)
			require.NoError(t, err)
			expected := 1
			if kind == schema.TaskTypeMatrix {
				expected = 2
			}
			require.Len(t, vars.Steps, expected)
			for _, result := range vars.Steps {
				assert.Equal(t, "{{ .Env.NOT_DEFINED }}", result.Value)
			}
		})
	}
}

func TestTestRunnerStarlarkRetainsParsedInputs(t *testing.T) {
	initControlTestIO(t)
	vars := stepPkg.NewVariables()
	vars.SetTemplateData(map[string]any{
		"Flags":     map[string]any{"enabled": true},
		"Arguments": map[string]string{"service": "api"},
	})
	check := schema.WorkflowStep{Name: "inspect", Type: schema.TaskTypeScript, Interpreter: "starlark", Script: `
if ctx.flags["enabled"] != True or ctx.arguments["service"] != "api":
    fail("parsed inputs were not inherited")
output = ctx.arguments["service"]`}
	parent := &schema.WorkflowStep{Name: "suite", Type: schema.TaskTypeTest, Steps: []schema.WorkflowStep{
		check,
		{Name: "parallel", Type: schema.TaskTypeParallel, Steps: []schema.WorkflowStep{check}},
		{Name: "matrix", Type: schema.TaskTypeMatrix, Matrix: map[string][]string{"region": {"east", "west"}}, Steps: []schema.WorkflowStep{check}},
	}}
	handler, ok := stepPkg.Get(schema.TaskTypeTest)
	require.True(t, ok)
	result, err := handler.Execute(t.Context(), parent, vars)
	require.NoError(t, err)
	assert.Equal(t, 4, result.Metadata["passed"])
	assert.Equal(t, true, vars.ScriptFlags()["enabled"])
	assert.Equal(t, "api", vars.ScriptArguments()["service"])
	assert.NotContains(t, vars.TemplateData(), "matrix")
}
