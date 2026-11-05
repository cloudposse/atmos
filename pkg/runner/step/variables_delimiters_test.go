package step

import (
	"bytes"
	"strings"
	"testing"
	"text/template"

	cerrors "github.com/cockroachdb/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/template/delimiters"
)

var (
	// Compile-time guards for the schema fields these tests use.
	_ = schema.WorkflowStep{Command: "", Script: "", Output: "", Env: map[string]string{}, LiteralFields: nil}
	_ = delimiters.Delimiters{Left: "", Right: ""}

	squareDelims = delimiters.Delimiters{Left: "[[", Right: "]]"}
)

// varsWithFirstStep returns Variables holding a finished step named "first" with value "first".
func varsWithFirstStep(delims delimiters.Delimiters) *Variables {
	vars := NewVariables()
	vars.SetTemplateDelimiters(delims)
	vars.Set("first", NewStepResult("first"))
	return vars
}

// delimitedRenderer mimics a host renderer: it parses each pass with the supplied delimiters.
func delimitedRenderer(delims delimiters.Delimiters) TemplateRenderer {
	return func(name, input string, data any) (string, error) {
		tmpl, err := delims.Apply(template.New(name)).Parse(input)
		if err != nil {
			return "", err
		}
		var buf bytes.Buffer
		if err := tmpl.Execute(&buf, data); err != nil {
			return "", err
		}
		return buf.String(), nil
	}
}

func TestVariablesResolveHonorsTemplateDelimiters(t *testing.T) {
	const input = "got [[ .steps.first.value ]] and {{ .steps.first.value }}"
	tests := []struct {
		name     string
		delims   delimiters.Delimiters
		renderer bool
		want     string
	}{
		{name: "custom delimiters render only the custom pair", delims: squareDelims, want: "got first and {{ .steps.first.value }}"},
		{name: "custom delimiters with a host renderer", delims: squareDelims, renderer: true, want: "got first and {{ .steps.first.value }}"},
		{name: "default delimiters render only the default pair", delims: delimiters.Default(), want: "got [[ .steps.first.value ]] and first"},
		{name: "default delimiters with a host renderer", delims: delimiters.Default(), renderer: true, want: "got [[ .steps.first.value ]] and first"},
		{name: "zero value is the default pair", delims: delimiters.Delimiters{}, want: "got [[ .steps.first.value ]] and first"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vars := varsWithFirstStep(tt.delims)
			if tt.renderer {
				vars.SetTemplateRenderer(delimitedRenderer(tt.delims))
			}
			got, err := vars.Resolve(input)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestVariablesResolveStepFieldsWithCustomDelimiters(t *testing.T) {
	vars := varsWithFirstStep(squareDelims)
	vars.SetFlag("mode", "none")
	step := &schema.WorkflowStep{
		Name:    "s",
		Command: "echo [[ .steps.first.value ]]",
		Script:  "print('[[ .steps.first.value ]]')",
		Output:  "[[ .Flags.mode ]]",
		Env:     map[string]string{"A": "[[ .steps.first.value ]]", "B": "{{ literal }}"},
	}

	command, err := vars.ResolveStepField(step, "command", step.Command)
	require.NoError(t, err)
	assert.Equal(t, "echo first", command)

	script, err := vars.ResolveStepField(step, "script", step.Script)
	require.NoError(t, err)
	assert.Equal(t, "print('first')", script)

	env, err := vars.ResolveStepEnvMap(step, step.Env)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"A": "first", "B": "{{ literal }}"}, env)

	mode, err := ResolveStepOutputMode(step, vars)
	require.NoError(t, err)
	assert.Equal(t, "none", mode)

	// The default delimiters leave the same text untemplated.
	defaults := varsWithFirstStep(delimiters.Default())
	defaults.SetFlag("mode", "none")
	command, err = defaults.ResolveStepField(step, "command", step.Command)
	require.NoError(t, err)
	assert.Equal(t, "echo [[ .steps.first.value ]]", command)
	_, err = ResolveStepOutputMode(step, defaults)
	require.ErrorIs(t, err, schema.ErrStepInvalidOutputMode, "an untemplated [[ ]] output is not a valid mode")
}

func TestVariablesResolveOutputTemplateIsNotDetectedForOtherDelimiters(t *testing.T) {
	step := &schema.WorkflowStep{Name: "s", Output: "{{ .Flags.mode }}"}
	vars := varsWithFirstStep(squareDelims)
	vars.SetFlag("mode", "none")

	_, err := ResolveStepOutputMode(step, vars)
	require.ErrorIs(t, err, schema.ErrStepInvalidOutputMode, "{{ }} is plain text under [[ ]] delimiters")
}

func TestVariablesProtectedRootsUnderCustomDelimiters(t *testing.T) {
	const (
		injected = "[[ .env.SECRET ]]"
		input    = "[[ .Flags.value ]]|{{ .Flags.value }}"
	)
	tests := []struct {
		name      string
		delims    delimiters.Delimiters
		flagValue string
		input     string
		want      string
	}{
		{name: "custom: flag holding the custom opening is not re-rendered", delims: squareDelims, flagValue: injected, input: input, want: injected + "|{{ .Flags.value }}"},
		{name: "default: flag holding the default opening is not re-rendered", delims: delimiters.Default(), flagValue: "{{ .env.SECRET }}", input: input, want: "[[ .Flags.value ]]|{{ .env.SECRET }}"},
		{name: "custom: a default opening in a flag stays literal text", delims: squareDelims, flagValue: "{{ .env.SECRET }}", input: input, want: "{{ .env.SECRET }}|{{ .Flags.value }}"},
	}
	for _, tt := range tests {
		for _, host := range []string{"fallback renderer", "host renderer"} {
			t.Run(tt.name+"/"+host, func(t *testing.T) {
				vars := varsWithFirstStep(tt.delims)
				vars.SetEnv("SECRET", "leaked")
				vars.SetTemplateEnv("SECRET", "leaked")
				vars.SetFlag("value", tt.flagValue)
				vars.SetTemplatePasses(3)
				vars.ProtectTemplateRoots("Flags", "flags")
				if host == "host renderer" {
					vars.SetTemplateRenderer(delimitedRenderer(tt.delims))
				}

				got, err := vars.Resolve(tt.input)
				require.NoError(t, err)
				assert.Equal(t, tt.want, got)
				assert.NotContains(t, got, "leaked", "a protected value must not be evaluated on a later pass")
				assert.NotContains(t, got, protectedTemplateOpen, "the protection placeholder must be restored")
			})
		}
	}
}

func TestVariablesMultiPassStopsWithoutCustomOpening(t *testing.T) {
	vars := varsWithFirstStep(squareDelims)
	vars.SetTemplatePasses(3)
	vars.SetTemplateEnv("NEXT", "[[ .steps.first.value ]]")

	// The first pass yields another custom template, which the second pass renders.
	got, err := vars.Resolve("[[ .env.NEXT ]]")
	require.NoError(t, err)
	assert.Equal(t, "first", got)

	// Text that only holds the default opening needs no further pass and stays literal.
	vars.SetTemplateEnv("NEXT", "{{ .steps.first.value }}")
	got, err = vars.Resolve("[[ .env.NEXT ]]")
	require.NoError(t, err)
	assert.Equal(t, "{{ .steps.first.value }}", got)
}

func TestVariablesTemplateDelimitersAccessors(t *testing.T) {
	var nilVars *Variables
	assert.Equal(t, delimiters.Default(), nilVars.TemplateDelimiters())

	vars := NewVariables()
	assert.Equal(t, delimiters.Default(), vars.TemplateDelimiters())

	vars.SetTemplateDelimiters(squareDelims)
	assert.Equal(t, squareDelims, vars.TemplateDelimiters())
	assert.Equal(t, squareDelims, vars.Clone().TemplateDelimiters(), "clones render with the same delimiters")

	vars.SetTemplateDelimiters(delimiters.Delimiters{})
	assert.Equal(t, delimiters.Default(), vars.TemplateDelimiters())
}

func TestVariablesApplyTemplateDelimiters(t *testing.T) {
	configWith := func(pin string, delims []string) *schema.AtmosConfiguration {
		cfg := &schema.AtmosConfiguration{Edition: pin}
		cfg.Templates.Settings.Delimiters = delims
		return cfg
	}
	tests := []struct {
		name    string
		cfg     *schema.AtmosConfiguration
		want    delimiters.Delimiters
		wantErr error
	}{
		{name: "nil config", cfg: nil, want: delimiters.Default()},
		{name: "unpinned custom", cfg: configWith("", []string{"[[", "]]"}), want: squareDelims},
		{name: "pinned before the transition", cfg: configWith("2026-10-18", []string{"[[", "]]"}), want: delimiters.Default()},
		{name: "pinned on the transition date", cfg: configWith("2026-10-19", []string{"[[", "]]"}), want: squareDelims},
		{name: "invalid delimiter list", cfg: configWith("", []string{"[["}), wantErr: errUtils.ErrInvalidTemplateSettings},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vars := NewVariables()
			err := vars.ApplyTemplateDelimiters(tt.cfg)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				assert.Equal(t, delimiters.Default(), vars.TemplateDelimiters(), "a failed resolution installs nothing")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, vars.TemplateDelimiters())
		})
	}
}

func TestTemplateFieldErrorNamesTheConfiguredOpening(t *testing.T) {
	step := &schema.WorkflowStep{Name: "s", Command: "echo [[ broken"}
	cause := assert.AnError

	custom := varsWithFirstStep(squareDelims).TemplateFieldError(step, "command", cause)
	require.ErrorIs(t, custom, errUtils.ErrTemplateEvaluation)
	assert.Contains(t, strings.Join(cerrors.GetAllHints(custom), "\n"), "contains `[[`")

	// The package-level form keeps assuming the default delimiters, so a [[ command gets no hint.
	plain := TemplateFieldError(step, "command", cause)
	require.ErrorIs(t, plain, errUtils.ErrTemplateEvaluation)
	assert.NotContains(t, strings.Join(cerrors.GetAllHints(plain), "\n"), "!literal")
}
