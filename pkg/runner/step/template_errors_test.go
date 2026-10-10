package step

import (
	"context"
	"errors"
	"strings"
	"testing"
	"text/template"

	cerrors "github.com/cockroachdb/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestTemplateFieldError(t *testing.T) {
	cause := errors.New("function \"a\" not defined")

	tests := []struct {
		name        string
		step        schema.WorkflowStep
		field       string
		wantMessage []string
		wantHint    bool
	}{
		{
			name:        "names the step and the field",
			step:        schema.WorkflowStep{Name: "fmt", Script: "print('{{ a }}')"},
			field:       "script",
			wantMessage: []string{`step "fmt" field script`, `function "a" not defined`},
			wantHint:    true,
		},
		{
			name:        "names the source file of an included script",
			step:        schema.WorkflowStep{Name: "inc", Script: "x = '{{ a }}'", ScriptSource: "/proj/scripts/braces.star"},
			field:       "script",
			wantMessage: []string{`step "inc" field script (/proj/scripts/braces.star)`},
			wantHint:    true,
		},
		{
			name:        "command bodies with braces get the literal hint",
			step:        schema.WorkflowStep{Name: "sh", Command: "echo {{ a }}"},
			field:       "command",
			wantMessage: []string{`step "sh" field command`},
			wantHint:    true,
		},
		{
			name:        "a script that is already literal gets no hint",
			step:        schema.WorkflowStep{Name: "lit", Script: "{{ a }}", LiteralFields: []string{"script"}},
			field:       "script",
			wantMessage: []string{`step "lit" field script`},
		},
		{
			name:        "a script without braces gets no hint",
			step:        schema.WorkflowStep{Name: "plain", Script: "x = 1"},
			field:       "script",
			wantMessage: []string{`step "plain" field script`},
		},
		{
			name:        "other fields get no hint and no source",
			step:        schema.WorkflowStep{Name: "wd", ScriptSource: "/proj/s.star", Script: "{{"},
			field:       "working_directory",
			wantMessage: []string{`step "wd" field working_directory`},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := TemplateFieldError(&tt.step, tt.field, cause)

			require.ErrorIs(t, err, errUtils.ErrTemplateEvaluation)
			require.ErrorIs(t, err, cause)
			for _, want := range tt.wantMessage {
				assert.Contains(t, err.Error(), want)
			}
			if tt.field != "script" || tt.step.ScriptSource == "" {
				assert.NotContains(t, err.Error(), "(/proj")
			}
			hints := strings.Join(cerrors.GetAllHints(err), "\n")
			if tt.wantHint {
				assert.Contains(t, hints, "!literal")
			} else {
				assert.NotContains(t, hints, "!literal")
			}
		})
	}
}

func TestScriptHandlerTemplateErrorNamesStepFieldAndSource(t *testing.T) {
	initShellTestIO(t)
	handler, ok := Get(schema.TaskTypeScript)
	require.True(t, ok)
	vars := NewVariables()
	vars.SetTemplateRenderer(func(name, input string, data any) (string, error) {
		tmpl, err := template.New(name).Parse(input)
		if err != nil {
			return "", err
		}
		var out strings.Builder
		if err := tmpl.Execute(&out, data); err != nil {
			return "", err
		}
		return out.String(), nil
	})

	_, err := handler.Execute(context.Background(), &schema.WorkflowStep{
		Name: "inc", Type: schema.TaskTypeScript, Interpreter: "starlark", Output: "none",
		Script: "print(\"{{ nope }}\")", ScriptSource: "/proj/scripts/braces.star",
	}, vars)

	require.ErrorIs(t, err, errUtils.ErrTemplateEvaluation)
	assert.Contains(t, err.Error(), `step "inc" field script (/proj/scripts/braces.star)`)
	assert.Contains(t, strings.Join(cerrors.GetAllHints(err), "\n"), "!literal")
}

func TestScriptHandlerLiteralScriptWithBracesRunsWithoutRendering(t *testing.T) {
	initShellTestIO(t)
	handler, ok := Get(schema.TaskTypeScript)
	require.True(t, ok)

	result, err := handler.Execute(context.Background(), &schema.WorkflowStep{
		Name: "lit", Type: schema.TaskTypeScript, Interpreter: "starlark", Output: "none",
		Script:        "output = \"{{ nope }}\"",
		LiteralFields: []string{"script"},
	}, NewVariables())

	require.NoError(t, err)
	assert.Equal(t, "{{ nope }}", result.Value)
}
