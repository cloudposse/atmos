package workflow

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile-time guard for the schema field these tests set.
var _ = schema.WorkflowStep{Source: "check.py"}

// TestControlExecutorRunsScriptSourceFile proves a parallel or matrix script child that names a
// source file runs the file's text with the interpreter inferred from its extension, resolving a
// relative path against the child's working directory.
func TestControlExecutorRunsScriptSourceFile(t *testing.T) {
	basePath := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(basePath, "scripts"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(basePath, "scripts", "check.py"), []byte("print('{{ literal }}')\n"), 0o600))

	var gotProgram string
	var gotArgs []string
	var gotDir, gotStdin string
	executor := &ControlCommandExecutor{
		BasePath: basePath,
		RunCommand: func(request *ControlCommandRequest) error {
			gotProgram, gotArgs, gotDir = request.Program, request.Args, request.Dir
			stdin, err := io.ReadAll(request.Streams.Stdin)
			require.NoError(t, err)
			gotStdin = string(stdin)
			return nil
		},
	}

	_, err := executor.Execute(context.Background(), &ControlChild{Step: schema.WorkflowStep{
		Name: "check", Type: schema.TaskTypeScript, Source: filepath.Join("scripts", "check.py"), WorkingDirectory: ".",
	}}, ControlChildOutput{Mode: ControlOutputNone})

	require.NoError(t, err)
	assert.Equal(t, "python3", gotProgram)
	assert.Equal(t, []string{"-"}, gotArgs)
	assert.Equal(t, filepath.Join(basePath, "."), gotDir)
	assert.Equal(t, "print('{{ literal }}')\n", gotStdin)
}

func TestControlExecutorScriptSourceErrors(t *testing.T) {
	tests := []struct {
		name string
		step schema.WorkflowStep
		err  error
	}{
		{name: "missing file", step: schema.WorkflowStep{Source: "gone.py"}, err: errUtils.ErrScriptSourceRead},
		{name: "source and script", step: schema.WorkflowStep{Source: "a.py", Script: "x"}, err: errUtils.ErrScriptSourceConflict},
		{name: "remote", step: schema.WorkflowStep{Source: "https://example.com/a.py"}, err: errUtils.ErrScriptSourceRemote},
		{name: "not a string", step: schema.WorkflowStep{Source: map[string]any{"uri": "a.py"}}, err: errUtils.ErrScriptSourceInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ran := false
			executor := &ControlCommandExecutor{
				BasePath:   t.TempDir(),
				RunCommand: func(*ControlCommandRequest) error { ran = true; return nil },
			}
			step := tt.step
			step.Name, step.Type = "child", schema.TaskTypeScript

			_, err := executor.Execute(context.Background(), &ControlChild{Step: step}, ControlChildOutput{Mode: ControlOutputNone})

			require.ErrorIs(t, err, tt.err)
			assert.False(t, ran, "nothing runs when the script cannot be resolved")
		})
	}
}

func TestResolveControlStepRendersScriptSource(t *testing.T) {
	step := &schema.WorkflowStep{Name: "child", Type: schema.TaskTypeScript, Source: "scripts/{{ .matrix.name }}.star"}

	resolved, err := resolveControlStep(step, map[string]string{"name": "check"}, nil)

	require.NoError(t, err)
	assert.Equal(t, "scripts/check.star", resolved.Source)
	assert.Equal(t, "scripts/{{ .matrix.name }}.star", step.Source, "the input step is never mutated")
}
