package workflow

import (
	"context"
	"strings"
	"sync"
	"testing"
	"text/template"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	stepPkg "github.com/cloudposse/atmos/pkg/runner/step"
	"github.com/cloudposse/atmos/pkg/schema"
)

// upperRender renders like the parent context's renderer: it knows an `upper` function, which
// plain text/template does not.
func upperRender(name, input string, data map[string]any) (string, error) {
	tmpl, err := template.New(name).Funcs(template.FuncMap{"upper": strings.ToUpper}).Parse(input)
	if err != nil {
		return "", err
	}
	var out strings.Builder
	if err := tmpl.Execute(&out, data); err != nil {
		return "", err
	}
	return out.String(), nil
}

func TestControlTemplatesResolve(t *testing.T) {
	matrix := map[string]string{"word": "hello"}

	t.Run("without a renderer children render with plain text/template", func(t *testing.T) {
		got, err := (*controlTemplates)(nil).resolve(`{{ .matrix.word }}`, "child", matrix)
		require.NoError(t, err)
		assert.Equal(t, "hello", got)

		_, err = (*controlTemplates)(nil).resolve(`{{ .matrix.word | upper }}`, "child", matrix)
		require.Error(t, err, "plain text/template has no upper function")
	})

	t.Run("with a renderer children see the parent's template functions", func(t *testing.T) {
		templates := &controlTemplates{render: upperRender}
		got, err := templates.resolve(`{{ .matrix.word | upper }}`, "child", matrix)
		require.NoError(t, err)
		assert.Equal(t, "HELLO", got)
	})

	t.Run("matrix, step name, and parent data are all in the render data", func(t *testing.T) {
		templates := &controlTemplates{
			render: upperRender,
			data: func(stepName string, _ map[string]string) map[string]any {
				return map[string]any{"flags": map[string]string{"env": "prod"}}
			},
		}
		got, err := templates.resolve(`{{ .step.name }}/{{ .matrix.word }}/{{ .flags.env }}`, "child", matrix)
		require.NoError(t, err)
		assert.Equal(t, "child/hello/prod", got)
	})

	t.Run("input without delimiters is returned untouched", func(t *testing.T) {
		got, err := (&controlTemplates{render: func(string, string, map[string]any) (string, error) {
			t.Fatal("the renderer must not run for a plain string")
			return "", nil
		}}).resolve("plain", "child", matrix)
		require.NoError(t, err)
		assert.Equal(t, "plain", got)
	})
}

func TestResolveControlStepUsesTheParentRenderer(t *testing.T) {
	step := &schema.WorkflowStep{
		Name:    "child",
		Type:    schema.TaskTypeScript,
		Script:  `print("{{ "x" | upper }}")`,
		Command: `echo {{ .matrix.word | upper }}`,
		Env:     map[string]string{"SHOUT": `{{ "quiet" | upper }}`, "KEEP": `{{ "lit" | upper }}`},

		LiteralFields: []string{"env.KEEP"},
	}

	resolved, err := resolveControlStep(step, map[string]string{"word": "hi"}, &controlTemplates{render: upperRender})

	require.NoError(t, err)
	assert.Equal(t, `print("X")`, resolved.Script)
	assert.Equal(t, "echo HI", resolved.Command)
	assert.Equal(t, "QUIET", resolved.Env["SHOUT"])
	assert.Equal(t, `{{ "lit" | upper }}`, resolved.Env["KEEP"], "a literal env value is not rendered")
}

func TestResolveControlStepErrorNamesStepAndField(t *testing.T) {
	step := &schema.WorkflowStep{Name: "child", Script: "print('{{ nope }}')", ScriptSource: "/proj/s.star"}

	_, err := resolveControlStep(step, nil, nil)

	require.ErrorIs(t, err, errUtils.ErrTemplateEvaluation)
	assert.Contains(t, err.Error(), `step "child" field script (/proj/s.star)`)

	_, err = resolveControlStep(&schema.WorkflowStep{Name: "child", Env: map[string]string{"A": "{{ nope }}"}}, nil, nil)
	require.ErrorIs(t, err, errUtils.ErrTemplateEvaluation)
	assert.Contains(t, err.Error(), `step "child" field env.A`)
}

func TestExecuteControlStepRendersChildrenWithTheParentContext(t *testing.T) {
	initControlTestIO(t)
	showSummary := false
	parent := &schema.WorkflowStep{
		Name:           "group",
		Type:           schema.TaskTypeParallel,
		ParallelOutput: &schema.ParallelOutputConfig{Mode: ControlOutputNone, ShowSummary: &showSummary},
		Steps: []schema.WorkflowStep{
			{Name: "a", Type: schema.TaskTypeScript, Interpreter: "starlark", Script: `print("{{ "a" | upper }}")`},
			{Name: "b", Type: schema.TaskTypeShell, Command: `echo {{ "b" | upper }}`},
		},
	}
	vars := stepPkg.NewVariables()
	vars.SetTemplateRenderer(func(name, input string, data any) (string, error) {
		return upperRender(name, input, data.(map[string]any))
	})

	var mu sync.Mutex
	rendered := map[string]string{}
	err := ExecuteControlStep(context.Background(), parent, func(_ context.Context, child *ControlChild, _ ControlChildOutput) (*ControlChildResult, error) {
		mu.Lock()
		defer mu.Unlock()
		rendered[child.Step.Name] = child.Step.Script + child.Step.Command
		return &ControlChildResult{}, nil
	}, ControlExecutionOptions{RenderTemplate: vars.ResolveWithData})

	require.NoError(t, err)
	assert.Equal(t, `print("A")`, rendered["a"])
	assert.Equal(t, "echo B", rendered["b"])
}
