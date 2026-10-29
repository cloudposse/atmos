package exec

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/schema"
)

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
	var node yaml.Node
	require.NoError(t, yaml.Unmarshal([]byte(authTemplateYAML(input)), &node))
	protected, restore := protectAuthEnvironmentTemplates(input)
	require.NotContains(t, protected, ".credentials", "protected source: %s; parsed source: %s", protected, authTemplateYAML(input))
	require.Equal(t, input, restore(protected))
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

func TestAuthEnvironmentTemplateProtectionPreservesSource(t *testing.T) {
	for _, source := range []string{
		`ordinary {{ .value }}`,
		`auth: {identities: [invalid], env: '{{ .value }}'}`,
		`auth: {identities: {service: {spec: {env: ['{{ .credentials.token }}']}}}}`,
		`auth: {identities: {service: {spec: {env: {TOKEN: 42}}}}}}`,
		`auth: {identities: {service: {kind: secrets/env}}}`,
		`auth: {identities: [invalid`,
		`# ATMOS_AUTH_ENV_TEMPLATE_
auth:
  identities:
    service:
      spec:
        env:
          SINGLE: 'Bearer ''quoted'' {{ .credentials.token }}'
          DOUBLE: "Bearer \"quoted\" {{ .credentials.token }}"
          PLAIN: Bearer {{ .credentials.token }}
`,
	} {
		protected, restore := protectAuthEnvironmentTemplates(source)
		require.Equal(t, source, restore(protected))
		if strings.Contains(source, "SINGLE:") {
			require.NotContains(t, protected, ".credentials")
		}
	}
}
