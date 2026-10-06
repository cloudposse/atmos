package step

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestScriptStarlark(t *testing.T) {
	initShellTestIO(t)
	vars := NewVariables()
	vars.SetEnv("UNDECLARED", "hidden")
	handler, ok := Get("script")
	require.True(t, ok)
	result, err := handler.Execute(context.Background(), &schema.WorkflowStep{
		Name: "embedded", Type: schema.TaskTypeScript, Interpreter: "starlark", Output: "capture",
		Env: map[string]string{"EXPLICIT": "yes"},
		Script: `
def value():
    print("branch")
    return env["EXPLICIT"]
output = {"results": steps.parallel(functions=[value, value]), "ambient": "UNDECLARED" in env}
`,
	}, vars)
	require.NoError(t, err)
	assert.JSONEq(t, `{"results":["yes","yes"],"ambient":false}`, result.Value)
	// Concurrent branches finish in any order; each line carries its task prefix.
	stdout, ok := result.Metadata["stdout"].(string)
	require.True(t, ok)
	assert.ElementsMatch(t, []string{"[value[0]] branch", "[value[1]] branch"}, strings.Split(strings.TrimSuffix(stdout, "\n"), "\n"))
	assert.Equal(t, 0, result.Metadata[exitCodeMetadata])
}

func TestScriptStarlarkPrintFallbackAndFailure(t *testing.T) {
	initShellTestIO(t)
	for _, tc := range []struct {
		source, output string
		fail           bool
	}{
		{`print("hello")`, "hello\n", false},
		{`fail("bad script")`, "", true},
	} {
		result, err := (&ScriptHandler{}).Execute(context.Background(), &schema.WorkflowStep{
			Name: "embedded", Interpreter: "starlark", Script: tc.source, Output: "capture",
		}, NewVariables())
		if tc.fail {
			require.ErrorIs(t, err, errUtils.ErrStarlark)
		} else {
			require.NoError(t, err)
		}
		assert.Equal(t, tc.output, result.Value)
	}
}

func TestScriptStarlarkDryRun(t *testing.T) {
	initShellTestIO(t)
	_, err := (&ScriptHandler{}).Execute(context.Background(), &schema.WorkflowStep{
		Interpreter: "starlark", Script: `fail("must not execute")`, DryRun: true,
	}, NewVariables())
	require.NoError(t, err)
}

func TestScriptStarlarkReadsParsedCommandInputs(t *testing.T) {
	initShellTestIO(t)
	vars := NewVariables()
	vars.SetFlag("replicas", "fallback-must-not-win")
	vars.SetTemplateData(map[string]any{
		"Flags":     map[string]any{"replicas": "3", "enabled": true, "literal": "{{ .Env.NOT_DEFINED }}"},
		"Arguments": map[string]string{"service": "api", "literal": "{{ fail \"must stay data\" }}"},
	})
	result, err := (&ScriptHandler{}).Execute(t.Context(), &schema.WorkflowStep{
		Name: "inputs", Interpreter: "starlark", Output: "capture",
		Script: `output = {
    "replicas": int(ctx.flags["replicas"]),
    "enabled": ctx.flags["enabled"],
    "literal_flag": ctx.flags["literal"],
    "service": ctx.arguments["service"],
    "literal_argument": ctx.arguments["literal"],
}`,
	}, vars)
	require.NoError(t, err)
	assert.JSONEq(t, `{"replicas":3,"enabled":true,"literal_flag":"{{ .Env.NOT_DEFINED }}","service":"api","literal_argument":"{{ fail \"must stay data\" }}"}`, result.Value)
}

func TestScriptStarlarkReadsWorkflowFlags(t *testing.T) {
	initShellTestIO(t)
	vars := NewVariables()
	vars.SetFlag("stack", "dev")
	result, err := (&ScriptHandler{}).Execute(t.Context(), &schema.WorkflowStep{
		Interpreter: "starlark", Output: "capture",
		Script: `output = {"stack": ctx.flags["stack"], "arguments": ctx.arguments}`,
	}, vars)
	require.NoError(t, err)
	assert.JSONEq(t, `{"stack":"dev","arguments":{}}`, result.Value)
}

func TestScriptStarlarkInputsSurviveClonedParallelBranches(t *testing.T) {
	initShellTestIO(t)
	vars := NewVariables()
	vars.SetTemplateData(map[string]any{
		"Flags":     map[string]any{"enabled": true},
		"Arguments": map[string]string{"service": "api"},
	})
	branch := vars.Clone()
	result, err := (&ScriptHandler{}).Execute(t.Context(), &schema.WorkflowStep{
		Interpreter: "starlark", Output: "capture",
		Script: `def inspect():
    return [ctx.flags["enabled"], ctx.arguments["service"]]
output = steps.parallel(functions=[inspect, inspect])`,
	}, branch)
	require.NoError(t, err)
	assert.JSONEq(t, `[[true,"api"],[true,"api"]]`, result.Value)
	// Exposing a snapshot must not give another host caller the backing map.
	copied := branch.ScriptFlags()
	copied["enabled"] = false
	assert.Equal(t, true, vars.ScriptFlags()["enabled"])
}

func TestScriptStarlarkLowercaseFlagTemplateRoot(t *testing.T) {
	initShellTestIO(t)
	vars := NewVariables()
	vars.SetTemplateData(map[string]any{"flags": map[string]string{"stack": "prod"}})
	result, err := (&ScriptHandler{}).Execute(t.Context(), &schema.WorkflowStep{
		Interpreter: "starlark", Output: "capture", Script: `output = ctx.flags["stack"]`,
	}, vars)
	require.NoError(t, err)
	assert.Equal(t, "prod", result.Value)
}

func TestScriptInputsWithoutVariables(t *testing.T) {
	var vars *Variables
	assert.Nil(t, vars.ScriptFlags())
	assert.Nil(t, vars.ScriptArguments())
}
