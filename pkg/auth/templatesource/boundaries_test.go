package templatesource

import (
	"bytes"
	"strings"
	"testing"
	"text/template"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestProtectLeavesApplicationVariablesForGeneralRendering(t *testing.T) {
	source := `vars:
  auth: {identities: {app: {spec: {env: {TOKEN: '{{ upper .stage }}'}}}}}
auth: {identities: {service: {spec: {env: {TOKEN: '{{ call .privileged }}'}}}}}
components:
  terraform:
    example:
      vars:
        auth: {identities: {app: {spec: {env: {TOKEN: '{{ upper .stage }}'}}}}}
      auth: {identities: {service: {spec: {env: {TOKEN: '{{ call .privileged }}'}}}}}
`
	protected, restore, err := Protect(source)
	require.NoError(t, err)
	require.Contains(t, protected, "upper .stage", "application data must retain ordinary template evaluation")
	require.NotContains(t, protected, "call .privileged", "root and component credentials must remain deferred")
	require.Equal(t, source, restore(protected))
	calls := 0
	renderer, err := template.New("stack").Funcs(template.FuncMap{"upper": strings.ToUpper}).Parse(protected)
	require.NoError(t, err)
	var output bytes.Buffer
	err = renderer.Execute(&output, map[string]any{"stage": "prod", "privileged": func() string {
		calls++
		return "must-not-run"
	}})
	require.NoError(t, err)
	require.Zero(t, calls)
	var document map[string]any
	require.NoError(t, yaml.Unmarshal([]byte(restore(output.String())), &document))
	for _, section := range []map[string]any{document, document["components"].(map[string]any)["terraform"].(map[string]any)["example"].(map[string]any)} {
		variables := section["vars"].(map[string]any)["auth"].(map[string]any)["identities"].(map[string]any)["app"].(map[string]any)["spec"].(map[string]any)["env"].(map[string]any)
		require.Equal(t, "PROD", variables["TOKEN"])
		credentials := section["auth"].(map[string]any)["identities"].(map[string]any)["service"].(map[string]any)["spec"].(map[string]any)["env"].(map[string]any)
		require.Equal(t, "{{ call .privileged }}", credentials["TOKEN"])
	}
}

func TestProtectDoesNotRestorePredictableRenderedData(t *testing.T) {
	source := "vars: {value: '{{ .ordinary }}'}\nauth: {identities: {service: {spec: {env: {TOKEN: '{{ .credentials.token }}'}}}}}"
	protected, restore, err := Protect(source)
	require.NoError(t, err)
	renderer, err := template.New("stack").Parse(protected)
	require.NoError(t, err)
	var output bytes.Buffer
	err = renderer.Execute(&output, map[string]any{"ordinary": "ATMOS_AUTH_ENV_TEMPLATE_0_END"})
	require.NoError(t, err)
	restored := restore(output.String())
	require.Contains(t, restored, "value: 'ATMOS_AUTH_ENV_TEMPLATE_0_END'")
	require.Contains(t, restored, "TOKEN: '{{ .credentials.token }}'")
}

func TestProtectRestorationIsIsolatedBetweenCalls(t *testing.T) {
	source := "auth: {identities: {service: {spec: {env: {TOKEN: '{{ .credentials.token }}'}}}}}"
	first, restoreFirst, err := Protect(source)
	require.NoError(t, err)
	second, restoreSecond, err := Protect(source)
	require.NoError(t, err)
	require.NotEqual(t, first, second, "each protection operation must own fresh restoration tokens")
	require.Equal(t, first, restoreSecond(first), "a later render must not restore data carrying an earlier operation's token")
	require.Equal(t, second, restoreFirst(second), "restoration must not cross operation boundaries")
	require.Equal(t, source, restoreFirst(first))
	require.Equal(t, source, restoreSecond(second))
}

func TestProtectRejectsInlineAuthControlWrappers(t *testing.T) {
	mapping := "auth: {identities: {service: {spec: {env: {TOKEN: '{{ call .privileged }}'}}}}}"
	for name, source := range map[string]string{
		"one line":              "{{ if .enabled }}" + mapping + "{{ end }}",
		"end on following line": "{{ if .enabled }}" + mapping + "\n{{ end }}\n",
		"nested component":      "components:\n  terraform:\n    example:\n      {{ if .enabled }}" + mapping + "{{ end }}\n",
	} {
		t.Run(name, func(t *testing.T) {
			calls := 0
			protected, _, protectErr := Protect(source)
			// Model the render entrypoints: execute only when protection succeeds.
			if protectErr == nil {
				renderer, err := template.New("stack").Parse(protected)
				require.NoError(t, err)
				var output bytes.Buffer
				err = renderer.Execute(&output, map[string]any{"enabled": true, "privileged": func() string {
					calls++
					return "must-not-run"
				}})
				require.NoError(t, err)
			}
			require.Zero(t, calls, "auth-bearing control wrappers must fail before general helpers run")
			require.ErrorIs(t, protectErr, errUtils.ErrInvalidAuthConfig)
			require.True(t, errUtils.HasHint(protectErr, "standalone"))
			require.Empty(t, protected)
		})
	}
}

func TestProtectAllowsConditionalAuthTextInApplicationData(t *testing.T) {
	for _, section := range []string{"vars", "settings"} {
		t.Run(section, func(t *testing.T) {
			source := section + `:
  payload: |-
    {{ if .enabled }}auth: {identities: {app: {spec: {env: {TOKEN: '{{ .stage }}'}}}}}{{ end }}
auth: {identities: {service: {spec: {env: {TOKEN: '{{ call .privileged }}'}}}}}
`
			protected, restore, err := Protect(source)
			require.NoError(t, err)
			require.Equal(t, source, restore(protected))
			require.NotContains(t, protected, "call .privileged")
			calls := 0
			renderer, err := template.New("stack").Parse(protected)
			require.NoError(t, err)
			var output bytes.Buffer
			err = renderer.Execute(&output, map[string]any{"enabled": true, "stage": "prod", "privileged": func() string {
				calls++
				return "must-not-run"
			}})
			require.NoError(t, err)
			require.Zero(t, calls)
			var document map[string]any
			require.NoError(t, yaml.Unmarshal([]byte(restore(output.String())), &document))
			require.Equal(t, "auth: {identities: {app: {spec: {env: {TOKEN: 'prod'}}}}}", document[section].(map[string]any)["payload"])
			credentials := document["auth"].(map[string]any)["identities"].(map[string]any)["service"].(map[string]any)["spec"].(map[string]any)["env"].(map[string]any)
			require.Equal(t, "{{ call .privileged }}", credentials["TOKEN"])
		})
	}
}
