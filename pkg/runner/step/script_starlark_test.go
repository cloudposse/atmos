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
