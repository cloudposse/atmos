package workflow

import (
	"context"
	"strings"
	"sync"
	"testing"
	"text/template"

	cerrors "github.com/cockroachdb/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	stepPkg "github.com/cloudposse/atmos/pkg/runner/step"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/template/delimiters"
)

// Compile-time guards for the fields these tests set.
var (
	_ = ControlExecutionOptions{Delimiters: delimiters.Default()}
	_ = schema.ParallelOutputConfig{Prefix: ""}

	squareDelims = delimiters.Delimiters{Left: "[[", Right: "]]"}
)

// squareRender renders like a host renderer configured with the [[ ]] delimiters.
func squareRender(name, input string, data map[string]any) (string, error) {
	tmpl, err := squareDelims.Apply(template.New(name).Funcs(template.FuncMap{"upper": strings.ToUpper})).Parse(input)
	if err != nil {
		return "", err
	}
	var out strings.Builder
	if err := tmpl.Execute(&out, data); err != nil {
		return "", err
	}
	return out.String(), nil
}

func TestControlTemplatesResolveWithCustomDelimiters(t *testing.T) {
	matrix := map[string]string{"word": "hello"}
	tests := []struct {
		name      string
		templates *controlTemplates
		input     string
		want      string
	}{
		{name: "plain renderer renders the custom pair", templates: &controlTemplates{delims: squareDelims}, input: "[[ .matrix.word ]] {{ .matrix.word }}", want: "hello {{ .matrix.word }}"},
		{name: "host renderer renders the custom pair", templates: &controlTemplates{delims: squareDelims, render: squareRender}, input: "[[ .matrix.word | upper ]] {{ .x }}", want: "HELLO {{ .x }}"},
		{name: "default pair is literal text for the custom detector", templates: &controlTemplates{delims: squareDelims}, input: "{{ .matrix.word }}", want: "{{ .matrix.word }}"},
		{name: "default delimiters still render the default pair", templates: &controlTemplates{delims: delimiters.Default()}, input: "{{ .matrix.word }} [[ .matrix.word ]]", want: "hello [[ .matrix.word ]]"},
		{name: "nil templates use the default pair", templates: nil, input: "{{ .matrix.word }} [[ .matrix.word ]]", want: "hello [[ .matrix.word ]]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.templates.resolve(tt.input, "child", matrix)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestResolveControlStepErrorHintNamesTheConfiguredOpening(t *testing.T) {
	step := &schema.WorkflowStep{Name: "child", Script: "print('[[ nope ]]')"}

	_, err := resolveControlStep(step, nil, &controlTemplates{delims: squareDelims})

	require.ErrorIs(t, err, errUtils.ErrTemplateEvaluation)
	assert.Contains(t, err.Error(), `step "child" field script`)
	assert.Contains(t, strings.Join(cerrors.GetAllHints(err), "\n"), "contains `[[`", "the !literal hint names the configured opening")
}

func TestExecuteControlStepRendersChildrenWithCustomDelimiters(t *testing.T) {
	initControlTestIO(t)
	showSummary := false
	parent := &schema.WorkflowStep{
		Name:           "group",
		Type:           schema.TaskTypeParallel,
		ParallelOutput: &schema.ParallelOutputConfig{Mode: ControlOutputPrefixed, ShowSummary: &showSummary},
		Steps: []schema.WorkflowStep{
			{Name: "a", Type: schema.TaskTypeShell, Command: `echo [[ "a" | upper ]] {{ "literal" }}`, Env: map[string]string{"E": "[[ .steps.first.value ]]"}},
			{Name: "b", Type: schema.TaskTypeShell, Command: `echo [[ .step.name ]]`},
		},
	}
	vars := stepPkg.NewVariables()
	vars.SetTemplateDelimiters(squareDelims)
	vars.Set("first", stepPkg.NewStepResult("first"))
	vars.SetTemplateRenderer(func(name, input string, data any) (string, error) {
		return squareRender(name, input, data.(map[string]any))
	})

	var mu sync.Mutex
	commands := map[string]string{}
	envs := map[string]string{}
	prefixes := map[string]string{}
	err := ExecuteControlStep(context.Background(), parent, func(_ context.Context, child *ControlChild, output ControlChildOutput) (*ControlChildResult, error) {
		mu.Lock()
		defer mu.Unlock()
		commands[child.Step.Name] = child.Step.Command
		envs[child.Step.Name] = child.Step.Env["E"]
		prefixes[child.Step.Name] = output.Prefix
		return &ControlChildResult{}, nil
	}, ControlExecutionOptions{
		RenderTemplate: vars.ResolveWithData,
		TemplateData:   func(string, map[string]string) map[string]any { return vars.TemplateData() },
		Delimiters:     vars.TemplateDelimiters(),
	})

	require.NoError(t, err)
	assert.Equal(t, `echo A {{ "literal" }}`, commands["a"])
	assert.Equal(t, "first", envs["a"])
	assert.Equal(t, "echo b", commands["b"])
	// The default prefix is built from the delimiters in effect, so it renders instead of staying literal.
	assert.Equal(t, "a", prefixes["a"])
	assert.Equal(t, "b", prefixes["b"])
}

func TestExecuteControlStepDefaultPrefixUsesTheEffectiveDelimiters(t *testing.T) {
	parent := &schema.WorkflowStep{
		Name:           "group",
		Type:           schema.TaskTypeParallel,
		ParallelOutput: &schema.ParallelOutputConfig{Mode: ControlOutputPrefixed, ShowSummary: boolPtr(false)},
		Steps:          []schema.WorkflowStep{{Name: "echo", Type: schema.TaskTypeShell, Command: "echo hi"}},
	}
	for _, tt := range []struct {
		name   string
		delims delimiters.Delimiters
		prefix string
		want   string
	}{
		{name: "custom delimiters, default prefix", delims: squareDelims, want: "echo"},
		{name: "default delimiters, default prefix", delims: delimiters.Default(), want: "echo"},
		{name: "custom delimiters, explicit custom prefix", delims: squareDelims, prefix: "[[ .step.name ]]-x", want: "echo-x"},
		{name: "custom delimiters, explicit default-pair prefix stays literal", delims: squareDelims, prefix: "{{ .step.name }}", want: "{{ .step.name }}"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			step := *parent
			step.ParallelOutput = &schema.ParallelOutputConfig{Mode: ControlOutputPrefixed, ShowSummary: boolPtr(false), Prefix: tt.prefix}
			var got string
			err := ExecuteControlStep(context.Background(), &step, func(_ context.Context, _ *ControlChild, output ControlChildOutput) (*ControlChildResult, error) {
				got = output.Prefix
				return &ControlChildResult{}, nil
			}, ControlExecutionOptions{Delimiters: tt.delims})
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestValidateEmbeddedInterpretersTreatsCustomTemplatedInterpretersAsTemplated(t *testing.T) {
	enabled := &schema.WorkflowContainer{}
	def := func(interpreter string) *schema.WorkflowDefinition {
		return &schema.WorkflowDefinition{Container: enabled, Steps: []schema.WorkflowStep{
			{Name: "tpl", Type: schema.TaskTypeScript, Interpreter: interpreter, Script: "x"},
		}}
	}
	tests := []struct {
		name        string
		interpreter string
		opts        []EmbeddedOption
		wantErr     bool
	}{
		{name: "custom-templated interpreter is skipped under the custom pair", interpreter: "[[ .vars.interp ]]", opts: []EmbeddedOption{WithEmbeddedDelimiters(squareDelims)}},
		{name: "default-templated interpreter is skipped conservatively under the custom pair", interpreter: "{{ .vars.interp }}", opts: []EmbeddedOption{WithEmbeddedDelimiters(squareDelims)}},
		{name: "default-templated interpreter is skipped by default", interpreter: "{{ .vars.interp }}"},
		{name: "embedded interpreter under a container is still rejected", interpreter: "starlark", opts: []EmbeddedOption{WithEmbeddedDelimiters(squareDelims)}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateEmbeddedInterpreters("wf", def(tt.interpreter), tt.opts...)
			if tt.wantErr {
				require.ErrorIs(t, err, errUtils.ErrScript)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestRenderScriptInterpreterHonorsDelimiters(t *testing.T) {
	render := func(value string) (string, error) { return strings.ToUpper(value), nil }
	tests := []struct {
		name        string
		interpreter string
		opts        []EmbeddedOption
		want        string
	}{
		{name: "custom opening renders", interpreter: "py[[x", opts: []EmbeddedOption{WithEmbeddedDelimiters(squareDelims)}, want: "PY[[X"},
		{name: "default opening is literal under the custom pair", interpreter: "py{{x", opts: []EmbeddedOption{WithEmbeddedDelimiters(squareDelims)}, want: "py{{x"},
		{name: "default opening renders by default", interpreter: "py{{x", want: "PY{{X"},
		{name: "custom opening is literal by default", interpreter: "py[[x", want: "py[[x"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			step := &schema.WorkflowStep{Name: "s", Type: schema.TaskTypeScript, Interpreter: tt.interpreter}
			require.NoError(t, RenderScriptInterpreter(step, render, tt.opts...))
			assert.Equal(t, tt.want, step.Interpreter)
		})
	}
}
