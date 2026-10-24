package step

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestScriptStarlarkEnvironmentAndFileWrites(t *testing.T) {
	initShellTestIO(t)
	dir := t.TempDir()
	vars := NewVariables()
	vars.SetEnv("INHERITED_SCRIPT_VALUE", "inherited")
	result, err := (&ScriptHandler{}).Execute(t.Context(), &schema.WorkflowStep{
		Name: "files", Type: schema.TaskTypeScript, Interpreter: "starlark", Output: "none", WorkingDirectory: dir,
		Env: map[string]string{"EXPLICIT": "step"},
		Script: `fs.write_file("generated.txt", exec.getenv("INHERITED_SCRIPT_VALUE"))
output = [fs.read_file("generated.txt"), exec.getenv("EXPLICIT"), "INHERITED_SCRIPT_VALUE" in env]`,
	}, vars)
	require.NoError(t, err)
	assert.JSONEq(t, `["inherited","step",false]`, result.Value)
	contents, err := os.ReadFile(filepath.Join(dir, "generated.txt"))
	require.NoError(t, err)
	assert.Equal(t, "inherited", string(contents))
}
