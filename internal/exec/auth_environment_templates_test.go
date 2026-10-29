package exec

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/template/delimiters"
)

var _ = schema.TemplatesSettingsGomplate{Enabled: true}

func TestAuthEnvironmentProtectionAllowsOrdinaryShellTemplates(t *testing.T) {
	input := `curl -d '{"auth": "{{ .token }}"}' https://example.invalid`
	output, err := ProcessTmpl(&schema.AtmosConfiguration{}, "shell-step", input, map[string]any{"token": "ordinary-test-token"}, false)
	require.NoError(t, err)
	require.Equal(t, `curl -d '{"auth": "ordinary-test-token"}' https://example.invalid`, output)
}

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
		func(source string) (string, error) {
			return ProcessTmplWithDatasourcesGomplate(config, "stack.yaml", source, map[string]any{"stage": "prod"}, false)
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

func TestAuthEnvironmentStepTemplatesRenderJSONCredentialPayloads(t *testing.T) {
	for name, delims := range map[string]delimiters.Delimiters{
		"default": {},
		"custom":  {Left: "[[", Right: "]]"},
	} {
		t.Run(name, func(t *testing.T) {
			expression := delims.Wrap(".token")
			source := strings.ReplaceAll(`curl -H "Content-Type: application/json" -H "Accept: application/json" -d '{"auth":{"identities":{"service":{"spec":{"env":{"TOKEN":"{{ .token }}"}}}}}}' https://example.invalid`, "{{ .token }}", expression)
			if !delims.IsDefault() {
				source += ` -H 'X-Literal-Template: {{ .runtime }}'`
			}
			output, err := ProcessStepTmpl(&schema.AtmosConfiguration{}, "shell-step", source, map[string]any{"token": "ordinary-test-token"}, delims)
			require.NoError(t, err)
			require.Equal(t, strings.ReplaceAll(source, expression, "ordinary-test-token"), output)
		})
	}
}

func TestAuthEnvironmentInvalidTemplateShapesFailBeforeGeneralRendering(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"token":"general-template-value"}`))
	}))
	defer server.Close()

	config := &schema.AtmosConfiguration{Templates: schema.Templates{Settings: schema.TemplatesSettings{Enabled: true, Gomplate: schema.TemplatesSettingsGomplate{Enabled: true}}}}
	helper := fmt.Sprintf(`{{ (ds %q).token }}`, server.URL)
	for name, render := range authEnvironmentTemplateRenderers(config) {
		t.Run(name, func(t *testing.T) {
			t.Run("ordinary datasource remains callable", func(t *testing.T) {
				calls.Store(0)
				output, err := render("vars: {token: '"+helper+"'}", map[string]any{})
				require.NoError(t, err)
				require.Equal(t, "vars: {token: 'general-template-value'}", output)
				require.Positive(t, calls.Load(), "the spy must observe ordinary general rendering")
			})
			for shape, env := range map[string]string{
				"mapping value":         "env: {TOKEN: {nested: 'TEMPLATE'}}",
				"sequence value":        "env: {TOKEN: ['TEMPLATE']}",
				"sequence env":          "env: ['TEMPLATE']",
				"scalar env":            "env: 'TEMPLATE'",
				"unquoted value":        "env: {TOKEN: TEMPLATE}",
				"unquoted nested value": "env: {TOKEN: {nested: TEMPLATE}}",
				"unquoted env":          "env: TEMPLATE",
				"unquoted key":          "env: { TEMPLATE: literal}",
				"quoted key":            "env: {'TEMPLATE': literal}",
			} {
				t.Run(shape, func(t *testing.T) {
					calls.Store(0)
					source := "auth: {identities: {service: {spec: {" + strings.ReplaceAll(env, "TEMPLATE", helper) + "}}}}"
					output, err := render(source, map[string]any{})
					require.Zero(t, calls.Load(), "invalid auth environment shapes must be rejected before general helpers execute")
					require.ErrorIs(t, err, errUtils.ErrInvalidAuthConfig)
					require.True(t, errUtils.HasHint(err, "inline"))
					require.Empty(t, output)
				})
			}
		})
	}
}

func authEnvironmentTemplateRenderers(config *schema.AtmosConfiguration) map[string]func(string, map[string]any) (string, error) {
	return map[string]func(string, map[string]any) (string, error){
		"plain": func(source string, data map[string]any) (string, error) {
			return ProcessTmpl(config, "atmos.yaml", source, data, false)
		},
		"datasources": func(source string, data map[string]any) (string, error) {
			return ProcessTmplWithDatasources(config, &schema.ConfigAndStacksInfo{}, schema.Settings{}, "stack.yaml", source, data, false)
		},
		"gomplate": func(source string, data map[string]any) (string, error) {
			return ProcessTmplWithDatasourcesGomplate(config, "stack.yaml", source, data, false)
		},
	}
}
