package step

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/automation"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestAutomationSequenceEnvironmentAndIsolation(t *testing.T) {
	t.Parallel()
	vars := NewVariables()
	vars.ScriptArgs = []string{"original"}
	library := NewAutomationLibrary(vars, nil)
	vars.ScriptArgs[0] = "changed in parent"
	var stdout bytes.Buffer
	tasks := schema.Tasks{
		{Type: "env", Vars: map[string]string{"SEQUENCE_TEST": "from first step"}},
		{Type: "script", Interpreter: "starlark", Script: `print(steps.join(content="{{ .env.SEQUENCE_TEST }}").value); print(ctx.args)`},
	}
	require.NoError(t, library.RunSteps(context.Background(), tasks, &automation.StepCall{WorkingDirectory: t.TempDir(), Stdout: &stdout, Stderr: &stdout}))
	assert.Contains(t, stdout.String(), "from first step")
	assert.Contains(t, stdout.String(), `["original"]`)
	library.vars.ScriptArgs[0] = "changed in child"
	assert.Equal(t, []string{"changed in parent"}, vars.ScriptArgs)
	assert.NotContains(t, vars.Env, "SEQUENCE_TEST", "step state must remain local")
	assert.Empty(t, tasks[0].Name, "default names must not mutate configuration")
	assert.Empty(t, tasks[1].WorkingDirectory)
	require.Error(t, library.RunSteps(context.Background(), nil, nil))
}
