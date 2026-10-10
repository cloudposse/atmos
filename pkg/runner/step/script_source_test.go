package step

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestScriptStarlarkLoadsRelativeToItsSourceFile(t *testing.T) {
	initShellTestIO(t)
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "scripts", "lib"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(root, "scripts", "lib", "util.star"), []byte("def greet(who):\n    return \"hello, \" + who\n"), 0o600))
	main := filepath.Join(root, "scripts", "main.star")
	body := "load(\"lib/util.star\", \"greet\")\noutput = greet(\"file\")\n"
	require.NoError(t, os.WriteFile(main, []byte(body), 0o600))
	workDir := t.TempDir()

	t.Run("a script from a file loads siblings of that file", func(t *testing.T) {
		result, err := (&ScriptHandler{}).Execute(context.Background(), &schema.WorkflowStep{
			Name: "from-file", Interpreter: "starlark", Script: body, ScriptSource: main,
			WorkingDirectory: workDir, Output: "none",
		}, NewVariables())
		require.NoError(t, err)
		assert.Equal(t, "hello, file", result.Value)
	})

	t.Run("an inline script loads relative to working_directory", func(t *testing.T) {
		_, err := (&ScriptHandler{}).Execute(context.Background(), &schema.WorkflowStep{
			Name: "inline", Interpreter: "starlark", Script: body,
			WorkingDirectory: workDir, Output: "none",
		}, NewVariables())
		require.Error(t, err)
		assert.Contains(t, err.Error(), filepath.Join(workDir, "lib", "util.star"))
	})
}
