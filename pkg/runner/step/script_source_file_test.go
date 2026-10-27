package step

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	cockroach "github.com/cockroachdb/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/process"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile-time guards for the schema fields these tests set.
var _ = schema.WorkflowStep{Source: "x.star", ScriptSource: "x.star", Interpreter: "starlark"}

// contextOf returns the builder context key-value pairs attached to err as one string.
func contextOf(err error) string {
	var details []string
	for _, payload := range cockroach.GetAllSafeDetails(err) {
		details = append(details, payload.SafeDetails...)
	}
	return strings.Join(details, " ")
}

// writeScriptFile writes content to dir/name, creating parent directories, and returns the path.
func writeScriptFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

func TestInferScriptInterpreter(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
		ok   bool
	}{
		{name: "starlark", path: "scripts/check.star", want: "starlark", ok: true},
		{name: "python", path: "scripts/check.py", want: "python3", ok: true},
		{name: "sh", path: "scripts/check.sh", want: "sh", ok: true},
		{name: "bash", path: "scripts/check.bash", want: "bash", ok: true},
		{name: "upper case", path: "scripts/CHECK.STAR", want: "starlark", ok: true},
		{name: "mixed case", path: "scripts/check.Py", want: "python3", ok: true},
		{name: "absolute path", path: filepath.Join(string(filepath.Separator), "opt", "check.bash"), want: "bash", ok: true},
		{name: "unknown extension", path: "scripts/check.rb"},
		{name: "javascript is not inferred", path: "scripts/check.js"},
		{name: "no extension", path: "scripts/check"},
		{name: "extension only in directory", path: "scripts.star/check"},
		{name: "empty", path: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := inferScriptInterpreter(tt.path)
			assert.Equal(t, tt.ok, ok)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestScriptHandlerValidateSource(t *testing.T) {
	handler := &ScriptHandler{}
	tests := []struct {
		name    string
		step    schema.WorkflowStep
		wantErr error
	}{
		{name: "source with inferred starlark", step: schema.WorkflowStep{Source: "scripts/a.star"}},
		{name: "source with inferred python", step: schema.WorkflowStep{Source: "scripts/a.py"}},
		{name: "explicit interpreter with unknown extension", step: schema.WorkflowStep{Source: "scripts/a.rb", Interpreter: "ruby"}},
		{name: "templated interpreter with unknown extension", step: schema.WorkflowStep{Source: "scripts/a.rb", Interpreter: "{{ .x }}"}},
		{name: "templated source defers inference", step: schema.WorkflowStep{Source: "scripts/{{ .name }}"}},
		{name: "included script infers from its file", step: schema.WorkflowStep{Script: "print(1)", ScriptSource: "/p/a.star"}},
		{name: "inline script with interpreter", step: schema.WorkflowStep{Script: "print(1)", Interpreter: "python3"}},
		{name: "source and script", step: schema.WorkflowStep{Source: "a.star", Script: "print(1)"}, wantErr: errUtils.ErrScriptSourceConflict},
		{name: "neither source nor script", step: schema.WorkflowStep{Interpreter: "starlark"}, wantErr: errUtils.ErrScriptBodyRequired},
		{name: "neither source nor script is also a missing field", step: schema.WorkflowStep{Interpreter: "starlark"}, wantErr: errUtils.ErrStepFieldRequired},
		{name: "source map", step: schema.WorkflowStep{Source: map[string]any{"uri": "a.star"}}, wantErr: errUtils.ErrScriptSourceInvalid},
		{name: "source list", step: schema.WorkflowStep{Source: []any{"a.star"}}, wantErr: errUtils.ErrScriptSourceInvalid},
		{name: "source number", step: schema.WorkflowStep{Source: 1}, wantErr: errUtils.ErrScriptSourceInvalid},
		{name: "source empty", step: schema.WorkflowStep{Source: ""}, wantErr: errUtils.ErrScriptSourceInvalid},
		{name: "source blank", step: schema.WorkflowStep{Source: "   "}, wantErr: errUtils.ErrScriptSourceInvalid},
		{name: "source url", step: schema.WorkflowStep{Source: "https://example.com/a.star"}, wantErr: errUtils.ErrScriptSourceRemote},
		{name: "source go-getter git", step: schema.WorkflowStep{Source: "git::https://example.com/repo.git//a.star"}, wantErr: errUtils.ErrScriptSourceRemote},
		{name: "source go-getter s3", step: schema.WorkflowStep{Source: "s3::bucket/a.star"}, wantErr: errUtils.ErrScriptSourceRemote},
		{name: "source go-getter upper case", step: schema.WorkflowStep{Source: "GIT::repo//a.star"}, wantErr: errUtils.ErrScriptSourceRemote},
		{name: "unknown extension without interpreter", step: schema.WorkflowStep{Source: "scripts/a.rb"}, wantErr: errUtils.ErrScriptInterpreterRequired},
		{name: "no extension without interpreter", step: schema.WorkflowStep{Source: "scripts/a"}, wantErr: errUtils.ErrScriptInterpreterRequired},
		{name: "included script with unknown extension", step: schema.WorkflowStep{Script: "x", ScriptSource: "/p/a.rb"}, wantErr: errUtils.ErrScriptInterpreterRequired},
		{name: "inline script without interpreter", step: schema.WorkflowStep{Script: "print(1)"}, wantErr: errUtils.ErrStepFieldRequired},
		{
			name:    "inferred embedded interpreter under a container",
			step:    schema.WorkflowStep{Source: "a.star", Container: &schema.WorkflowContainer{Image: "example"}},
			wantErr: errUtils.ErrScript,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			step := tt.step
			step.Name, step.Type = "check", schema.TaskTypeScript
			err := handler.Validate(&step)
			if tt.wantErr == nil {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, tt.wantErr)
		})
	}
}

func TestScriptHandlerValidateSourceErrorsNameTheStep(t *testing.T) {
	err := (&ScriptHandler{}).Validate(&schema.WorkflowStep{Name: "validate", Type: schema.TaskTypeScript, Source: "a.star", Script: "x"})
	require.ErrorIs(t, err, errUtils.ErrScriptSourceConflict)
	assert.Contains(t, contextOf(err), "validate")
	assert.Contains(t, hintsOf(err), "Set either `script` (inline body) or `source` (path to a script file), not both.")
}

func TestScriptHandlerExecuteSource(t *testing.T) {
	initShellTestIO(t)
	root := t.TempDir()
	writeScriptFile(t, root, filepath.Join("scripts", "lib", "util.star"), "def greet(who):\n    return \"hello, \" + who\n")
	writeScriptFile(t, root, filepath.Join("scripts", "main.star"), "load(\"lib/util.star\", \"greet\")\noutput = greet(\"source\")\n")
	writeScriptFile(t, root, filepath.Join("scripts", "plain.star"), "output = \"plain\"\n")
	// The file is used as written, so template delimiters in it are not rendered.
	writeScriptFile(t, root, filepath.Join("scripts", "braces.star"), "output = \"{{ not a template }}\"\n")
	writeScriptFile(t, root, "other.txt", "output = \"explicit\"\n")
	workDir := t.TempDir()

	tests := []struct {
		name    string
		step    schema.WorkflowStep
		want    string
		wantErr error
	}{
		{
			name: "absolute source infers starlark and loads siblings of the file",
			step: schema.WorkflowStep{Source: filepath.Join(root, "scripts", "main.star"), WorkingDirectory: workDir},
			want: "hello, source",
		},
		{
			name: "relative source resolves against working_directory",
			step: schema.WorkflowStep{Source: filepath.Join("scripts", "main.star"), WorkingDirectory: root},
			want: "hello, source",
		},
		{
			name:    "relative source is not found in a different working_directory",
			step:    schema.WorkflowStep{Source: filepath.Join("scripts", "main.star"), WorkingDirectory: workDir},
			wantErr: errUtils.ErrScriptSourceRead,
		},
		{
			name: "explicit interpreter overrides inference",
			step: schema.WorkflowStep{Source: filepath.Join(root, "other.txt"), Interpreter: "starlark"},
			want: "explicit",
		},
		{
			name: "source file is not rendered as a template",
			step: schema.WorkflowStep{Source: filepath.Join("scripts", "braces.star"), WorkingDirectory: root},
			want: "{{ not a template }}",
		},
		{
			name: "templated source is rendered",
			step: schema.WorkflowStep{Source: "scripts/{{ printf \"plain\" }}.star", WorkingDirectory: root},
			want: "plain",
		},
		{
			name: "included script infers its interpreter from the included file",
			step: schema.WorkflowStep{Script: "output = \"included\"\n", ScriptSource: filepath.Join(root, "scripts", "plain.star")},
			want: "included",
		},
		{
			name: "explicit interpreter beats the included file extension",
			step: schema.WorkflowStep{Script: "output = 'x'", ScriptSource: filepath.Join(root, "other.py"), Interpreter: "starlark"},
			want: "x",
		},
		{
			name:    "unknown extension without interpreter",
			step:    schema.WorkflowStep{Source: filepath.Join(root, "other.txt")},
			wantErr: errUtils.ErrScriptInterpreterRequired,
		},
		{
			name:    "missing file",
			step:    schema.WorkflowStep{Source: filepath.Join(root, "scripts", "missing.star")},
			wantErr: errUtils.ErrScriptSourceRead,
		},
		{
			name:    "source and script",
			step:    schema.WorkflowStep{Source: filepath.Join(root, "scripts", "plain.star"), Script: "output = 1"},
			wantErr: errUtils.ErrScriptSourceConflict,
		},
		{
			name:    "source url",
			step:    schema.WorkflowStep{Source: "https://example.com/a.star"},
			wantErr: errUtils.ErrScriptSourceRemote,
		},
		{
			name:    "templated source that renders to a url",
			step:    schema.WorkflowStep{Source: "{{ printf \"https://example.com/a.star\" }}"},
			wantErr: errUtils.ErrScriptSourceRemote,
		},
		{
			name:    "source map",
			step:    schema.WorkflowStep{Source: map[string]any{"uri": "a.star"}},
			wantErr: errUtils.ErrScriptSourceInvalid,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			step := tt.step
			step.Name, step.Type, step.Output = "run", schema.TaskTypeScript, "none"
			result, err := (&ScriptHandler{}).Execute(context.Background(), &step, NewVariables())
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, result.Value)
		})
	}
}

func TestScriptHandlerExecuteMissingSourceNamesResolvedPath(t *testing.T) {
	initShellTestIO(t)
	root := t.TempDir()
	step := &schema.WorkflowStep{
		Name: "missing", Type: schema.TaskTypeScript, Source: filepath.Join("scripts", "gone.star"),
		WorkingDirectory: root, Output: "none",
	}

	_, err := (&ScriptHandler{}).Execute(context.Background(), step, NewVariables())

	require.ErrorIs(t, err, errUtils.ErrScriptSourceRead)
	require.ErrorIs(t, err, os.ErrNotExist)
	assert.Contains(t, contextOf(err), filepath.Join(root, "scripts", "gone.star"))
	require.NotEmpty(t, hintsOf(err))
}

// TestScriptHandlerSourceExposesAbsoluteScriptPath proves the resolved absolute path is the
// engine's source path, which anchors load() and names the file in tracebacks.
func TestScriptHandlerSourceExposesAbsoluteScriptPath(t *testing.T) {
	initShellTestIO(t)
	root := t.TempDir()
	path := writeScriptFile(t, root, filepath.Join("scripts", "where.star"), "output = ctx.script.path\n")

	result, err := (&ScriptHandler{}).Execute(context.Background(), &schema.WorkflowStep{
		Name: "where", Type: schema.TaskTypeScript, Source: filepath.Join("scripts", "where.star"),
		WorkingDirectory: root, Output: "none",
	}, NewVariables())

	require.NoError(t, err)
	assert.Equal(t, path, result.Value)
}

// TestScriptHandlerResolveInvocationExternalInterpreter checks the invocation of a non-embedded
// interpreter without running it: the file text becomes the script body, and the interpreter is
// inferred from the extension unless one is set.
func TestScriptHandlerResolveInvocationExternalInterpreter(t *testing.T) {
	root := t.TempDir()
	writeScriptFile(t, root, "check.py", "print('ok')\n")
	writeScriptFile(t, root, "check.bash", "echo ok\n")
	tests := []struct {
		name        string
		step        schema.WorkflowStep
		interpreter string
		argv        []string
	}{
		{
			name:        "python is inferred and fed on stdin",
			step:        schema.WorkflowStep{Source: "check.py", WorkingDirectory: root},
			interpreter: "python3",
			argv:        []string{"python3", "-"},
		},
		{
			name:        "bash is inferred and given the body",
			step:        schema.WorkflowStep{Source: "check.bash", WorkingDirectory: root},
			interpreter: "bash",
			argv:        []string{"bash", "-c", "echo ok\n"},
		},
		{
			name:        "explicit interpreter wins",
			step:        schema.WorkflowStep{Source: "check.py", WorkingDirectory: root, Interpreter: "python"},
			interpreter: "python",
			argv:        []string{"python", "-"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			step := tt.step
			step.Name, step.Type = "ext", schema.TaskTypeScript
			invocation, err := (&ScriptHandler{}).resolveInvocation(&step, NewVariables())
			require.NoError(t, err)
			assert.Equal(t, tt.interpreter, invocation.interpreter)
			assert.Equal(t, root, invocation.workDir)
			assert.True(t, filepath.IsAbs(invocation.sourcePath))
			argv, _ := process.ScriptInvocation(invocation.interpreter, invocation.script)
			assert.Equal(t, tt.argv, argv)
		})
	}
}

func TestMaterializeScriptSource(t *testing.T) {
	root := t.TempDir()
	path := writeScriptFile(t, root, "check.star", "output = '{{ literal }}'\n")

	t.Run("reads the file into an inline literal script", func(t *testing.T) {
		step := &schema.WorkflowStep{Name: "m", Type: schema.TaskTypeScript, Source: "check.star", LiteralFields: []string{"timeout"}}
		original := step.LiteralFields

		require.NoError(t, MaterializeScriptSource(step, root, nil))

		assert.Equal(t, "output = '{{ literal }}'\n", step.Script)
		assert.Equal(t, path, step.ScriptSource)
		assert.Equal(t, "starlark", step.Interpreter)
		assert.Nil(t, step.Source)
		assert.Equal(t, []string{"timeout", "script"}, step.LiteralFields)
		assert.Equal(t, []string{"timeout"}, original, "the caller's literal list is not modified")
	})

	t.Run("renders a templated source", func(t *testing.T) {
		step := &schema.WorkflowStep{Name: "m", Type: schema.TaskTypeScript, Source: "{{ .name }}.star"}
		require.NoError(t, MaterializeScriptSource(step, root, func(string) (string, error) { return "check.star", nil }))
		assert.Equal(t, path, step.ScriptSource)
	})

	t.Run("infers the interpreter of an included script", func(t *testing.T) {
		step := &schema.WorkflowStep{Name: "m", Type: schema.TaskTypeScript, Script: "x", ScriptSource: "/p/a.py"}
		require.NoError(t, MaterializeScriptSource(step, root, nil))
		assert.Equal(t, "python3", step.Interpreter)
	})

	t.Run("leaves other step types and inline scripts alone", func(t *testing.T) {
		other := &schema.WorkflowStep{Name: "m", Type: schema.TaskTypeShell, Source: map[string]any{"uri": "x"}}
		require.NoError(t, MaterializeScriptSource(other, root, nil))
		assert.NotNil(t, other.Source)

		inline := &schema.WorkflowStep{Name: "m", Type: schema.TaskTypeScript, Script: "x", Interpreter: "python3"}
		require.NoError(t, MaterializeScriptSource(inline, root, nil))
		assert.Equal(t, "x", inline.Script)
		assert.Empty(t, inline.LiteralFields)

		empty := &schema.WorkflowStep{Name: "m", Type: schema.TaskTypeScript, Interpreter: "python3"}
		require.NoError(t, MaterializeScriptSource(empty, root, nil), "a missing body is for the runner to judge")
		require.NoError(t, MaterializeScriptSource(nil, root, nil))
	})

	t.Run("reports invalid steps", func(t *testing.T) {
		for _, tt := range []struct {
			name string
			step schema.WorkflowStep
			err  error
		}{
			{name: "missing file", step: schema.WorkflowStep{Source: "gone.star"}, err: errUtils.ErrScriptSourceRead},
			{name: "remote", step: schema.WorkflowStep{Source: "git::x"}, err: errUtils.ErrScriptSourceRemote},
			{name: "conflict", step: schema.WorkflowStep{Source: "check.star", Script: "x"}, err: errUtils.ErrScriptSourceConflict},
			{name: "unknown extension", step: schema.WorkflowStep{Script: "x", ScriptSource: "/p/a.rb"}, err: errUtils.ErrScriptInterpreterRequired},
		} {
			t.Run(tt.name, func(t *testing.T) {
				step := tt.step
				step.Name, step.Type = "m", schema.TaskTypeScript
				require.ErrorIs(t, MaterializeScriptSource(&step, root, nil), tt.err)
			})
		}
	})
}

func TestScriptLibrarySourceDirectCall(t *testing.T) {
	root := t.TempDir()
	writeScriptFile(t, root, filepath.Join("lib", "util.star"), "def greet(who):\n    return \"hello, \" + who\n")
	script := writeScriptFile(t, root, filepath.Join("scripts", "greet.star"), "load(\"../lib/util.star\", \"greet\")\noutput = greet(\"direct\")\n")

	result, _, err := runLibrarySource(t, `output = steps.script(source = "`+filepath.ToSlash(script)+`").value`)
	require.NoError(t, err)
	assert.Equal(t, "hello, direct", result.Value)

	t.Run("script and source together are rejected", func(t *testing.T) {
		_, _, err := runLibrarySource(t, `steps.script(source = "`+filepath.ToSlash(script)+`", script = "output = 1")`)
		require.ErrorIs(t, err, errUtils.ErrScriptSourceConflict)
	})
	t.Run("a non-string source is rejected", func(t *testing.T) {
		_, _, err := runLibrarySource(t, `steps.script(source = {"uri": "x.star"})`)
		require.ErrorIs(t, err, errUtils.ErrScriptSourceInvalid)
	})
}
