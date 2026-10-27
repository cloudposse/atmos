package step

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestScriptPathMayBeTemplated(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{path: "scripts/casts/init-basic.star", want: false},
		{path: `C:\scripts\run.py`, want: false},
		{path: "~/bin/tool-v1.2+x@main.sh", want: false},
		{path: "x.rb", want: false},
		{path: "{{ .flags.script }}", want: true},
		{path: "[[ .flags.script ]]", want: true},
		{path: "<< .flags.script >>", want: true},
		{path: "${{ inputs.script }}", want: true},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			assert.Equal(t, tt.want, scriptPathMayBeTemplated(tt.path))
		})
	}
}

// TestScriptHandlerValidateDefersCustomDelimitedSource guards projects that configure
// template delimiters: a source path written with them cannot be inferred until it is
// rendered, so validation must defer instead of demanding an interpreter.
func TestScriptHandlerValidateDefersCustomDelimitedSource(t *testing.T) {
	handler := &ScriptHandler{}
	for _, source := range []string{"[[ .flags.script ]]", "{{ .flags.script }}"} {
		t.Run(source, func(t *testing.T) {
			require.NoError(t, handler.Validate(&schema.WorkflowStep{Name: "s", Type: schema.TaskTypeScript, Source: source}))
		})
	}
	t.Run("plain path with an unknown extension still fails early", func(t *testing.T) {
		err := handler.Validate(&schema.WorkflowStep{Name: "s", Type: schema.TaskTypeScript, Source: "scripts/tool.rb"})
		require.ErrorIs(t, err, errUtils.ErrScriptInterpreterRequired)
	})
}

// TestMaterializeScriptSourceRendersCustomDelimiters checks that a source path is passed to the
// host's renderer even when it holds no default "{{" delimiter.
func TestMaterializeScriptSourceRendersCustomDelimiters(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "check.star")
	require.NoError(t, os.WriteFile(path, []byte(`print("ok")`), 0o600))

	var rendered []string
	render := func(value string) (string, error) {
		rendered = append(rendered, value)
		if value == "[[ .name ]].star" {
			return "check.star", nil
		}
		return value, nil
	}
	step := &schema.WorkflowStep{Name: "m", Type: schema.TaskTypeScript, Source: "[[ .name ]].star"}
	require.NoError(t, MaterializeScriptSource(step, root, render))
	assert.Equal(t, []string{"[[ .name ]].star"}, rendered)
	assert.Equal(t, path, step.ScriptSource)
	assert.Equal(t, "starlark", step.Interpreter)
	assert.Equal(t, `print("ok")`, step.Script)
}
