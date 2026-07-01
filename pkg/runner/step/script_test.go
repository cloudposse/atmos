package step

import (
	"context"
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func requirePython3(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not found on PATH")
	}
}

func TestScriptHandlerExecution(t *testing.T) {
	initShellTestIO(t)
	requirePython3(t)

	handler, ok := Get("script")
	require.True(t, ok)

	t.Run("python script receives stdin and env", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:        "test_python",
			Type:        schema.TaskTypeScript,
			Interpreter: "python3",
			Script:      "import os\nprint(os.environ['SCRIPT_TEST_VAR'])\n",
			Output:      "capture",
			Env: map[string]string{
				"SCRIPT_TEST_VAR": "custom_value",
			},
		}

		result, err := handler.Execute(context.Background(), step, NewVariables())
		require.NoError(t, err)
		assert.Contains(t, result.Value, "custom_value")
		assert.Equal(t, 0, result.Metadata[exitCodeMetadata])
	})

	t.Run("python exit code propagates", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:        "test_python_exit",
			Type:        schema.TaskTypeScript,
			Interpreter: "python3",
			Script:      "import sys\nsys.exit(7)\n",
			Output:      "capture",
		}

		result, err := handler.Execute(context.Background(), step, NewVariables())
		require.Error(t, err)
		assert.Equal(t, 7, result.Metadata[exitCodeMetadata])
	})
}

func TestScriptHandlerExecuteWithWorkflowCastValidationSnippet(t *testing.T) {
	initShellTestIO(t)
	requirePython3(t)

	handler, ok := Get("script")
	require.True(t, ok)
	scriptHandler := handler.(*ScriptHandler)

	t.Run("validates file content with pathlib", func(t *testing.T) {
		dir := t.TempDir()
		path := dir + "/sops-secrets.cast"
		require.NoError(t, os.WriteFile(path, []byte("describe (masking on)\nencrypted at rest\nOK: reveal without key failed as expected\nAll proofs passed\n"), 0o600))

		step := &schema.WorkflowStep{
			Name:        "validate",
			Type:        schema.TaskTypeScript,
			Interpreter: "python3",
			Script: `
from pathlib import Path
text = Path("sops-secrets.cast").read_text()
for needle in ["describe (masking on)", "encrypted at rest", "OK: reveal without key failed as expected", "All proofs passed"]:
    if needle not in text:
        raise SystemExit(f"missing {needle!r}")
`,
			WorkingDirectory: dir,
			Output:           "capture",
		}

		result, err := scriptHandler.ExecuteWithWorkflow(context.Background(), step, NewVariables(), &schema.WorkflowDefinition{Output: "capture"})
		require.NoError(t, err)
		assert.Equal(t, 0, result.Metadata[exitCodeMetadata])
	})
}
