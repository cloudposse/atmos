package exec

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestAuthEnvironmentTemplateAliasesFailBeforeGeneralRendering(t *testing.T) {
	input := `shared: &credential '{{ call .privileged }}'
auth:
  identities:
    service:
      spec:
        env:
          TOKEN: *credential
`
	config := &schema.AtmosConfiguration{Templates: schema.Templates{Settings: schema.TemplatesSettings{Enabled: true}}}
	for name, render := range map[string]func(string, map[string]any) (string, error){
		"plain": func(source string, data map[string]any) (string, error) {
			return ProcessTmpl(config, "stack.yaml", source, data, false)
		},
		"datasources": func(source string, data map[string]any) (string, error) {
			return ProcessTmplWithDatasources(config, &schema.ConfigAndStacksInfo{}, schema.Settings{}, "stack.yaml", source, data, false)
		},
		"gomplate": func(source string, data map[string]any) (string, error) {
			return ProcessTmplWithDatasourcesGomplate(config, "stack.yaml", source, data, false)
		},
	} {
		t.Run(name, func(t *testing.T) {
			calls := 0
			output, err := render(input, map[string]any{"privileged": func() string {
				calls++
				return "must-not-be-rendered"
			}})
			require.Zero(t, calls, "auth aliases must be rejected before any general template executes")
			require.ErrorIs(t, err, errUtils.ErrInvalidAuthConfig)
			require.True(t, errUtils.HasHint(err, "inline"))
			require.Empty(t, output)
		})
	}
}

func TestAuthEnvironmentTemplatesSurviveGeneralRendering(t *testing.T) {
	input := `vars:
  stage: '{{ .stage }}'
auth:
  identities:
    service:
      kind: secrets/env
      spec:
        env:
          API_TOKEN: '{{ index .credentials "api-token" }}'
          LITERAL: '{{ env "MUST_NOT_RUN" }}'
          INVALID: '{{ .credentials.token'
`
	config := &schema.AtmosConfiguration{Templates: schema.Templates{Settings: schema.TemplatesSettings{Enabled: true, Evaluations: 2}}}
	for _, render := range []func(string) (string, error){
		func(source string) (string, error) {
			return ProcessTmpl(config, "stack.yaml", source, map[string]any{"stage": "prod"}, false)
		},
		func(source string) (string, error) {
			return ProcessTmplWithDatasources(config, &schema.ConfigAndStacksInfo{}, schema.Settings{}, "component", source, map[string]any{"stage": "prod"}, false)
		},
	} {
		output, err := render(input)
		require.NoError(t, err)
		var decoded map[string]any
		require.NoError(t, yaml.Unmarshal([]byte(output), &decoded))
		require.Equal(t, "prod", decoded["vars"].(map[string]any)["stage"])
		env := decoded["auth"].(map[string]any)["identities"].(map[string]any)["service"].(map[string]any)["spec"].(map[string]any)["env"]
		require.Equal(t, map[string]any{"API_TOKEN": `{{ index .credentials "api-token" }}`, "LITERAL": `{{ env "MUST_NOT_RUN" }}`, "INVALID": "{{ .credentials.token"}, env)
	}
}

func TestAuthEnvironmentTemplatesSurviveTemplateControlsAndBlocks(t *testing.T) {
	input := `{{ if .enabled }}
components:
  terraform:
    example:
      auth:
        identities:
          service:
            spec:
              env:
                TOKEN: |-
                  {{ if .credentials.token }}
                  Bearer {{ .credentials.token }}
                  {{ end }}
                OTHER: 'literal {{ index .credentials "api-key" }}'
{{ end }}
`
	output, err := ProcessTmpl(&schema.AtmosConfiguration{}, "stack.yaml.tmpl", input, map[string]any{"enabled": true}, false)
	require.NoError(t, err)
	var decoded map[string]any
	require.NoError(t, yaml.Unmarshal([]byte(output), &decoded))
	env := decoded["components"].(map[string]any)["terraform"].(map[string]any)["example"].(map[string]any)["auth"].(map[string]any)["identities"].(map[string]any)["service"].(map[string]any)["spec"].(map[string]any)["env"]
	require.Equal(t, map[string]any{
		"TOKEN": "{{ if .credentials.token }}\nBearer {{ .credentials.token }}\n{{ end }}",
		"OTHER": `literal {{ index .credentials "api-key" }}`,
	}, env)
}

func TestAuthEnvironmentLiteralAliasesSurviveGeneralRendering(t *testing.T) {
	input := `shared: &token ordinary-literal
vars:
  stage: '{{ .stage }}'
auth:
  identities:
    service:
      spec:
        env:
          TOKEN: *token
`
	output, err := ProcessTmpl(&schema.AtmosConfiguration{}, "stack.yaml", input, map[string]any{"stage": "prod"}, false)
	require.NoError(t, err)
	require.Contains(t, output, "stage: 'prod'")
	var decoded map[string]any
	require.NoError(t, yaml.Unmarshal([]byte(output), &decoded))
	env := decoded["auth"].(map[string]any)["identities"].(map[string]any)["service"].(map[string]any)["spec"].(map[string]any)["env"]
	require.Equal(t, map[string]any{"TOKEN": "ordinary-literal"}, env)
}
