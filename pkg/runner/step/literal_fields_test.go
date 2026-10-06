package step

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile-time sentinel: a rename of the field these tests rely on must fail the build.
var _ = schema.WorkflowStep{LiteralFields: nil}

func TestResolveStepField(t *testing.T) {
	vars := NewVariables()
	literalStep := &schema.WorkflowStep{LiteralFields: []string{"script"}}

	tests := []struct {
		name  string
		step  *schema.WorkflowStep
		field string
		value string
		want  string
	}{
		{"literal field is kept", literalStep, "script", "{{ x }}", "{{ x }}"},
		{"other field still renders", literalStep, "command", `{{ "ok" }}`, "ok"},
		{"nil step renders", nil, "script", `{{ "ok" }}`, "ok"},
		{"empty value stays empty", literalStep, "script", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := vars.ResolveStepField(tt.step, tt.field, tt.value)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}

	t.Run("rendering a broken template still fails", func(t *testing.T) {
		_, err := vars.ResolveStepField(literalStep, "command", "{{ nope")
		require.Error(t, err)
	})
}

func TestResolveStepEnvMap(t *testing.T) {
	vars := NewVariables()
	step := &schema.WorkflowStep{LiteralFields: []string{"env.LIT", "script"}}
	env := map[string]string{"LIT": "{{ keep }}", "TPL": `{{ "rendered" }}`}

	got, err := vars.ResolveStepEnvMap(step, env)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"LIT": "{{ keep }}", "TPL": "rendered"}, got)
	assert.Equal(t, "{{ keep }}", env["LIT"], "the input map is not modified")

	t.Run("case-insensitive env name", func(t *testing.T) {
		got, err := vars.ResolveStepEnvMap(&schema.WorkflowStep{LiteralFields: []string{"env.lit"}}, env)
		require.NoError(t, err)
		assert.Equal(t, "{{ keep }}", got["LIT"])
	})

	t.Run("no literal fields behaves like ResolveEnvMap", func(t *testing.T) {
		got, err := vars.ResolveStepEnvMap(&schema.WorkflowStep{}, map[string]string{"A": `{{ "x" }}`})
		require.NoError(t, err)
		assert.Equal(t, map[string]string{"A": "x"}, got)
	})

	t.Run("nil map and nil step", func(t *testing.T) {
		got, err := vars.ResolveStepEnvMap(step, nil)
		require.NoError(t, err)
		assert.Nil(t, got)
		got, err = vars.ResolveStepEnvMap(nil, map[string]string{"A": "1"})
		require.NoError(t, err)
		assert.Equal(t, map[string]string{"A": "1"}, got)
	})

	t.Run("a broken non-literal value still fails", func(t *testing.T) {
		_, err := vars.ResolveStepEnvMap(step, map[string]string{"LIT": "{{ keep }}", "BAD": "{{ nope"})
		require.Error(t, err)
	})
}

func TestScriptStepLiteralScript(t *testing.T) {
	initShellTestIO(t)
	handler := &ScriptHandler{}

	t.Run("a literal script keeps braces and dollar-brace text", func(t *testing.T) {
		result, err := handler.Execute(context.Background(), &schema.WorkflowStep{
			Name: "literal", Interpreter: "starlark", Output: "none",
			Script:        `print("{{ x }} ${VAR} " + "{}".format("fmt"))`,
			LiteralFields: []string{"script"},
		}, NewVariables())
		require.NoError(t, err)
		assert.Equal(t, "{{ x }} ${VAR} fmt\n", result.Value)
	})

	t.Run("the same script without the marker is rendered and fails", func(t *testing.T) {
		_, err := handler.Execute(context.Background(), &schema.WorkflowStep{
			Name: "plain", Interpreter: "starlark", Output: "none",
			Script: `print("{{ x }}")`,
		}, NewVariables())
		require.ErrorIs(t, err, errUtils.ErrTemplateEvaluation)
	})

	t.Run("a non-literal sibling field still renders", func(t *testing.T) {
		result, err := handler.Execute(context.Background(), &schema.WorkflowStep{
			Name: "mixed", Output: "none",
			Interpreter:   `{{ "star" }}{{ "lark" }}`,
			Script:        `print("{{ x }}")`,
			LiteralFields: []string{"script"},
		}, NewVariables())
		require.NoError(t, err)
		assert.Equal(t, "{{ x }}\n", result.Value)
	})

	t.Run("literal and templated env values", func(t *testing.T) {
		result, err := handler.Execute(context.Background(), &schema.WorkflowStep{
			Name: "env", Interpreter: "starlark", Output: "none",
			Script:        `print(env["LIT"] + "|" + env["TPL"])`,
			Env:           map[string]string{"LIT": "{{ keep }}", "TPL": `{{ "rendered" }}`},
			LiteralFields: []string{"script", "env.LIT"},
		}, NewVariables())
		require.NoError(t, err)
		assert.Equal(t, "{{ keep }}|rendered\n", result.Value)
	})

	t.Run("a literal working_directory is used as written", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "{{ dir }}")
		require.NoError(t, os.MkdirAll(dir, 0o700))
		result, err := handler.Execute(context.Background(), &schema.WorkflowStep{
			Name: "dir", Interpreter: "starlark", Output: "none",
			Script:           `print("ran")`,
			WorkingDirectory: dir,
			LiteralFields:    []string{"script", "working_directory"},
		}, NewVariables())
		require.NoError(t, err)
		assert.Equal(t, "ran\n", result.Value)
	})
}

func TestShellStepLiteralCommand(t *testing.T) {
	initShellTestIO(t)
	handler, ok := Get("shell")
	require.True(t, ok)

	t.Run("a literal command echoes its braces verbatim", func(t *testing.T) {
		result, err := handler.Execute(context.Background(), &schema.WorkflowStep{
			Name: "literal", Type: "shell", Output: "none",
			Command:       `echo "{{ y }}"`,
			LiteralFields: []string{"command"},
		}, NewVariables())
		require.NoError(t, err)
		assert.Contains(t, result.Value, "{{ y }}")
	})

	t.Run("the same command without the marker is rendered and fails", func(t *testing.T) {
		_, err := handler.Execute(context.Background(), &schema.WorkflowStep{
			Name: "plain", Type: "shell", Output: "none",
			Command: `echo "{{ y }}"`,
		}, NewVariables())
		require.ErrorIs(t, err, errUtils.ErrTemplateEvaluation)
	})

	t.Run("a non-literal env value renders next to a literal command", func(t *testing.T) {
		result, err := handler.Execute(context.Background(), &schema.WorkflowStep{
			Name: "mixed", Type: "shell", Output: "none",
			Command:       `echo "{{ y }} $TPL $LIT"`,
			Env:           map[string]string{"TPL": `{{ "rendered" }}`, "LIT": "{{ keep }}"},
			LiteralFields: []string{"command", "env.LIT"},
		}, NewVariables())
		require.NoError(t, err)
		assert.Contains(t, result.Value, "{{ y }} rendered {{ keep }}")
	})

	t.Run("ExecuteWithWorkflow honors the marker too", func(t *testing.T) {
		shell, ok := handler.(*ShellHandler)
		require.True(t, ok)
		result, err := shell.ExecuteWithWorkflow(context.Background(), &schema.WorkflowStep{
			Name: "wf", Type: "shell", Output: "none",
			Command:       `echo "{{ y }}"`,
			LiteralFields: []string{"command"},
		}, NewVariables(), &schema.WorkflowDefinition{Output: "none"})
		require.NoError(t, err)
		assert.Contains(t, result.Value, "{{ y }}")
	})
}

func TestWorkingDirectoryLiteralMarker(t *testing.T) {
	const raw = "{{ .steps.dir.value }}"
	vars := NewVariables()
	vars.Set("dir", NewStepResult("rendered"))

	t.Run("spin keeps a literal working_directory as written", func(t *testing.T) {
		handler, ok := Get("spin")
		require.True(t, ok)
		opts, err := handler.(*SpinHandler).prepareExecution(context.Background(), &schema.WorkflowStep{
			Name: "s", Type: "spin", Command: "ls", WorkingDirectory: raw, LiteralFields: []string{"working_directory"},
		}, vars)

		require.NoError(t, err)
		assert.Equal(t, raw, opts.workDir)
	})

	t.Run("spin still renders an unmarked working_directory", func(t *testing.T) {
		handler, ok := Get("spin")
		require.True(t, ok)
		opts, err := handler.(*SpinHandler).prepareExecution(context.Background(), &schema.WorkflowStep{
			Name: "s", Type: "spin", Command: "ls", WorkingDirectory: raw, LiteralFields: []string{"command"},
		}, vars)

		require.NoError(t, err)
		assert.Equal(t, "rendered", opts.workDir)
	})

	t.Run("container run keeps a literal working_directory as written", func(t *testing.T) {
		got, err := resolveWorkDir(vars, &schema.WorkflowStep{
			Name: "c", Type: "container", WorkingDirectory: raw, LiteralFields: []string{"working_directory"},
		})

		require.NoError(t, err)
		assert.Equal(t, filepath.Base(got), raw, "the literal text is used, then anchored to an absolute path")
		assert.True(t, filepath.IsAbs(got))
	})

	t.Run("container run still renders an unmarked working_directory", func(t *testing.T) {
		got, err := resolveWorkDir(vars, &schema.WorkflowStep{Name: "c", Type: "container", WorkingDirectory: raw})

		require.NoError(t, err)
		assert.Equal(t, "rendered", filepath.Base(got))
	})
}
