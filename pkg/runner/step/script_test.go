package step

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
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
		path := filepath.Join(dir, "sops-secrets.cast")
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

func TestScriptHandlerTemplateResolutionErrorsUseSentinel(t *testing.T) {
	handler := &ScriptHandler{}
	vars := NewVariables()

	_, err := handler.resolveInvocation(&schema.WorkflowStep{
		Name:             "bad-workdir",
		Interpreter:      "python3",
		Script:           "print('ok')",
		WorkingDirectory: "{{ range .steps }}",
	}, vars)
	require.ErrorIs(t, err, errUtils.ErrTemplateEvaluation)

	_, err = handler.resolveEnv(&schema.WorkflowStep{
		Name: "bad-env",
		Env: map[string]string{
			"BROKEN": "{{ range .steps }}",
		},
	}, vars)
	require.ErrorIs(t, err, errUtils.ErrTemplateEvaluation)
}

func TestScriptHandlerResolveInvocationInterpreterTemplateError(t *testing.T) {
	handler := &ScriptHandler{}
	vars := NewVariables()

	_, err := handler.resolveInvocation(&schema.WorkflowStep{
		Name:        "bad-interpreter",
		Interpreter: "{{ range .steps }}",
		Script:      "print('ok')",
	}, vars)
	require.ErrorIs(t, err, errUtils.ErrTemplateEvaluation)
	stepName, ok := errUtils.GetContext(err, "step")
	require.True(t, ok)
	assert.Equal(t, "bad-interpreter", stepName)
	field, ok := errUtils.GetContext(err, "field")
	require.True(t, ok)
	assert.Equal(t, "interpreter", field)
}

func TestScriptHandlerResolveInvocationScriptTemplateError(t *testing.T) {
	handler := &ScriptHandler{}
	vars := NewVariables()

	_, err := handler.resolveInvocation(&schema.WorkflowStep{
		Name:        "bad-script",
		Interpreter: "python3",
		Script:      "{{ range .steps }}",
	}, vars)
	require.ErrorIs(t, err, errUtils.ErrTemplateEvaluation)
	stepName, ok := errUtils.GetContext(err, "step")
	require.True(t, ok)
	assert.Equal(t, "bad-script", stepName)
	field, ok := errUtils.GetContext(err, "field")
	require.True(t, ok)
	assert.Equal(t, "script", field)
}

func TestScriptHandlerResolveInvocationSucceedsWithoutWorkingDirectory(t *testing.T) {
	handler := &ScriptHandler{}
	vars := NewVariables()

	invocation, err := handler.resolveInvocation(&schema.WorkflowStep{
		Name:        "no-workdir",
		Interpreter: "python3",
		Script:      "print('ok')",
	}, vars)
	require.NoError(t, err)
	assert.Equal(t, "python3", invocation.interpreter)
	assert.Equal(t, "print('ok')", invocation.script)
	assert.Empty(t, invocation.workDir)
}

func TestScriptHandlerResolveEnvDefaultsToOSEnvironWhenVariablesEnvEmpty(t *testing.T) {
	t.Setenv("SCRIPT_RESOLVEENV_MARKER", "present")

	handler := &ScriptHandler{}
	// A zero-value Variables has a nil Env map, so EnvSlice() returns an
	// empty slice and resolveEnv must fall back to os.Environ().
	vars := &Variables{}

	env, err := handler.resolveEnv(&schema.WorkflowStep{Name: "no-env"}, vars)
	require.NoError(t, err)
	require.NotEmpty(t, env)

	found := false
	for _, entry := range env {
		if entry == "SCRIPT_RESOLVEENV_MARKER=present" {
			found = true
			break
		}
	}
	assert.True(t, found, "expected os.Environ() fallback to include SCRIPT_RESOLVEENV_MARKER=present")
}

func TestScriptHandlerExecutePropagatesInvocationResolutionError(t *testing.T) {
	handler, ok := Get("script")
	require.True(t, ok)

	step := &schema.WorkflowStep{
		Name:        "bad-interpreter-execute",
		Type:        schema.TaskTypeScript,
		Interpreter: "{{ range .steps }}",
		Script:      "print('unreachable')",
	}

	result, err := handler.Execute(context.Background(), step, NewVariables())
	require.Error(t, err)
	require.Nil(t, result)
	assert.ErrorIs(t, err, errUtils.ErrTemplateEvaluation)
}

func TestScriptHandlerExecutePropagatesEnvResolutionError(t *testing.T) {
	handler, ok := Get("script")
	require.True(t, ok)

	step := &schema.WorkflowStep{
		Name:        "bad-env-execute",
		Type:        schema.TaskTypeScript,
		Interpreter: "python3",
		Script:      "print('unreachable')",
		Env: map[string]string{
			"BROKEN": "{{ range .steps }}",
		},
	}

	result, err := handler.Execute(context.Background(), step, NewVariables())
	require.Error(t, err)
	require.Nil(t, result)
	assert.ErrorIs(t, err, errUtils.ErrTemplateEvaluation)
}

func TestScriptHandlerExecuteDefaultsOutputModeWhenUnset(t *testing.T) {
	initShellTestIO(t)
	requirePython3(t)

	handler, ok := Get("script")
	require.True(t, ok)

	// No Output field set and no workflow passed, so execute() must fall
	// back to OutputModeLog via `if mode == "" { mode = OutputModeLog }`.
	step := &schema.WorkflowStep{
		Name:        "test_default_mode",
		Type:        schema.TaskTypeScript,
		Interpreter: "python3",
		Script:      "print('default_mode_output')\n",
	}

	result, err := handler.Execute(context.Background(), step, NewVariables())
	require.NoError(t, err)
	assert.Equal(t, 0, result.Metadata[exitCodeMetadata])
}
